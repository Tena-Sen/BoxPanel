package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/models"
)

// handleServerBandwidth tests download bandwidth for a single server.
// Requires the core to be running (proxy must be reachable).
// POST /api/servers/{id}/bandwidth
func (s *APIServer) handleServerBandwidth(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := s.store.GetServer(r.Context(), id)
	if err != nil || srv == nil {
		writeError(w, 404, "not found")
		return
	}

	// Parse optional timeout from query (?timeout=15)
	timeout := 10 * time.Second
	if ts := r.URL.Query().Get("timeout"); ts != "" {
		if d, err := time.ParseDuration(ts + "s"); err == nil && d > 0 && d <= 60*time.Second {
			timeout = d
		}
	}

	result := s.bw.TestOne(r.Context(), *srv, timeout)
	if result.Error != "" {
		writeJSON(w, 200, map[string]any{
			"id":         id,
			"mbps":       nil,
			"error":      result.Error,
			"bytes_read": result.BytesRead,
			"duration":   result.Duration,
		})
		return
	}

	// Persist bandwidth result
	mbps := result.Mbps
	srv.LastBandwidth = &mbps
	_ = s.store.SaveServer(r.Context(), *srv)

	writeJSON(w, 200, map[string]any{
		"id":         id,
		"mbps":       result.Mbps,
		"bytes_read": result.BytesRead,
		"duration":   result.Duration,
	})
}

// handleBatchBandwidth runs bandwidth test for the currently active server only.
// Since all servers share the same proxy tunnel, testing more than one is redundant.
// POST /api/servers/batch-bandwidth
func (s *APIServer) handleBatchBandwidth(w http.ResponseWriter, r *http.Request) {
	// Only test the current server (all nodes share the same proxy pipe)
	st, _ := s.store.GetSettings(r.Context())
	var srv *models.Server
	var err error
	if st.CurrentServerID != "" {
		srv, err = s.store.GetServer(r.Context(), st.CurrentServerID)
		if err != nil {
			srv = nil
		}
	}
	if srv == nil {
		// Fallback: pick first server
		servers, serr := s.store.ListServers(r.Context())
		if serr != nil || len(servers) == 0 {
			writeError(w, 400, "no server available for bandwidth test")
			return
		}
		srv = &servers[0]
	}

	timeout := 10 * time.Second
	if ts := r.URL.Query().Get("timeout"); ts != "" {
		if d, err := time.ParseDuration(ts + "s"); err == nil && d > 0 && d <= 60*time.Second {
			timeout = d
		}
	}

	result := s.bw.TestOne(r.Context(), *srv, timeout)

	// Write back
	if result.Error == "" {
		mbps := result.Mbps
		srv.LastBandwidth = &mbps
		_ = s.store.SaveServer(r.Context(), *srv)
	}

	resp := map[string]any{}
	if result.Error != "" {
		resp[srv.ID] = map[string]any{"mbps": nil, "error": result.Error}
	} else {
		resp[srv.ID] = map[string]any{
			"mbps":       result.Mbps,
			"bytes_read": result.BytesRead,
			"duration":   result.Duration,
		}
	}
	writeJSON(w, 200, resp)
}
