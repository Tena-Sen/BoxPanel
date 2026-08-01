// Package models defines the domain types shared across the application.
//
// Server 等结构覆盖所有受支持协议的字段；仓储层将完整对象序列化为 JSON
// 存入 SQLite 的 data 列（文档模式），新增协议字段无需改 schema。
package models

import "time"

// Protocol identifiers.
const (
	ProtoVless        = "vless"
	ProtoVMess        = "vmess"
	ProtoTrojan       = "trojan"
	ProtoShadowsocks  = "shadowsocks"
	ProtoHysteria2    = "hysteria2"
	ProtoTUIC         = "tuic"
	ProtoAnyTLS       = "anytls"
	ProtoHTTP         = "http"
	ProtoSOCKS        = "socks"
)

// Group types (Clash-compatible).
const (
	GroupSelector    = "selector"
	GroupURLTest     = "url_test"
	GroupFallback    = "fallback"
	GroupLoadBalance = "load_balance"
)

// RoutingRule types.
const (
	RuleDomain       = "domain"
	RuleDomainSuffix = "domain_suffix"
	RuleDomainKeyword= "domain_keyword"
	RuleDomainRegex  = "domain_regex"
	RuleIPCIDR       = "ip_cidr"
	RuleGeoIP        = "geoip"
	RuleGeoSite      = "geosite"
	RuleProcess      = "process"
	RuleProtocol     = "protocol"
	RulePort         = "port"
)

// Outbound targets for routing.
const (
	OutProxy  = "proxy"
	OutDirect = "direct"
	OutBlock  = "block"
)

// Server is a single proxy node. Fields are flattened across protocols;
// protocol-specific ones are optional and only populated when relevant.
type Server struct {
	ID          string    `json:"id"`
	Protocol    string    `json:"protocol"`
	Name        string    `json:"name"`
	Server      string    `json:"server"`
	ServerPort  int       `json:"server_port"`
	AddedAt     time.Time `json:"added_at"`
	LastLatency    *int      `json:"last_latency_ms,omitempty"`
	LastBandwidth  *float64  `json:"last_bandwidth_mbps,omitempty"`
	RawLink        string    `json:"raw_link,omitempty"`

	// Identity
	UUID    string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	AlterID int    `json:"alter_id,omitempty"`
	Method  string `json:"method,omitempty"` // shadowsocks cipher
	Flow    string `json:"flow,omitempty"`  // vless xtls-rprx-vision

	// Transport
	TransportType         string            `json:"transport_type,omitempty"`
	TransportPath         string            `json:"transport_path,omitempty"`
	TransportHost         string            `json:"transport_host,omitempty"`
	TransportMode         string            `json:"transport_mode,omitempty"`
	TransportXPaddingBytes string           `json:"transport_x_padding_bytes,omitempty"`
	TransportHeaders      map[string]string `json:"transport_headers,omitempty"`

	// TLS
	TLSEnabled      bool     `json:"tls_enabled,omitempty"`
	TLSServerName   string   `json:"tls_server_name,omitempty"`
	TLSALPN         []string `json:"tls_alpn,omitempty"`
	TLSFingerprint  string   `json:"utls_fingerprint,omitempty"` // uTLS fingerprint
	TLSInsecure     bool     `json:"tls_insecure,omitempty"`
	RealityEnabled  bool     `json:"reality_enabled,omitempty"`
	RealityPublicKey string  `json:"reality_public_key,omitempty"`
	RealityShortID  string   `json:"reality_short_id,omitempty"`
	RealitySpiderX  string   `json:"reality_spider_x,omitempty"`

	// Hysteria2
	Hy2UpMbps       int    `json:"hy2_up_mbps,omitempty"`
	Hy2DownMbps     int    `json:"hy2_down_mbps,omitempty"`
	Hy2Obfs         string `json:"hy2_obfs,omitempty"`
	Hy2ObfsPassword string `json:"hy2_obfs_password,omitempty"`

	// TUIC
	TUICUUID            string `json:"tuic_uuid,omitempty"`
	TUICPassword        string `json:"tuic_password,omitempty"`
	TUICCongestionControl string `json:"tuic_congestion_control,omitempty"`
	TUICUDPRelayMode    string `json:"tuic_udp_relay_mode,omitempty"`

	// SOCKS/HTTP
	Username string `json:"username,omitempty"`
}

// Group is a proxy group (selector / url_test / fallback / load_balance).
type Group struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"` // selector | url_test | fallback | load_balance
	URL        string    `json:"url,omitempty"`
	Interval   int       `json:"interval,omitempty"` // seconds
	Tolerance  int       `json:"tolerance,omitempty"`
	ServerIDs  []string  `json:"server_ids"`
	AddedAt    time.Time `json:"added_at"`
}

// Subscription is a remote subscription source.
type Subscription struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	UserAgent     string    `json:"user_agent,omitempty"`
	IntervalHours int       `json:"interval_hours"`
	LastRefresh   time.Time `json:"last_refresh,omitempty"`
	LastStatus    string    `json:"last_status,omitempty"`
	LastAdded     int       `json:"last_added,omitempty"`
	LastUpdated   int       `json:"last_updated,omitempty"`
	LastRemoved   int       `json:"last_removed,omitempty"`
	ServerCount   int       `json:"server_count,omitempty"`
	AddedAt       time.Time `json:"added_at"`
}

// RoutingRule is a single ordered routing rule.
type RoutingRule struct {
	ID        string    `json:"id"`
	ProfileID string    `json:"profile_id"`
	Order     int       `json:"order"`
	Type      string    `json:"type"`    // see Rule* constants
	Values    []string  `json:"values"`  // matched values
	Outbound  string    `json:"outbound"`// proxy | direct | block | <group tag>
	Invert    bool      `json:"invert,omitempty"`
	AddedAt   time.Time `json:"added_at"`
}

// RuleSet is a sing-box rule-set (local .srs or remote).
type RuleSet struct {
	ID            string    `json:"id"`
	Tag           string    `json:"tag"`
	Type          string    `json:"type"` // local | remote
	Format        string    `json:"format"` // binary | source
	Path          string    `json:"path,omitempty"`
	URL           string    `json:"url,omitempty"`
	DownloadDetour string   `json:"download_detour,omitempty"`
	UpdateInterval int      `json:"update_interval,omitempty"` // days
	Enabled       bool      `json:"enabled"`
	AddedAt       time.Time `json:"added_at"`
}

// Profile is a named configuration (replaces the 3 fixed presets).
type Profile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Mode          string `json:"mode"` // normal | ai | global
	Listen        string `json:"listen"`
	ListenPort    int    `json:"listen_port"`
	Sniff         bool   `json:"sniff"`
	SniffOverride bool   `json:"sniff_override"`
	// TUN
	TunEnabled      bool   `json:"tun_enabled"`
	TunInterface    string `json:"tun_interface"`
	TunAddress      string `json:"tun_address"`
	TunStack        string `json:"tun_stack"`
	TunMTU          int    `json:"tun_mtu"`
	TunAutoRoute    bool   `json:"tun_auto_route"`
	TunStrictRoute  bool   `json:"tun_strict_route"`
	// DNS
	DirectDNS string `json:"direct_dns"`
	ProxyDNS  string `json:"proxy_dns"`
	// Route
	RouteFinal              string `json:"route_final"`
	DefaultDomainResolver   string `json:"default_domain_resolver"`
	AddedAt                 time.Time `json:"added_at"`
}

// Core kind identifiers — each kind maps to a distinct proxy engine.
const (
	CoreKindSingBox   = "singbox"
	CoreKindXray      = "xray"
	CoreKindMihomo    = "mihomo"
	CoreKindHysteria2 = "hysteria2"
)

// SupportedProtocolsByKind returns the protocols each core kind can handle.
func SupportedProtocolsByKind(kind string) []string {
	switch kind {
	case CoreKindSingBox:
		return []string{ProtoVless, ProtoVMess, ProtoTrojan, ProtoShadowsocks, ProtoHysteria2, ProtoTUIC, ProtoAnyTLS, ProtoHTTP, ProtoSOCKS}
	case CoreKindXray:
		return []string{ProtoVless, ProtoVMess, ProtoTrojan, ProtoShadowsocks, ProtoHTTP, ProtoSOCKS}
	case CoreKindMihomo:
		return []string{ProtoShadowsocks, ProtoVMess, ProtoTrojan, ProtoHysteria2, ProtoTUIC, ProtoHTTP, ProtoSOCKS}
	case CoreKindHysteria2:
		return []string{ProtoHysteria2}
	default:
		return nil
	}
}

// CoreConfig describes a single proxy core binary (one of multiple supported).
type CoreConfig struct {
	ID      string `json:"id"`       // cor_xxx
	Kind    string `json:"kind"`     // singbox | xray | mihomo | hysteria2
	Label   string `json:"label"`    // human-readable ("sing-box 1.14", "Xray 24.1")
	Version string `json:"version"`  // semver-ish: "1.14.0-alpha.1"
	Path    string `json:"path"`     // absolute exe path
	Default bool   `json:"default"`  // auto-detected on first launch
}

// SupportsProtocol reports whether this core can handle the given protocol.
func (c CoreConfig) SupportsProtocol(proto string) bool {
	for _, p := range SupportedProtocolsByKind(c.Kind) {
		if p == proto {
			return true
		}
	}
	return false
}

// Settings is the application settings (persisted as KV).
type Settings struct {
	Theme             string `json:"theme"`              // dark | light
	Language          string `json:"language"`           // zh-CN | en
	LogLevel          string `json:"log_level"`
	Mode              string `json:"mode"`               // normal | ai | global
	CurrentServerID   string `json:"current_server_id"`
	CurrentProfileID  string `json:"current_profile_id"`
	ListenPort        int    `json:"listen_port"`
	LatencyTestURL    string `json:"latency_test_url"`
	SubscriptionUA    string `json:"subscription_user_agent"`
	AutoRefreshSubs   bool   `json:"auto_refresh_sub_on_start"`
	AutoStartCore     bool   `json:"auto_start_core"`
	ClashAPIPort      int    `json:"clash_api_port"`
	ClashAPISecret    string `json:"clash_api_secret"`

	// 多内核支持
	Cores           []CoreConfig `json:"cores"`            // 已注册内核列表
	ActiveCoreID    string       `json:"active_core_id"`   // 当前激活的内核 ID
	CustomDownloadMirrors []string `json:"custom_download_mirrors"` // 用户自定义下载源前缀（如 "https://my-mirror.com/"）

	// 累计流量（跨会话持久化，内核停止时累加当前会话值）
	TrafficUpTotal   int64 `json:"traffic_up_total"`
	TrafficDownTotal int64 `json:"traffic_down_total"`
}

// DefaultSettings returns the initial settings.
func DefaultSettings() Settings {
	return Settings{
		Theme:           "dark",
		Language:        "zh-CN",
		LogLevel:        "warn", // 默认 warn：日常使用安静；调试节点问题时改为 info
		Mode:            "normal",
		ListenPort:      20808,
		LatencyTestURL:  "http://www.gstatic.com/generate_204",
		SubscriptionUA:  "clash-meta",
		AutoRefreshSubs: true,
		ClashAPIPort:    9090,
	}
}
