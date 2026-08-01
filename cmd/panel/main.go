// Command boxpanel is the single-binary sing-box management panel.
//
// 启动流程：打开/迁移 SQLite -> 注入服务 -> 起 HTTP（API + 内嵌前端）-> 开浏览器。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"boxpanel/internal/api"
	"boxpanel/internal/bootstrap"
	"boxpanel/internal/config"
	"boxpanel/internal/core"
	"boxpanel/internal/core/mihomo"
	"boxpanel/internal/core/singbox"
	"boxpanel/internal/core/xray"
	"boxpanel/internal/core/hysteria2"
	"boxpanel/internal/core/configgen"
	"boxpanel/internal/coredl"
	"boxpanel/internal/models"
	"boxpanel/internal/rulesets"
	"boxpanel/internal/store/sqlite"
	"boxpanel/internal/subscription"
	"boxpanel/internal/sysproxy"
	"boxpanel/internal/tray"
	"boxpanel/internal/web"
)

// 引用 bootstrap 以触发协议插件 init() 注册
var _ = bootstrap.BootstrapMarker

func main() {
	port := flag.Int("port", config.DefaultPort, "HTTP listen port")
	host := flag.String("host", config.Host, "HTTP listen host")
	noBrowser := flag.Bool("no-browser", false, "do not open browser automatically")
	noTray := flag.Bool("no-tray", false, "disable system tray (use when running headless)")
	flag.Parse()

	slog.Info("starting boxpanel", "version", config.Version, "base_dir", config.BaseDir())

	// 0. 迁移旧数据库文件名 sbpanel.db → boxpanel.db
	config.MigrateLegacyDB()

	slog.Info("sing-box exe", "path", config.ExePath(),
		"exists", fileExists(config.ExePath()))

	// 1. 打开存储（自动迁移）
	st, err := sqlite.Open(config.DBPath())
	if err != nil {
		slog.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	slog.Info("store ready", "db", config.DBPath())

	// 首次启动写入默认设置 + 自动探测默认内核
	ctx := context.Background()
	settings, _ := st.GetSettings(ctx)
	if err := st.SaveSettings(ctx, settings); err != nil {
		slog.Warn("seed settings", "err", err)
	}

	// 自动检测默认内核（若 Cores 为空，扫描 BASE_DIR 和 bin/ 下的 sing-box.exe）
	if len(settings.Cores) == 0 {
		if v, p := detectDefaultCore(); v != "" {
			settings.Cores = []models.CoreConfig{{
				ID: models.NewID("cor"), Label: v, Version: v, Path: p, Default: true,
			}}
			settings.ActiveCoreID = settings.Cores[0].ID
			_ = st.SaveSettings(ctx, settings)
			slog.Info("default core auto-detected", "version", v, "path", p)
		}
	}

	// 2. 注入服务
	runner := core.NewRunner(config.ExePath(), config.BaseDir())
	gen := configgen.New(st)

	// Register all core implementations
	coreRunner := core.NewRunner(config.ExePath(), config.BaseDir()) // shared runner for CoreManager
	coreMgr := core.NewManager(coreRunner)
	coreMgr.Register(singbox.New(gen))
	coreMgr.Register(xray.New())
	coreMgr.Register(mihomo.New())
	coreMgr.Register(hysteria2.New())

	subs := subscription.New(st)
	sys := sysproxy.New()
	rsDown := rulesets.New()
	coreDl := coredl.New()
	coreCache, _ := coredl.NewCache(coreDl.BinDir())
	apiSrv := api.New(st, runner, gen, coreMgr, subs, sys, rsDown, coreDl, coreCache, func() {
		slog.Info("quit requested via API")
		os.Exit(0)
	})

	// 3. HTTP 路由：API + 前端 SPA
	mux := http.NewServeMux()
	mux.Handle("/api/", apiSrv.Router())
	mux.Handle("/", web.Handler())

	// 找空闲端口（preferred 被占用则 +1）
	addr := fmt.Sprintf("%s:%d", *host, *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ln, err = findFreePort(*host, *port+1)
		if err != nil {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}
	url := fmt.Sprintf("http://%s", ln.Addr().String())
	server := &http.Server{Handler: mux}

	// 4. 启动时自动刷新订阅（best-effort）
	if settings.AutoRefreshSubs {
		go func() {
			time.Sleep(500 * time.Millisecond)
			subs.AutoRefresh(context.Background())
		}()
	}
	// 启动时静默检查内核本地缓存：缺失的 stable 自动后台下载
	go func() {
		time.Sleep(5 * time.Second)
		releases, err := coreDl.ListReleases(context.Background(), false)
		if err != nil {
			slog.Warn("cache auto-check: list releases", "err", err)
			return
		}
		for _, rel := range releases {
			ver := strings.TrimPrefix(rel.TagName, "v")
			if coreCache.Has(ver) {
				continue
			}
			go func(v, tag string) {
				cctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				core, err := coreDl.Download(cctx, v, settings.CustomDownloadMirrors, nil)
				if err != nil {
					slog.Warn("cache auto-check download", "version", v, "err", err)
					return
				}
				_ = coreCache.Add(&coredl.CachedCore{Version: v, TagName: tag, Path: core.Path})
				slog.Info("cache auto-check: cached", "version", v)
			}(ver, rel.TagName)
		}
	}()
	// 启动时刷新过期的远程规则集
	go func() {
		time.Sleep(1 * time.Second)
		sets, _ := st.ListRuleSets(context.Background())
		results := rsDown.RefreshAll(context.Background(), sets)
		for _, r := range results {
			if !r.OK && r.Error != "" {
				slog.Warn("ruleset refresh", "id", r.ID, "err", r.Error)
			} else if r.OK {
				slog.Info("ruleset refreshed", "id", r.ID, "bytes", r.Bytes)
			}
		}
	}()

	// 5. 开浏览器
	if !*noBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = openBrowser(url)
		}()
	}

	// 6. 系统托盘
	if !*noTray {
		exePath, _ := os.Executable()
		go tray.Run(tray.Config{
			AppName: "BoxPanel",
			URL:     url,
			ExePath: exePath,
			OpenBrowser: func(u string) {
				_ = openBrowser(u)
			},
			GetAutostart: func() bool {
				return tray.IsAutostartEnabled("BoxPanel", exePath)
			},
			OnToggleAutostart: func() bool {
				cur := tray.IsAutostartEnabled("BoxPanel", exePath)
				if err := tray.SetAutostart("BoxPanel", exePath, !cur); err != nil {
					slog.Warn("toggle autostart", "err", err)
					return cur
				}
				return !cur
			},
			Quit: func() {
				slog.Info("tray: quit requested")
				if runner.IsRunning() {
					_ = runner.Stop()
				}
				os.Exit(0)
			},
		})
	}

	// 6. 信号处理
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		if runner.IsRunning() {
			_ = runner.Stop()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Printf("  boxpanel 已启动\n  %s\n  按 Ctrl+C 关闭\n", url)
	fmt.Println("============================================================")

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}

func findFreePort(host string, start int) (net.Listener, error) {
	for p := start; p < start+config.PortRange; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, p))
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port in %d-%d", start, start+config.PortRange)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// openBrowser opens the URL in the default browser (best-effort, cross-platform).
func openBrowser(url string) error {
	switch syscallOS() {
	case "windows":
		return execCommand("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return execCommand("open", url)
	default:
		return execCommand("xdg-open", url)
	}
}
