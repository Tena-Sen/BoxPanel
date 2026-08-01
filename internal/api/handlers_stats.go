package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"boxpanel/internal/core/clashapi"
)

func (s *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	// 实时流量来自 Clash API（运行时）；未运行则返回零值。
	if s.clash == nil || !s.clashReachable() {
		writeJSON(w, 200, map[string]any{
			"up_bps": 0, "down_bps": 0, "up_total": 0, "down_total": 0, "running": false,
		})
		return
	}
	conns, err := s.clash.Connections(r.Context())
	if err != nil {
		writeJSON(w, 200, map[string]any{"running": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"running":       true,
		"up_total":      conns.UploadTotal,
		"down_total":    conns.DownloadTotal,
		"connections":   len(conns.Connections),
		"memory":        conns.Memory,
	})
}

// handleLogsSSE streams sing-box log lines to the client via Server-Sent Events.
func (s *APIServer) handleLogsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	flusher.Flush()

	// 同时订阅 clash logs（若可用）与 runner 日志广播
	ch, cancel := s.logBroadcaster.Subscribe()
	defer cancel()

	// 可选：clash logs WS（goroutine 在 ctx 取消时自动退出）
	var clashCh chan clashapi.LogEntry
	if s.clashReachable() {
		clashCh = make(chan clashapi.LogEntry, 200)
		go func() {
			_ = s.clash.ConsumeLogs(r.Context(), clashCh)
		}()
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if msg.isExit {
				writeSSE(w, flusher, "exit", fmt.Sprintf("%d", msg.exitCode))
			} else {
				writeSSE(w, flusher, "log", msg.line)
			}
		case le := <-clashCh:
			writeSSE(w, flusher, "clashlog", le.Type+" "+le.Payload)
		case <-ticker.C:
			writeSSE(w, flusher, "ping", "")
		}
	}
}

// handleTraffic streams live up/down traffic via SSE (polled from Clash API).
func (s *APIServer) handleTraffic(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	if s.clash == nil {
		writeSSE(w, flusher, "error", "clash api unavailable")
		return
	}

	trafficCh := make(chan clashapi.Traffic, 50)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.clash.ConsumeTraffic(r.Context(), trafficCh)
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			return
		case t, ok := <-trafficCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(t)
			writeSSE(w, flusher, "traffic", string(data))
		}
	}
}

// writeSSE writes a Server-Sent Event with an optional event name and flushes.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	for _, line := range splitNewlines(data) {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
	flusher.Flush()
}

func splitNewlines(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	out = append(out, cur)
	return out
}
