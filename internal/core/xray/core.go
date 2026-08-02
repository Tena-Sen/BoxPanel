// Package xray adapts Xray-core as a proxy backend for BoxPanel.
//
// Xray supports: vless, vmess, trojan, shadowsocks, http, socks.
// It does NOT have a Clash API; we expose a no-op ClashAPI.
// Runtime control (group switching) is unavailable — users must
// restart to switch nodes, or use sing-box/mihomo instead.
package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"boxpanel/internal/config"
	"boxpanel/internal/core"
	"boxpanel/internal/models"
)

// Core implements core.Core for Xray.
type Core struct {
	exePath string
	cmd     *exec.Cmd
	pid     int
}

// New creates an Xray Core adapter.
func New() *Core { return &Core{} }

func (c *Core) Name() string { return "Xray" }
func (c *Core) Kind() string { return models.CoreKindXray }

func (c *Core) ExePath() string   { return c.exePath }
func (c *Core) SetExePath(p string) { c.exePath = p }

// SupportsProtocol reports whether Xray can handle the given protocol.
func (c *Core) SupportsProtocol(proto string) bool {
	return models.CoreConfig{Kind: models.CoreKindXray}.SupportsProtocol(proto)
}

// BuildConfig generates an Xray JSON config from the unified BuildRequest.
func (c *Core) BuildConfig(_ context.Context, req core.BuildRequest, outPath string) error {
	cfg := map[string]any{}

	// 1. Log
	cfg["log"] = map[string]any{
		"loglevel": xrayLogLevel(req.Settings.LogLevel),
	}

	// 2. Inbounds — mixed socks+http on the same port (socks handles HTTP CONNECT too)
	listen := nonEmpty(req.Profile.Listen, "127.0.0.1")
	port := orDefault(req.Profile.ListenPort, config.MixedInboundPort)
	inbounds := []map[string]any{
		{
			"tag":      "socks-in",
			"protocol": "socks",
			"listen":   listen,
			"port":     port,
			"settings": map[string]any{
				"auth": "noauth",
				"udp":  true,
			},
			"sniffing": map[string]any{
				"enabled":      req.Profile.Sniff,
				"destOverride": []string{"http", "tls"},
				"routeOnly":    !req.Profile.SniffOverride,
			},
		},
		{
			"tag":      "http-in",
			"protocol": "http",
			"listen":   listen,
			"port":     port + 1,
		},
	}
	cfg["inbounds"] = inbounds

	// 3. Outbounds — translate each server
	var outbounds []map[string]any
	for _, srv := range req.AllServers {
		ob, err := buildXrayOutbound(srv)
		if err != nil {
			continue // skip unsupported protocols
		}
		outbounds = append(outbounds, ob)
	}

	// Current server as the default "proxy" outbound
	if len(req.AllServers) > 0 {
		ob, err := buildXrayOutbound(req.CurrentServer)
		if err == nil {
			ob["tag"] = "proxy"
			outbounds = append(outbounds, ob)
		}
	}
	if len(outbounds) == 0 {
		outbounds = append(outbounds, map[string]any{"tag": "proxy", "protocol": "freedom"})
	}

	// Direct & block
	outbounds = append(outbounds,
		map[string]any{"tag": "direct", "protocol": "freedom"},
		map[string]any{"tag": "block", "protocol": "blackhole"},
	)
	cfg["outbounds"] = outbounds

	// 4. Routing
	var rules []map[string]any

	// Check if geodata files exist in Xray's working directory.
	// Xray needs geoip.dat and geosite.dat for geoip:/geosite: rules.
	// If missing, skip geodata-based rules to avoid startup failure.
	exeDir := filepath.Dir(c.exePath)
	hasGeoip := fileExists(filepath.Join(exeDir, "geoip.dat"))
	hasGeosite := fileExists(filepath.Join(exeDir, "geosite.dat"))

	// Private IP -> direct (requires geoip.dat)
	if hasGeoip {
		rules = append(rules, map[string]any{
			"type":        "field",
			"outboundTag": "direct",
			"ip":          []string{"geoip:private"},
		})
	}
	// CN sites -> direct (requires geosite.dat)
	if hasGeosite {
		rules = append(rules, map[string]any{
			"type":        "field",
			"outboundTag": "direct",
			"domain":      []string{"geosite:cn"},
		})
	}
	// CN IPs -> direct (requires geoip.dat)
	if hasGeoip {
		rules = append(rules, map[string]any{
			"type":        "field",
			"outboundTag": "direct",
			"ip":          []string{"geoip:cn"},
		})
	}
	// User-defined routing rules
	for _, r := range req.RoutingRules {
		if rule := compileXrayRule(r); rule != nil {
			rules = append(rules, rule)
		}
	}
	cfg["routing"] = map[string]any{
		"domainStrategy": "IPIfNonMatch",
		"rules":          rules,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, outPath)
}

// Start launches xray with the given config path.
func (c *Core) Start(_ context.Context, configPath string) error {
	killExistingXray()

	c.cmd = exec.Command(c.exePath, "run", "-c", configPath)
	c.cmd.Dir = filepath.Dir(c.exePath)
	hideWindow(c.cmd)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start xray: %w", err)
	}
	c.pid = c.cmd.Process.Pid
	// Reap process in background to prevent zombie
	go func() { _ = c.cmd.Wait() }()
	return nil
}

func (c *Core) Stop() error {
	killExistingXray()
	c.pid = 0
	return nil
}

func (c *Core) IsRunning() bool {
	if c.pid <= 0 || c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	// Check if process is still alive
	return c.cmd.Process.Signal(syscall.Signal(0)) == nil
}

func (c *Core) PID() int { return c.pid }

func (c *Core) Check(_ context.Context, configPath string) error {
	cmd := exec.Command(c.exePath, "run", "-c", configPath, "-test")
	cmd.Dir = filepath.Dir(c.exePath)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray config test: %s: %w", string(out), err)
	}
	return nil
}

// ClashAPI — Xray doesn't have Clash API, return nil.
func (c *Core) ClashAPI() core.ClashAPI { return nil }

// ----- Xray outbound builder -----

func buildXrayOutbound(srv models.Server) (map[string]any, error) {
	tag := "srv-" + srv.ID
	base := map[string]any{
		"tag":      tag,
		"settings": map[string]any{},
	}

	switch srv.Protocol {
	case models.ProtoVless:
		base["protocol"] = "vless"
		flow := srv.Flow
		if flow == "" {
			flow = "" // vless without XTLS
		}
		settings := map[string]any{
			"vnext": []map[string]any{{
				"address": srv.Server,
				"port":    srv.ServerPort,
				"users": []map[string]any{{
					"id":         srv.UUID,
					"encryption": "none",
					"flow":       flow,
				}},
			}},
		}
		// Remove empty flow field
		if flow == "" {
			users := settings["vnext"].([]map[string]any)[0]["users"].([]map[string]any)
			delete(users[0], "flow")
		}
		base["settings"] = settings
		addXrayStream(base, srv)

	case models.ProtoVMess:
		base["protocol"] = "vmess"
		security := srv.Method
		if security == "" {
			security = "auto"
		}
		settings := map[string]any{
			"vnext": []map[string]any{{
				"address": srv.Server,
				"port":    srv.ServerPort,
				"users": []map[string]any{{
					"id":       srv.UUID,
					"alterId":   srv.AlterID,
					"security": security,
				}},
			}},
		}
		base["settings"] = settings
		addXrayStream(base, srv)

	case models.ProtoTrojan:
		base["protocol"] = "trojan"
		settings := map[string]any{
			"servers": []map[string]any{{
				"address":  srv.Server,
				"port":     srv.ServerPort,
				"password": srv.Password,
			}},
		}
		base["settings"] = settings
		addXrayStream(base, srv)

	case models.ProtoShadowsocks:
		base["protocol"] = "shadowsocks"
		settings := map[string]any{
			"servers": []map[string]any{{
				"address":  srv.Server,
				"port":     srv.ServerPort,
				"method":   nonEmpty(srv.Method, "aes-256-gcm"),
				"password": srv.Password,
			}},
		}
		base["settings"] = settings

	case models.ProtoSOCKS:
		base["protocol"] = "socks"
		settings := map[string]any{
			"servers": []map[string]any{{
				"address": srv.Server,
				"port":    srv.ServerPort,
			}},
		}
		if srv.Username != "" {
			settings["servers"].([]map[string]any)[0]["users"] = []map[string]any{{
				"user": srv.Username,
				"pass": srv.Password,
			}}
		}
		base["settings"] = settings

	case models.ProtoHTTP:
		base["protocol"] = "http"
		settings := map[string]any{
			"servers": []map[string]any{{
				"address": srv.Server,
				"port":    srv.ServerPort,
			}},
		}
		if srv.Username != "" {
			servers := settings["servers"].([]map[string]any)
			servers[0]["username"] = srv.Username
			servers[0]["password"] = srv.Password
		}
		base["settings"] = settings

	default:
		return nil, fmt.Errorf("xray: unsupported protocol %s", srv.Protocol)
	}

	return base, nil
}

func addXrayStream(ob map[string]any, srv models.Server) {
	stream := map[string]any{}

	if srv.TLSEnabled || srv.RealityEnabled {
		if srv.RealityEnabled {
			stream["security"] = "reality"
			rs := map[string]any{
				"serverName": nonEmpty(srv.TLSServerName, srv.Server),
				"publicKey":  srv.RealityPublicKey,
				"shortId":    nonEmpty(srv.RealityShortID, ""),
				"fingerprint": nonEmpty(srv.TLSFingerprint, "chrome"),
			}
			if srv.RealitySpiderX != "" {
				rs["spiderX"] = srv.RealitySpiderX
			}
			stream["realitySettings"] = rs
		} else {
			stream["security"] = "tls"
			tlsSettings := map[string]any{
				"serverName": nonEmpty(srv.TLSServerName, srv.Server),
			}
			if srv.TLSInsecure {
				tlsSettings["allowInsecure"] = true
			}
			if srv.TLSFingerprint != "" {
				tlsSettings["fingerprint"] = srv.TLSFingerprint
			}
			if len(srv.TLSALPN) > 0 {
				tlsSettings["alpn"] = srv.TLSALPN
			}
			stream["tlsSettings"] = tlsSettings
		}
	}

	// Transport
	switch srv.TransportType {
	case "ws":
		ws := map[string]any{
			"path":    nonEmpty(srv.TransportPath, "/"),
			"headers": map[string]any{},
		}
		if srv.TransportHost != "" {
			ws["headers"].(map[string]any)["Host"] = srv.TransportHost
		}
		stream["network"] = "ws"
		stream["wsSettings"] = ws
	case "grpc":
		stream["network"] = "grpc"
		stream["grpcSettings"] = map[string]any{
			"serviceName": srv.TransportPath,
		}
	case "h2":
		stream["network"] = "h2"
		stream["httpSettings"] = map[string]any{
			"path": nonEmpty(srv.TransportPath, "/"),
			"host": []string{nonEmpty(srv.TransportHost, srv.Server)},
		}
	case "splithttp", "xhttp":
		stream["network"] = "splithttp"
		stream["splithttpSettings"] = map[string]any{
			"path": nonEmpty(srv.TransportPath, "/"),
			"host": srv.TransportHost,
		}
	case "httpupgrade":
		stream["network"] = "httpupgrade"
		stream["httpupgradeSettings"] = map[string]any{
			"path": nonEmpty(srv.TransportPath, "/"),
			"host": srv.TransportHost,
		}
	}

	ob["streamSettings"] = stream
}

// compileXrayRule compiles a routing rule into Xray format.
func compileXrayRule(r models.RoutingRule) map[string]any {
	outbound := r.Outbound
	if outbound == "direct" {
		outbound = "direct"
	} else if outbound == "block" {
		outbound = "block"
	} else if outbound == "proxy" {
		outbound = "proxy"
	}

	switch r.Type {
	case models.RuleDomain:
		if len(r.Values) > 0 {
			return map[string]any{
				"type":        "field",
				"outboundTag": outbound,
				"domain":      r.Values,
			}
		}
	case models.RuleDomainSuffix:
		vals := make([]string, len(r.Values))
		for i, v := range r.Values {
			vals[i] = v
		}
		return map[string]any{
			"type":        "field",
			"outboundTag": outbound,
			"domain":      vals,
		}
	case models.RuleDomainKeyword:
		return map[string]any{
			"type":        "field",
			"outboundTag": outbound,
			"domain":      r.Values,
		}
	case models.RuleIPCIDR:
		return map[string]any{
			"type":        "field",
			"outboundTag": outbound,
			"ip":          r.Values,
		}
	case models.RuleProcess:
		return map[string]any{
			"type":        "field",
			"outboundTag": outbound,
			"process":     r.Values,
		}
	case models.RulePort:
		return map[string]any{
			"type":        "field",
			"outboundTag": outbound,
			"port":        r.Values,
		}
	}
	return nil
}

func xrayLogLevel(level string) string {
	switch level {
	case "debug":
		return "debug"
	case "info":
		return "info"
	case "warn":
		return "warning"
	case "error":
		return "error"
	default:
		return "warning"
	}
}

func killExistingXray() {
	name := "xray.exe"
	if runtime.GOOS != "windows" {
		name = "xray"
	}
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("taskkill", "/f", "/t", "/im", name).Run()
	default:
		_ = exec.Command("pkill", "-f", "xray").Run()
	}
}

func hideWindow(cmd *exec.Cmd) {
	if runtime.GOOS != "windows" {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
