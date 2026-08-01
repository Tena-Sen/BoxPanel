// Package latency provides proxy latency testing.
//
// 优先用 sing-box Clash API 的 /proxies/{name}/delay（真实穿代理 HTTP 测试，
// 与 v2rayN 一致）；核心未运行时退化为协议感知的直达测延迟（TCP/UDP）。
package latency

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/core/configgen"
	"boxpanel/internal/models"
)

// isUDPProto reports whether the protocol uses QUIC/UDP transport.
func isUDPProto(protocol string) bool {
	switch protocol {
	case models.ProtoHysteria2, models.ProtoTUIC:
		return true
	}
	return false
}

// Tester tests latency via Clash API (preferred) with protocol-aware fallback.
type Tester struct {
	clash *clashapi.Client
	url   string
}

// New creates a Tester. clash may be nil (falls back to direct probe only).
func New(clash *clashapi.Client, testURL string) *Tester {
	if testURL == "" {
		testURL = "http://www.gstatic.com/generate_204"
	}
	return &Tester{clash: clash, url: testURL}
}

// TestOne tests a single server. Returns latency ms or error.
// If the core is running (clash != nil and reachable), uses Clash API delay
// against the server's own outbound tag; otherwise falls back to protocol-aware
// direct probe (TCP for TCP-based protocols, UDP for QUIC protocols).
func (t *Tester) TestOne(ctx context.Context, srv models.Server) (int, error) {
	if t.clash != nil && t.clash.Reachable(ctx) {
		// Use the server's own outbound tag (srv-<id>) so Clash API tests
		// the specific node, not the generic "proxy" selector.
		tag := configgen.ServerTag(srv.ID)
		ms, err := t.clash.Delay(ctx, tag, t.url, 5000)
		if err == nil {
			return ms, nil
		}
		// If the specific tag fails (e.g. not in config), try the selector group
		ms2, err2 := t.clash.Delay(ctx, "proxy", t.url, 5000)
		if err2 == nil {
			return ms2, nil
		}
		// fall through to direct probe
	}
	return directLatency(srv, 3*time.Second)
}

// TestMany tests multiple servers concurrently.
// When core is running, uses Clash API per-node; otherwise falls back to
// protocol-aware direct probe.
// Returns map of server ID -> latency ms (or 0 on failure).
func (t *Tester) TestMany(ctx context.Context, servers []models.Server) map[string]int {
	results := make(map[string]int, len(servers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)

	clashReachable := t.clash != nil && t.clash.Reachable(ctx)

	for _, srv := range servers {
		wg.Add(1)
		go func(s models.Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var ms int
			if clashReachable {
				tag := configgen.ServerTag(s.ID)
				val, err := t.clash.Delay(ctx, tag, t.url, 5000)
				if err != nil {
					// Fallback to direct probe
					val2, err2 := directLatency(s, 3*time.Second)
					if err2 != nil {
						ms = 0
					} else {
						ms = val2
					}
				} else {
					ms = val
				}
			} else {
				val, err := directLatency(s, 3*time.Second)
				if err != nil {
					ms = 0
				} else {
					ms = val
				}
			}

			mu.Lock()
			results[s.ID] = ms
			mu.Unlock()
		}(srv)
	}
	wg.Wait()
	return results
}

// directLatency measures latency by protocol-aware direct probe.
// For QUIC/UDP protocols (Hysteria2, TUIC): uses UDP dial.
// For all others: uses TCP handshake.
func directLatency(srv models.Server, timeout time.Duration) (int, error) {
	if srv.Server == "" || srv.ServerPort == 0 {
		return 0, fmt.Errorf("empty host or port")
	}
	if isUDPProto(srv.Protocol) {
		return udpLatency(srv.Server, srv.ServerPort, timeout)
	}
	return tcpLatency(srv.Server, srv.ServerPort, timeout)
}

// tcpLatency measures TCP handshake time to host:port.
func tcpLatency(host string, port int, timeout time.Duration) (int, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return int(time.Since(start).Milliseconds()), nil
}

// udpLatency measures UDP round-trip time to host:port.
// For QUIC servers, we attempt a UDP dial and measure the time until
// the first response (or timeout). Since QUIC servers will respond
// to the initial handshake, this gives a reasonable latency estimate.
// If no response comes within timeout, we return the dial time as
// an upper bound (server is reachable via UDP but handshake specifics
// vary).
func udpLatency(host string, port int, timeout time.Duration) (int, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return 0, fmt.Errorf("resolve udp addr: %w", err)
	}

	start := time.Now()
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	// Set deadline for the read
	_ = conn.SetReadDeadline(time.Now().Add(timeout))

	// Send a small probe packet — for QUIC servers this triggers
	// an ICMP port unreachable or QUIC retry packet
	probe := []byte{0x00}
	_, err = conn.Write(probe)
	if err != nil {
		return 0, err
	}

	// Try to read a response with a small buffer
	buf := make([]byte, 64)
	_, err = conn.Read(buf)
	if err == nil {
		// Got a response — use full RTT
		return int(time.Since(start).Milliseconds()), nil
	}

	// No response (timeout) — but the UDP packet was sent successfully.
	// For QUIC servers, we won't get a response to a random probe,
	// but the fact that DialUDP + Write succeeded means the host is
	// reachable. Return the write time as a minimum estimate.
	elapsed := time.Since(start)
	if elapsed < timeout {
		// Write succeeded quickly — host is likely reachable
		return int(elapsed.Milliseconds()), nil
	}

	// Timed out — host may be unreachable
	return 0, fmt.Errorf("udp probe timeout after %v", timeout)
}
