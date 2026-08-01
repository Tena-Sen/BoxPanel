package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/compat"
	"boxpanel/internal/models"
)

var singBoxVersionRe = regexp.MustCompile(`sing-box version (\S+)`)
var xrayVersionRe = regexp.MustCompile(`Xray (\S+)`)
var mihomoVersionRe = regexp.MustCompile(`Mihomo\s+(\S+)`)
var hysteria2VersionRe = regexp.MustCompile(`hysteria2? (\S+)`)

func (s *APIServer) handleListCores(w http.ResponseWriter, r *http.Request) {
	st, _ := s.store.GetSettings(r.Context())
	cores := st.Cores
	if cores == nil {
		cores = []models.CoreConfig{}
	}
	// Enrich: fill Kind if empty (backward compat with old data)
	for i := range cores {
		if cores[i].Kind == "" {
			cores[i].Kind = detectKindFromPath(cores[i].Path)
		}
	}
	writeJSON(w, 200, map[string]any{
		"cores":          cores,
		"active_core_id": st.ActiveCoreID,
	})
}

func (s *APIServer) handleAddCore(w http.ResponseWriter, r *http.Request) {
	var c models.CoreConfig
	if err := readJSON(r, &c); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if c.Path == "" {
		writeError(w, 400, "path 必填")
		return
	}
	if c.Label == "" {
		c.Label = c.Version
	}
	if c.ID == "" {
		c.ID = models.NewID("cor")
	}
	// Auto-detect kind from path if not specified
	if c.Kind == "" {
		c.Kind = detectKindFromPath(c.Path)
	}
	// 自动探测版本
	if c.Version == "" {
		if v, err := probeVersion(c.Path); err == nil {
			c.Version = v
			if c.Label == "" {
				c.Label = v
			}
		}
	}
	st, _ := s.store.GetSettings(r.Context())
	st.Cores = append(st.Cores, c)
	if st.ActiveCoreID == "" {
		st.ActiveCoreID = c.ID
	}
	_ = s.store.SaveSettings(r.Context(), st)
	writeJSON(w, 200, c)
}

func (s *APIServer) handleDeleteCore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, _ := s.store.GetSettings(r.Context())

	// Find the core being deleted
	var target *models.CoreConfig
	for i := range st.Cores {
		if st.Cores[i].ID == id {
			target = &st.Cores[i]
			break
		}
	}
	if target == nil {
		writeError(w, 404, "core not found")
		return
	}

	// If this is the active core, stop it first (Windows locks running .exe)
	if st.ActiveCoreID == id && s.runner.IsRunning() {
		if err := s.runner.Stop(); err != nil {
			slog.Warn("stop core before delete", "err", err)
		}
	}

	// Remove from settings.Cores
	var deletedPath string
	out := make([]models.CoreConfig, 0, len(st.Cores))
	for _, c := range st.Cores {
		if c.ID == id {
			deletedPath = c.Path
			continue
		}
		out = append(out, c)
	}
	st.Cores = out
	if st.ActiveCoreID == id {
		st.ActiveCoreID = ""
		if len(out) > 0 {
			st.ActiveCoreID = out[0].ID
		}
	}
	_ = s.store.SaveSettings(r.Context(), st)

	// Delete the executable file and its parent directory
	if deletedPath != "" {
		dir := filepath.Dir(deletedPath)
		if isUnderBinDir(deletedPath) {
			if err := os.RemoveAll(dir); err != nil {
				slog.Warn("delete core files", "dir", dir, "err", err)
			} else {
				slog.Info("deleted core files", "dir", dir)
			}
		} else {
			// External path: only delete the exe itself, not the whole directory
			if err := os.Remove(deletedPath); err != nil {
				slog.Warn("delete core exe", "path", deletedPath, "err", err)
			}
		}
	}

	// Also remove from cache index if present
	if target.Version != "" {
		_ = s.coreCache.Remove(target.Version)
	}

	writeJSON(w, 200, map[string]string{"deleted": id})
}

// isUnderBinDir checks whether the path is under our managed data/bin directory.
func isUnderBinDir(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	// Check if path contains data/bin/ pattern (our managed download directory)
	lower := strings.ToLower(filepath.ToSlash(abs))
	return strings.Contains(lower, "data/bin/")
}

func (s *APIServer) handleActivateCore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, _ := s.store.GetSettings(r.Context())
	found := false
	for _, c := range st.Cores {
		if c.ID == id {
			found = true
			break
		}
	}
	if !found {
		writeError(w, 404, "core not found")
		return
	}
	st.ActiveCoreID = id
	_ = s.store.SaveSettings(r.Context(), st)
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()
	writeJSON(w, 200, map[string]string{"active_core_id": id})
}

func (s *APIServer) handleTestCore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, _ := s.store.GetSettings(r.Context())
	var target *models.CoreConfig
	for i := range st.Cores {
		if st.Cores[i].ID == id {
			target = &st.Cores[i]
			break
		}
	}
	if target == nil {
		writeError(w, 404, "core not found")
		return
	}
	version, err := probeVersion(target.Path)
	if err != nil {
		writeError(w, 500, "探测失败: "+err.Error())
		return
	}
	// 更新探测到的版本
	target.Version = version
	if target.Label == "" {
		target.Label = version
	}
	_ = s.store.SaveSettings(r.Context(), st)
	writeJSON(w, 200, map[string]any{
		"path":    target.Path,
		"version": version,
	})
}

func probeVersion(exePath string) (string, error) {
	// Try "version" subcommand first (sing-box style)
	cmd := exec.Command(exePath, "version")
	out, err := cmd.Output()
	if err == nil {
		if m := singBoxVersionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
			return strings.TrimSpace(m[1]), nil
		}
		if m := xrayVersionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
			return strings.TrimSpace(m[1]), nil
		}
		if m := mihomoVersionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
			return strings.TrimSpace(m[1]), nil
		}
		if m := hysteria2VersionRe.FindStringSubmatch(string(out)); len(m) >= 2 {
			return strings.TrimSpace(m[1]), nil
		}
	}
	// Fallback: try "--version" flag (some binaries use this)
	cmd = exec.Command(exePath, "--version")
	out, err = cmd.Output()
	if err == nil {
		s := strings.TrimSpace(string(out))
		for _, re := range []*regexp.Regexp{singBoxVersionRe, xrayVersionRe, mihomoVersionRe, hysteria2VersionRe} {
			if m := re.FindStringSubmatch(s); len(m) >= 2 {
				return strings.TrimSpace(m[1]), nil
			}
		}
		// Generic: first token after any known prefix
		return strings.Fields(s)[0], nil
	}
	return "", fmt.Errorf("version probe failed")
}

// detectKindFromPath infers the core kind from the exe filename.
func detectKindFromPath(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "xray"):
		return models.CoreKindXray
	case strings.Contains(lower, "mihomo"):
		return models.CoreKindMihomo
	case strings.Contains(lower, "hysteria"):
		return models.CoreKindHysteria2
	default:
		return models.CoreKindSingBox
	}
}

// GET /api/compat/servers — 每个 server 对每个 core 的兼容性矩阵
func (s *APIServer) handleCompatServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	servers, _ := s.store.ListServers(ctx)
	st, _ := s.store.GetSettings(ctx)
	results := make([]compat.Result, 0, len(servers))
	for _, srv := range servers {
		for _, c := range st.Cores {
			results = append(results, compat.CheckServer(srv, c.Version, c.Kind))
		}
	}
	writeJSON(w, 200, map[string]any{"results": results})
}

// POST /api/core/start — 启动前返回兼容性警告（Phase 2：启动前警告 + 一键切换）
func (s *APIServer) handleCorePreflight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx)
	srv, err := s.store.GetServer(ctx, st.CurrentServerID)
	if err != nil || srv == nil {
		writeError(w, 400, "未选中服务器")
		return
	}
	// 当前激活内核
	var active *models.CoreConfig
	for i := range st.Cores {
		if st.Cores[i].ID == st.ActiveCoreID {
			active = &st.Cores[i]
			break
		}
	}
	if active == nil && len(st.Cores) > 0 {
		active = &st.Cores[0]
	}
	if active == nil {
		writeJSON(w, 200, map[string]any{"ok": true, "reason": "no core configured"})
		return
	}
	res := compat.CheckServer(*srv, active.Version, active.Kind)
	recommended := compat.SuggestCore(*srv, st.Cores)
	recommendedID := ""
	if recommended != nil && recommended.ID != active.ID {
		recommendedID = recommended.ID
	}
	writeJSON(w, 200, map[string]any{
		"current_core":      active.Version,
		"current_core_id":   active.ID,
		"compatibility":     res,
		"recommended_id":    recommendedID,
		"recommended_version": func() string { if recommended != nil { return recommended.Version }; return "" }(),
	})
}