// Package mihomo adapts mihomo (Clash.Meta) as a proxy backend for BoxPanel.
//
// mihomo supports: shadowsocks, vmess, trojan, hysteria2, tuic, http, socks.
// It has a native Clash API, so full runtime control (group switching, latency
// tests, traffic stats) works out of the box.
// Config format: YAML.
package mihomo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"boxpanel/internal/config"
	"boxpanel/internal/core"
	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/models"
)

// Core implements core.Core for mihomo.
type Core struct {
	exePath     string
	clash       *clashapi.Client
	clashHost   string
	clashPort   int
	clashSecret string
}

// New creates a mihomo Core adapter.
func New() *Core { return &Core{} }

func (c *Core) Name() string { return "mihomo" }
func (c *Core) Kind() string { return models.CoreKindMihomo }

func (c *Core) ExePath() string    { return c.exePath }
func (c *Core) SetExePath(p string) { c.exePath = p }

// SupportsProtocol reports whether mihomo can handle the given protocol.
func (c *Core) SupportsProtocol(proto string) bool {
	return models.CoreConfig{Kind: models.CoreKindMihomo}.SupportsProtocol(proto)
}

// BuildConfig generates a mihomo YAML config from the unified BuildRequest.
func (c *Core) BuildConfig(_ context.Context, req core.BuildRequest, outPath string) error {
	var yaml strings.Builder

	mixedPort := orDefault(req.Profile.ListenPort, config.MixedInboundPort)
	listenAddr := nonEmpty(req.Profile.Listen, "127.0.0.1")

	yaml.WriteString("mixed-port: " + itoa(mixedPort) + "\n")
	yaml.WriteString("allow-lan: false\n")
	yaml.WriteString("bind-address: " + listenAddr + "\n")
	yaml.WriteString("mode: rule\n")
	yaml.WriteString("log-level: " + mihomoLogLevel(req.Settings.LogLevel) + "\n")
	yaml.WriteString("external-controller: " + fmt.Sprintf("%s:%d", config.ClashAPIHost, req.Settings.ClashAPIPort) + "\n")
	if req.Settings.ClashAPISecret != "" {
		yaml.WriteString("secret: " + req.Settings.ClashAPISecret + "\n")
	}

	// DNS
	yaml.WriteString("\ndns:\n  enable: true\n  enhanced-mode: fake-ip\n")
	yaml.WriteString("  nameserver:\n    - " + nonEmpty(req.Profile.DirectDNS, "223.5.5.5") + "\n")
	yaml.WriteString("  fallback:\n    - " + nonEmpty(req.Profile.ProxyDNS, "8.8.8.8") + "\n")
	yaml.WriteString("  fallback-filter:\n    geoip: true\n    geoip-code: CN\n")

	// Proxies
	yaml.WriteString("\nproxies:\n")
	for _, srv := range req.AllServers {
		lines := buildMihomoProxy(srv)
		if lines != "" {
			yaml.WriteString(lines + "\n")
		}
	}

	// Proxy groups
	yaml.WriteString("\nproxy-groups:\n")
	var proxyNames []string
	for _, srv := range req.AllServers {
		proxyNames = append(proxyNames, mihomoProxyName(srv))
	}
	if len(proxyNames) > 0 {
		// Selector group
		yaml.WriteString("  - name: proxy\n    type: select\n    proxies:\n")
		for _, n := range proxyNames {
			yaml.WriteString("      - " + n + "\n")
		}
	}
	// URL-Test group for auto-select
	if len(proxyNames) > 1 {
		yaml.WriteString("  - name: auto\n    type: url-test\n    url: http://www.gstatic.com/generate_204\n    interval: 300\n    proxies:\n")
		for _, n := range proxyNames {
			yaml.WriteString("      - " + n + "\n")
		}
	}
	// Fallback group
	if len(proxyNames) > 1 {
		yaml.WriteString("  - name: fallback\n    type: fallback\n    url: http://www.gstatic.com/generate_204\n    interval: 300\n    proxies:\n")
		for _, n := range proxyNames {
			yaml.WriteString("      - " + n + "\n")
		}
	}
	// User-defined groups
	for _, g := range req.Groups {
		yaml.WriteString("  - name: " + g.Name + "\n    type: " + g.Type + "\n")
		if g.URL != "" {
			yaml.WriteString("    url: " + g.URL + "\n")
		}
		if g.Interval > 0 {
			yaml.WriteString("    interval: " + itoa(g.Interval) + "\n")
		}
		yaml.WriteString("    proxies:\n")
		for _, sid := range g.ServerIDs {
			for _, srv := range req.AllServers {
				if srv.ID == sid {
					yaml.WriteString("      - " + mihomoProxyName(srv) + "\n")
				}
			}
		}
	}

	// Rules
	yaml.WriteString("\nrules:\n")
	yaml.WriteString("  - GEOIP,private,DIRECT\n")
	yaml.WriteString("  - GEOSITE,CN,DIRECT\n")
	yaml.WriteString("  - GEOIP,CN,DIRECT\n")
	for _, r := range req.RoutingRules {
		if rule := compileMihomoRule(r); rule != "" {
			yaml.WriteString("  - " + rule + "\n")
		}
	}
	yaml.WriteString("  - MATCH,proxy\n")

	data := []byte(yaml.String())
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, outPath)
}

func (c *Core) Start(_ context.Context, configPath string) error {
	killExistingMihomo()

	cmd := exec.Command(c.exePath, "-f", configPath)
	cmd.Dir = filepath.Dir(c.exePath)
	hideWindow(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mihomo: %w", err)
	}
	return nil
}

func (c *Core) Stop() error {
	killExistingMihomo()
	return nil
}

func (c *Core) IsRunning() bool { return false }
func (c *Core) PID() int        { return 0 }

func (c *Core) Check(_ context.Context, configPath string) error {
	cmd := exec.Command(c.exePath, "-t", "-f", configPath)
	cmd.Dir = filepath.Dir(c.exePath)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mihomo config test: %s: %w", string(out), err)
	}
	return nil
}

// SetClashAPI configures the Clash API endpoint.
func (c *Core) SetClashAPI(host string, port int, secret string) {
	c.clashHost = host
	c.clashPort = port
	c.clashSecret = secret
	c.clash = clashapi.New(host, port, secret)
}

// ClashAPI — mihomo has a native Clash API.
func (c *Core) ClashAPI() core.ClashAPI {
	if c.clash == nil {
		return nil
	}
	return &mihomoClashWrapper{cli: c.clash}
}

type mihomoClashWrapper struct {
	cli *clashapi.Client
}

func (w *mihomoClashWrapper) Proxies(ctx context.Context) (map[string]any, error) {
	r, err := w.cli.Proxies(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"proxies": r.Proxies}, nil
}
func (w *mihomoClashWrapper) SelectProxy(ctx context.Context, group, name string) error {
	return w.cli.SelectProxy(ctx, group, name)
}
func (w *mihomoClashWrapper) Delay(ctx context.Context, name, url string, timeoutMs int) (int, error) {
	return w.cli.Delay(ctx, name, url, timeoutMs)
}
func (w *mihomoClashWrapper) Connections(ctx context.Context) (any, error) {
	return w.cli.Connections(ctx)
}
func (w *mihomoClashWrapper) Reachable(ctx context.Context) bool {
	return w.cli.Reachable(ctx)
}

// ----- proxy builder -----

func buildMihomoProxy(srv models.Server) string {
	name := mihomoProxyName(srv)
	var b strings.Builder

	switch srv.Protocol {
	case models.ProtoShadowsocks:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: ss\n    server: %s\n    port: %d\n    cipher: %s\n    password: %s",
			name, srv.Server, srv.ServerPort, nonEmpty(srv.Method, "aes-256-gcm"), srv.Password))

	case models.ProtoVMess:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: vmess\n    server: %s\n    port: %d\n    uuid: %s\n    alterId: %d\n    cipher: %s",
			name, srv.Server, srv.ServerPort, srv.UUID, srv.AlterID, nonEmpty(srv.Method, "auto")))
		writeMihomoTLS(&b, srv)
		writeMihomoTransport(&b, srv)

	case models.ProtoTrojan:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: trojan\n    server: %s\n    port: %d\n    password: %s",
			name, srv.Server, srv.ServerPort, srv.Password))
		writeMihomoTLS(&b, srv)
		writeMihomoTransport(&b, srv)

	case models.ProtoHysteria2:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: hysteria2\n    server: %s\n    port: %d\n    password: %s",
			name, srv.Server, srv.ServerPort, srv.Password))
		if srv.TLSInsecure {
			b.WriteString("\n    skip-cert-verify: true")
		}
		if srv.TLSServerName != "" {
			b.WriteString("\n    sni: " + srv.TLSServerName)
		}
		if srv.Hy2UpMbps > 0 {
			b.WriteString(fmt.Sprintf("\n    up: %d", srv.Hy2UpMbps))
		}
		if srv.Hy2DownMbps > 0 {
			b.WriteString(fmt.Sprintf("\n    down: %d", srv.Hy2DownMbps))
		}
		if srv.Hy2Obfs != "" {
			b.WriteString("\n    obfs: " + srv.Hy2Obfs)
			if srv.Hy2ObfsPassword != "" {
				b.WriteString("\n    obfs-password: " + srv.Hy2ObfsPassword)
			}
		}

	case models.ProtoTUIC:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: tuic\n    server: %s\n    port: %d\n    uuid: %s\n    password: %s",
			name, srv.Server, srv.ServerPort, srv.TUICUUID, srv.TUICPassword))
		if srv.TLSInsecure {
			b.WriteString("\n    skip-cert-verify: true")
		}
		if srv.TLSServerName != "" {
			b.WriteString("\n    sni: " + srv.TLSServerName)
		}
		if srv.TUICCongestionControl != "" {
			b.WriteString("\n    congestion-controller: " + srv.TUICCongestionControl)
		}

	case models.ProtoHTTP:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: http\n    server: %s\n    port: %d",
			name, srv.Server, srv.ServerPort))
		if srv.Username != "" {
			b.WriteString("\n    username: " + srv.Username)
		}
		if srv.Password != "" {
			b.WriteString("\n    password: " + srv.Password)
		}
		writeMihomoTLS(&b, srv)

	case models.ProtoSOCKS:
		b.WriteString(fmt.Sprintf("- name: %s\n    type: socks5\n    server: %s\n    port: %d",
			name, srv.Server, srv.ServerPort))
		if srv.Username != "" {
			b.WriteString("\n    username: " + srv.Username)
		}
		if srv.Password != "" {
			b.WriteString("\n    password: " + srv.Password)
		}

	default:
		return ""
	}

	return b.String()
}

func writeMihomoTLS(b *strings.Builder, srv models.Server) {
	if srv.TLSEnabled {
		b.WriteString("\n    tls: true")
		if srv.TLSInsecure {
			b.WriteString("\n    skip-cert-verify: true")
		}
		if srv.TLSServerName != "" {
			b.WriteString("\n    sni: " + srv.TLSServerName)
		}
		if srv.TLSFingerprint != "" {
			b.WriteString("\n    client-fingerprint: " + srv.TLSFingerprint)
		}
		if len(srv.TLSALPN) > 0 {
			b.WriteString("\n    alpn:\n")
			for _, a := range srv.TLSALPN {
				b.WriteString("      - " + a + "\n")
			}
		}
	}
	if srv.RealityEnabled {
		b.WriteString("\n    tls: true")
		b.WriteString("\n    reality-opts:\n      public-key: " + srv.RealityPublicKey)
		if srv.RealityShortID != "" {
			b.WriteString("\n      short-id: " + srv.RealityShortID)
		}
		if srv.TLSServerName != "" {
			b.WriteString("\n    sni: " + srv.TLSServerName)
		}
		if srv.TLSFingerprint != "" {
			b.WriteString("\n    client-fingerprint: " + srv.TLSFingerprint)
		}
	}
}

func writeMihomoTransport(b *strings.Builder, srv models.Server) {
	switch srv.TransportType {
	case "ws":
		b.WriteString("\n    network: ws")
		b.WriteString("\n    ws-opts:\n      path: " + nonEmpty(srv.TransportPath, "/"))
		if srv.TransportHost != "" {
			b.WriteString("\n      headers:\n        Host: " + srv.TransportHost)
		}
	case "grpc":
		b.WriteString("\n    network: grpc")
		b.WriteString("\n    grpc-opts:\n      grpc-service-name: " + nonEmpty(srv.TransportPath, ""))
	case "h2":
		b.WriteString("\n    network: h2")
		b.WriteString("\n    h2-opts:\n      host:\n        - " + nonEmpty(srv.TransportHost, srv.Server))
		b.WriteString("\n      path: " + nonEmpty(srv.TransportPath, "/"))
	}
}

func mihomoProxyName(srv models.Server) string {
	// Use name directly (must be unique in mihomo)
	return srv.Name
}

func compileMihomoRule(r models.RoutingRule) string {
	outbound := r.Outbound
	switch outbound {
	case "proxy":
		outbound = "proxy"
	case "direct":
		outbound = "DIRECT"
	case "block":
		outbound = "REJECT"
	}

	switch r.Type {
	case models.RuleDomain:
		if len(r.Values) > 0 {
			return "DOMAIN," + r.Values[0] + "," + outbound
		}
	case models.RuleDomainSuffix:
		if len(r.Values) > 0 {
			return "DOMAIN-SUFFIX," + r.Values[0] + "," + outbound
		}
	case models.RuleDomainKeyword:
		if len(r.Values) > 0 {
			return "DOMAIN-KEYWORD," + r.Values[0] + "," + outbound
		}
	case models.RuleIPCIDR:
		if len(r.Values) > 0 {
			return "IP-CIDR," + r.Values[0] + "," + outbound
		}
	case models.RuleProcess:
		if len(r.Values) > 0 {
			return "PROCESS-NAME," + r.Values[0] + "," + outbound
		}
	case models.RulePort:
		if len(r.Values) > 0 {
			return "DST-PORT," + r.Values[0] + "," + outbound
		}
	}
	return ""
}

func mihomoLogLevel(level string) string {
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

func killExistingMihomo() {
	name := "mihomo.exe"
	if runtime.GOOS != "windows" {
		name = "mihomo"
	}
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("taskkill", "/f", "/t", "/im", name).Run()
	default:
		_ = exec.Command("pkill", "-f", "mihomo").Run()
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

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
