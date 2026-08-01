// Package api implements the HTTP/WS API layer (chi router).
//
// 依赖通过 APIServer 注入（store/runner/clashapi/configgen/subs/latency/sysproxy），
// 路由按资源分组，每加一类资源新增一个 handler 文件 + Register 调用。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/api/middleware"
	"boxpanel/internal/bandwidth"
	"boxpanel/internal/config"
	"boxpanel/internal/core"
	"boxpanel/internal/core/clashapi"
	"boxpanel/internal/core/configgen"
	"boxpanel/internal/coredl"
	"boxpanel/internal/latency"
	"boxpanel/internal/models"
	"boxpanel/internal/rulesets"
	"boxpanel/internal/store"
	"boxpanel/internal/subscription"
	"boxpanel/internal/sysproxy"
)

// APIServer holds all service dependencies and the HTTP mux.
type APIServer struct {
	store     store.Store
	runner    *core.Runner
	gen       *configgen.Builder
	subs      *subscription.Manager
	sys       sysproxy.Controller
	rs        *rulesets.Downloader
	coredl    *coredl.Downloader
	coreCache *coredl.Cache
	coreMgr   *core.Manager
	multiDl   *coredl.MultiCoreDownloader

	mu            sync.Mutex
	clash         *clashapi.Client
	lat           *latency.Tester
	bw            *bandwidth.Tester
	logBroadcaster *LogBroadcaster
	settings      models.Settings
	onQuit        func() // called by POST /api/quit

	// traffic cumulative tracking (in-memory)
	trafficMu         sync.Mutex
	sessionUpTotal    int64 // last seen upload_total from Clash API this session
	sessionDownTotal  int64 // last seen download_total from Clash API this session
	persistedUpTotal  int64 // cumulative upload from DB (before this session)
	persistedDownTotal int64 // cumulative download from DB (before this session)

	// probe method tracking
	lastProbeMethod string // "socks5" | "clash_api" | "tcp"
}

// New creates an APIServer wired to all services.
func New(s store.Store, runner *core.Runner, gen *configgen.Builder,
	subs *subscription.Manager, sys sysproxy.Controller, rs *rulesets.Downloader,
	coredl_ *coredl.Downloader, coreCache_ *coredl.Cache, onQuit func()) *APIServer {
	srv := &APIServer{
		store:          s,
		runner:         runner,
		gen:            gen,
		subs:           subs,
		sys:            sys,
		rs:             rs,
		coredl:         coredl_,
		coreCache:      coreCache_,
		logBroadcaster: NewLogBroadcaster(),
		onQuit:         onQuit,
	}
	// load settings
	ctx := context.Background()
	st, _ := s.GetSettings(ctx)
	srv.settings = st
	// load persisted cumulative traffic
	srv.persistedUpTotal = st.TrafficUpTotal
	srv.persistedDownTotal = st.TrafficDownTotal
	// wire clash client (initially nil; refreshed on core start)
	srv.refreshClashClient()
	// runner log -> broadcaster
	runner.SetLogHandler(func(line string) {
		srv.logBroadcaster.Broadcast(line)
	})
	runner.SetExitHandler(func(code int) {
		srv.logBroadcaster.BroadcastExit(code)
		srv.refreshClashClient()
	})
	srv.lat = latency.New(srv.clash, st.LatencyTestURL)
	srv.bw = bandwidth.New(srv.clash, srv.proxyListenAddr(), nil)
	srv.coreMgr = core.NewManager(runner)
	srv.multiDl = coredl.NewMultiCoreDownloader()
	return srv
}

func (s *APIServer) refreshClashClient() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings.ClashAPIPort == 0 {
		s.settings.ClashAPIPort = config.ClashAPIPort
	}
	s.clash = clashapi.New(config.ClashAPIHost, s.settings.ClashAPIPort, s.settings.ClashAPISecret)
	s.lat = latency.New(s.clash, s.settings.LatencyTestURL)
	s.bw = bandwidth.New(s.clash, s.proxyListenAddr(), nil)
}

// Router builds the chi router with all routes registered.
func (s *APIServer) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	// state
	r.Get("/api/state", s.handleState)
	r.Get("/api/health", s.handleHealth)

	// servers
	r.Get("/api/servers", s.handleListServers)
	r.Post("/api/servers", s.handleCreateServer)
	r.Get("/api/servers/{id}", s.handleGetServer)
	r.Put("/api/servers/{id}", s.handleUpdateServer)
	r.Delete("/api/servers/{id}", s.handleDeleteServer)
	r.Post("/api/servers/{id}/latency", s.handleServerLatency)
	r.Post("/api/servers/{id}/select", s.handleSelectServer)
	r.Post("/api/servers/batch-latency", s.handleBatchLatency)
	r.Post("/api/servers/import", s.handleImport)
	r.Post("/api/servers/export", s.handleExport)

	// groups
	r.Get("/api/groups", s.handleListGroups)
	r.Post("/api/groups", s.handleCreateGroup)
	r.Put("/api/groups/{id}", s.handleUpdateGroup)
	r.Delete("/api/groups/{id}", s.handleDeleteGroup)

	// subscriptions
	r.Get("/api/subscriptions", s.handleListSubs)
	r.Post("/api/subscriptions", s.handleCreateSub)
	r.Put("/api/subscriptions/{id}", s.handleUpdateSub)
	r.Delete("/api/subscriptions/{id}", s.handleDeleteSub)
	r.Post("/api/subscriptions/{id}/refresh", s.handleRefreshSub)

	// profiles
	r.Get("/api/profiles", s.handleListProfiles)
	r.Post("/api/profiles", s.handleCreateProfile)
	r.Put("/api/profiles/{id}", s.handleUpdateProfile)
	r.Delete("/api/profiles/{id}", s.handleDeleteProfile)

	// routing
	r.Get("/api/routing/rules", s.handleListRoutingRules)
	r.Post("/api/routing/rules", s.handleCreateRoutingRule)
	r.Put("/api/routing/rules/{id}", s.handleUpdateRoutingRule)
	r.Delete("/api/routing/rules/{id}", s.handleDeleteRoutingRule)
	r.Post("/api/routing/reorder", s.handleReorderRules)
	r.Get("/api/rule-sets", s.handleListRuleSets)
	r.Post("/api/rule-sets", s.handleSaveRuleSet)
	r.Delete("/api/rule-sets/{id}", s.handleDeleteRuleSet)
	r.Get("/api/rule-sets/builtin", s.handleBuiltinRuleSets)
	r.Get("/api/rule-sets/status", s.handleRuleSetsStatus)
	r.Post("/api/rule-sets/refresh-all", s.handleRefreshAllRuleSets)
	r.Post("/api/rule-sets/{id}/refresh", s.handleRefreshRuleSet)

	// core control
	r.Post("/api/core/start", s.handleCoreStart)
	r.Post("/api/core/stop", s.handleCoreStop)
	r.Post("/api/core/restart", s.handleCoreRestart)
	r.Get("/api/core/proxies", s.handleClashProxies)
	r.Put("/api/core/proxies/{name}", s.handleClashSelect)
	r.Get("/api/core/connections", s.handleClashConnections)

	// system proxy
	r.Get("/api/sysproxy", s.handleSysProxyGet)
	r.Post("/api/sysproxy/enable", s.handleSysProxyEnable)
	r.Post("/api/sysproxy/disable", s.handleSysProxyDisable)

	// stats / logs / traffic (streaming)
	r.Get("/api/stats", s.handleStats)
	r.Get("/api/logs", s.handleLogsSSE)
	r.Get("/api/traffic", s.handleTraffic)

	// settings
	r.Get("/api/settings", s.handleGetSettings)
	r.Put("/api/settings", s.handleSetSettings)

	// cores (multi-version sing-box)
	r.Get("/api/cores", s.handleListCores)
	r.Post("/api/cores", s.handleAddCore)
	r.Delete("/api/cores/{id}", s.handleDeleteCore)
	r.Post("/api/cores/{id}/test", s.handleTestCore)
	r.Post("/api/cores/{id}/activate", s.handleActivateCore)

	// 兼容性（动态内核匹配）
	r.Get("/api/compat/servers", s.handleCompatServers)
	r.Get("/api/core/preflight", s.handleCorePreflight)

	// 内核下载（实时匹配）
	r.Get("/api/cores/available", s.handleListAvailableCores)
	r.Post("/api/cores/download", s.handleDownloadCore)
	r.Get("/api/cores/download/status", s.handleDownloadStatus)
	r.Post("/api/cores/auto-match", s.handleAutoMatchCore)
	// 本地缓存池
	r.Get("/api/cores/cache", s.handleCacheList)
	r.Post("/api/cores/check-update", s.handleCheckUpdate)
	r.Delete("/api/cores/cache/{version}", s.handleCacheRemove)

	// 多内核类型（v2 — cross-engine）
	r.Get("/api/cores/kinds", s.handleListCoreKinds)
	r.Post("/api/cores/{id}/switch-kind", s.handleSwitchCoreKind)
	r.Get("/api/cores/kinds/{kind}/available", s.handleListKindAvailable)

	// bandwidth test
	r.Post("/api/servers/{id}/bandwidth", s.handleServerBandwidth)
	r.Post("/api/servers/batch-bandwidth", s.handleBatchBandwidth)

	// import file
	r.Post("/api/import/file", s.handleImportFile)

	// quit BoxPanel
	r.Post("/api/quit", s.handleQuit)

	return r
}

// ----- helpers -----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// proxyListenAddr returns the local proxy address (host:port) for bandwidth tests.
func (s *APIServer) proxyListenAddr() string {
	port := s.settings.ListenPort
	if port == 0 {
		port = config.MixedInboundPort
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}
