package api

import (
	"context"
	"net/http"
	"time"

	"boxpanel/internal/config"
)

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "version": config.Version})
}

// POST /api/quit — gracefully shut down BoxPanel.
// Stops the core (if running), persists traffic, then calls the onQuit callback (os.Exit in main).
func (s *APIServer) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"quitting": true})
	// Flush response before exiting
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		if s.coreIsRunning() {
			_ = s.coreStop()
		}
		s.persistTraffic()
		if s.onQuit != nil {
			s.onQuit()
		}
	}()
}

func (s *APIServer) handleState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx)
	servers, _ := s.store.ListServers(ctx)
	subs, _ := s.store.ListSubscriptions(ctx)

	var current any
	for i := range servers {
		if servers[i].ID == st.CurrentServerID {
			current = servers[i]
			break
		}
	}

	writeJSON(w, 200, map[string]any{
		"running":            s.coreIsRunning(),
		"pid":                s.corePID(),
		"uptime_seconds":     int(s.runner.Uptime().Seconds()),
		"version":            config.Version,
		"base_dir":           config.BaseDir(),
		"exe_path":           config.ExePath(),
		"settings":           st,
		"current_server":     current,
		"server_count":       len(servers),
		"subscription_count": len(subs),
		"sys_proxy":          s.sys.Get(),
		"clash_reachable":    s.clashReachable(),
		"probe_method":       s.lastProbeMethod,
	})
}

// clashReachable reports whether the Clash API responds (short timeout).
func (s *APIServer) clashReachable() bool {
	if s.clash == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	return s.clash.Reachable(ctx)
}
