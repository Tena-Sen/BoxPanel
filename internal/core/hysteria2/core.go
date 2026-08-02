// Package hysteria2 adapts Hysteria2 as a standalone proxy backend for BoxPanel.
//
// Hysteria2 only supports the hysteria2 protocol (QUIC-based).
// It has its own REST API but no Clash API — we expose a no-op ClashAPI.
// Best used for pure hysteria2 nodes where QUIC performance matters.
package hysteria2

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

// Core implements core.Core for Hysteria2.
type Core struct {
	exePath string
	cmd     *exec.Cmd
	pid     int
}

// New creates a Hysteria2 Core adapter.
func New() *Core { return &Core{} }

func (c *Core) Name() string { return "Hysteria2" }
func (c *Core) Kind() string { return models.CoreKindHysteria2 }

func (c *Core) ExePath() string    { return c.exePath }
func (c *Core) SetExePath(p string) { c.exePath = p }

// SupportsProtocol reports whether Hysteria2 can handle the given protocol.
func (c *Core) SupportsProtocol(proto string) bool {
	return models.CoreConfig{Kind: models.CoreKindHysteria2}.SupportsProtocol(proto)
}

// BuildConfig generates a Hysteria2 JSON config from the unified BuildRequest.
// Hysteria2 is a single-protocol client; we only use the first (or current) hysteria2 node.
func (c *Core) BuildConfig(_ context.Context, req core.BuildRequest, outPath string) error {
	srv := req.CurrentServer
	if srv.Protocol != models.ProtoHysteria2 {
		// Find first hysteria2 server
		for _, s := range req.AllServers {
			if s.Protocol == models.ProtoHysteria2 {
				srv = s
				break
			}
		}
		if srv.Protocol != models.ProtoHysteria2 {
			return fmt.Errorf("hysteria2 core requires a hysteria2 node, but none found")
		}
	}

	listenAddr := nonEmpty(req.Profile.Listen, "127.0.0.1")
	socksPort := orDefault(req.Profile.ListenPort, config.MixedInboundPort)
	httpPort := socksPort + 1

	cfg := map[string]any{
		"server": fmt.Sprintf("%s:%d", srv.Server, srv.ServerPort),
		"auth":   srv.Password,
		"socks5": map[string]any{
			"listen": fmt.Sprintf("%s:%d", listenAddr, socksPort),
		},
		"http": map[string]any{
			"listen": fmt.Sprintf("%s:%d", listenAddr, httpPort),
		},
	}

	// TLS options
	if srv.TLSInsecure {
		cfg["insecure"] = true
	}
	if srv.TLSServerName != "" {
		cfg["sni"] = srv.TLSServerName
	}
	if len(srv.TLSALPN) > 0 {
		cfg["alpn"] = srv.TLSALPN
	}

	// Bandwidth limits
	if srv.Hy2UpMbps > 0 {
		cfg["up_mbps"] = srv.Hy2UpMbps
	}
	if srv.Hy2DownMbps > 0 {
		cfg["down_mbps"] = srv.Hy2DownMbps
	}

	// Obfuscation
	if srv.Hy2Obfs != "" {
		cfg["obfs"] = map[string]any{
			"type":     srv.Hy2Obfs,
			"password": srv.Hy2ObfsPassword,
		}
	}

	// Transport (Hysteria2 uses QUIC, only transport-related is obfs)
	// Hysteria2 does not support ws/grpc/h2 transports — those are TCP-based

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

func (c *Core) Start(_ context.Context, configPath string) error {
	killExistingHysteria2()

	cmd := exec.Command(c.exePath, "-c", configPath)
	cmd.Dir = filepath.Dir(c.exePath)
	hideWindow(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start hysteria2: %w", err)
	}
	c.cmd = cmd
	c.pid = cmd.Process.Pid

	// Reap process in background to prevent zombies
	go func() { _ = cmd.Wait() }()
	return nil
}

func (c *Core) Stop() error {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Signal(syscall.Signal(0))
		killExistingHysteria2()
		c.cmd = nil
		c.pid = 0
	} else {
		killExistingHysteria2()
	}
	return nil
}

func (c *Core) IsRunning() bool {
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	return c.cmd.Process.Signal(syscall.Signal(0)) == nil
}

func (c *Core) PID() int { return c.pid }

func (c *Core) Check(_ context.Context, configPath string) error {
	// Hysteria2 doesn't have a built-in config test, just verify the file is valid JSON
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var v any
	return json.Unmarshal(data, &v)
}

// ClashAPI — Hysteria2 doesn't have Clash API, return nil.
func (c *Core) ClashAPI() core.ClashAPI { return nil }

func killExistingHysteria2() {
	name := "hysteria2.exe"
	if runtime.GOOS != "windows" {
		name = "hysteria2"
	}
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("taskkill", "/f", "/t", "/im", name).Run()
	default:
		_ = exec.Command("pkill", "-f", "hysteria2").Run()
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
