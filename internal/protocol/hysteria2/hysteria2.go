// Package hysteria2 implements the Hysteria2 protocol.
package hysteria2

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

const name = "hysteria2"

func init() { protocol.Register(&Hysteria2{}) }

// Hysteria2 implements protocol.Protocol for Hysteria2.
type Hysteria2 struct{}

func (Hysteria2) Name() string      { return name }
func (Hysteria2) Schemes() []string { return []string{"hysteria2", "hy2"} }

func (Hysteria2) Parse(uri string) (*models.Server, error) {
	if !strings.HasPrefix(uri, "hysteria2://") && !strings.HasPrefix(uri, "hy2://") {
		return nil, fmt.Errorf("hysteria2 link must start with hysteria2:// or hy2://")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse hysteria2 url: %w", err)
	}
	password := u.User.Username()
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	q := u.Query()
	up, _ := strconv.Atoi(q.Get("up"))
	down, _ := strconv.Atoi(q.Get("down"))
	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	return &models.Server{
		Protocol:       models.ProtoHysteria2,
		Name:           name,
		Server:         u.Hostname(),
		ServerPort:     port,
		Password:       password,
		TLSEnabled:     true,
		TLSServerName:  q.Get("sni"),
		TLSInsecure:    q.Get("insecure") == "1",
		Hy2UpMbps:      up,
		Hy2DownMbps:    down,
		Hy2Obfs:        q.Get("obfs"),
		Hy2ObfsPassword: q.Get("obfs-password"),
	}, nil
}

func (Hysteria2) ToURI(srv models.Server) (string, error) {
	q := url.Values{}
	if srv.TLSServerName != "" {
		q.Set("sni", srv.TLSServerName)
	}
	if srv.TLSInsecure {
		q.Set("insecure", "1")
	}
	if srv.Hy2UpMbps > 0 {
		q.Set("up", strconv.Itoa(srv.Hy2UpMbps))
	}
	if srv.Hy2DownMbps > 0 {
		q.Set("down", strconv.Itoa(srv.Hy2DownMbps))
	}
	if srv.Hy2Obfs != "" {
		q.Set("obfs", srv.Hy2Obfs)
	}
	if srv.Hy2ObfsPassword != "" {
		q.Set("obfs-password", srv.Hy2ObfsPassword)
	}
	host := fmt.Sprintf("%s@%s:%d", srv.Password, srv.Server, srv.ServerPort)
	qs := ""
	if len(q) > 0 {
		qs = "?" + q.Encode()
	}
	return fmt.Sprintf("hysteria2://%s%s#%s", host, qs, url.QueryEscape(srv.Name)), nil
}

func (Hysteria2) Outbound(srv models.Server) (map[string]any, error) {
	ob := map[string]any{
		"type":        "hysteria2",
		"tag":         "proxy",
		"server":      srv.Server,
		"server_port": srv.ServerPort,
	}
	if srv.Password != "" {
		ob["password"] = srv.Password
	}
	if srv.Hy2UpMbps > 0 {
		ob["up_mbps"] = srv.Hy2UpMbps
	}
	if srv.Hy2DownMbps > 0 {
		ob["down_mbps"] = srv.Hy2DownMbps
	}
	if srv.Hy2Obfs != "" {
		ob["obfs"] = map[string]any{
			"type":     srv.Hy2Obfs,
			"password": srv.Hy2ObfsPassword,
		}
	}
	tls := map[string]any{"enabled": true}
	if srv.TLSServerName != "" {
		tls["server_name"] = srv.TLSServerName
	}
	if srv.TLSInsecure {
		tls["insecure"] = true
	}
	ob["tls"] = tls
	return ob, nil
}
