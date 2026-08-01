// Package singbox adapts sing-box as a proxy backend for sbpanel.
//
// sing-box supports all protocols and has a native Clash API.
// It is the default and most feature-complete core.
package singbox

import (
	"context"
	"sync"

	"boxpanel/internal/core"
	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/core/configgen"
	"boxpanel/internal/models"
)

// Core implements core.Core for sing-box.
type Core struct {
	mu       sync.Mutex
	exePath  string
	gen      *configgen.Builder
	clash   *clashapi.Client
	clashHost string
	clashPort int
	clashSecret string
}

// New creates a sing-box Core adapter.
func New(gen *configgen.Builder) *Core {
	return &Core{gen: gen}
}

func (c *Core) Name() string { return "sing-box" }
func (c *Core) Kind() string { return "singbox" }

// SupportsProtocol — sing-box supports all protocols.
func (c *Core) SupportsProtocol(proto string) bool {
	for _, p := range models.SupportedProtocolsByKind(models.CoreKindSingBox) {
		if p == proto {
			return true
		}
	}
	return false
}

func (c *Core) ExePath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exePath
}
func (c *Core) SetExePath(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exePath = p
}

// SetClashAPI configures the Clash API endpoint (called before ClashAPI()).
func (c *Core) SetClashAPI(host string, port int, secret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clashHost = host
	c.clashPort = port
	c.clashSecret = secret
	c.clash = clashapi.New(host, port, secret)
}

// BuildConfig generates sing-box JSON via configgen.
func (c *Core) BuildConfig(_ context.Context, req core.BuildRequest, outPath string) error {
	br := configgen.BuildRequest{
		Profile:       req.Profile,
		CurrentServer: req.CurrentServer,
		AllServers:    req.AllServers,
		Groups:        req.Groups,
		RoutingRules:  req.RoutingRules,
		RuleSets:      req.RuleSets,
		Settings:      req.Settings,
	}
	_, err := c.gen.Build(br)
	return err
}

// Start / Stop / IsRunning / PID / Check
// Currently delegated to the shared Runner via APIServer.
// These stubs exist to satisfy the Core interface.
func (c *Core) Start(_ context.Context, _ string) error {
	return nil // managed by Runner
}
func (c *Core) Stop() error   { return nil }
func (c *Core) IsRunning() bool { return false }
func (c *Core) PID() int       { return 0 }
func (c *Core) Check(_ context.Context, _ string) error { return nil }

// ClashAPI returns the Clash API client (sing-box has native Clash API).
func (c *Core) ClashAPI() core.ClashAPI {
	c.mu.Lock()
	cli := c.clash
	c.mu.Unlock()
	if cli == nil {
		return nil
	}
	return &clashWrapper{cli: cli}
}

type clashWrapper struct {
	cli *clashapi.Client
}

func (w *clashWrapper) Proxies(ctx context.Context) (map[string]any, error) {
	r, err := w.cli.Proxies(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"proxies": r.Proxies}, nil
}
func (w *clashWrapper) SelectProxy(ctx context.Context, group, name string) error {
	return w.cli.SelectProxy(ctx, group, name)
}
func (w *clashWrapper) Delay(ctx context.Context, name, url string, timeoutMs int) (int, error) {
	return w.cli.Delay(ctx, name, url, timeoutMs)
}
func (w *clashWrapper) Connections(ctx context.Context) (any, error) {
	return w.cli.Connections(ctx)
}
func (w *clashWrapper) Reachable(ctx context.Context) bool {
	return w.cli.Reachable(ctx)
}

var _ core.Core = (*Core)(nil)
