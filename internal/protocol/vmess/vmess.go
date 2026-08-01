// Package vmess implements the VMess protocol (base64 JSON share link).
package vmess

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

const name = "vmess"

func init() { protocol.Register(&VMess{}) }

// VMess implements protocol.Protocol for VMess.
type VMess struct{}

func (VMess) Name() string      { return name }
func (VMess) Schemes() []string { return []string{"vmess"} }

type vmessLink struct {
	V        any    `json:"v"`
	PS       string `json:"ps"`
	Add      string `json:"add"`
	Port     any    `json:"port"`
	ID       string `json:"id"`
	Aid      any    `json:"aid"`
	Net      string `json:"net"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	TLS      string `json:"tls"`
	SNI      string `json:"sni"`
	ALPN     string `json:"alpn"`
	FP       string `json:"fp"`
}

func (VMess) Parse(uri string) (*models.Server, error) {
	if !strings.HasPrefix(uri, "vmess://") {
		return nil, fmt.Errorf("vmess link must start with vmess://")
	}
	payload := strings.TrimSpace(uri[len("vmess://"):])
	decoded, ok := protocol.Base64Decode(payload)
	if !ok {
		return nil, fmt.Errorf("vmess base64 decode failed")
	}
	var v vmessLink
	if err := json.Unmarshal([]byte(decoded), &v); err != nil {
		return nil, fmt.Errorf("vmess json parse: %w", err)
	}
	port := toInt(v.Port)
	transport := v.Net
	if transport == "" {
		transport = "tcp"
	}
	if transport == "http" {
		transport = "httpupgrade" // sing-box naming
	}
	tlsEnabled := v.TLS == "tls" || v.TLS == "reality"
	name := v.PS
	if name == "" {
		name = fmt.Sprintf("%s:%d", v.Add, port)
	}
	var alpn []string
	if v.ALPN != "" {
		alpn = strings.Split(v.ALPN, ",")
	}
	return &models.Server{
		Protocol:       models.ProtoVMess,
		Name:           name,
		Server:         v.Add,
		ServerPort:     port,
		UUID:           v.ID,
		AlterID:        toInt(v.Aid),
		TransportType:  transport,
		TransportPath:  v.Path,
		TransportHost:  v.Host,
		TLSEnabled:     tlsEnabled,
		TLSServerName:  v.SNI,
		TLSFingerprint: v.FP,
		TLSALPN:        alpn,
	}, nil
}

func (VMess) ToURI(srv models.Server) (string, error) {
	net := srv.TransportType
	if net == "" {
		net = "tcp"
	}
	if net == "httpupgrade" {
		net = "http"
	}
	v := vmessLink{
		V:    "2",
		PS:   srv.Name,
		Add:  srv.Server,
		Port: strconv.Itoa(srv.ServerPort),
		ID:   srv.UUID,
		Aid:  strconv.Itoa(srv.AlterID),
		Net:  net,
		Host: srv.TransportHost,
		Path: srv.TransportPath,
		TLS:  ternary(srv.TLSEnabled, "tls", ""),
		SNI:  srv.TLSServerName,
	}
	raw, _ := json.Marshal(v)
	encoded := base64.StdEncoding.EncodeToString(raw)
	return "vmess://" + encoded, nil
}

func (VMess) Outbound(srv models.Server) (map[string]any, error) {
	ob := map[string]any{
		"type":        "vmess",
		"tag":         "proxy",
		"server":      srv.Server,
		"server_port": srv.ServerPort,
		"uuid":        srv.UUID,
		"alter_id":    srv.AlterID,
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
	if srv.TLSEnabled {
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
	}
	return ob, nil
}

func toInt(v any) int {
	switch n := v.(type) {
	case string:
		x, _ := strconv.Atoi(n)
		return x
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
