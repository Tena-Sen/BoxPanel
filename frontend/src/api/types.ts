// API types - 对齐后端 models
export interface Server {
  id: string
  protocol: string
  name: string
  server: string
  server_port: number
  added_at?: string
  last_latency_ms?: number | null
  last_bandwidth_mbps?: number | null
  raw_link?: string
  uuid?: string
  password?: string
  method?: string
  flow?: string
  transport_type?: string
  tls_enabled?: boolean
  reality_enabled?: boolean
  tls_server_name?: string
  utls_fingerprint?: string
}

export interface Group {
  id: string
  name: string
  type: 'selector' | 'url_test' | 'fallback' | 'load_balance'
  url?: string
  interval?: number
  tolerance?: number
  server_ids: string[]
  added_at?: string
}

// Clash API /proxies 单项
export interface ClashProxy {
  name: string
  type: string
  now?: string
  all?: string[]
  history?: Array<{ time?: string; delay?: number; mean_delay?: number }>
}

export interface ClashProxiesResp {
  proxies: Record<string, ClashProxy>
}

export interface CoreConfig {
  id: string
  kind: 'singbox' | 'xray' | 'mihomo' | 'hysteria2'
  label: string
  version: string
  path: string
  default: boolean
}

export interface CoresList {
  cores: CoreConfig[]
  active_core_id: string
}

export type CompatLevel = 'ok' | 'warn' | 'bad'

export interface CompatReason {
  code: string
  message: string
  action: string
}

export interface CompatResult {
  server_id: string
  core_version: string
  level: CompatLevel
  reasons: CompatReason[]
  min_version: string
}

export interface PreflightResponse {
  current_core: string
  current_core_id: string
  compatibility: CompatResult
  recommended_id: string
  recommended_version: string
}

export interface StartAttempt {
  core_id: string
  version: string
  path: string
  ok: boolean
  error?: string
  started?: boolean
  pid?: number
  duration?: string
}

export interface AvailableCore {
  version: string
  tag_name: string
  name: string
  prerelease: boolean
  downloaded: boolean
}

export interface DownloadProgress {
  stage: 'starting' | 'fetch_releases' | 'downloading' | 'verifying' | 'extracting' | 'done' | 'error'
  version: string
  bytes_done?: number
  bytes_total?: number
  pct?: number
  source?: string
  error?: string
}

export interface RoutingRule {
  id: string
  profile_id: string
  order: number
  type: string           // domain | domain_suffix | ip_cidr | process | geosite | geoip | ...
  values: string[]
  outbound: string       // proxy | direct | block | grp-xxx
  invert?: boolean
  added_at?: string
}

export interface RuleSet {
  id: string
  tag: string
  type: 'local' | 'remote'
  format: string         // binary | source
  path?: string
  url?: string
  update_interval?: number
  download_detour?: string
  enabled: boolean
}

export interface RuleSetStatus {
  id: string
  tag: string
  url: string
  cached: boolean
  cached_at?: string
  path?: string
  size: number
  sha256?: string
  last_error?: string
  next_check: string
}

export interface RuleSetRefreshResult {
  id: string
  ok: boolean
  path?: string
  bytes: number
  sha256?: string
  duration?: string
  error?: string
}

export interface Subscription {
  id: string
  name: string
  url: string
  user_agent?: string
  interval_hours?: number
  last_refresh?: string
  last_status?: string
  server_count?: number
}

export interface Settings {
  theme: 'dark' | 'light'
  language: 'zh-CN' | 'en'
  log_level: string
  mode: 'normal' | 'ai' | 'global'
  current_server_id: string
  listen_port: number
  latency_test_url: string
  subscription_user_agent: string
  auto_refresh_sub_on_start: boolean
  clash_api_port: number
  clash_api_secret: string
  cores?: CoreConfig[]
  active_core_id?: string
}

export interface SysProxyState {
  supported: boolean
  enabled: boolean
  server: string
  bypass: string
}

export interface AppState {
  running: boolean
  pid: number
  uptime_seconds: number
  version: string
  base_dir: string
  exe_path: string
  exe_exists?: boolean
  settings: Settings
  current_server: Server | null
  server_count: number
  subscription_count: number
  sys_proxy: SysProxyState
  clash_reachable: boolean
}

export interface Stats {
  running: boolean
  up_total?: number
  down_total?: number
  up_bps?: number
  down_bps?: number
  connections?: number
  memory?: number
  cumulative_up_total?: number
  cumulative_down_total?: number
}

export interface CoreKindInfo {
  kind: string
  name: string
  protocols: string[]
  has_clash_api: boolean
}

export interface BandwidthResult {
  id: string
  mbps: number | null
  bytes_read?: number
  duration?: string
  error?: string
}

export interface ImportResult {
  added: number
  updated: number
  total?: number
  imported?: number
}