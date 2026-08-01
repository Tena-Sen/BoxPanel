// Package shadowsocks implements the Shadowsocks protocol (SIP002 + legacy).
package shadowsocks

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

const name = "shadowsocks"

func init() { protocol.Register(&Shadowsocks{}) }

// Shadowsocks implements protocol.Protocol for Shadowsocks.
type Shadowsocks struct{}

func (Shadowsocks) Name() string      { return name }
func (Shadowsocks) Schemes() []string { return []string{"ss"} }

func (Shadowsocks) Parse(uri string) (*models.Server, error) {
	if !strings.HasPrefix(uri, "ss://") {
		return nil, fmt.Errorf("ss link must start with ss://")
	}
	body := uri[len("ss://"):]
	name := ""
	if i := strings.Index(body, "#"); i >= 0 {
		name, _ = url.QueryUnescape(body[i+1:])
		body = body[:i]
	}
	// SIP002 整体 base64
	if decoded, ok := protocol.Base64Decode(body); ok && strings.Contains(decoded, "@") {
		body = decoded
	}
	if !strings.Contains(body, "@") {
		return nil, fmt.Errorf("ss link malformed (no @)")
	}
	at := strings.LastIndex(body, "@")
	userinfo := body[:at]
	hostport := body[at+1:]
	// legacy: userinfo 整体 base64
	if !strings.Contains(userinfo, ":") {
		if decoded, ok := protocol.Base64Decode(userinfo); ok {
			userinfo = decoded
		}
	}
	if !strings.Contains(userinfo, ":") {
		return nil, fmt.Errorf("ss link missing method:password")
	}
	colon := strings.Index(userinfo, ":")
	method := userinfo[:colon]
	password := userinfo[colon+1:]
	host := hostport
	port := 443
	if i := strings.LastIndex(hostport, ":"); i >= 0 {
		host = hostport[:i]
		port, _ = strconv.Atoi(hostport[i+1:])
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	return &models.Server{
		Protocol:   models.ProtoShadowsocks,
		Name:       name,
		Server:     host,
		ServerPort: port,
		Method:     method,
		Password:   password,
	}, nil
}

func (Shadowsocks) ToURI(srv models.Server) (string, error) {
	method := srv.Method
	if method == "" {
		method = "chacha20-ietf-poly1305"
	}
	userinfo := base64.URLEncoding.EncodeToString([]byte(method + ":" + srv.Password))
	userinfo = strings.TrimRight(userinfo, "=")
	return fmt.Sprintf("ss://%s@%s:%d#%s",
		userinfo, srv.Server, srv.ServerPort, url.QueryEscape(srv.Name)), nil
}

func (Shadowsocks) Outbound(srv models.Server) (map[string]any, error) {
	method := srv.Method
	if method == "" {
		method = "chacha20-ietf-poly1305"
	}
	ob := map[string]any{
		"type":        "shadowsocks",
		"tag":         "proxy",
		"server":      srv.Server,
		"server_port": srv.ServerPort,
		"method":      method,
		"password":    srv.Password,
	}
	return ob, nil
}
