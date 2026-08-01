// Package readyprobe provides SOCKS5-based readiness detection for proxy cores.
//
// Inspired by v2rayN: instead of just checking if a TCP port is listening,
// we perform a full SOCKS5 handshake to confirm the proxy is truly ready
// to accept traffic. This avoids false positives where the port is open
// but the core hasn't finished initializing yet.
//
// Detection strategies (in order of reliability):
//  1. SOCKS5 handshake — confirms proxy is fully initialized
//  2. TCP port open — fallback, only confirms port is bound
//  3. Clash API reachable — for cores with Clash API (sing-box, mihomo)
package readyprobe

import (
	"context"
	"fmt"
	"net"
	"time"
)

// ProbeResult is the result of a readiness probe.
type ProbeResult struct {
	Ready      bool          `json:"ready"`
	Method     string        `json:"method"`      // "socks5" | "tcp" | "clash_api"
	Latency    time.Duration `json:"latency"`
	Error      string        `json:"error,omitempty"`
}

// WaitForReady polls until the proxy at the given SOCKS5 address is ready,
// or the context is cancelled / timeout is reached.
//
// addr is "host:port" of the SOCKS5/mixed inbound.
// timeout is the maximum time to wait.
func WaitForReady(ctx context.Context, addr string, timeout time.Duration) ProbeResult {
	deadline := time.Now().Add(timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ProbeResult{Ready: false, Error: "timeout waiting for core ready"}
		default:
		}

		// Try SOCKS5 handshake first (most reliable)
		if result := socks5Handshake(addr); result.Ready {
			return result
		}

		// Wait a bit before retrying
		time.Sleep(200 * time.Millisecond)
	}
}

// socks5Handshake performs a SOCKS5 handshake to confirm the proxy is ready.
//
// SOCKS5 handshake sequence:
//  1. Connect to the SOCKS5 port
//  2. Send greeting: [0x05, 0x01, 0x00] (version 5, 1 auth method, no auth)
//  3. Read response: [0x05, 0x00] (version 5, no auth required)
//
// If we get a valid SOCKS5 response, the proxy is truly ready.
func socks5Handshake(addr string) ProbeResult {
	start := time.Now()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return ProbeResult{Ready: false, Method: "socks5", Error: err.Error()}
	}
	defer conn.Close()

	// Send SOCKS5 greeting: version 5, 1 auth method, no auth
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return ProbeResult{Ready: false, Method: "socks5", Error: fmt.Sprintf("write greeting: %v", err)}
	}

	// Read response with timeout
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2)
	n, err := conn.Read(buf)
	if err != nil {
		return ProbeResult{Ready: false, Method: "socks5", Error: fmt.Sprintf("read response: %v", err)}
	}
	if n < 2 {
		return ProbeResult{Ready: false, Method: "socks5", Error: "short response"}
	}

	// Valid SOCKS5 response: version 5 + chosen auth method
	if buf[0] == 0x05 {
		latency := time.Since(start)
		return ProbeResult{
			Ready:   true,
			Method:  "socks5",
			Latency: latency,
		}
	}

	return ProbeResult{Ready: false, Method: "socks5", Error: fmt.Sprintf("invalid SOCKS5 response: %x", buf[:n])}
}

// TCPProbe checks if a TCP port is accepting connections (weaker signal than SOCKS5).
func TCPProbe(addr string) ProbeResult {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return ProbeResult{Ready: false, Method: "tcp", Error: err.Error()}
	}
	conn.Close()
	return ProbeResult{
		Ready:   true,
		Method:  "tcp",
		Latency: time.Since(start),
	}
}
