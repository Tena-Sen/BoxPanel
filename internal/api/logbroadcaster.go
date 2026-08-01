package api

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"boxpanel/internal/config"
)

// LogBroadcaster fans out sing-box log lines to multiple SSE subscribers.
//
// 同时维护一个环形缓冲区（最近 maxBacklog 行），新客户端 Subscribe 时立即收到缓存，
// 解决"核心启动早期日志在 SSE 客户端连上之前被丢弃"的问题。
//
// 日志持久化：每条日志追加写入 data/logs/core.log，重启后从文件恢复历史日志。
type LogBroadcaster struct {
	mu          sync.Mutex
	subscribers map[chan logMsg]struct{}
	backlog     []logMsg            // 环形缓冲
	maxBacklog  int
	logFile     *os.File
	logWriter   *bufio.Writer
}

type logMsg struct {
	line     string
	isExit   bool
	exitCode int
}

// NewLogBroadcaster creates a LogBroadcaster with default backlog (500 lines).
// Loads recent history from the persistent log file.
func NewLogBroadcaster() *LogBroadcaster {
	b := &LogBroadcaster{
		subscribers: map[chan logMsg]struct{}{},
		maxBacklog:  500,
	}
	b.openLogFile()
	b.loadHistory()
	return b
}

// Broadcast sends a log line to all subscribers + appends to backlog + writes to file.
func (b *LogBroadcaster) Broadcast(line string) {
	b.mu.Lock()
	msg := logMsg{line: line}
	b.appendBacklogLocked(msg)
	b.writeToFileLocked(line)
	for ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			// 慢消费者丢弃
		}
	}
	b.mu.Unlock()
}

// BroadcastExit notifies subscribers the process exited + records in backlog + file.
func (b *LogBroadcaster) BroadcastExit(code int) {
	b.mu.Lock()
	msg := logMsg{isExit: true, exitCode: code}
	b.appendBacklogLocked(msg)
	exitLine := fmt.Sprintf("[%s] === core process exited (code %d) ===", time.Now().Format(time.RFC3339), code)
	b.writeToFileLocked(exitLine)
	for ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
		}
	}
	b.mu.Unlock()
}

// Subscribe returns a channel receiving future log messages. The cancel func
// unsubscribes; the channel will also receive the current backlog before
// live messages.
func (b *LogBroadcaster) Subscribe() (<-chan logMsg, func()) {
	ch := make(chan logMsg, b.maxBacklog+100)
	b.mu.Lock()
	// 先发缓存（确保 SSE 客户端立即看到历史日志）
	for _, m := range b.backlog {
		ch <- m
	}
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
		go func() {
			for range ch {
			}
		}()
		close(ch)
	}
	return ch, cancel
}

// Close flushes and closes the log file.
func (b *LogBroadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.logWriter != nil {
		b.logWriter.Flush()
	}
	if b.logFile != nil {
		b.logFile.Close()
		b.logFile = nil
		b.logWriter = nil
	}
}

// appendBacklogLocked adds msg to backlog, trimming to maxBacklog.
func (b *LogBroadcaster) appendBacklogLocked(msg logMsg) {
	b.backlog = append(b.backlog, msg)
	if len(b.backlog) > b.maxBacklog {
		b.backlog = b.backlog[len(b.backlog)-b.maxBacklog:]
	}
}

// openLogFile creates or opens the persistent log file for appending.
func (b *LogBroadcaster) openLogFile() {
	logDir := filepath.Join(config.DataDir(), "logs")
	_ = os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "core.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Non-fatal: log persistence is best-effort
		return
	}
	b.logFile = f
	b.logWriter = bufio.NewWriterSize(f, 4096)
}

// writeToFileLocked appends a log line to the persistent log file.
func (b *LogBroadcaster) writeToFileLocked(line string) {
	if b.logWriter == nil {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	b.logWriter.WriteString(ts + " " + line + "\n")
	// Flush periodically (every write for safety; bufio batches small writes anyway)
	b.logWriter.Flush()
}

// loadHistory reads the last N lines from the persistent log file into backlog.
func (b *LogBroadcaster) loadHistory() {
	logPath := filepath.Join(config.DataDir(), "logs", "core.log")
	data, err := os.ReadFile(logPath)
	if err != nil || len(data) == 0 {
		return
	}

	lines := strings.Split(string(data), "\n")
	// Take last maxBacklog non-empty lines
	start := len(lines) - b.maxBacklog
	if start < 0 {
		start = 0
	}
	for _, l := range lines[start:] {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		// Strip timestamp prefix (format: "2026-08-01T10:00:00+08:00 <actual log>")
		actualLine := l
		if idx := strings.IndexByte(l, ' '); idx > 0 {
			candidate := l[idx+1:]
			// Check if prefix looks like a timestamp
			if len(l[:idx]) > 16 { // RFC3339 is at least 19 chars
				actualLine = candidate
			}
		}
		// Check for exit marker
		if strings.Contains(actualLine, "=== core process exited") {
			b.appendBacklogLocked(logMsg{isExit: true, exitCode: -1})
		} else {
			b.appendBacklogLocked(logMsg{line: actualLine})
		}
	}
}
