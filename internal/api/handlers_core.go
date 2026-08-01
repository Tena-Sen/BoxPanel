package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/config"
	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/core/configgen"
	"boxpanel/internal/models"
	"boxpanel/internal/nodevalidator"
	"boxpanel/internal/readyprobe"
)

func (s *APIServer) handleCoreStart(w http.ResponseWriter, r *http.Request) {
	if s.runner.IsRunning() {
		writeError(w, 409, "already running")
		return
	}
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
			a.PID = s.runner.PID()
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
		})
		return
	}
	writeJSON(w, 500, map[string]any{
		"error":    lastAttempt.Error,
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

	// --- NodeValidator 前置校验（借鉴 v2rayN：排序而非硬拦截） ---
	coreKind := core.Kind
	if coreKind == "" {
		coreKind = models.CoreKindSingBox
	}
	if vr := nodevalidator.Validate(srv, coreKind); !vr.Valid {
		return fmt.Errorf("core %s (%s) incompatible: %s", core.Label, coreKind, vr.Errors[0].Message)
	}

	// 确保 core.Version 有值（splithttp/xhttp 等版本适配依赖此字段）
	coreVersion := core.Version
	if coreVersion == "" {
		if v, err := probeVersion(core.Path); err == nil {
			coreVersion = v
		}
	}
	if coreVersion == "" {
		// 探测也失败：给保守默认值，避免 Adapter 跳过版本适配
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

	// 切换 runner exe
	s.runner.SetExePath(core.Path)

	target, err := s.gen.Build(configgen.BuildRequest{
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
	// check 失败视为硬错（不算启动成功）
	if err := s.runner.Check(target); err != nil {
		return fmt.Errorf("config check: %w", err)
	}
	if err := s.runner.Start(target); err != nil {
		return fmt.Errorf("start: %w", err)
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
	s.runner.Stop()
	return fmt.Errorf("core started but not reachable via SOCKS5/Clash API/TCP (socks5: %s)", socksResult.Error)
}

func (s *APIServer) handleCoreStop(w http.ResponseWriter, r *http.Request) {
	if !s.runner.IsRunning() {
		writeError(w, 409, "not running")
		return
	}
	s.persistTraffic() // save cumulative traffic before core stops
	_ = s.runner.Stop()
	writeJSON(w, 200, map[string]bool{"stopping": true})
}

func (s *APIServer) handleCoreRestart(w http.ResponseWriter, r *http.Request) {
	s.persistTraffic() // save cumulative traffic before core stops
	_ = s.runner.Stop()
	// 重新构建并启动
	s.handleCoreStart(w, r)
}

// ----- Clash API passthrough -----

func (s *APIServer) handleClashProxies(w http.ResponseWriter, r *http.Request) {
	if !s.runner.IsRunning() {
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
	if !s.runner.IsRunning() {
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
	if !s.runner.IsRunning() {
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
