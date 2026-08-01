// Package protocol defines the protocol plugin registry.
//
// 每个协议实现 Protocol 接口并在 init() 中注册，
// 新增协议只需新增子包，零改动其他代码（开闭原则）。
package protocol

import (
	"fmt"
	"sync"

	"boxpanel/internal/models"
)

// Protocol is the interface every protocol plugin implements.
type Protocol interface {
	// Name returns the protocol identifier (e.g. "vless").
	Name() string
	// Schemes returns the share-link URI schemes it parses (e.g. ["vless"]).
	Schemes() []string
	// Parse parses a share-link URI into a Server.
	Parse(uri string) (*models.Server, error)
	// ToURI serializes a Server back to a share-link URI.
	ToURI(srv models.Server) (string, error)
	// Outbound builds a sing-box outbound JSON object from a Server.
	Outbound(srv models.Server) (map[string]any, error)
}

var (
	mu        sync.RWMutex
	byName    = map[string]Protocol{}
	byScheme  = map[string]Protocol{}
)

// Register registers a protocol plugin. Called from sub-package init().
func Register(p Protocol) {
	mu.Lock()
	defer mu.Unlock()
	byName[p.Name()] = p
	for _, s := range p.Schemes() {
		byScheme[s] = p
	}
}

// Get returns a protocol by name.
func Get(name string) (Protocol, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byName[name]
	if !ok {
		return nil, fmt.Errorf("unknown protocol: %s", name)
	}
	return p, nil
}

// ByScheme returns the protocol handling a URI scheme.
func ByScheme(scheme string) (Protocol, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byScheme[scheme]
	return p, ok
}

// All returns all registered protocols.
func All() []Protocol {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Protocol, 0, len(byName))
	for _, p := range byName {
		out = append(out, p)
	}
	return out
}

// ParseURI auto-detects the scheme and dispatches to the right protocol.
func ParseURI(uri string) (*models.Server, error) {
	scheme := extractScheme(uri)
	p, ok := ByScheme(scheme)
	if !ok {
		return nil, fmt.Errorf("unsupported scheme: %s", scheme)
	}
	return p.Parse(uri)
}

// Outbound builds the outbound JSON for a server, dispatching by protocol name.
func Outbound(srv models.Server) (map[string]any, error) {
	p, err := Get(srv.Protocol)
	if err != nil {
		return nil, err
	}
	return p.Outbound(srv)
}

func extractScheme(uri string) string {
	for i := 0; i < len(uri); i++ {
		c := uri[i]
		if c == ':' {
			return uri[:i]
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.') {
			return ""
		}
	}
	return ""
}
