// Package compat implements dynamic compatibility matching between a sing-box
// Server configuration and a sing-box core version.
//
// 规则基于 sing-box 官方 release notes / changelog 与协议支持矩阵：
//   https://github.com/SagerNet/sing-box/releases
//
// 返回 ok / warn / bad 三级，方便 UI 用颜色编码。
package compat

import (
	"boxpanel/internal/models"
)

// Level is the compatibility level of a server against a core version.
type Level string

const (
	OK   Level = "ok"   // 完全兼容
	Warn Level = "warn" // 能用但有字段被剥离/降级
	Bad  Level = "bad"  // 不兼容（节点或协议根本不被该版本支持）
)

// Reason describes why a non-ok level was assigned.
type Reason struct {
	Code    string `json:"code"`     // 机器可读
	Message string `json:"message"`  // 人类可读
	Action  string `json:"action"`   // 建议（"use core >= 1.10"）
}

// Result is the compatibility assessment of one server against one core.
type Result struct {
	ServerID string  `json:"server_id"`
	CoreVersion string `json:"core_version"`
	Level    Level   `json:"level"`
	Reasons  []Reason `json:"reasons,omitempty"`
	MinVersion string `json:"min_version"` // 节点所需的最低版本
}

// Matrix 返回 server 对每个 core 的兼容性矩阵。
type Matrix struct {
	Results []Result `json:"results"`
}

// CheckServer assesses one server against one core version.
//
// design:
// - If coreKind is non-empty, first check if the core kind supports the protocol
// - Then check sing-box version-specific compatibility (for singbox cores)
func CheckServer(srv models.Server, coreVersion string, coreKind ...string) Result {
	res := Result{
		ServerID:    srv.ID,
		CoreVersion: coreVersion,
		Level:       OK,
		MinVersion:  "1.0.0",
	}

	kind := ""
	if len(coreKind) > 0 {
		kind = coreKind[0]
	}

	// Cross-engine protocol compatibility check
	if kind != "" && kind != models.CoreKindSingBox {
		protocols := models.SupportedProtocolsByKind(kind)
		supported := false
		for _, p := range protocols {
			if p == srv.Protocol {
				supported = true
				break
			}
		}
		if !supported {
			res.Level = Bad
			res.Reasons = append(res.Reasons, Reason{
				Code:    "PROTO_UNSUPPORTED",
				Message: kindLabel(kind) + " does not support " + srv.Protocol,
				Action:  "Use sing-box or a core that supports " + srv.Protocol,
			})
			return res
		}
		// Non-sing-box cores: protocol is supported, version check is not applicable
		// Return OK since we already confirmed protocol support
		if coreVersion == "" {
			return res
		}
	}

	if coreVersion == "" {
		return res // unknown version, skip further checks
	}

	// 协议层最低版本
	protoMin := minVersionForProtocol(srv)
	if CompareVersions(coreVersion, protoMin) < 0 {
		res.Level = Bad
		res.Reasons = append(res.Reasons, Reason{
			Code:    "PROTO_UNSUPPORTED",
			Message: srv.Protocol + " 协议需要 sing-box >= " + protoMin,
			Action:  "使用内核 >= " + protoMin,
		})
	}

	// TLS / Reality 层
	if srv.TLSEnabled {
		tlsMin := "1.1.0" // 现代 TLS 配置基线
		if CompareVersions(coreVersion, tlsMin) < 0 {
			// 旧内核的 TLS 字段不完整（如 uTLS / reality 在 1.8+ 才稳定）
			res.Level = worse(res.Level, Warn)
			res.Reasons = append(res.Reasons, Reason{
				Code:    "TLS_OLD_CORE",
				Message: "TLS/uTLS/Reality 在 1.1.0+ 更稳定",
				Action:  "考虑升级到 1.1+",
			})
		}
	}

	// Reality 1.14+ 移除了 spider_x 字段
	if srv.RealityEnabled {
		// 检查是否使用 spider_x（v1.10~1.13 特有功能）
		hasSpiderX := srv.RealitySpiderX != ""
		isNewCore := CompareVersions(coreVersion, "1.14.0") >= 0
		if hasSpiderX && isNewCore {
			res.Level = worse(res.Level, Warn)
			res.Reasons = append(res.Reasons, Reason{
				Code:    "REALITY_SPIDERX_DROPPED",
				Message: "reality.spider_x 字段在 sing-box 1.14+ 已移除，将被自动剥离",
				Action:  "若节点握手失败，降级到 1.10~1.13",
			})
		}
		// 1.14 alpha Reality 协议可能与旧节点服务端不兼容（用户已踩坑）
		if isNewCore {
			res.Level = worse(res.Level, Warn)
			res.Reasons = append(res.Reasons, Reason{
				Code:    "REALITY_NEW_CORE_RISK",
				Message: "1.14 alpha Reality 协议握手实现更严格，可能与老节点服务端不兼容",
				Action:  "老 Reality 节点建议使用 1.10.7 stable",
			})
		}
	}

	// Transport / 协议特性
	if srv.TransportType != "" && srv.TransportType != "tcp" {
		ttMin := minVersionForTransport(srv.TransportType)
		if CompareVersions(coreVersion, ttMin) < 0 {
			res.Level = worse(res.Level, Bad)
			res.Reasons = append(res.Reasons, Reason{
				Code:    "TRANSPORT_UNSUPPORTED",
				Message: srv.TransportType + " 传输需要 sing-box >= " + ttMin,
				Action:  "升级内核或换传输",
			})
		}
	}

	// packet_encoding xudp → 1.9+
	if srv.Protocol == models.ProtoVless || srv.Protocol == models.ProtoVMess {
		// vless/vmess 通常 packet_encoding 字段默认 xudp（生成时强写）
		encMin := "1.9.0"
		if CompareVersions(coreVersion, encMin) < 0 {
			res.Level = worse(res.Level, Warn)
			res.Reasons = append(res.Reasons, Reason{
				Code:    "XUDP_OLD_CORE",
				Message: "packet_encoding=xudp 在 1.9+ 才支持，旧内核会忽略",
				Action:  "升级内核",
			})
		}
	}

	// VMess alterId > 0 → 已被移除
	if srv.Protocol == models.ProtoVMess && srv.AlterID > 0 {
		res.Level = worse(res.Level, Bad)
		res.Reasons = append(res.Reasons, Reason{
			Code:    "VMESS_AID_DEPRECATED",
			Message: "VMess alterId > 0 已废弃，1.11+ 完全移除",
			Action:  "alterId 必须为 0，否则无法工作",
		})
	}

	// Shadowsocks 旧 cipher
	if srv.Protocol == models.ProtoShadowsocks {
		oldCipher := map[string]bool{
			"aes-128-cfb": true, "aes-256-cfb": true,
			"chacha20": true, "rc4-md5": true,
		}
		if oldCipher[srv.Method] {
			res.Level = worse(res.Level, Warn)
			res.Reasons = append(res.Reasons, Reason{
				Code:    "SS_OLD_CIPHER",
				Message: "Shadowsocks cipher " + srv.Method + " 已不安全，新内核可能警告",
				Action:  "改用 chacha20-ietf-poly1305 / aes-256-gcm",
			})
		}
	}

	// 综合 min version
	res.MinVersion = maxVersion(res.MinVersion, protoMin)
	return res
}

// minVersionForProtocol returns the minimum sing-box version supporting the protocol.
func minVersionForProtocol(srv models.Server) string {
	switch srv.Protocol {
	case models.ProtoVless:
		return "1.1.0" // vless 早期版本就有，但现代字段需 1.1+
	case models.ProtoVMess:
		if srv.AlterID > 0 {
			return "1.1.0" // 1.11+ 移除了 alterId > 0
		}
		return "1.0.0"
	case models.ProtoTrojan:
		return "1.1.0"
	case models.ProtoShadowsocks:
		return "1.0.0"
	case models.ProtoHysteria2:
		return "1.8.0" // hy2 较晚加入
	case models.ProtoTUIC:
		return "1.9.0" // tuic 较晚加入
	case models.ProtoAnyTLS:
		return "1.11.0"
	}
	return "1.0.0"
}

func minVersionForTransport(t string) string {
	// xhttp/splithttp 是 Xray-core 独有协议，sing-box 从未支持。
	// 返回 "99.99.0" 使版本检查始终 Bad，由 NodeValidator/SupportsTransport 精确拦截。
	if t == "xhttp" || t == "splithttp" {
		return "99.99.0"
	}
	switch t {
	case "httpupgrade":
		return "1.8.0"
	case "ws", "grpc", "h2":
		return "1.0.0"
	}
	return "1.0.0"
}

func worse(a, b Level) Level {
	rank := map[Level]int{OK: 0, Warn: 1, Bad: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func maxVersion(a, b string) string {
	if CompareVersions(a, b) >= 0 {
		return a
	}
	return b
}

// CompareVersions: a<b -1, a==b 0, a>b 1. 简化版（与 configgen.compat 重复，
// 这里独立是因为不想 import 私有路径，将来可合并到 routing 或 core 包）。
func CompareVersions(a, b string) int {
	pa := parseV(a)
	pb := parseV(b)
	for i := 0; i < 4; i++ {
		va, _ := atOrZeroV(pa, i)
		vb, _ := atOrZeroV(pb, i)
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseV(v string) []int {
	for i, c := range v {
		if c == '-' || c == '+' {
			v = v[:i]
			break
		}
	}
	parts := []int{}
	cur := 0
	has := false
	for _, c := range v {
		if c == '.' {
			if has {
				parts = append(parts, cur)
			}
			cur, has = 0, false
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		cur = cur*10 + int(c-'0')
		has = true
	}
	if has {
		parts = append(parts, cur)
	}
	return parts
}

func atOrZeroV(s []int, i int) (int, bool) {
	if i < len(s) {
		return s[i], true
	}
	return 0, false
}

// SuggestCore returns the best core from a list, given a server.
//
// Now supports multiple core kinds (not just sing-box versions).
// Priority: protocol-match > OK > Warn > Bad.
func SuggestCore(srv models.Server, cores []models.CoreConfig) *models.CoreConfig {
	if len(cores) == 0 {
		return nil
	}
	rank := map[Level]int{OK: 0, Warn: 1, Bad: 2}
	var best *models.CoreConfig
	var bestRank int
	for i := range cores {
		c := &cores[i]
		r := CheckServer(srv, c.Version, c.Kind)
		if best == nil {
			best = c
			bestRank = rank[r.Level]
			continue
		}
		cr := rank[r.Level]
		if cr < bestRank {
			best, bestRank = c, cr
		} else if cr == bestRank {
			// Same level: prefer cores with Clash API support
			bestKind := best.Kind
			if bestKind == "" {
				bestKind = models.CoreKindSingBox
			}
			cKind := c.Kind
			if cKind == "" {
				cKind = models.CoreKindSingBox
			}
			bestHasClash := bestKind == models.CoreKindSingBox || bestKind == models.CoreKindMihomo
			cHasClash := cKind == models.CoreKindSingBox || cKind == models.CoreKindMihomo
			if cHasClash && !bestHasClash {
				best = c
			} else if cHasClash == bestHasClash && CompareVersions(c.Version, best.Version) < 0 {
				best = c
			}
		}
	}
	return best
}

// kindLabel returns a human-readable name for a core kind.
func kindLabel(kind string) string {
	switch kind {
	case models.CoreKindSingBox:
		return "sing-box"
	case models.CoreKindXray:
		return "Xray"
	case models.CoreKindMihomo:
		return "mihomo"
	case models.CoreKindHysteria2:
		return "Hysteria2"
	default:
		return kind
	}
}