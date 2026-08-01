// Package core manages the sing-box subprocess and Clash API integration.
package core

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"boxpanel/internal/config"
)

// Runner manages the sing-box subprocess lifecycle (cross-platform).
type Runner struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	exePath  string
	baseDir  string
	onLog    func(line string)
	onExit   func(code int)
	startedAt time.Time // when the process was last started
}

// NewRunner creates a Runner for the sing-box executable at exePath.
func NewRunner(exePath, baseDir string) *Runner {
	return &Runner{exePath: exePath, baseDir: baseDir}
}

// ExePath returns the current sing-box executable path.
func (r *Runner) ExePath() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exePath
}

// SetExePath changes the sing-box executable path (used to swap between multiple cores).
func (r *Runner) SetExePath(p string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exePath = p
}

// SetLogHandler registers a callback invoked per stdout/stderr line.
func (r *Runner) SetLogHandler(fn func(line string)) { r.onLog = fn }

// SetExitHandler registers a callback invoked when the process exits.
func (r *Runner) SetExitHandler(fn func(code int)) { r.onExit = fn }

// IsRunning reports whether sing-box is currently running.
func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil && r.cmd.Process != nil && r.cmd.ProcessState == nil
}

// PID returns the running process PID, or 0 if not running.
func (r *Runner) PID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil && r.cmd.Process != nil {
		return r.cmd.Process.Pid
	}
	return 0
}

// Uptime returns how long the core has been running, or 0 if not running.
func (r *Runner) Uptime() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil || r.cmd.Process == nil || r.cmd.ProcessState != nil {
		return 0
	}
	if r.startedAt.IsZero() {
		return 0
	}
	return time.Since(r.startedAt)
}

// Start launches sing-box with the given config path.
func (r *Runner) Start(configPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil && r.cmd.ProcessState == nil {
		return fmt.Errorf("sing-box already running")
	}
	// 与旧 .bat 一致：先杀掉残留进程
	killExistingSingBox()

	cmd := exec.Command(r.exePath, "run", "-c", configPath)
	cmd.Dir = r.baseDir
	hideWindow(cmd)

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // 合并 stderr 到 stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start sing-box: %w", err)
	}
	r.cmd = cmd
	r.startedAt = time.Now()

	// 读日志行
	go r.pump(pipe)

	// 等待退出
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = -1
			}
		}
		r.mu.Lock()
		r.cmd = nil
		r.mu.Unlock()
		if r.onExit != nil {
			r.onExit(code)
		}
	}()
	return nil
}

func (r *Runner) pump(pipe interface{ Read([]byte) (int, error) }) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := pipe.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			// 按行切分
			for {
				idx := -1
				for i, b := range buf {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx < 0 {
					break
				}
				line := string(buf[:idx])
				buf = buf[idx+1:]
				if r.onLog != nil {
					r.onLog(line)
				}
			}
		}
		if err != nil {
			if len(buf) > 0 && r.onLog != nil {
				r.onLog(string(buf))
			}
			return
		}
	}
}

// Stop terminates the running sing-box process (and its children).
func (r *Runner) Stop() error {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// 优雅停止：先 taskkill /T（子进程树），跨平台
	killProcessTree(cmd.Process.Pid)
	// 等待退出
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// 强制 kill
		_ = cmd.Process.Kill()
	}
	return nil
}

// Restart stops and starts with the given config.
func (r *Runner) Restart(configPath string) error {
	_ = r.Stop()
	time.Sleep(200 * time.Millisecond)
	return r.Start(configPath)
}

// Check runs `sing-box check -c <config>` and returns any error.
func (r *Runner) Check(configPath string) error {
	cmd := exec.Command(r.exePath, "check", "-c", configPath)
	cmd.Dir = r.baseDir
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}

// killExistingSingBox kills any leftover sing-box process (best-effort).
func killExistingSingBox() {
	name := config.SingBoxExeName()
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("taskkill", "/f", "/t", "/im", name).Run()
	default:
		_ = exec.Command("pkill", "-f", "sing-box").Run()
	}
}

// killProcessTree kills a process and its children.
func killProcessTree(pid int) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("taskkill", "/f", "/t", "/pid", fmt.Sprintf("%d", pid)).Run()
	default:
		_ = exec.Command("kill", "-TERM", fmt.Sprintf("%d", pid)).Run()
	}
}

// hideWindow sets creation flags to hide the console window on Windows.
func hideWindow(cmd *exec.Cmd) {
	if runtime.GOOS != "windows" {
		return
	}
	// CREATE_NO_WINDOW = 0x08000000
	cmd.SysProcAttr = hideWindowAttr()
}
