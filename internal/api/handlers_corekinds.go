package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/coreinfo"
	"boxpanel/internal/models"
)

// GET /api/cores/kinds — list all registered core kinds with their capabilities
func (s *APIServer) handleListCoreKinds(w http.ResponseWriter, r *http.Request) {
	if s.coreMgr == nil {
		writeJSON(w, 200, map[string]any{"kinds": []any{}})
		return
	}
	// Merge CoreMgr KindInfo with CoreInfo metadata
	writeJSON(w, 200, map[string]any{
		"kinds":       s.coreMgr.KindInfo(),
		"core_info":   coreinfo.AllInfo(),
		"active_kind": s.coreMgr.ActiveKind(),
	})
}

// POST /api/cores/{id}/switch-kind — change the kind of an existing core entry
func (s *APIServer) handleSwitchCoreKind(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Kind string `json:"kind"`
	}
	if err := readJSON(r, &body); err != nil || body.Kind == "" {
		writeError(w, 400, "kind is required")
		return
	}
	// Validate kind
	validKinds := map[string]bool{
		models.CoreKindSingBox:   true,
		models.CoreKindXray:      true,
		models.CoreKindMihomo:    true,
		models.CoreKindHysteria2: true,
	}
	if !validKinds[body.Kind] {
		writeError(w, 400, "invalid kind, must be one of: singbox, xray, mihomo, hysteria2")
		return
	}

	st, _ := s.store.GetSettings(r.Context())
	found := false
	for i := range st.Cores {
		if st.Cores[i].ID == id {
			st.Cores[i].Kind = body.Kind
			found = true
			break
		}
	}
	if !found {
		writeError(w, 404, "core not found")
		return
	}
	_ = s.store.SaveSettings(r.Context(), st)
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()

	writeJSON(w, 200, map[string]any{
		"id":   id,
		"kind": body.Kind,
	})
}
