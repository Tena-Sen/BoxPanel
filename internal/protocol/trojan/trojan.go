// Package trojan implements the Trojan protocol.
package trojan

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

const name = "trojan"

func init() { protocol.Register(&Trojan{}) }

// Trojan implements protocol.Protocol for Trojan.
type Trojan struct{}

func (Trojan) Name() string      { return name }
func (Trojan) Schemes() []string { return []string{"trojan"} }

func (Trojan) Parse(uri string) (*models.Server, error) {
	if !strings.HasPrefix(uri, "trojan://") {
		return nil, fmt.Errorf("trojan link must start with trojan://")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse trojan url: %w", err)
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("trojan link missing password")
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	q := u.Query()
	transport := strings.ToLower(q.Get("type"))
	if transport == "" {
		transport = "tcp"
	}
	if transport == "http" {
		transport = "httpupgrade"
	}
	if transport == "splithttp" {
		transport = "xhttp" // 入库统一规范, 输出时由 Adapter 按版本动态转换
	}
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	return &models.Server{
		Protocol:       models.ProtoTrojan,
		Name:           name,
		Server:         u.Hostname(),
		ServerPort:     port,
		Password:       password,
		TransportType:  transport,
		TransportPath:  q.Get("path"),
		TransportHost:  q.Get("host"),
		TLSEnabled:     true,
		TLSServerName:  sni,
		TLSFingerprint: q.Get("fp"),
	}, nil
}

func (Trojan) ToURI(srv models.Server) (string, error) {
	q := url.Values{}
	if srv.TransportType != "" && srv.TransportType != "tcp" {
		q.Set("type", srv.TransportType)
	}
	if srv.TransportPath != "" {
		q.Set("path", srv.TransportPath)
	}
	if srv.TransportHost != "" {
		q.Set("host", srv.TransportHost)
	}
	if srv.TLSServerName != "" {
		q.Set("sni", srv.TLSServerName)
	}
	if srv.TLSFingerprint != "" {
		q.Set("fp", srv.TLSFingerprint)
	}
	if srv.TLSInsecure {
		q.Set("allowInsecure", "1")
	}
	host := fmt.Sprintf("%s@%s:%d", srv.Password, srv.Server, srv.ServerPort)
	qs := ""
	if len(q) > 0 {
		qs = "?" + q.Encode()
	}
	return fmt.Sprintf("trojan://%s%s#%s", host, qs, url.QueryEscape(srv.Name)), nil
}

func (Trojan) Outbound(srv models.Server) (map[string]any, error) {
	ob := map[string]any{
		"type":        "trojan",
		"tag":         "proxy",
		"server":      srv.Server,
		"server_port": srv.ServerPort,
		"password":    srv.Password,
	}
	if srv.TransportType != "" && srv.TransportType != "tcp" {
		ttype := srv.TransportType
		// 注：xhttp vs splithttp 由 Adapter.Apply() 根据版本动态处理
		transport := map[string]any{"type": ttype}
		if srv.TransportPath != "" {
			transport["path"] = srv.TransportPath
		}
		if srv.TransportHost != "" {
			transport["host"] = srv.TransportHost
		}
		ob["transport"] = transport
	}
	tls := map[string]any{"enabled": true}
	if srv.TLSServerName != "" {
		tls["server_name"] = srv.TLSServerName
	}
	if srv.TLSFingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": srv.TLSFingerprint}
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
