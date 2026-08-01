package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/compat"
	"boxpanel/internal/coredl"
	"boxpanel/internal/models"
)

// 进度缓存（按版本）
var (
	dlMu       sync.Mutex
	dlProgress = map[string]*coredl.Progress{}
)

// updateMu 防止并发 auto-update
var updateMu sync.Mutex

// GET /api/cores/available - 列出 GitHub 可下载版本（标本地缓存）
func (s *APIServer) handleListAvailableCores(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	includePre := r.URL.Query().Get("prerelease") == "1"
	releases, err := s.coredl.ListReleases(ctx, includePre)
	if err != nil {
		writeError(w, 502, "GitHub API: "+err.Error())
		return
	}
	type item struct {
		Version    string `json:"version"`
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		Prerelease bool   `json:"prerelease"`
		Installed  bool   `json:"installed"` // 本地缓存有
	}
	out := make([]item, 0, len(releases))
	for _, rel := range releases {
		ver := strings.TrimPrefix(rel.TagName, "v")
		installed := s.coreCache.Has(ver)
		out = append(out, item{
			Version:    ver,
			TagName:    rel.TagName,
			Name:       rel.Name,
			Prerelease: isPrereleaseTag(rel.TagName),
			Installed:  installed,
		})
	}
	writeJSON(w, 200, map[string]any{
		"items":         out,
		"cache_dir":     s.coredl.BinDir(),
		"last_checked":  s.coreCache.LastChecked(),
	})
}

// GET /api/cores/cache - 本地缓存列表
func (s *APIServer) handleCacheList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"items":        s.coreCache.List(),
		"cache_dir":    s.coredl.BinDir(),
		"last_checked": s.coreCache.LastChecked(),
	})
}

// POST /api/cores/download - 异步下载到缓存
// Body: { "version": "1.10.7", "kind": "singbox"|"xray"|"mihomo"|"hysteria2", "activate": bool }
// If kind is not sing-box, uses MultiCoreDownloader.
func (s *APIServer) handleDownloadCore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version  string `json:"version"`
		Kind     string `json:"kind"`
		Activate bool   `json:"activate"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if body.Version == "" {
		writeError(w, 400, "version 必填")
		return
	}
	if body.Kind == "" {
		body.Kind = models.CoreKindSingBox
	}

	// Validate kind
	validKind := map[string]bool{
		models.CoreKindSingBox:   true,
		models.CoreKindXray:     true,
		models.CoreKindMihomo:   true,
		models.CoreKindHysteria2: true,
	}
	if !validKind[body.Kind] {
		writeError(w, 400, "invalid kind, must be singbox/xray/mihomo/hysteria2")
		return
	}

	version := body.Version

	// Non-sing-box cores: always use MultiCoreDownloader
	if body.Kind != models.CoreKindSingBox {
		dlMu.Lock()
		dlProgress[body.Kind+":"+version] = &coredl.Progress{Stage: "starting", Version: version}
		dlMu.Unlock()
		go s.downloadAndInstallMultiCore(body.Kind, version, body.Activate)
		writeJSON(w, 202, map[string]any{
			"status":  "downloading",
			"version": version,
			"kind":    body.Kind,
			"poll":    "/api/cores/download/status?version=" + body.Kind + ":" + version,
		})
		return
	}

	// sing-box: existing logic

	// 本地缓存已有 → 直接激活，不下载
	if s.coreCache.Has(version) {
		cached := s.coreCache.Path(version)
		st, _ := s.store.GetSettings(r.Context())
		dup := false
		var dupID string
		for _, c := range st.Cores {
			if c.Version == version {
				dup = true
				dupID = c.ID
				break
			}
		}
		core := coreRegFromCache(version, cached)
		if !dup {
			st.Cores = append(st.Cores, *core)
			dupID = core.ID
		}
		if body.Activate {
			st.ActiveCoreID = dupID
		}
		_ = s.store.SaveSettings(r.Context(), st)
		s.mu.Lock()
		s.settings = st
		s.mu.Unlock()
		dlMu.Lock()
		dlProgress[version] = &coredl.Progress{Stage: "done", Version: version, Source: "cached:" + cached}
		dlMu.Unlock()
		writeJSON(w, 200, map[string]any{
			"status":  "cached",
			"version": version,
			"path":    cached,
		})
		return
	}

	// 否则异步下载
	dlMu.Lock()
	dlProgress[version] = &coredl.Progress{Stage: "starting", Version: version}
	dlMu.Unlock()
	go s.downloadAndInstall(version, body.Activate)

	writeJSON(w, 202, map[string]any{
		"status":  "downloading",
		"version": version,
		"poll":    "/api/cores/download/status?version=" + version,
	})
}

// downloadAndInstall 后台下载并加入缓存 + 激活。
func (s *APIServer) downloadAndInstall(version string, activate bool) {
	ctx := context.Background()
	st, _ := s.store.GetSettings(ctx)

	// 拉 GitHub 找 tag
	releases, err := s.coredl.ListReleases(ctx, true)
	var tagName string
	var prerelease bool
	if err == nil {
		for _, r := range releases {
			v := strings.TrimPrefix(r.TagName, "v")
			if v == version {
				tagName = r.TagName
				prerelease = isPrereleaseTag(r.TagName)
				break
			}
		}
	}

	core, err := s.coredl.DownloadAndCache(ctx, version, st.CustomDownloadMirrors, tagName, prerelease, s.coreCache, func(p coredl.Progress) {
		dlMu.Lock()
		snap := p
		dlProgress[version] = &snap
		dlMu.Unlock()
	})
	if err != nil {
		dlMu.Lock()
		dlProgress[version] = &coredl.Progress{Stage: "error", Version: version, Error: err.Error()}
		dlMu.Unlock()
		return
	}

	// 写入 settings.Cores
	st2, _ := s.store.GetSettings(ctx)
	dup := false
	var dupID string
	for i, c := range st2.Cores {
		if c.Version == version {
			st2.Cores[i].Path = core.Path
			dup = true
			dupID = c.ID
			break
		}
	}
	if !dup {
		st2.Cores = append(st2.Cores, *core)
		dupID = core.ID
	}
	if activate {
		st2.ActiveCoreID = dupID
	}
	_ = s.store.SaveSettings(ctx, st2)
	s.mu.Lock()
	s.settings = st2
	s.mu.Unlock()

	dlMu.Lock()
	dlProgress[version] = &coredl.Progress{Stage: "done", Version: version, Source: core.Path}
	dlMu.Unlock()
}

// GET /api/cores/download/status?version=xxx
func (s *APIServer) handleDownloadStatus(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	dlMu.Lock()
	p, ok := dlProgress[version]
	dlMu.Unlock()
	if !ok || p == nil {
		writeJSON(w, 200, map[string]any{"stage": "idle"})
		return
	}
	writeJSON(w, 200, p)
}

// POST /api/cores/check-update - 立即检查 GitHub + 下载缺失
// 返回本次会下载的版本列表（实际下载在后台进行）
func (s *APIServer) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if !updateMu.TryLock() {
		writeError(w, 409, "already updating")
		return
	}
	defer updateMu.Unlock()

	releases, err := s.coredl.ListReleases(ctx, false)
	if err != nil {
		writeError(w, 502, "GitHub: "+err.Error())
		return
	}
	st, _ := s.store.GetSettings(r.Context())

	// 缺失列表
	type missing struct {
		Version string `json:"version"`
		TagName string `json:"tag_name"`
		Size    string `json:"size_hint"`
	}
	var toDownload []missing
	for _, rel := range releases {
		ver := strings.TrimPrefix(rel.TagName, "v")
		if !s.coreCache.Has(ver) {
			toDownload = append(toDownload, missing{
				Version: ver,
				TagName: rel.TagName,
				Size:    "~20MB",
			})
		}
	}

	// 后台下载缺失的（只在缓存缺时才下，已有的不重下）
	go func() {
		gctx := context.Background()
		for _, rel := range releases {
			ver := strings.TrimPrefix(rel.TagName, "v")
			if s.coreCache.Has(ver) {
				continue
			}
			core, err := s.coredl.DownloadAndCache(gctx, ver, st.CustomDownloadMirrors,
				rel.TagName, isPrereleaseTag(rel.TagName), s.coreCache, nil)
			if err != nil {
				slog.Warn("check-update download failed", "version", ver, "err", err)
				continue
			}
			_ = s.coreCache.Add(&coredl.CachedCore{
				Version: ver, TagName: rel.TagName, Path: core.Path,
				Prerelease: isPrereleaseTag(rel.TagName),
			})
		}
	}()

	writeJSON(w, 200, map[string]any{
		"checked":      len(releases),
		"to_download":  toDownload,
		"cache_count":  len(s.coreCache.List()),
		"last_checked": time.Now(),
	})
}

// POST /api/cores/cache/{version}/remove - 删除本地缓存
func (s *APIServer) handleCacheRemove(w http.ResponseWriter, r *http.Request) {
	version := chi.URLParam(r, "version")

	// Check if this version is the active core and running
	st, _ := s.store.GetSettings(r.Context())
	for _, c := range st.Cores {
		if c.Version == version && st.ActiveCoreID == c.ID && s.coreIsRunning() {
			if err := s.coreStop(); err != nil {
				slog.Warn("stop core before cache remove", "err", err)
			}
			break
		}
	}

	if err := s.coreCache.Remove(version); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	// 从 settings.Cores 中也移除
	out := make([]models.CoreConfig, 0, len(st.Cores))
	for _, c := range st.Cores {
		if c.Version != version {
			out = append(out, c)
		}
	}
	st.Cores = out
	if st.ActiveCoreID != "" {
		// 简单：不清除 active_core_id（可能指错），后续 start 时再校验
	}
	_ = s.store.SaveSettings(r.Context(), st)
	writeJSON(w, 200, map[string]string{"removed": version})
}

// coreLite 是 settings.CoreConfig 的最小子集（避免循环依赖导入）
type coreLite = struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

// coreRegFromCache 从本地缓存构造一个 models.CoreConfig（用于缓存命中时不下载）
func coreRegFromCache(version, path string) *models.CoreConfig {
	return &models.CoreConfig{
		ID:      "cached_" + version,
		Label:   version,
		Version: version,
		Path:    path,
	}
}

// 兼容老接口：POST /api/cores/auto-match
func (s *APIServer) handleAutoMatchCore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServerID string `json:"server_id"`
	}
	_ = readJSON(r, &body)
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx)
	sid := body.ServerID
	if sid == "" {
		sid = st.CurrentServerID
	}
	srv, err := s.store.GetServer(ctx, sid)
	if err != nil || srv == nil {
		writeError(w, 400, "未指定节点")
		return
	}

	// 1. local cores with compatible kernel?
	for _, c := range st.Cores {
		res := compat.CheckServer(*srv, c.Version, c.Kind)
		if res.Level == compat.OK {
			st.ActiveCoreID = c.ID
			_ = s.store.SaveSettings(ctx, st)
			s.mu.Lock()
			s.settings = st
			s.mu.Unlock()
			writeJSON(w, 200, map[string]any{
				"action":  "activate_existing",
				"core_id": c.ID,
				"version": c.Version,
			})
			return
		}
	}

	// 2. local cache pool with compatible kernel?
	for _, cached := range s.coreCache.List() {
		res := compat.CheckServer(*srv, cached.Version, models.CoreKindSingBox)
		if res.Level == compat.OK {
			core := coreRegFromCache(cached.Version, cached.Path)
			st.Cores = append(st.Cores, *core)
			st.ActiveCoreID = core.ID
			_ = s.store.SaveSettings(ctx, st)
			s.mu.Lock()
			s.settings = st
			s.mu.Unlock()
			writeJSON(w, 200, map[string]any{
				"action":  "activate_from_cache",
				"version": cached.Version,
			})
			return
		}
	}

	// 3. recommend download
	probe := compat.CheckServer(*srv, "", models.CoreKindSingBox)
	minVer := probe.MinVersion
	if minVer == "" {
		minVer = "1.1.0"
	}
	suggested, err := s.coredl.SuggestVersionForServer(ctx, minVer)
	if err != nil {
		writeError(w, 500, "无法找到合适版本: "+err.Error())
		return
	}

	// 4. 下载并激活
	core, err := s.coredl.DownloadAndCache(ctx, suggested, st.CustomDownloadMirrors,
		"v"+suggested, false, s.coreCache, func(p coredl.Progress) {
			dlMu.Lock()
			snap := p
			dlProgress[suggested] = &snap
			dlMu.Unlock()
		})
	if err != nil {
		writeError(w, 500, "下载失败: "+err.Error())
		return
	}
	st.Cores = append(st.Cores, *core)
	st.ActiveCoreID = core.ID
	_ = s.store.SaveSettings(ctx, st)
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"action":  "downloaded",
		"version": suggested,
		"path":    core.Path,
	})
}

func isPrereleaseTag(tag string) bool {
	t := strings.ToLower(tag)
	return strings.Contains(t, "alpha") || strings.Contains(t, "beta") ||
		strings.Contains(t, "rc") || strings.Contains(t, "-dev")
}

// 强制类型检查避免 import 漏掉
var _ = compat.OK

// downloadAndInstallMultiCore downloads a non-singbox core in the background.
func (s *APIServer) downloadAndInstallMultiCore(kind, version string, activate bool) {
	ctx := context.Background()
	key := kind + ":" + version
	st, _ := s.store.GetSettings(ctx)

	core, err := s.multiDl.DownloadCore(ctx, kind, version, st.CustomDownloadMirrors, func(p coredl.Progress) {
		dlMu.Lock()
		snap := p
		dlProgress[key] = &snap
		dlMu.Unlock()
	})
	if err != nil {
		dlMu.Lock()
		dlProgress[key] = &coredl.Progress{Stage: "error", Version: version, Error: err.Error()}
		dlMu.Unlock()
		return
	}

	// Write to settings.Cores
	st2, _ := s.store.GetSettings(ctx)
	dup := false
	var dupID string
	for i, c := range st2.Cores {
		if c.Kind == kind && c.Version == version {
			st2.Cores[i].Path = core.Path
			dup = true
			dupID = c.ID
			break
		}
	}
	if !dup {
		st2.Cores = append(st2.Cores, *core)
		dupID = core.ID
	}
	if activate {
		st2.ActiveCoreID = dupID
	}
	_ = s.store.SaveSettings(ctx, st2)
	s.mu.Lock()
	s.settings = st2
	s.mu.Unlock()

	dlMu.Lock()
	dlProgress[key] = &coredl.Progress{Stage: "done", Version: version, Source: core.Path}
	dlMu.Unlock()
}

// GET /api/cores/kinds/{kind}/available - list available versions for a specific core kind
func (s *APIServer) handleListKindAvailable(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	validKind := map[string]bool{
		models.CoreKindSingBox:   true,
		models.CoreKindXray:     true,
		models.CoreKindMihomo:   true,
		models.CoreKindHysteria2: true,
	}
	if !validKind[kind] {
		writeError(w, 400, "invalid kind")
		return
	}

	// For sing-box, delegate to the existing handler
	if kind == models.CoreKindSingBox {
		s.handleListAvailableCores(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	includePre := r.URL.Query().Get("prerelease") == "1"

	releases, err := s.multiDl.ListAvailableVersions(ctx, kind, includePre)
	if err != nil {
		writeError(w, 502, "GitHub API: "+err.Error())
		return
	}

	type item struct {
		Version    string `json:"version"`
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		Prerelease bool   `json:"prerelease"`
	}
	out := make([]item, 0, len(releases))
	for _, rel := range releases {
		ver := strings.TrimPrefix(rel.TagName, "v")
		out = append(out, item{
			Version:    ver,
			TagName:    rel.TagName,
			Name:       rel.Name,
			Prerelease: isPrereleaseTag(rel.TagName),
		})
	}
	writeJSON(w, 200, map[string]any{
		"kind":  kind,
		"items": out,
	})
}

// GET /api/cores/download/status now supports "kind:version" format
// (the existing handler works as-is since we use kind:version as the key)