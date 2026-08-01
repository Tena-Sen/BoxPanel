// Package bandwidth provides download-speed testing for proxy nodes.
//
// 带宽测试通过 HTTP 下载方式测量节点的实际下行速度（Mbps）。
//
// 核心改进：
//   - SOCKS5 代理优先（Hysteria2 等 QUIC 协议兼容更好），失败回退 HTTP CONNECT
//   - 多源测速 URL（Cloudflare/nginx/caddy 大厂 CDN），单源失败自动切换
//   - 内核未运行时拒绝测试（直连下载测的是本机带宽，无意义）
package bandwidth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/proxy"

	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/models"
)

// SpeedTestURLs are reliable CDN-hosted download URLs (~10-25 MB).
// Ordered by reliability: Cloudflare > nginx > caddy > fallback.
var SpeedTestURLs = []string{
	"https://speed.cloudflare.com/__down?bytes=25000000",    // Cloudflare 25MB
	"https://speed.cloudflare.com/__down?bytes=10000000",    // Cloudflare 10MB
	"https://bahaha.pp.ru/generate_10mb",                   // caddy test file
	"https://proof.ovh.net/files/10Mb.dat",                 // OVH 10MB
	"http://cachefly.cachefly.net/10mb.test",               // CacheFly 10MB
	"http://speedtest.tele2.net/10MB.zip",                  // Tele2 10MB
}

// Result holds a single bandwidth test outcome.
type Result struct {
	ServerID  string  `json:"server_id"`
	Mbps      float64 `json:"mbps"`
	BytesRead int64   `json:"bytes_read"`
	Duration  string  `json:"duration"`
	Error     string  `json:"error,omitempty"`
	Source    string  `json:"source,omitempty"`  // which URL was used
	ProxyMode string `json:"proxy_mode,omitempty"` // "socks5" or "http"
}

// Tester measures download bandwidth through a proxy.
type Tester struct {
	clash     *clashapi.Client
	dlURLs    []string
	proxyAddr string // local proxy listen address (e.g. "127.0.0.1:2080")
}

// New creates a bandwidth Tester.
func New(clash *clashapi.Client, proxyAddr string, dlURLs []string) *Tester {
	if len(dlURLs) == 0 {
		dlURLs = SpeedTestURLs
	}
	return &Tester{
		clash:     clash,
		dlURLs:    dlURLs,
		proxyAddr: proxyAddr,
	}
}

// TestOne measures download bandwidth for a single server.
// It tries SOCKS5 proxy first (better for QUIC protocols like Hysteria2),
// then falls back to HTTP CONNECT proxy. Also tries multiple download URLs
// if the first one fails.
func (t *Tester) TestOne(ctx context.Context, srv models.Server, timeout time.Duration) Result {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	result := Result{ServerID: srv.ID}

	// Guard: proxy must be reachable
	if t.proxyAddr == "" {
		result.Error = "未配置代理地址，请先启动内核"
		return result
	}
	if !t.isProxyReachable(ctx) {
		result.Error = "代理未连通，请先启动内核再测速"
		return result
	}

	// Try SOCKS5 first, then HTTP CONNECT
	proxyModes := []struct {
		name    string
		buildFn func(time.Duration) *http.Client
	}{
		{"socks5", t.buildSOCKS5Client},
		{"http", t.buildHTTPClient},
	}

	var lastErr string
	for _, mode := range proxyModes {
		client := mode.buildFn(timeout)

		// Try each download URL
		for _, dlURL := range t.dlURLs {
			ctx2, cancel := context.WithTimeout(ctx, timeout)
			start := time.Now()

			req, err := http.NewRequestWithContext(ctx2, http.MethodGet, dlURL, nil)
			if err != nil {
				cancel()
				lastErr = fmt.Sprintf("create request: %v", err)
				continue
			}
			// Don't follow redirects for Cloudflare speed test (it may redirect)
			client2 := *client
			client2.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				// Allow up to 5 redirects
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			}

			resp, err := client2.Do(req)
			if err != nil {
				cancel()
				lastErr = fmt.Sprintf("download via %s: %v", mode.name, err)
				continue // try next URL
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
				resp.Body.Close()
				cancel()
				lastErr = fmt.Sprintf("status %d from %s", resp.StatusCode, dlURL)
				continue
			}

			// Read the response body, discarding data but counting bytes
			n, readErr := io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			cancel()

			elapsed := time.Since(start)

			// Context deadline is expected (we read as much as we can in the timeout)
			if readErr != nil && ctx2.Err() != context.DeadlineExceeded && ctx.Err() != context.DeadlineExceeded {
				// Real error (not just timeout)
				if elapsed < 500*time.Millisecond {
					// Failed very quickly — likely a connection error, try next URL
					lastErr = fmt.Sprintf("read body: %v", readErr)
					continue
				}
				// Read some data before error — use what we have
			}

			// If we got less than 50KB, this URL/source probably didn't work
			if n < 50*1024 && elapsed < 2*time.Second {
				lastErr = fmt.Sprintf("too little data (%d bytes) from %s", n, dlURL)
				continue
			}

			result.BytesRead = n
			result.Duration = elapsed.Truncate(time.Millisecond).String()
			result.Source = dlURL
			result.ProxyMode = mode.name

			// Mbps = bytes * 8 / seconds / 1_000_000
			seconds := elapsed.Seconds()
			if seconds > 0 {
				result.Mbps = float64(n) * 8 / seconds / 1_000_000
			}
			return result
		}
	}

	// All attempts failed
	result.Error = lastErr
	if result.Error == "" {
		result.Error = "所有测速源均失败，请检查网络连接"
	}
	return result
}

// isProxyReachable checks if the local proxy port is accepting connections.
func (t *Tester) isProxyReachable(ctx context.Context) bool {
	if t.clash != nil && t.clash.Reachable(ctx) {
		return true
	}
	// Fallback: try TCP dial
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", t.proxyAddr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// buildSOCKS5Client creates an HTTP client that routes through a SOCKS5 proxy.
// SOCKS5 is preferred for Hysteria2 and other QUIC protocols because
// it doesn't require HTTP CONNECT tunneling.
func (t *Tester) buildSOCKS5Client(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxConnsPerHost:     2,
		MaxIdleConns:        2,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	if t.proxyAddr != "" {
		proxyAddr := t.proxyAddr
		// Custom DialContext that does SOCKS5 handshake with timeout support
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Use a dedicated dialer with timeout
			dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil,
				&net.Dialer{Timeout: 5 * time.Second})
			if err != nil {
				return nil, err
			}
			// SOCKS5 dial (doesn't natively support context, but the
			// underlying Dialer has a timeout)
			conn, err := dialer.Dial(network, addr)
			if err != nil {
				return nil, fmt.Errorf("socks5 dial %s via %s: %w", addr, proxyAddr, err)
			}
			return conn, nil
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// buildHTTPClient creates an HTTP client that routes through an HTTP CONNECT proxy.
func (t *Tester) buildHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxConnsPerHost:     2,
		MaxIdleConns:        2,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	if t.proxyAddr != "" {
		proxyURL := &url.URL{
			Scheme: "http",
			Host:   t.proxyAddr,
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// ParseProxyAddr validates a host:port string for proxy use.
func ParseProxyAddr(addr string, _ string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid proxy address %q: %w", addr, err)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", fmt.Errorf("invalid port in %q", addr)
	}
	_ = p
	return net.JoinHostPort(host, port), nil
}
