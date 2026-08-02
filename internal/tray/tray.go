// Package tray 提供跨平台系统托盘集成 + Windows 开机自启。
//
// 退出模型：systray.Run 阻塞事件循环，必须放 goroutine。
// main.go 启 HTTP server，systray 在另一个 goroutine，二者通过 context 通信。
package tray

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
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

// getIcon returns a 32x32 BoxPanel icon (blue rounded square + "BP" text).
func getIcon() []byte {
	const sz = 32
	m := image.NewRGBA(image.Rect(0, 0, sz, sz))

	// Background: transparent
	draw.Draw(m, m.Bounds(), image.Transparent, image.Point{}, draw.Src)

	// Solid blue rounded square
	accent := color.RGBA{0x89, 0xb4, 0xfa, 0xff}
	r := 6 // corner radius
	pad := 2
	for y := pad; y < sz-pad; y++ {
		for x := pad; x < sz-pad; x++ {
			dx, dy := 0, 0
			if x < pad+r {
				dx = pad + r - x
			} else if x > sz-pad-r-1 {
				dx = x - (sz - pad - r - 1)
			}
			if y < pad+r {
				dy = pad + r - y
			} else if y > sz-pad-r-1 {
				dy = y - (sz - pad - r - 1)
			}
			// Inside rounded rect: fill; on/inside corner circle: fill
			if dx*dx+dy*dy <= r*r || dx == 0 || dy == 0 {
				m.Set(x, y, accent)
			}
		}
	}

	// Draw "BP" text in white
	face := basicfont.Face7x13
	d := font.Drawer{
		Dst:  m,
		Src:  &image.Uniform{color.RGBA{0xff, 0xff, 0xff, 0xff}},
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(8), Y: fixed.I(21)},
	}
	d.DrawString("BP")

	var buf bytes.Buffer
	png.Encode(&buf, m)
	return buf.Bytes()
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