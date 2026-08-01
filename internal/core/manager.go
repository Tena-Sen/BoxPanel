// Package core manages multiple proxy cores (sing-box / xray / mihomo / hysteria2).
//
// CoreManager is the single entry point for:
//   - Registering and listing available cores
//   - Selecting the best core for a given server/protocol
//   - Switching active core at runtime (stop old → swap → start new)
//   - Dispatching config generation to the correct adapter
package core

import (
	"context"
	"fmt"
	"sync"

	"boxpanel/internal/models"
)

// Manager manages multiple Core implementations.
type Manager struct {
	mu      sync.RWMutex
	cores   map[string]Core // kind -> Core instance
	active  string          // active core kind
	runner  *Runner
}

// NewManager creates a Manager with the given runner for subprocess management.
func NewManager(runner *Runner) *Manager {
	return &Manager{
		cores:  make(map[string]Core),
		runner: runner,
	}
}

// Register adds a Core implementation. Called once at startup for each kind.
func (m *Manager) Register(c Core) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cores[c.Kind()] = c
}

// Kinds returns all registered core kinds.
func (m *Manager) Kinds() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.cores))
	for k := range m.cores {
		out = append(out, k)
	}
	return out
}

// Get returns a Core by kind.
func (m *Manager) Get(kind string) (Core, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.cores[kind]
	return c, ok
}

// Active returns the currently active Core.
func (m *Manager) Active() Core {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active == "" {
		return nil
	}
	return m.cores[m.active]
}

// SetActive switches the active core kind.
func (m *Manager) SetActive(kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cores[kind]; !ok {
		return fmt.Errorf("core kind %q not registered", kind)
	}
	m.active = kind
	return nil
}

// ActiveKind returns the kind string of the active core.
func (m *Manager) ActiveKind() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// BestCoreForServer picks the best core kind for a given server's protocol.
//
// Priority:
//  1. Core with Clash API support + protocol match
//  2. Core with protocol match (no Clash API)
//  3. sing-box as fallback
func (m *Manager) BestCoreForServer(srv models.Server) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bestWithoutClash := ""
	bestWithClash := ""

	for kind, c := range m.cores {
		if !c.SupportsProtocol(srv.Protocol) {
			continue
		}
		if bestWithClash == "" && c.ClashAPI() != nil {
			bestWithClash = kind
		}
		if bestWithoutClash == "" {
			bestWithoutClash = kind
		}
	}

	if bestWithClash != "" {
		return bestWithClash
	}
	if bestWithoutClash != "" {
		return bestWithoutClash
	}
	return models.CoreKindSingBox // fallback
}

// SupportsProtocol checks if a core kind supports the given protocol.
func (m *Manager) SupportsProtocol(kind, proto string) bool {
	m.mu.RLock()
	c, ok := m.cores[kind]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return c.SupportsProtocol(proto)
}

// SupportsProtocol is a convenience method on Core interface.
// Defined here to avoid circular import with models.
type protocolChecker interface {
	SupportsProtocol(proto string) bool
}

// CoreKindInfo describes a registered core kind for API responses.
type CoreKindInfo struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Protocols   []string `json:"protocols"`
	HasClashAPI bool     `json:"has_clash_api"`
}

// KindInfo returns metadata about all registered core kinds.
func (m *Manager) KindInfo() []CoreKindInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]CoreKindInfo, 0, len(m.cores))
	for kind, c := range m.cores {
		out = append(out, CoreKindInfo{
			Kind:        kind,
			Name:        c.Name(),
			Protocols:   models.SupportedProtocolsByKind(kind),
			HasClashAPI: c.ClashAPI() != nil,
		})
	}
	return out
}

// BuildConfig dispatches config generation to the active core.
func (m *Manager) BuildConfig(ctx context.Context, req BuildRequest, outPath string) error {
	m.mu.RLock()
	c, ok := m.cores[m.active]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no active core")
	}
	return c.BuildConfig(ctx, req, outPath)
}

// defaultCoreOrder is the preference order when choosing a core.
var defaultCoreOrder = []string{
	models.CoreKindSingBox,
	models.CoreKindMihomo,
	models.CoreKindXray,
	models.CoreKindHysteria2,
}
