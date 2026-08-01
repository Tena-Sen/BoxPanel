// Package tuic implements the TUIC v5 protocol.
package tuic

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

const name = "tuic"

func init() { protocol.Register(&TUIC{}) }

// TUIC implements protocol.Protocol for TUIC v5.
type TUIC struct{}

func (TUIC) Name() string      { return name }
func (TUIC) Schemes() []string { return []string{"tuic"} }

func (TUIC) Parse(uri string) (*models.Server, error) {
	if !strings.HasPrefix(uri, "tuic://") {
		return nil, fmt.Errorf("tuic link must start with tuic://")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse tuic url: %w", err)
	}
	uuid := u.User.Username()
	password, _ := u.User.Password()
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	q := u.Query()
	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	cc := q.Get("congestion_control")
	if cc == "" {
		cc = "bbr"
	}
	return &models.Server{
		Protocol:              models.ProtoTUIC,
		Name:                  name,
		Server:                u.Hostname(),
		ServerPort:            port,
		TUICUUID:              uuid,
		TUICPassword:          password,
		TUICCongestionControl: cc,
		TUICUDPRelayMode:      q.Get("udp_relay_mode"),
		TLSEnabled:            true,
		TLSServerName:         q.Get("sni"),
		TLSInsecure:           q.Get("allow_insecure") == "1",
		TLSALPN:               splitCSV(q.Get("alpn")),
	}, nil
}

func (TUIC) ToURI(srv models.Server) (string, error) {
	q := url.Values{}
	if srv.TLSServerName != "" {
		q.Set("sni", srv.TLSServerName)
	}
	if srv.TLSInsecure {
		q.Set("allow_insecure", "1")
	}
	if srv.TUICCongestionControl != "" {
		q.Set("congestion_control", srv.TUICCongestionControl)
	}
	if srv.TUICUDPRelayMode != "" {
		q.Set("udp_relay_mode", srv.TUICUDPRelayMode)
	}
	if len(srv.TLSALPN) > 0 {
		q.Set("alpn", strings.Join(srv.TLSALPN, ","))
	}
	userinfo := srv.TUICUUID
	if srv.TUICPassword != "" {
		userinfo = fmt.Sprintf("%s:%s", srv.TUICUUID, srv.TUICPassword)
	}
	host := fmt.Sprintf("%s@%s:%d", userinfo, srv.Server, srv.ServerPort)
	qs := ""
	if len(q) > 0 {
		qs = "?" + q.Encode()
	}
	return fmt.Sprintf("tuic://%s%s#%s", host, qs, url.QueryEscape(srv.Name)), nil
}

func (TUIC) Outbound(srv models.Server) (map[string]any, error) {
	ob := map[string]any{
		"type":        "tuic",
		"tag":         "proxy",
		"server":      srv.Server,
		"server_port": srv.ServerPort,
		"uuid":        srv.TUICUUID,
	}
	if srv.TUICPassword != "" {
		ob["password"] = srv.TUICPassword
	}
	if srv.TUICCongestionControl != "" {
		ob["congestion_control"] = srv.TUICCongestionControl
	}
	if srv.TUICUDPRelayMode != "" {
		ob["udp_relay_mode"] = srv.TUICUDPRelayMode
	}
	tls := map[string]any{"enabled": true}
	if srv.TLSServerName != "" {
		tls["server_name"] = srv.TLSServerName
	}
	if srv.TLSInsecure {
		tls["insecure"] = true
	}
	if len(srv.TLSALPN) > 0 {
		tls["alpn"] = srv.TLSALPN
	}
	ob["tls"] = tls
	return ob, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
