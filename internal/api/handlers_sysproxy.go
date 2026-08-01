package api

import (
	"net/http"
)

func (s *APIServer) handleSysProxyGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.sys.Get())
}

func (s *APIServer) handleSysProxyEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Server string `json:"server"`
		Bypass string `json:"bypass"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if body.Server == "" {
		body.Server = "127.0.0.1:20808"
	}
	if body.Bypass == "" {
		body.Bypass = "<local>;127.*;10.*;172.16.*;192.168.*"
	}
	st, err := s.sys.Enable(body.Server, body.Bypass)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *APIServer) handleSysProxyDisable(w http.ResponseWriter, r *http.Request) {
	st, err := s.sys.Disable()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}
