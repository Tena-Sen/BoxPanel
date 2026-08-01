package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/core/configgen"
	"boxpanel/internal/import_"
	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

func (s *APIServer) handleListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.store.ListServers(r.Context())
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if servers == nil {
		servers = []models.Server{}
	}
	writeJSON(w, 200, servers)
}

func (s *APIServer) handleGetServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := s.store.GetServer(r.Context(), id)
	if err != nil || srv == nil {
		writeError(w, 404, "not found")
		return
	}
	writeJSON(w, 200, srv)
}

func (s *APIServer) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var srv models.Server
	if err := readJSON(r, &srv); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if srv.Server == "" || srv.ServerPort == 0 {
		writeError(w, 400, "server / server_port required")
		return
	}
	if srv.ID == "" {
		srv.ID = models.NewID("srv")
	}
	if srv.Protocol == "" {
		srv.Protocol = models.ProtoVless
	}
	if srv.Name == "" {
		srv.Name = srv.Server
	}
	if err := s.store.SaveServer(r.Context(), srv); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, srv)
}

func (s *APIServer) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var srv models.Server
	if err := readJSON(r, &srv); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	srv.ID = id
	if err := s.store.SaveServer(r.Context(), srv); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, srv)
}

func (s *APIServer) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteServer(r.Context(), id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	// 从分组移除
	groups, _ := s.store.ListGroups(r.Context())
	for _, g := range groups {
		changed := false
		out := make([]string, 0, len(g.ServerIDs))
		for _, sid := range g.ServerIDs {
			if sid == id {
				changed = true
				continue
			}
			out = append(out, sid)
		}
		if changed {
			g.ServerIDs = out
			_ = s.store.SaveGroup(r.Context(), g)
		}
	}
	writeJSON(w, 200, map[string]string{"deleted": id})
}

func (s *APIServer) handleSelectServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, _ := s.store.GetSettings(r.Context())
	st.CurrentServerID = id
	if err := s.store.SaveSettings(r.Context(), st); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()

	// Hot-switch: if core is running and Clash API is available, switch proxy
	// without restart (sing-box/mihomo selector outbound supports this)
	if s.runner.IsRunning() && s.clash != nil && s.clash.Reachable(r.Context()) {
		tag := configgen.ServerTag(id)
		if err := s.clash.SelectProxy(r.Context(), "proxy", tag); err != nil {
			// Hot-switch failed (e.g. tag not found) — log but don't fail
			// The user can still restart manually
			_ = err
		}
	}

	writeJSON(w, 200, map[string]string{"selected": id})
}

func (s *APIServer) handleServerLatency(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := s.store.GetServer(r.Context(), id)
	if err != nil || srv == nil {
		writeError(w, 404, "not found")
		return
	}
	ms, err := s.lat.TestOne(r.Context(), *srv)
	if err != nil {
		writeJSON(w, 200, map[string]any{"id": id, "latency_ms": nil, "error": err.Error()})
		return
	}
	v := ms
	srv.LastLatency = &v
	_ = s.store.SaveServer(r.Context(), *srv)
	writeJSON(w, 200, map[string]any{"id": id, "latency_ms": ms})
}

func (s *APIServer) handleBatchLatency(w http.ResponseWriter, r *http.Request) {
	servers, err := s.store.ListServers(r.Context())
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	results := s.lat.TestMany(r.Context(), servers)
	// 写回 last_latency_ms
	for i := range servers {
		if ms, ok := results[servers[i].ID]; ok {
			if ms > 0 {
				v := ms
				servers[i].LastLatency = &v
			} else {
				servers[i].LastLatency = nil
			}
		}
	}
	_ = s.store.BatchSaveServers(r.Context(), servers)
	writeJSON(w, 200, results)
}

func (s *APIServer) handleImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	incoming, err := import_.FromText(body.Text)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if len(incoming) == 0 {
		writeError(w, 400, "未识别到任何服务器")
		return
	}
	existing, _ := s.store.ListServers(r.Context())
	merged, stats := mergeServers(existing, incoming, false)
	_ = s.store.BatchSaveServers(r.Context(), merged)
	writeJSON(w, 200, map[string]any{
		"added": stats.added, "updated": stats.updated, "total": stats.added + stats.updated,
	})
}

func (s *APIServer) handleImportFile(w http.ResponseWriter, r *http.Request) {
	// raw body
	if r.Body == nil {
		writeError(w, 400, "empty body")
		return
	}
	defer r.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	incoming, err := import_.FromBytes(buf, r.URL.Query().Get("filename"))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	existing, _ := s.store.ListServers(r.Context())
	merged, _ := mergeServers(existing, incoming, false)
	_ = s.store.BatchSaveServers(r.Context(), merged)
	writeJSON(w, 200, map[string]any{"imported": len(incoming)})
}

func (s *APIServer) handleExport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	_ = readJSON(r, &body)
	servers, _ := s.store.ListServers(r.Context())
	want := map[string]bool{}
	for _, id := range body.IDs {
		want[id] = true
	}
	var lines []string
	for _, srv := range servers {
		if len(want) > 0 && !want[srv.ID] {
			continue
		}
		p, err := protocol.Get(srv.Protocol)
		if err != nil {
			continue
		}
		uri, err := p.ToURI(srv)
		if err != nil {
			continue
		}
		lines = append(lines, uri)
	}
	writeJSON(w, 200, map[string]string{"text": strings.Join(lines, "\n")})
}

// mergeServers dedups by protocol|server|port|cred.
type mergeStats struct{ added, updated int }

func mergeServers(existing, incoming []models.Server, replace bool) ([]models.Server, mergeStats) {
	byKey := map[string]models.Server{}
	for _, s := range existing {
		byKey[dedupKey(s)] = s
	}
	var out []models.Server
	seen := map[string]bool{}
	stats := mergeStats{}
	for _, s := range incoming {
		k := dedupKey(s)
		if seen[k] {
			// 同一批次内重复 - 静默跳过，避免 added 双计
			continue
		}
		seen[k] = true
		if old, ok := byKey[k]; ok {
			s.ID = old.ID
			s.AddedAt = old.AddedAt
			if old.LastLatency != nil {
				v := *old.LastLatency
				s.LastLatency = &v
			}
			stats.updated++
		} else {
			stats.added++
		}
		out = append(out, s)
	}
	if !replace {
		for _, s := range existing {
			if !seen[dedupKey(s)] {
				out = append(out, s)
			}
		}
	}
	return out, stats
}

func dedupKey(s models.Server) string {
	cred := s.UUID
	if s.Protocol == models.ProtoShadowsocks || s.Protocol == models.ProtoHysteria2 {
		cred = s.Password
	}
	if s.Protocol == models.ProtoTUIC {
		cred = s.TUICUUID
	}
	return s.Protocol + "|" + s.Server + "|" + strconv.Itoa(s.ServerPort) + "|" + cred
}
