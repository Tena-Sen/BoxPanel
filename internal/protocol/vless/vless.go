// Package vless implements the VLESS protocol: share-link parse/serialize
// and sing-box outbound generation.
package vless

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"boxpanel/internal/models"
	"boxpanel/internal/protocol"
)

const name = "vless"

func init() { protocol.Register(&VLESS{}) }

// VLESS implements protocol.Protocol for VLESS.
type VLESS struct{}

func (VLESS) Name() string         { return name }
func (VLESS) Schemes() []string    { return []string{"vless"} }

// Parse parses a vless:// share link.
func (VLESS) Parse(uri string) (*models.Server, error) {
	if !strings.HasPrefix(uri, "vless://") {
		return nil, fmt.Errorf("vless link must start with vless://")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse vless url: %w", err)
	}
	if u.User.Username() == "" {
		return nil, fmt.Errorf("vless link missing uuid")
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("vless link missing server")
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		return nil, fmt.Errorf("vless link missing port")
	}
	q := u.Query()
 ttype := strings.ToLower(q.Get("type"))
	if ttype == "" {
		ttype = "tcp"
	}
	// 入库统一规范: splithttp→xhttp (输出时由 Adapter 按目标内核版本动态转换)
	if ttype == "splithttp" {
		ttype = "xhttp"
	}

	srv := &models.Server{
		ID:          "",
		Protocol:    models.ProtoVless,
		Name:        u.Fragment,
		Server:      u.Hostname(),
		ServerPort:  port,
		UUID:        u.User.Username(),
		Flow:        q.Get("flow"),
		TransportType: ttype,
		TransportPath: q.Get("path"),
		TransportHost: q.Get("host"),
		TransportMode: q.Get("mode"),
		TransportXPaddingBytes: q.Get("x_padding_bytes"),
	}
	if srv.Name == "" {
		srv.Name = fmt.Sprintf("%s:%d", srv.Server, srv.ServerPort)
	}

	// extra 字段（xhttp 透明填充等）
	if extra := q.Get("extra"); extra != "" {
		var obj map[string]any
		if err := json.Unmarshal([]byte(extra), &obj); err == nil {
			if v, ok := obj["xPaddingBytes"]; ok && srv.TransportXPaddingBytes == "" {
				srv.TransportXPaddingBytes = fmt.Sprintf("%v", v)
			}
		}
	}

	// security
	security := strings.ToLower(q.Get("security"))
	if security == "tls" || security == "reality" {
		srv.TLSEnabled = true
		srv.TLSServerName = q.Get("sni")
		if fp := q.Get("fp"); fp != "" {
			srv.TLSFingerprint = fp
		}
		if alpn := q.Get("alpn"); alpn != "" {
			srv.TLSALPN = strings.Split(alpn, ",")
		}
		if security == "reality" {
			srv.RealityEnabled = true
			srv.RealityPublicKey = q.Get("pbk")
			srv.RealityShortID = q.Get("sid")
			srv.RealitySpiderX = q.Get("spx")
		}
	}
	return srv, nil
}

// ToURI serializes a Server to a vless:// share link.
func (VLESS) ToURI(srv models.Server) (string, error) {
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
	if srv.TransportMode != "" {
		q.Set("mode", srv.TransportMode)
	}
	if srv.TransportXPaddingBytes != "" {
		q.Set("x_padding_bytes", srv.TransportXPaddingBytes)
	}
	if srv.RealityPublicKey != "" {
		q.Set("pbk", srv.RealityPublicKey)
	}
	if srv.RealityShortID != "" {
		q.Set("sid", srv.RealityShortID)
	}
	if srv.RealitySpiderX != "" {
		q.Set("spx", srv.RealitySpiderX)
	}
	if srv.TLSServerName != "" {
		q.Set("sni", srv.TLSServerName)
	}
	if srv.TLSFingerprint != "" {
		q.Set("fp", srv.TLSFingerprint)
	}
	if srv.RealityEnabled {
		q.Set("security", "reality")
	} else if srv.TLSEnabled {
		q.Set("security", "tls")
	} else {
		q.Set("security", "none")
	}
	if len(srv.TLSALPN) > 0 {
		q.Set("alpn", strings.Join(srv.TLSALPN, ","))
	}
	host := fmt.Sprintf("%s@%s:%d", srv.UUID, srv.Server, srv.ServerPort)
	return fmt.Sprintf("vless://%s?%s#%s", host, q.Encode(), url.QueryEscape(srv.Name)), nil
}

// Outbound builds a sing-box VLESS outbound.
func (VLESS) Outbound(srv models.Server) (map[string]any, error) {
	ob := map[string]any{
		"type":          "vless",
		"tag":           "proxy",
		"server":        srv.Server,
		"server_port":   srv.ServerPort,
		"uuid":          srv.UUID,
		"packet_encoding": "xudp",
	}
	if srv.Flow != "" && srv.TransportType == "tcp" {
		ob["flow"] = srv.Flow
	}
	// transport
	if srv.TransportType != "" && srv.TransportType != "tcp" {
		ttype := srv.TransportType
		// 注：xhttp vs splithttp 的选择由 Adapter.Apply() 根据目标 sing-box 版本动态处理
		// 这里保留原始值不做转换
		transport := map[string]any{"type": ttype}
		if srv.TransportPath != "" {
			transport["path"] = srv.TransportPath
		}
		if srv.TransportHost != "" {
			transport["host"] = srv.TransportHost
		}
		if srv.TransportMode != "" {
			switch srv.TransportType {
			case "xhttp", "splithttp", "httpupgrade":
				transport["mode"] = srv.TransportMode
			}
		}
		if srv.TransportXPaddingBytes != "" {
			transport["x_padding_bytes"] = srv.TransportXPaddingBytes
		}
		if len(srv.TransportHeaders) > 0 {
			transport["headers"] = srv.TransportHeaders
		}
		ob["transport"] = transport
	}
	// tls
	if srv.TLSEnabled {
		ob["tls"] = buildTLS(srv)
	}
	return ob, nil
}

func buildTLS(srv models.Server) map[string]any {
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
	if srv.RealityEnabled {
		reality := map[string]any{"enabled": true}
		if srv.RealityPublicKey != "" {
			reality["public_key"] = srv.RealityPublicKey
		}
		if srv.RealityShortID != "" {
			reality["short_id"] = srv.RealityShortID
		}
		// 注：sing-box 1.14+ 移除了 reality.spider_x 字段，客户端无需该参数
		tls["reality"] = reality
	}
	return tls
}
