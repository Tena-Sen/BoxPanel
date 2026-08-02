package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/config"
	corepkg "boxpanel/internal/core"
	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/core/configgen"
	"boxpanel/internal/coredl"
	"boxpanel/internal/coreinfo"
	"boxpanel/internal/models"
	"boxpanel/internal/nodevalidator"
	"boxpanel/internal/readyprobe"
)

// coreIsRunning returns true if any core (sing-box or other) is running.
func (s *APIServer) coreIsRunning() bool {
	if s.runner.IsRunning() {
		return true
	}
	if s.activeCoreImpl != nil && s.activeCoreImpl.IsRunning() {
		return true
	}
	return false
}

// corePID returns the PID of the running core, or 0.
func (s *APIServer) corePID() int {
	if s.runner.IsRunning() {
		return s.runner.PID()
	}
	if s.activeCoreImpl != nil && s.activeCoreImpl.IsRunning() {
		return s.activeCoreImpl.PID()
	}
	return 0
}

// coreStop stops whatever core is running.
func (s *APIServer) coreStop() error {
	if s.runner.IsRunning() {
		return s.runner.Stop()
	}
	if s.activeCoreImpl != nil && s.activeCoreImpl.IsRunning() {
		err := s.activeCoreImpl.Stop()
		s.activeCoreImpl = nil
		s.runningKind = ""
		return err
	}
	return nil
}

func (s *APIServer) handleCoreStart(w http.ResponseWriter, r *http.Request) {
	if s.coreIsRunning() {
		writeError(w, 409, "already running")
		return
	}

	// Ensure any previous core process is fully stopped and port released.
	// This handles the case where the old process exited but the TCP port
	// is still in TIME_WAIT state.
	_ = s.coreStop()
	time.Sleep(200 * time.Millisecond)
	ctx := r.Context()
	var body struct {
		AutoFallback bool `json:"auto_fallback"`
	}
	_ = readJSON(r, &body)
	if body.AutoFallback {
		body.AutoFallback = true // 显式 true
	}

	st, _ := s.store.GetSettings(ctx)
	srv, err := s.store.GetServer(ctx, st.CurrentServerID)
	if err != nil || srv == nil {
		writeError(w, 400, "未选中服务器")
		return
	}

	// 候选内核列表：默认仅 active，auto_fallback=true 时按 SuggestCore 顺序回退
	cores := candidateCores(st, srv, body.AutoFallback)
	if len(cores) == 0 {
		writeError(w, 400, "未配置任何内核")
		return
	}

	// 自动补充兼容内核：如果所有已配置内核都不兼容当前节点，
	// 自动下载兼容的内核（如 xhttp 节点需要 Xray 内核）
	var downloadMsg string
	cores, dm, dlErr := s.ensureCompatibleCore(cores, srv, st)
	if dlErr != nil {
		// 下载失败不阻断启动 — 降级为警告，让后续流程尝试用不兼容内核启动
		// （compat.go 兜底会将不支持的 transport 替换为 block outbound）
		slog.Warn("auto-download compatible core failed, will try with existing cores",
			"error", dlErr, "transport", srv.TransportType)
		downloadMsg = "自动下载兼容内核失败: " + dlErr.Error()
	} else if dm != "" {
		downloadMsg = dm
	}
	// 刷新 settings（ensureCompatibleCore 可能已追加新内核并保存）
	if dm != "" {
		st, _ = s.store.GetSettings(ctx)
		// 自动下载的内核已在列表末尾，重新排序让兼容内核排在前面
		cores = candidateCores(st, srv, true)
	}

	type attempt struct {
		CoreID   string `json:"core_id"`
		Version  string `json:"version"`
		Path     string `json:"path"`
		OK       bool   `json:"ok"`
		Error    string `json:"error,omitempty"`
		Started  bool   `json:"started,omitempty"`
		PID      int    `json:"pid,omitempty"`
		Duration string `json:"duration,omitempty"`
	}

	var attempts []attempt
	for i, core := range cores {
		if i > 0 && !body.AutoFallback {
			break
		}
		a := attempt{CoreID: core.ID, Version: core.Version, Path: core.Path}
		start := time.Now()
		err := s.startWithCore(*srv, core, st)
		a.Duration = time.Since(start).String()
		if err == nil {
			a.OK = true
			a.Started = true
			a.PID = s.corePID()
			attempts = append(attempts, a)
			break
		}
		a.Error = err.Error()
		attempts = append(attempts, a)
		// 切下一个内核（先把 runner exe 路径还原）
		if !body.AutoFallback {
			break
		}
		if i+1 >= len(cores) || i+1 >= 3 {
			break
		}
	}

	// 聚合结果
	lastAttempt := attempts[len(attempts)-1]
	if lastAttempt.OK {
		// 自动切换 active_core_id 为实际使用的内核（v2rayN 路线：根据节点协议自动选内核）
		prevActiveID := st.ActiveCoreID
		if lastAttempt.CoreID != prevActiveID {
			st.ActiveCoreID = lastAttempt.CoreID
			_ = s.store.SaveSettings(ctx, st)
			s.mu.Lock()
			s.settings = st
			s.mu.Unlock()
			slog.Info("auto-switched active core", "core_id", lastAttempt.CoreID, "version", lastAttempt.Version, "prev", prevActiveID)
		}
		writeJSON(w, 200, map[string]any{
			"started":       true,
			"pid":           lastAttempt.PID,
			"core":          lastAttempt.Path,
			"core_version":  lastAttempt.Version,
			"core_id":       lastAttempt.CoreID,
			"server":        srv.Name,
			"attempts":      attempts,
			"probe_method":  s.lastProbeMethod,
			"auto_switched": lastAttempt.CoreID != prevActiveID,
			"auto_download": downloadMsg,
		})
		return
	}

	// All candidates failed — build a helpful aggregated error message
	finalError := lastAttempt.Error
	if len(attempts) > 1 {
		// Multiple candidates tried: summarize
		parts := make([]string, 0, len(attempts))
		for _, a := range attempts {
			if a.Error != "" {
				parts = append(parts, a.Error)
			}
		}
		finalError = "所有候选内核均不兼容: " + strings.Join(parts, "; ")
	}
	// Add actionable suggestion
	transport := srv.TransportType
	if transport != "" && transport != "tcp" && transport != "raw" {
		finalError += fmt.Sprintf("。节点传输类型 %q 需要兼容内核（如 Xray），请在设置中下载", transport)
	}
	writeJSON(w, 500, map[string]any{
		"error":    finalError,
		"attempts": attempts,
	})
}

// candidateCores returns the cores to try in order.
// Uses NodeValidator to prioritize cores that support the server's protocol/transport.
// This is the v2rayN approach: auto-select the best core, not hard-block.
func candidateCores(st models.Settings, srv *models.Server, autoFallback bool) []models.CoreConfig {
	if len(st.Cores) == 0 {
		return nil
	}

	// Score each core: valid=0 (best), warnings=1, invalid=2
	type scoredCore struct {
		core  models.CoreConfig
		score int
	}
	var scored []scoredCore
	for i := range st.Cores {
		c := &st.Cores[i]
		kind := c.Kind
		if kind == "" {
			kind = models.CoreKindSingBox
		}
		vr := nodevalidator.Validate(*srv, kind)
		score := 2 // default: invalid
		if vr.Valid {
			if len(vr.Warnings) > 0 {
				score = 1 // valid but with warnings
			} else {
				score = 0 // perfectly valid
			}
		}
		scored = append(scored, scoredCore{core: *c, score: score})
	}

	// Sort by score (best first), stable sort preserves original order within same score
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	out := make([]models.CoreConfig, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.core)
	}

	// If not autoFallback, only try the best core
	if !autoFallback && len(out) > 0 {
		return []models.CoreConfig{out[0]}
	}

	// Try up to 3 cores in priority order
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// startWithCore 切换 exe 并启动核心。
func (s *APIServer) startWithCore(srv models.Server, core models.CoreConfig, st models.Settings) error {
	ctx := context.Background()

	// --- NodeValidator 前置校验：仅警告，不硬拦截 ---
	// 核心理念：内核启动后可以切换节点，不应因为当前节点不兼容就阻止启动。
	// 不兼容节点的 outbound 由 compat.go 兜底替换为 block outbound，其他节点正常工作。
	coreKind := core.Kind
	if coreKind == "" {
		coreKind = models.CoreKindSingBox
	}
	if vr := nodevalidator.Validate(srv, coreKind); !vr.Valid {
		msgs := make([]string, 0, len(vr.Errors))
		for _, e := range vr.Errors {
			msgs = append(msgs, e.Message)
		}
		slog.Warn("current server incompatible with core, using block outbound fallback",
			"core", core.Label, "kind", coreKind, "server", srv.Name,
			"errors", strings.Join(msgs, "; "))
	}

	// 确保 core.Version 有值（版本适配依赖此字段）
	coreVersion := core.Version
	if coreVersion == "" {
		if v, err := probeVersion(core.Path); err == nil {
			coreVersion = v
		}
	}
	if coreVersion == "" {
		coreVersion = "1.10.0"
		slog.Warn("core version unknown, using conservative default", "path", core.Path)
	}

	profile := defaultProfile(st)
	if st.CurrentProfileID != "" {
		if p, err := s.store.GetProfile(ctx, st.CurrentProfileID); err == nil && p != nil {
			profile = *p
		}
	}
	groups, _ := s.store.ListGroups(ctx)
	allServers, _ := s.store.ListServers(ctx)
	rules, _ := s.store.ListRoutingRules(ctx, "default")
	ruleSets := defaultRuleSets()
	if dbSets, _ := s.store.ListRuleSets(ctx); len(dbSets) > 0 {
		ruleSets = dbSets
	}

	if st.ClashAPISecret == "" {
		st.ClashAPISecret = "boxpanel"
	}
	if st.ClashAPIPort == 0 {
		st.ClashAPIPort = config.ClashAPIPort
	}
	_ = s.store.SaveSettings(ctx, st)
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()

	// ---- 按内核类型分发配置生成 + 校验 + 启动 ----
	//
	// sing-box: 使用 configgen.Builder 生成 sing-box JSON，runner.Check/Start
	// Xray/mihomo/hysteria2: 使用各自的 Core.BuildConfig 生成配置，Core.Check/Start

	target := config.GeneratedConfigPath()

	switch coreKind {
	case models.CoreKindSingBox:
		// sing-box: 使用 Builder 生成 + Runner 管理
		s.activeCoreImpl = nil
		s.runningKind = models.CoreKindSingBox
		s.runner.SetExePath(core.Path)
		t, err := s.gen.Build(configgen.BuildRequest{
			Profile:       profile,
			CurrentServer: srv,
			AllServers:    allServers,
			Groups:        groups,
			RoutingRules:  rules,
			RuleSets:      ruleSets,
			Settings:      st,
			CoreVersion:   coreVersion,
		})
		if err != nil {
			return fmt.Errorf("build config: %w", err)
		}
		target = t
		if err := s.runner.Check(target); err != nil {
			return fmt.Errorf("config check: %w", err)
		}
		if err := s.runner.Start(target); err != nil {
			return fmt.Errorf("start: %w", err)
		}

	default:
		// Xray / mihomo / hysteria2: 使用 CoreManager 分发
		coreImpl, ok := s.coreMgr.Get(coreKind)
		if !ok {
			return fmt.Errorf("core kind %q not registered in CoreManager", coreKind)
		}
		coreImpl.SetExePath(core.Path)
		s.activeCoreImpl = coreImpl
		s.runningKind = coreKind

		buildReq := corepkg.BuildRequest{
			Profile:       profile,
			CurrentServer: srv,
			AllServers:    allServers,
			Groups:        groups,
			RoutingRules:  rules,
			RuleSets:      ruleSets,
			Settings:      st,
		}
		if err := coreImpl.BuildConfig(ctx, buildReq, target); err != nil {
			return fmt.Errorf("build %s config: %w", coreKind, err)
		}
		if err := coreImpl.Check(ctx, target); err != nil {
			return fmt.Errorf("config check: %w", err)
		}
		if err := coreImpl.Start(ctx, target); err != nil {
			return fmt.Errorf("start: %w", err)
		}
	}

	// Quick check: did the process exit immediately? (e.g. port conflict)
	// If the core crashed during startup, don't waste time on readiness probes.
	if !s.coreIsRunning() {
		s.coreStop()
		return fmt.Errorf("core exited immediately (port conflict or config error)")
	}

	// 等待内核就绪：优先 SOCKS5 握手探测（借鉴 v2rayN），回退到 Clash API 探测
	mixedAddr := fmt.Sprintf("%s:%d", profile.Listen, profile.ListenPort)
	if profile.ListenPort == 0 {
		mixedAddr = fmt.Sprintf("127.0.0.1:%d", config.MixedInboundPort)
	}

	// Strategy 1: SOCKS5 handshake (works for all cores with mixed/socks inbound)
	socksResult := readyprobe.WaitForReady(ctx, mixedAddr, 4*time.Second)
	if socksResult.Ready {
		// SOCKS5 ready — core is fully initialized
		if c := s.clash; c != nil {
			s.refreshClashClient()
		}
		slog.Info("core ready (SOCKS5 probe)", "latency", socksResult.Latency)
		s.lastProbeMethod = "socks5"
		return nil
	}

	// Strategy 2: Clash API reachable (for sing-box / mihomo)
	if c := s.clash; c != nil {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if c.Reachable(ctx) {
				s.refreshClashClient()
				slog.Info("core ready (Clash API probe)")
				s.lastProbeMethod = "clash_api"
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Strategy 3: Fallback TCP port probe
	tcpResult := readyprobe.TCPProbe(mixedAddr)
	if tcpResult.Ready {
		slog.Info("core ready (TCP probe, fallback)")
		s.lastProbeMethod = "tcp"
		return nil
	}

	// All probes failed — core not ready
	s.coreStop()
	return fmt.Errorf("core started but not reachable via SOCKS5/Clash API/TCP (socks5: %s)", socksResult.Error)
}

func (s *APIServer) handleCoreStop(w http.ResponseWriter, r *http.Request) {
	if !s.coreIsRunning() {
		writeError(w, 409, "not running")
		return
	}
	s.persistTraffic() // save cumulative traffic before core stops
	_ = s.coreStop()
	writeJSON(w, 200, map[string]bool{"stopping": true})
}

func (s *APIServer) handleCoreRestart(w http.ResponseWriter, r *http.Request) {
	s.persistTraffic() // save cumulative traffic before core stops
	_ = s.coreStop()
	// 重新构建并启动
	s.handleCoreStart(w, r)
}

// ----- Clash API passthrough -----

func (s *APIServer) handleClashProxies(w http.ResponseWriter, r *http.Request) {
	if !s.coreIsRunning() {
		writeError(w, 503, "内核未运行")
		return
	}
	if s.clash == nil {
		writeError(w, 503, "Clash API 不可用")
		return
	}
	proxies, err := s.clash.Proxies(r.Context())
	if err != nil {
		if errors.Is(err, clashapi.ErrCoreNotRunning) {
			writeError(w, 503, "内核未运行")
		} else {
			writeError(w, 502, err.Error())
		}
		return
	}
	writeJSON(w, 200, proxies)
}

func (s *APIServer) handleClashSelect(w http.ResponseWriter, r *http.Request) {
	if !s.coreIsRunning() {
		writeError(w, 503, "内核未运行，请先启动内核")
		return
	}
	if s.clash == nil {
		writeError(w, 503, "Clash API 不可用")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = readJSON(r, &body)
	group := chi.URLParam(r, "name")
	if err := s.clash.SelectProxy(r.Context(), group, body.Name); err != nil {
		if errors.Is(err, clashapi.ErrCoreNotRunning) {
			writeError(w, 503, "内核未运行，请先启动内核")
		} else {
			writeError(w, 502, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]string{"selected": body.Name, "group": group})
}

func (s *APIServer) handleClashConnections(w http.ResponseWriter, r *http.Request) {
	if !s.coreIsRunning() {
		writeError(w, 503, "内核未运行")
		return
	}
	if s.clash == nil {
		writeError(w, 503, "Clash API 不可用")
		return
	}
	conns, err := s.clash.Connections(r.Context())
	if err != nil {
		if errors.Is(err, clashapi.ErrCoreNotRunning) {
			writeError(w, 503, "内核未运行")
		} else {
			writeError(w, 502, err.Error())
		}
		return
	}
	writeJSON(w, 200, conns)
}

// defaultProfile returns a sensible default profile derived from settings/mode.
func defaultProfile(st models.Settings) models.Profile {
	return models.Profile{
		ID:                    "default",
		Name:                  "默认",
		Mode:                  st.Mode,
		Listen:                "127.0.0.1",
		ListenPort:            st.ListenPort,
		Sniff:                 true,
		TunEnabled:            st.Mode == "ai",
		TunInterface:          "sing-box-tun",
		TunAddress:            "172.19.0.1/30",
		TunStack:              "system",
		TunMTU:                1500,
		TunAutoRoute:          true,
		TunStrictRoute:        true,
		DirectDNS:             "223.5.5.5",
		ProxyDNS:              "8.8.8.8",
		RouteFinal:            "direct",
		DefaultDomainResolver: "dns-direct",
	}
}

// defaultRuleSets returns the built-in CN/geolocation rule-sets.
func defaultRuleSets() []models.RuleSet {
	return []models.RuleSet{
		{ID: "rs_cn", Tag: "geosite-cn", Type: "local", Format: "binary",
			Path: "geosite-cn.srs", Enabled: true},
		{ID: "rs_notcn", Tag: "geosite-geolocation-!cn", Type: "local", Format: "binary",
			Path: "geosite-geolocation-!cn.srs", Enabled: true},
	}
}

// ensureCompatibleCore checks whether any of the candidate cores can handle
// the given server. If not, it automatically downloads a compatible core
// (e.g. Xray for xhttp/splithttp transport) and appends it to the list.
//
// Returns the (possibly extended) cores list, a download message if a core
// was auto-downloaded, or an error if the download failed.
func (s *APIServer) ensureCompatibleCore(cores []models.CoreConfig, srv *models.Server, st models.Settings) ([]models.CoreConfig, string, error) {
	transport := srv.TransportType
	if transport == "" || transport == "tcp" || transport == "raw" {
		return cores, "", nil // no transport incompatibility possible
	}

	// Check if any candidate core supports this transport
	for _, c := range cores {
		kind := c.Kind
		if kind == "" {
			kind = models.CoreKindSingBox
		}
		if coreinfo.SupportsTransport(kind, transport) {
			return cores, "", nil // already have a compatible core
		}
	}

	// All candidates are incompatible — find which core kind supports this transport
	// and auto-download its latest stable version
	var requiredKind string
	for _, kind := range []string{models.CoreKindXray, models.CoreKindMihomo, models.CoreKindHysteria2} {
		if coreinfo.SupportsTransport(kind, transport) && coreinfo.SupportsProtocol(kind, srv.Protocol) {
			requiredKind = kind
			break
		}
	}
	if requiredKind == "" {
		return cores, "", nil // no core supports this combo, give up (builder will block it)
	}

	info := coreinfo.GetInfo(requiredKind)
	slog.Info("no compatible core for transport, auto-downloading",
		"transport", transport, "required_kind", requiredKind, "core_name", info.Name)

	// Download the latest stable version of the required core
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	coreCfg, err := s.downloadLatestCore(ctx, requiredKind, st)
	if err != nil {
		return cores, "", fmt.Errorf("下载 %s 失败: %w", info.Name, err)
	}

	// Add to settings.Cores and save
	st.Cores = append(st.Cores, *coreCfg)
	if err := s.store.SaveSettings(ctx, st); err != nil {
		return cores, "", fmt.Errorf("保存 %s 配置失败: %w", info.Name, err)
	}
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()

	// Append to candidate list
	cores = append(cores, *coreCfg)
	msg := fmt.Sprintf("已自动下载 %s %s", info.Name, coreCfg.Version)
	slog.Info("auto-downloaded compatible core", "kind", requiredKind, "version", coreCfg.Version, "path", coreCfg.Path)
	return cores, msg, nil
}

// downloadLatestCore downloads the latest stable version of a core kind.
func (s *APIServer) downloadLatestCore(ctx context.Context, kind string, st models.Settings) (*models.CoreConfig, error) {
	// Find latest stable version
	releases, err := s.multiDl.ListAvailableVersions(ctx, kind, false)
	if err != nil {
		return nil, fmt.Errorf("获取 %s 版本列表失败: %w", kind, err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%s 没有可用版本", kind)
	}
	latestVersion := strings.TrimPrefix(releases[0].TagName, "v")

	// Download
	coreCfg, err := s.multiDl.DownloadCore(ctx, kind, latestVersion, st.CustomDownloadMirrors, func(p coredl.Progress) {
		slog.Info("downloading core", "kind", kind, "stage", p.Stage, "version", p.Version, "pct", fmt.Sprintf("%.0f%%", p.Pct))
	})
	if err != nil {
		return nil, err
	}
	return coreCfg, nil
}
