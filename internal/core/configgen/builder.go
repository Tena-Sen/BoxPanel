// Package configgen assembles a complete sing-box JSON config from a profile,
// the selected server, routing rules and rule-sets.
//
// 关键：每个生成的配置都注入 experimental.clash_api，使面板后端能作为
// Clash API 客户端获得真实代理选择/测速/流量/日志能力。
package configgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"boxpanel/internal/config"
	"boxpanel/internal/coreinfo"
	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
	"boxpanel/internal/routing"
	"boxpanel/internal/store"
)

// Builder builds sing-box configs.
type Builder struct {
	store store.Store
}

// New creates a Builder backed by the given store.
func New(s store.Store) *Builder { return &Builder{store: s} }

// BuildRequest holds the inputs needed to build a runtime config.
//
// IMPORTANT: BuildRequest contains reference types (slices, maps) that can be
// mutated by concurrent goroutines (e.g. user editing servers while Build runs).
// Callers should either:
//   - Pass a value copy (BuildRequest is already a value, but slices inside share backing arrays)
//   - Call Freeze() to deep-copy all slices/maps into an immutable snapshot
//
// Inspired by v2rayN's immutable config context (C# record).
type BuildRequest struct {
	Profile       models.Profile
	CurrentServer models.Server   // 当前选中节点（作为 proxy selector 的 default）
	AllServers    []models.Server // 所有节点（每个 emit 一个独立 outbound，供 group 引用 + selector 切换）
	Groups        []models.Group
	RoutingRules  []models.RoutingRule
	RuleSets      []models.RuleSet
	Settings      models.Settings
	CoreVersion   string // 目标 sing-box 版本（如 "1.14.0-alpha.1"），空 = 不适配
}

// Freeze returns a deep-copied snapshot of this BuildRequest that is safe from
// concurrent modification. The returned BuildRequest shares no backing arrays
// with the original.
func (req BuildRequest) Freeze() BuildRequest {
	// Deep copy slices
	if req.AllServers != nil {
		cp := make([]models.Server, len(req.AllServers))
		copy(cp, req.AllServers)
		req.AllServers = cp
	}
	if req.Groups != nil {
		cp := make([]models.Group, len(req.Groups))
		copy(cp, req.Groups)
		req.Groups = cp
	}
	if req.RoutingRules != nil {
		cp := make([]models.RoutingRule, len(req.RoutingRules))
		copy(cp, req.RoutingRules)
		req.RoutingRules = cp
	}
	if req.RuleSets != nil {
		cp := make([]models.RuleSet, len(req.RuleSets))
		copy(cp, req.RuleSets)
		req.RuleSets = cp
	}
	// Deep copy ServerIDs slices within Groups
	for i := range req.Groups {
		if req.Groups[i].ServerIDs != nil {
			cp := make([]string, len(req.Groups[i].ServerIDs))
			copy(cp, req.Groups[i].ServerIDs)
			req.Groups[i].ServerIDs = cp
		}
	}
	// Deep copy Values slices within RoutingRules
	for i := range req.RoutingRules {
		if req.RoutingRules[i].Values != nil {
			cp := make([]string, len(req.RoutingRules[i].Values))
			copy(cp, req.RoutingRules[i].Values)
			req.RoutingRules[i].Values = cp
		}
	}
	// Deep copy TLSALPN slices within Servers
	for i := range req.AllServers {
		if req.AllServers[i].TLSALPN != nil {
			cp := make([]string, len(req.AllServers[i].TLSALPN))
			copy(cp, req.AllServers[i].TLSALPN)
			req.AllServers[i].TLSALPN = cp
		}
		if req.AllServers[i].TransportHeaders != nil {
			cp := make(map[string]string, len(req.AllServers[i].TransportHeaders))
			for k, v := range req.AllServers[i].TransportHeaders {
				cp[k] = v
			}
			req.AllServers[i].TransportHeaders = cp
		}
	}
	return req
}

// Build assembles the full sing-box config and writes it to the runtime path.
// Returns the path to the generated config.
//
// Build freezes the request before use to prevent concurrent modification.
func (b *Builder) Build(req BuildRequest) (string, error) {
	// Freeze: create an immutable snapshot to prevent data races
	// (inspired by v2rayN's immutable config context)
	req = req.Freeze()

	cfg := map[string]any{}

	// 1. log
	cfg["log"] = map[string]any{
		"level":     req.Settings.LogLevel,
		"timestamp": true,
	}

	// 2. dns
	cfg["dns"] = b.buildDNS(req)

	// 3. inbounds
	inbounds, err := b.buildInbounds(req)
	if err != nil {
		return "", err
	}
	cfg["inbounds"] = inbounds

	// 4. outbounds: proxy + groups + direct/block/dns
	outbounds, err := b.buildOutbounds(req)
	if err != nil {
		return "", err
	}
	cfg["outbounds"] = outbounds

	// 5. route
	route, err := b.buildRoute(req)
	if err != nil {
		return "", err
	}
	cfg["route"] = route

	// 6. experimental.clash_api（枢纽）
	secret := req.Settings.ClashAPISecret
	if secret == "" {
		secret = "boxpanel"
	}
	cfg["experimental"] = map[string]any{
		"clash_api": map[string]any{
			"external_controller": fmt.Sprintf("%s:%d", config.ClashAPIHost, req.Settings.ClashAPIPort),
			"secret":              secret,
		},
		"cache_file": map[string]any{
			"enabled": true,
		},
	}

	// 写入 runtime 配置文件
	target := config.GeneratedConfigPath()
	tmp := target + ".tmp"

	// 应用 schema 适配器（按目标内核版本剔除不兼容字段）
	// 始终运行：空版本用保守默认值 "1.10.0"（确保 xhttp 不会被误转为 splithttp）
	coreVersion := req.CoreVersion
	if coreVersion == "" {
		coreVersion = "1.10.0" // 保守默认：不转 splithttp，不剥新字段
	}
	NewAdapter(coreVersion).Apply(cfg)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", fmt.Errorf("rename config: %w", err)
	}
	return target, nil
}

// ValidateConfig runs `sing-box check` on the generated config (best-effort).
func ValidateConfig(configPath, exePath string) error {
	// 由 runner 执行；这里仅占位接口，实际由调用方通过 Runner.Check 调用。
	_ = configPath
	_ = exePath
	return nil
}

// ----- DNS -----

func (b *Builder) buildDNS(req BuildRequest) map[string]any {
	if req.Profile.Mode == "global" {
		return map[string]any{
			"servers": []map[string]any{{
				"tag":    "dns-proxy",
				"type":   "udp",
				"server": nonEmpty(req.Profile.ProxyDNS, "8.8.8.8"),
				"detour": "proxy",
			}},
			"final": "dns-proxy",
		}
	}
	servers := []map[string]any{
		{"tag": "dns-direct", "type": "udp", "server": nonEmpty(req.Profile.DirectDNS, "223.5.5.5")},
		{"tag": "dns-proxy", "type": "udp", "server": nonEmpty(req.Profile.ProxyDNS, "8.8.8.8"), "detour": "proxy"},
	}
	rules := []map[string]any{}
	if req.Profile.Mode == "ai" {
		// AI 模式：特定域名走代理 DNS
	}
	rules = append(rules,
		map[string]any{"rule_set": []string{"geosite-cn"}, "server": "dns-direct"},
		map[string]any{"rule_set": []string{"geosite-geolocation-!cn"}, "server": "dns-proxy"},
	)
	return map[string]any{
		"servers": servers,
		"rules":   rules,
		"final":   "dns-direct",
	}
}

// ----- inbounds -----

func (b *Builder) buildInbounds(req BuildRequest) ([]map[string]any, error) {
	var inbounds []map[string]any
	if req.Profile.Mode == "ai" || req.Profile.TunEnabled {
		inbounds = append(inbounds, map[string]any{
			"type":          "tun",
			"tag":           "tun-in",
			"interface_name": nonEmpty(req.Profile.TunInterface, "sing-box-tun"),
			"address":       nonEmpty(req.Profile.TunAddress, "172.19.0.1/30"),
			"auto_route":    req.Profile.TunAutoRoute,
			"strict_route":  req.Profile.TunStrictRoute,
			"stack":         nonEmpty(req.Profile.TunStack, "system"),
			"mtu":           orDefault(req.Profile.TunMTU, 1500),
		})
	}
	mixed := map[string]any{
		"type":        "mixed",
		"tag":         "mixed-in",
		"listen":      nonEmpty(req.Profile.Listen, "127.0.0.1"),
		"listen_port": orDefault(req.Profile.ListenPort, config.MixedInboundPort),
	}
	// Note: sniff / sniff_override_destination are NOT set on inbound anymore.
	// Since sing-box 1.11.0 these are route rule actions (sniff / resolve).
	// They are added in buildRoute() below.
	inbounds = append(inbounds, mixed)
	return inbounds, nil
}

// ----- outbounds -----

func (b *Builder) buildOutbounds(req BuildRequest) ([]map[string]any, error) {
	var outbounds []map[string]any

	// 1. 每个 server 一个独立 outbound（tag: srv-<id>）
	//    供 main selector 和 group 引用，使 Clash API 可在运行时切换。
	//
	//    对 sing-box 不支持的 transport type（如 xhttp/splithttp），
	//    生成 block outbound 而非包含非法 transport 的 outbound，
	//    避免 sing-box check 报 "unknown transport type" 错误。
	serverTags := make([]string, 0, len(req.AllServers))
	for _, srv := range req.AllServers {
		transport := srv.TransportType
		if transport == "" || transport == "tcp" || transport == "raw" {
			transport = "" // no transport
		}
		// Check: if this transport type is unsupported by sing-box, emit a block outbound instead
		if transport != "" && !coreinfo.SupportsTransport(models.CoreKindSingBox, transport) {
			// sing-box does not support this transport — emit a placeholder block outbound
			// so the tag is still resolvable (for groups/selectors) but won't cause config errors.
			ob := map[string]any{
				"tag":  ServerTag(srv.ID),
				"type": "block",
			}
			outbounds = append(outbounds, ob)
			serverTags = append(serverTags, ServerTag(srv.ID))
			continue
		}

		ob, err := protocol.Outbound(srv)
		if err != nil {
			return nil, fmt.Errorf("build outbound for %s: %w", srv.Protocol, err)
		}
		ob["tag"] = ServerTag(srv.ID)
		outbounds = append(outbounds, ob)
		serverTags = append(serverTags, ServerTag(srv.ID))
	}

	// 2. 主 proxy = selector 包含所有 server，default = 当前选中
	//    如果当前选中的 server 被 block（transport 不兼容），自动选择第一个兼容的 server
	if len(serverTags) > 0 {
		defaultTag := ServerTag(req.CurrentServer.ID)
		// Check if the current server's outbound is a block (transport-incompatible)
		currentTransport := req.CurrentServer.TransportType
		if currentTransport == "" || currentTransport == "tcp" || currentTransport == "raw" {
			currentTransport = ""
		}
		if currentTransport != "" && !coreinfo.SupportsTransport(models.CoreKindSingBox, currentTransport) {
			// Current server is incompatible — find first compatible server
			defaultTag = ""
			for _, srv := range req.AllServers {
				t := srv.TransportType
				if t == "" || t == "tcp" || t == "raw" {
					t = ""
				}
				if t == "" || coreinfo.SupportsTransport(models.CoreKindSingBox, t) {
					defaultTag = ServerTag(srv.ID)
					break
				}
			}
			if defaultTag == "" {
				defaultTag = serverTags[0] // fallback: first tag even if all blocked
			}
		}
		proxySel := map[string]any{
			"type":      "selector",
			"tag":       "proxy",
			"outbounds": serverTags,
			"default":   defaultTag,
		}
		outbounds = append(outbounds, proxySel)
	} else {
		// 没有 server 时仍要保留 proxy 占位（fallback 到 direct）
		outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "proxy"})
	}

	// 3. 用户自定义分组
	for _, g := range req.Groups {
		gOb := b.buildGroupOutbound(g)
		if gOb != nil {
			outbounds = append(outbounds, gOb)
		}
	}

	// 4. 固定出站（sing-box 1.13+ 移除 type: dns，由 route action 接管）
	outbounds = append(outbounds,
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
	)
	return outbounds, nil
}

// ServerTag returns the outbound tag for a server ID.
func ServerTag(serverID string) string { return "srv-" + serverID }

// buildGroupOutbound compiles a group into a sing-box outbound.
// Maps Clash-style group types to sing-box outbound types:
//
//	Clash          →  sing-box
//	selector       →  selector
//	url_test       →  urltest
//	fallback       →  fallback (1.11-1.12) / urltest (1.13+, fallback removed)
//	load_balance   →  urltest (sing-box has no load_balance, best-effort fallback)
func (b *Builder) buildGroupOutbound(g models.Group) map[string]any {
	var members []string
	for _, sid := range g.ServerIDs {
		members = append(members, ServerTag(sid))
	}
	if len(members) == 0 {
		return nil
	}

	// Map Clash-style group type → sing-box outbound type
	singboxType := mapGroupType(g.Type)

	ob := map[string]any{
		"type":      singboxType,
		"tag":       GroupTag(g.ID),
		"outbounds": members,
	}
	switch g.Type {
	case models.GroupURLTest, models.GroupFallback, models.GroupLoadBalance:
		if g.URL != "" {
			ob["url"] = g.URL
		} else {
			ob["url"] = "http://www.gstatic.com/generate_204"
		}
		if g.Interval > 0 {
			ob["interval"] = fmt.Sprintf("%ds", g.Interval)
		}
		if g.Tolerance > 0 {
			ob["tolerance"] = g.Tolerance
		}
	}
	return ob
}

// mapGroupType maps a Clash-style group type to the sing-box outbound type name.
func mapGroupType(clashType string) string {
	switch clashType {
	case models.GroupURLTest:
		return "urltest" // sing-box uses "urltest" (no underscore)
	case models.GroupFallback:
		return "fallback" // sing-box 1.11-1.12; 1.13+ removed, Adapter will handle
	case models.GroupLoadBalance:
		return "urltest" // sing-box has no load_balance, fall back to urltest
	case models.GroupSelector:
		return "selector"
	default:
		return clashType // unknown: pass through (may cause sing-box error, but better than crash)
	}
}

// GroupTag returns the outbound tag for a group ID.
func GroupTag(groupID string) string { return "grp-" + groupID }

// ----- route -----

func (b *Builder) buildRoute(req BuildRequest) (map[string]any, error) {
	rules := []map[string]any{}

	// Sniff + DNS hijack (sing-box 1.11+ rule actions, replacing legacy inbound sniff fields)
	if req.Profile.Sniff {
		sniffAction := map[string]any{"action": "sniff"}
		if req.Profile.SniffOverride {
			sniffAction["override_destination"] = true
		}
		rules = append(rules, sniffAction)
		// hijack-dns replaces the old outbound type: dns
		rules = append(rules, map[string]any{
			"protocol": "dns",
			"action":   "hijack-dns",
		})
	}

	// 私网直连
	rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})

	// AI 模式：指定进程走代理
	if req.Profile.Mode == "ai" {
		procs := aiProcessNames()
		if len(procs) > 0 {
			rules = append(rules, map[string]any{"process_name": procs, "outbound": "proxy"})
		}
	}

	// 用户自定义规则（按 order）
	for _, r := range req.RoutingRules {
		if c := routing.Compile(r); c != nil {
			rules = append(rules, c)
		}
	}

	// 规则集规则
	for _, rs := range req.RuleSets {
		if !rs.Enabled {
			continue
		}
		if rs.Tag == "geosite-cn" {
			rules = append(rules, map[string]any{"rule_set": []string{"geosite-cn"}, "outbound": "direct"})
		} else if rs.Tag == "geosite-geolocation-!cn" {
			rules = append(rules, map[string]any{"rule_set": []string{"geosite-geolocation-!cn"}, "outbound": "proxy"})
		}
	}

	route := map[string]any{
		"rules": rules,
		"final": nonEmpty(req.Profile.RouteFinal, "direct"),
	}
	if req.Profile.DefaultDomainResolver != "" {
		route["default_domain_resolver"] = req.Profile.DefaultDomainResolver
	}

	// rule_set 定义
	var ruleSets []map[string]any
	for _, rs := range req.RuleSets {
		if !rs.Enabled {
			continue
		}
		rsDef := map[string]any{
			"tag":    rs.Tag,
			"type":   rs.Type,
			"format": rs.Format,
		}
		if rs.Type == "local" {
			// 使用绝对路径
			rsDef["path"] = filepath.Join(config.RuleSetDir(), rs.Path)
		} else {
			// 远程：默认走本地缓存路径（让 sing-box 不必每次启动去外网拉）
			// 但保留 url + update_interval 让 sing-box 也能自检更新
			cacheDir := filepath.Join(config.DataDir(), "rulesets")
			tag := sanitizeTag(rs.Tag)
			ext := ".srs"
			if rs.Format == "source" {
				ext = ".source"
			}
			cached := filepath.Join(cacheDir, tag+ext)
			if _, err := os.Stat(cached); err == nil {
				rsDef["type"] = "local"
				rsDef["path"] = cached
			} else {
				rsDef["url"] = rs.URL
				if rs.DownloadDetour != "" {
					rsDef["download_detour"] = rs.DownloadDetour
				}
				if rs.UpdateInterval > 0 {
					rsDef["update_interval"] = fmt.Sprintf("%dd", rs.UpdateInterval)
				}
			}
		}
		ruleSets = append(ruleSets, rsDef)
	}
	if len(ruleSets) > 0 {
		route["rule_set"] = ruleSets
	}
	return route, nil
}

func sanitizeTag(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return r.Replace(s)
}

// compileRule 旧实现已抽到 internal/routing 包（Compile / CompileAll）

// aiProcessNames returns the process names routed to proxy in AI/TUN mode.
func aiProcessNames() []string {
	return []string{"Claude.exe", "Cursor.exe", "Codex.exe", "Claude x64.exe"}
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func orDefault(v, def int) int {
	if v != 0 {
		return v
	}
	return def
}

// EnsureConfigDir ensures the data directory exists.
func EnsureConfigDir() error {
	return os.MkdirAll(config.DataDir(), 0o755)
}
