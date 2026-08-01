// Package import_ parses multi-format server lists into []models.Server.
//
// 支持格式：share link (vless/vmess/trojan/ss/hy2/tuic)、sing-box JSON config、
// Clash YAML、base64 包裹的订阅、剪贴板混合文本、文件自动识别。
package import_

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

// Result holds import statistics.
type Result struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Total   int `json:"total"`
	Servers []models.Server `json:"-"`
}

// FromText parses a blob of text (clipboard / subscription body) into servers.
func FromText(text string) ([]models.Server, error) {
	text = strings.TrimLeft(text, "\ufeff \t\r\n") // strip BOM + leading whitespace
	lines := splitLines(text)
	if len(lines) == 0 {
		// 可能整体是 JSON 或 YAML
		if srvs, err := tryStructured(text); err == nil && len(srvs) > 0 {
			return srvs, nil
		}
		return nil, nil
	}
	var out []models.Server
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// share link?
		if isShareLink(line) {
			if srv, err := protocol.ParseURI(line); err == nil {
				srv.RawLink = line
				out = append(out, *srv)
				continue
			}
		}
		// base64-wrapped (v2ray 订阅常见)?
		if decoded, ok := protocol.Base64Decode(line); ok && decoded != line {
			if sub, err := FromText(decoded); err == nil {
				out = append(out, sub...)
				continue
			}
		}
		// structured single line?
		if srvs, err := tryStructured(line); err == nil && len(srvs) > 0 {
			out = append(out, srvs...)
			continue
		}
	}
	return normalizeAll(out), nil
}

// FromBytes auto-detects format from raw bytes + filename hint.
func FromBytes(content []byte, filename string) ([]models.Server, error) {
	text := string(content)
	fn := strings.ToLower(filename)
	if strings.HasSuffix(fn, ".json") || strings.HasPrefix(strings.TrimSpace(text), "{") {
		if srvs, err := FromSingBoxJSON(text); err == nil && len(srvs) > 0 {
			return srvs, nil
		}
	}
	if strings.HasSuffix(fn, ".yaml") || strings.HasSuffix(fn, ".yml") || strings.Contains(text, "proxies:") {
		if srvs, err := FromClashYAML(text); err == nil && len(srvs) > 0 {
			return srvs, nil
		}
	}
	return FromText(text)
}

// FromSingBoxJSON extracts servers from a sing-box config JSON.
func FromSingBoxJSON(text string) ([]models.Server, error) {
	var cfg struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		return nil, fmt.Errorf("sing-box json: %w", err)
	}
	var out []models.Server
	for _, ob := range cfg.Outbounds {
		srv := serverFromSingBoxOutbound(ob)
		if srv != nil {
			out = append(out, *srv)
		}
	}
	return normalizeAll(out), nil
}

func serverFromSingBoxOutbound(ob map[string]any) *models.Server {
	typ, _ := ob["type"].(string)
	srv := &models.Server{}
	srv.Server, _ = ob["server"].(string)
	if p, ok := ob["server_port"].(float64); ok {
		srv.ServerPort = int(p)
	}
	srv.Name, _ = ob["tag"].(string)

	switch typ {
	case "vless":
		srv.Protocol = models.ProtoVless
		srv.UUID, _ = ob["uuid"].(string)
		srv.Flow, _ = ob["flow"].(string)
	case "vmess":
		srv.Protocol = models.ProtoVMess
		srv.UUID, _ = ob["uuid"].(string)
		if p, ok := ob["alter_id"].(float64); ok {
			srv.AlterID = int(p)
		}
	case "trojan":
		srv.Protocol = models.ProtoTrojan
		srv.Password, _ = ob["password"].(string)
	case "shadowsocks":
		srv.Protocol = models.ProtoShadowsocks
		srv.Method, _ = ob["method"].(string)
		srv.Password, _ = ob["password"].(string)
	case "hysteria2":
		srv.Protocol = models.ProtoHysteria2
		srv.Password, _ = ob["password"].(string)
	case "tuic":
		srv.Protocol = models.ProtoTUIC
		srv.TUICUUID, _ = ob["uuid"].(string)
		srv.TUICPassword, _ = ob["password"].(string)
	default:
		return nil
	}
	// transport
	if tr, ok := ob["transport"].(map[string]any); ok {
		srv.TransportType, _ = tr["type"].(string)
		srv.TransportPath, _ = tr["path"].(string)
		srv.TransportHost, _ = tr["host"].(string)
		srv.TransportMode, _ = tr["mode"].(string)
		if x, ok := tr["x_padding_bytes"]; ok {
			srv.TransportXPaddingBytes = fmt.Sprintf("%v", x)
		}
	}
	// tls
	if tls, ok := ob["tls"].(map[string]any); ok {
		srv.TLSEnabled, _ = tls["enabled"].(bool)
		srv.TLSServerName, _ = tls["server_name"].(string)
		if alpn, ok := tls["alpn"].([]any); ok {
			for _, a := range alpn {
				srv.TLSALPN = append(srv.TLSALPN, fmt.Sprintf("%v", a))
			}
		}
		if utls, ok := tls["utls"].(map[string]any); ok {
			srv.TLSFingerprint, _ = utls["fingerprint"].(string)
		}
		if r, ok := tls["reality"].(map[string]any); ok {
			srv.RealityEnabled, _ = r["enabled"].(bool)
			srv.RealityPublicKey, _ = r["public_key"].(string)
			srv.RealityShortID, _ = r["short_id"].(string)
			srv.RealitySpiderX, _ = r["spider_x"].(string)
		}
	}
	if srv.Name == "" {
		srv.Name = fmt.Sprintf("%s:%d", srv.Server, srv.ServerPort)
	}
	return srv
}

// FromClashYAML extracts proxies from a Clash/Mihomo YAML config.
func FromClashYAML(text string) ([]models.Server, error) {
	var cfg struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
		return nil, fmt.Errorf("clash yaml: %w", err)
	}
	var out []models.Server
	for _, p := range cfg.Proxies {
		srv := serverFromClashProxy(p)
		if srv != nil {
			out = append(out, *srv)
		}
	}
	return normalizeAll(out), nil
}

func serverFromClashProxy(p map[string]any) *models.Server {
	typ, _ := p["type"].(string)
	typ = strings.ToLower(typ)
	srv := &models.Server{}
	srv.Server, _ = p["server"].(string)
	if port, ok := p["port"].(int); ok {
		srv.ServerPort = port
	}
	srv.Name, _ = p["name"].(string)

	switch typ {
	case "ss":
		srv.Protocol = models.ProtoShadowsocks
		srv.Method, _ = p["cipher"].(string)
		srv.Password, _ = p["password"].(string)
	case "vmess":
		srv.Protocol = models.ProtoVMess
		srv.UUID, _ = p["uuid"].(string)
		if aid, ok := p["alterId"].(int); ok {
			srv.AlterID = aid
		}
		srv.TransportType = clashNet(p)
		srv.TransportPath, _ = p["path"].(string)
		srv.TransportHost, _ = p["host"].(string)
		srv.TLSEnabled = clashBool(p["tls"])
		srv.TLSServerName, _ = p["servername"].(string)
	case "vless":
		srv.Protocol = models.ProtoVless
		srv.UUID, _ = p["uuid"].(string)
		srv.Flow, _ = p["flow"].(string)
		srv.TransportType = clashNet(p)
		srv.TransportPath, _ = p["path"].(string)
		srv.TransportHost, _ = p["host"].(string)
		srv.TLSEnabled = true
		srv.TLSServerName, _ = p["servername"].(string)
		srv.TLSFingerprint, _ = p["client-fingerprint"].(string)
		if r, ok := p["reality-opts"].(map[string]any); ok {
			srv.RealityEnabled = true
			srv.RealityPublicKey, _ = r["public-key"].(string)
			srv.RealityShortID, _ = r["short-id"].(string)
		}
	case "trojan":
		srv.Protocol = models.ProtoTrojan
		srv.Password, _ = p["password"].(string)
		srv.TLSEnabled = true
		srv.TLSServerName, _ = p["sni"].(string)
	case "hysteria2", "hy2":
		srv.Protocol = models.ProtoHysteria2
		srv.Password, _ = p["password"].(string)
		srv.TLSServerName, _ = p["sni"].(string)
		srv.TLSInsecure = clashBool(p["skip-cert-verify"])
		if up, ok := p["up"].(string); ok {
			srv.Hy2UpMbps = parseMbps(up)
		}
		if down, ok := p["down"].(string); ok {
			srv.Hy2DownMbps = parseMbps(down)
		}
	case "tuic":
		srv.Protocol = models.ProtoTUIC
		srv.TUICUUID, _ = p["uuid"].(string)
		srv.TUICPassword, _ = p["password"].(string)
		srv.TLSServerName, _ = p["sni"].(string)
	default:
		return nil
	}
	if srv.Name == "" {
		srv.Name = fmt.Sprintf("%s:%d", srv.Server, srv.ServerPort)
	}
	return srv
}

// ----- helpers -----

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func isShareLink(s string) bool {
	for _, p := range []string{"vless://", "vmess://", "trojan://", "ss://", "hysteria2://", "hy2://", "tuic://"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func tryStructured(text string) ([]models.Server, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		return FromSingBoxJSON(text)
	}
	if strings.Contains(text, "proxies:") || strings.HasPrefix(text, "- ") {
		if !strings.HasPrefix(text, "proxies:") {
			text = "proxies:\n" + text
		}
		return FromClashYAML(text)
	}
	return nil, nil
}

func normalizeAll(in []models.Server) []models.Server {
	for i := range in {
		if in[i].ID == "" {
			in[i].ID = newID("srv")
		}
		if in[i].AddedAt.IsZero() {
			in[i].AddedAt = now()
		}
		if in[i].Protocol == "" {
			in[i].Protocol = models.ProtoVless
		}
	}
	return in
}

func clashNet(p map[string]any) string {
	net, _ := p["network"].(string)
	if net == "" {
		return "tcp"
	}
	if net == "http" {
		return "httpupgrade"
	}
	return net
}

func clashBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.ToLower(b) == "true" || b == "1"
	}
	return false
}

func parseMbps(s string) int {
	s = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(s), " mbps"))
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}
