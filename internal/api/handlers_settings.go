package api

import (
	"net/http"

	"boxpanel/internal/models"
)

func (s *APIServer) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, _ := s.store.GetSettings(r.Context())
	writeJSON(w, 200, st)
}

func (s *APIServer) handleSetSettings(w http.ResponseWriter, r *http.Request) {
	var patch models.Settings
	if err := readJSON(r, &patch); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	cur, _ := s.store.GetSettings(r.Context())
	// 合并：非零字段覆盖
	merged := mergeSettings(cur, patch)
	if err := s.store.SaveSettings(r.Context(), merged); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	s.mu.Lock()
	s.settings = merged
	s.mu.Unlock()
	// 设置变更可能影响 clash client / latency url
	s.refreshClashClient()
	writeJSON(w, 200, merged)
}

// mergeSettings overlays non-zero fields of patch onto cur.
func mergeSettings(cur, patch models.Settings) models.Settings {
	out := cur
	if patch.Theme != "" {
		out.Theme = patch.Theme
	}
	if patch.Language != "" {
		out.Language = patch.Language
	}
	if patch.LogLevel != "" {
		out.LogLevel = patch.LogLevel
	}
	if patch.Mode != "" {
		out.Mode = patch.Mode
	}
	if patch.CurrentServerID != "" {
		out.CurrentServerID = patch.CurrentServerID
	}
	if patch.CurrentProfileID != "" {
		out.CurrentProfileID = patch.CurrentProfileID
	}
	if patch.ListenPort != 0 {
		out.ListenPort = patch.ListenPort
	}
	if patch.LatencyTestURL != "" {
		out.LatencyTestURL = patch.LatencyTestURL
	}
	if patch.SubscriptionUA != "" {
		out.SubscriptionUA = patch.SubscriptionUA
	}
	if patch.ClashAPIPort != 0 {
		out.ClashAPIPort = patch.ClashAPIPort
	}
	if patch.ClashAPISecret != "" {
		out.ClashAPISecret = patch.ClashAPISecret
	}
	// bool 字段直接覆盖（无法区分未设置）
	out.AutoRefreshSubs = patch.AutoRefreshSubs
	out.AutoStartCore = patch.AutoStartCore
	return out
}
