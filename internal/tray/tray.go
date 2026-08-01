// Package tray 提供跨平台系统托盘集成 + Windows 开机自启。
//
// 退出模型：systray.Run 阻塞事件循环，必须放 goroutine。
// main.go 启 HTTP server，systray 在另一个 goroutine，二者通过 context 通信。
package tray

import (
	"context"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"
)

// Config 托盘配置
type Config struct {
	AppName    string            // "BoxPanel"
	URL        string            // "http://127.0.0.1:7820"
	ExePath    string            // 自身路径（自启用）
	OpenBrowser func(url string) // 调默认浏览器
	OnToggleAutostart func() bool // 切换自启：返回新状态
	GetAutostart     func() bool // 读自启状态
	Quit       func()            // 退出
}

// Run 启托盘（阻塞直到用户选退出）。
// 必须在独立 goroutine 调用。
func Run(cfg Config) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		// Linux 上 systray 需要 libayatana-appindicator 等
		// 这里做个软失败：暂不启用托盘，main 正常跑
		slog.Info("tray: skipped (unsupported on " + runtime.GOOS + ")")
		select {} // 永久阻塞
	}

	onExit := func() {
		slog.Info("tray: exit")
		if cfg.Quit != nil {
			cfg.Quit()
		}
	}

	onReady := func() {
		slog.Info("tray: ready")
		systray.SetIcon(getIcon())
		systray.SetTitle(cfg.AppName)
		systray.SetTooltip(cfg.AppName + " - " + cfg.URL)

		mOpen := systray.AddMenuItem("打开面板", "Open in browser")
		systray.AddSeparator()
		mAuto := systray.AddMenuItemCheckbox("开机自启", "Start on boot", cfg.GetAutostart != nil && cfg.GetAutostart())
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "Stop core and exit")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					if cfg.OpenBrowser != nil {
						cfg.OpenBrowser(cfg.URL)
					}
				case <-mAuto.ClickedCh:
					if cfg.OnToggleAutostart != nil {
						newState := cfg.OnToggleAutostart()
						if newState {
							mAuto.Check()
						} else {
							mAuto.Uncheck()
						}
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}

	systray.Run(onReady, onExit)
}

// OpenBrowserByOS uses OS default browser to open URL.
func OpenBrowserByOS(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// getIcon returns a tiny embedded icon (16x16 black square PNG).
// 真实项目应该用 .ico 文件；这里用最小占位图标让 systray 不报错。
func getIcon() []byte {
	// 16x16 黑色 PNG（最简）
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG header
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR
		0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, // 16x16
		0x08, 0x00, 0x00, 0x00, 0x00, 0x3B, 0x7E, 0x9B, 0x55, // bit depth
		0x00, 0x00, 0x00, 0x1A, 0x49, 0x44, 0x41, 0x54, // IDAT
		0x78, 0x9C, 0xED, 0xC1, 0x01, 0x0D, 0x00, 0x00,
		0xC0, 0xA0, 0xFF, 0xC0, 0x40, 0x04, 0x1B, 0x30,
		0x06, 0xE3, 0x60, 0x00, 0x00, 0x00, 0x00, 0xC0,
		0xA0, 0x0F, 0x00, 0x00, 0x5B, 0x07, 0x5F, 0x6D,
		0x9F, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
		0x44, 0xAE, 0x42, 0x60, 0x82, // IEND
	}
}

// 确保 http 包被引用（占位）
var _ = http.MethodGet

// 防止 context 警告（保留供未来使用）
var _ = context.TODO

// IsAutostartEnabled 跨平台自启查询（Windows 实现，其他平台返回 false）。
func IsAutostartEnabled(appName, exePath string) bool {
	return autostartEnabled(appName, exePath)
}

// SetAutostart 跨平台自启设置。
func SetAutostart(appName, exePath string, enable bool) error {
	return setAutostart(appName, exePath, enable)
}