// Package clashapi is a client for sing-box's experimental clash_api.
//
// 这是重构的枢纽：通过 Clash API 获得真实的代理选择、穿代理测速、
// 实时流量/日志/连接，替代旧版的手写 current_server_id、TCP 握手测速、
// 正则解析日志等脆弱实现。
package clashapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrCoreNotRunning is returned when the Clash API endpoint is unreachable,
// typically because the proxy core is not running.
var ErrCoreNotRunning = fmt.Errorf("内核未运行，Clash API 不可达")

// isConnRefused reports whether the error is a TCP connection-refused or
// similar network-level failure (not an HTTP-level error).
func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	// net.OpError: dial tcp 127.0.0.1:9090: connectex: No connection could be made...
	if _, ok := err.(*net.OpError); ok {
		return true
	}
	// Wrap in url.Error: Put "http://...": dial tcp ...
	if _, ok := err.(*url.Error); ok {
		return true
	}
	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "No connection could be made") ||
		strings.Contains(err.Error(), "connectex:")
}

// Client talks to a sing-box clash_api endpoint.
type Client struct {
	baseURL  string
	secret   string
	http     *http.Client
	mu       sync.Mutex
	wsDialer *websocket.Dialer
}

// New creates a Client for http://host:port with optional secret.
func New(host string, port int, secret string) *Client {
	base := fmt.Sprintf("http://%s:%d", host, port)
	return &Client{
		baseURL:  base,
		secret:   secret,
		http:     &http.Client{Timeout: 5 * time.Second},
		wsDialer: &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
	}
}

// Reachable reports whether the clash API is responding.
func (c *Client) Reachable(ctx context.Context) bool {
	_, err := c.get(ctx, "/version")
	return err == nil
}

// Version returns the clash API version info.
func (c *Client) Version(ctx context.Context) (map[string]any, error) {
	return c.get(ctx, "/version")
}

// ProxyEntry describes a proxy in the /proxies response.
type ProxyEntry struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Now     string         `json:"now,omitempty"`
	History []map[string]any `json:"history,omitempty"`
	All     []string       `json:"all,omitempty"`
}

// ProxiesResp is the /proxies response.
type ProxiesResp struct {
	Proxies map[string]ProxyEntry `json:"proxies"`
}

// Proxies returns all proxies and groups.
func (c *Client) Proxies(ctx context.Context) (ProxiesResp, error) {
	var resp ProxiesResp
	err := c.getJSON(ctx, "/proxies", &resp)
	return resp, wrapErr(err)
}

// SelectProxy selects a member in a selector group.
func (c *Client) SelectProxy(ctx context.Context, group, name string) error {
	body := fmt.Sprintf(`{"name":%q}`, name)
	return wrapErr(c.putJSON(ctx, "/proxies/"+url.PathEscape(group), []byte(body)))
}

// Delay tests latency of a proxy through the core (real HTTP test).
func (c *Client) Delay(ctx context.Context, name, testURL string, timeoutMs int) (int, error) {
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeoutMs))
	path := "/proxies/" + url.PathEscape(name) + "/delay?" + q.Encode()
	resp, err := c.get(ctx, path)
	if err != nil {
		return 0, wrapErr(err)
	}
	// {"delay": 123} or {"message": "..."}
	if v, ok := resp["delay"]; ok {
		switch n := v.(type) {
		case float64:
			return int(n), nil
		}
	}
	if msg, ok := resp["message"].(string); ok {
		return 0, fmt.Errorf("%s", msg)
	}
	return 0, fmt.Errorf("unexpected delay response: %v", resp)
}

// Traffic is an instantaneous up/down sample (bytes/sec).
type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// ConsumeTraffic connects to the /traffic WS and streams samples until ctx done.
func (c *Client) ConsumeTraffic(ctx context.Context, ch chan<- Traffic) error {
	return c.consumeWS(ctx, "/traffic", func(data []byte) {
		var t Traffic
		if err := json.Unmarshal(data, &t); err == nil {
			select {
			case ch <- t:
			default:
			}
		}
	})
}

// LogEntry is a single log line from /logs.
type LogEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// ConsumeLogs connects to the /logs WS and streams log entries until ctx done.
func (c *Client) ConsumeLogs(ctx context.Context, ch chan<- LogEntry) error {
	return c.consumeWS(ctx, "/logs", func(data []byte) {
		var e LogEntry
		if err := json.Unmarshal(data, &e); err == nil {
			select {
			case ch <- e:
			default:
			}
		}
	})
}

// Connection is an active connection (subset of fields).
type Connection struct {
	ID          string         `json:"id"`
	Upload      int64          `json:"upload"`
	Download    int64          `json:"download"`
	Start       string         `json:"start"`
	Chains      []string       `json:"chains"`
	Rule        string         `json:"rule"`
	RulePayload string         `json:"rulePayload"`
	Metadata    map[string]any `json:"metadata"`
}

// ConnectionsResp is the /connections response.
type ConnectionsResp struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        int64        `json:"memory"`
}

// Connections returns the current active connections.
func (c *Client) Connections(ctx context.Context) (ConnectionsResp, error) {
	var resp ConnectionsResp
	err := c.getJSON(ctx, "/connections", &resp)
	return resp, wrapErr(err)
}

// wrapErr replaces raw TCP-level errors with ErrCoreNotRunning so callers
// never see ugly "dial tcp 127.0.0.1:9090: connectex: ..." messages.
func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	if isConnRefused(err) {
		return ErrCoreNotRunning
	}
	return err
}

// ----- HTTP helpers -----

func (c *Client) get(ctx context.Context, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.addAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("clash api %s: %s", path, string(body))
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		// 可能是非 JSON（如纯文本错误）
		return map[string]any{"_text": string(body)}, nil
	}
	return obj, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.addAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("clash api %s: %s", path, string(body))
	}
	return json.Unmarshal(body, out)
}

func (c *Client) putJSON(ctx context.Context, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	c.addAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clash api PUT %s: %s", path, string(b))
	}
	return nil
}

func (c *Client) addAuth(req *http.Request) {
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
}

// ----- WebSocket helpers -----

func (c *Client) consumeWS(ctx context.Context, path string, onMsg func([]byte)) error {
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1) + path
	header := http.Header{}
	if c.secret != "" {
		header.Set("Authorization", "Bearer "+c.secret)
	}
	conn, _, err := c.wsDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})
	go func() {
		// ping keepalive
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			}
		}
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		onMsg(data)
	}
}
