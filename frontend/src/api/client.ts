// Typed fetch wrapper for BoxPanel API.
const BASE = ''

export class ApiError extends Error {
  status: number
  data: any
  constructor(status: number, message: string, data?: any) {
    super(message)
    this.status = status
    this.data = data
  }
}

// 更宽松的 options：body 可以是任意值，request 函数自己 JSON.stringify
type ApiOptions = Omit<RequestInit, 'body'> & { body?: any; signal?: AbortSignal }

async function request<T = any>(path: string, options: ApiOptions = {}): Promise<T> {
  const { signal, ...rest } = options
  const opts: RequestInit = {
    headers: { 'Content-Type': 'application/json' },
    ...rest,
    signal,
  }
  if (opts.body && typeof opts.body !== 'string') {
    opts.body = JSON.stringify(opts.body)
  }
  let r: Response
  try {
    r = await fetch(BASE + path, opts)
  } catch (e: any) {
    throw new ApiError(0, `服务不可用，请检查 BoxPanel 是否运行`)
  }
  const ct = r.headers.get('Content-Type') || ''
  let data: any = null
  if (ct.includes('application/json')) {
    try { data = await r.json() } catch { data = null }
  } else {
    try { data = await r.text() } catch { data = null }
  }
  if (!r.ok) {
    const msg = (data && (data.error || data.detail)) || `${r.status} ${r.statusText}`
    throw new ApiError(r.status, msg, data)
  }
  return data as T
}

import type { AppState, Server, Settings, Subscription, ImportResult, Stats, Group, ClashProxiesResp, RoutingRule, RuleSet, RuleSetStatus, RuleSetRefreshResult, CoreConfig, CoresList, CoreKindInfo, CoreKindsResponse, CompatResult, PreflightResponse, StartAttempt, AvailableCore, DownloadProgress, BandwidthResult } from './types'

export const api = {
  // state
  getState: () => request<AppState>('/api/state'),

  // servers
  listServers: () => request<Server[]>('/api/servers'),
  createServer: (s: Partial<Server>) => request<Server>('/api/servers', { method: 'POST', body: s as any }),
  updateServer: (s: Server) => request<Server>(`/api/servers/${s.id}`, { method: 'PUT', body: s as any }),
  deleteServer: (id: string) => request<{ deleted: string }>(`/api/servers/${id}`, { method: 'DELETE' }),
  selectServer: (id: string) => request<{ selected: string }>(`/api/servers/${id}/select`, { method: 'POST' }),
  testLatency: (id: string) => request<{ id: string; latency_ms: number | null; error?: string }>(`/api/servers/${id}/latency`, { method: 'POST' }),
  batchLatency: () => request<Record<string, number>>('/api/servers/batch-latency', { method: 'POST' }),
  testBandwidth: (id: string, timeoutSec = 10) =>
    request<BandwidthResult>(`/api/servers/${id}/bandwidth?timeout=${timeoutSec}`, { method: 'POST' }),
  batchBandwidth: (timeoutSec = 10) =>
    request<Record<string, BandwidthResult>>(`/api/servers/batch-bandwidth?timeout=${timeoutSec}`, { method: 'POST' }),
  importText: (text: string) => request<ImportResult>('/api/servers/import', { method: 'POST', body: { text } }),

  // groups
  listGroups: () => request<Group[]>('/api/groups'),
  createGroup: (g: Partial<Group>) => request<Group>('/api/groups', { method: 'POST', body: g as any }),
  updateGroup: (g: Group) => request<Group>(`/api/groups/${g.id}`, { method: 'PUT', body: g as any }),
  deleteGroup: (id: string) => request<{ deleted: string }>(`/api/groups/${id}`, { method: 'DELETE' }),

  // clash api
  getProxies: () => request<ClashProxiesResp>('/api/core/proxies'),
  selectProxy: (group: string, name: string) =>
    request<{ group: string; selected: string }>(`/api/core/proxies/${encodeURIComponent(group)}`, { method: 'PUT', body: { name } }),

  // routing rules
  listRoutingRules: (profileId?: string) =>
    request<RoutingRule[]>(`/api/routing/rules${profileId ? '?profile_id=' + encodeURIComponent(profileId) : ''}`),
  createRoutingRule: (r: Partial<RoutingRule>) => request<RoutingRule>('/api/routing/rules', { method: 'POST', body: r as any }),
  updateRoutingRule: (r: RoutingRule) => request<RoutingRule>(`/api/routing/rules/${r.id}`, { method: 'PUT', body: r as any }),
  deleteRoutingRule: (id: string) => request<{ deleted: string }>(`/api/routing/rules/${id}`, { method: 'DELETE' }),
  reorderRules: (profileId: string, ids: string[]) =>
    request<{ ok: boolean }>('/api/routing/reorder', { method: 'POST', body: { profile_id: profileId, ids } }),

  // rule sets
  listRuleSets: () => request<RuleSet[]>('/api/rule-sets'),
  builtinRuleSets: () => request<{ items: RuleSet[] }>('/api/rule-sets/builtin'),
  ruleSetsStatus: () => {
    // 8 秒超时，防止状态查询阻塞页面
    const ctrl = new AbortController()
    const timer = setTimeout(() => ctrl.abort(), 8_000)
    return request<RuleSetStatus[]>('/api/rule-sets/status', { signal: ctrl.signal })
      .finally(() => clearTimeout(timer))
  },
  saveRuleSet: (r: Partial<RuleSet>) => request<RuleSet>('/api/rule-sets', { method: 'POST', body: r as any }),
  deleteRuleSet: (id: string) => request<{ deleted: string }>(`/api/rule-sets/${id}`, { method: 'DELETE' }),
  refreshRuleSet: (id: string) => request<RuleSetRefreshResult>(`/api/rule-sets/${id}/refresh`, { method: 'POST' }),
  refreshAllRuleSets: () => request<{ results: RuleSetRefreshResult[] }>('/api/rule-sets/refresh-all', { method: 'POST' }),

  // subscriptions
  listSubs: () => request<Subscription[]>('/api/subscriptions'),
  createSub: (s: Partial<Subscription>) => request<Subscription>('/api/subscriptions', { method: 'POST', body: s as any }),
  updateSub: (s: Subscription) => request<Subscription>(`/api/subscriptions/${s.id}`, { method: 'PUT', body: s as any }),
  deleteSub: (id: string) => request<{ deleted: string }>(`/api/subscriptions/${id}`, { method: 'DELETE' }),
  refreshSub: (id: string, replace = false) => request<any>(`/api/subscriptions/${id}/refresh`, { method: 'POST', body: { replace } }),

  // core
  startCore: (opts?: { auto_fallback?: boolean }) =>
    request<{ started: boolean; pid: number; server: string; core: string; core_version: string; attempts?: StartAttempt[] }>('/api/core/start', { method: 'POST', body: opts || {} }),
  stopCore: () => request<{ stopping: boolean }>('/api/core/stop', { method: 'POST' }),
  restartCore: () => request<any>('/api/core/restart', { method: 'POST' }),
  preflight: () => request<PreflightResponse>('/api/core/preflight'),

  // 兼容性
  compatServers: () => request<{ results: CompatResult[] }>('/api/compat/servers'),

  // sysproxy
  getSysProxy: () => request<any>('/api/sysproxy'),
  enableSysProxy: (server: string, bypass: string) => request<any>('/api/sysproxy/enable', { method: 'POST', body: { server, bypass } }),
  disableSysProxy: () => request<any>('/api/sysproxy/disable', { method: 'POST' }),

  // stats
  getStats: () => request<Stats>('/api/stats'),

  // settings
  getSettings: () => request<Settings>('/api/settings'),
  saveSettings: (s: Partial<Settings>) => request<Settings>('/api/settings', { method: 'PUT', body: s as any }),

  // cores (multi-version sing-box)
  listCores: () => request<CoresList>('/api/cores'),
  addCore: (c: Partial<CoreConfig>) => request<CoreConfig>('/api/cores', { method: 'POST', body: c as any }),
  deleteCore: (id: string) => request<{ deleted: string }>(`/api/cores/${id}`, { method: 'DELETE' }),
  testCore: (id: string) => request<{ path: string; version: string }>(`/api/cores/${id}/test`, { method: 'POST' }),
  activateCore: (id: string) => request<{ active_core_id: string }>(`/api/cores/${id}/activate`, { method: 'POST' }),

  // 内核下载（实时匹配）
  listAvailableCores: (prerelease = false) =>
    request<{ items: AvailableCore[] }>(`/api/cores/available?prerelease=${prerelease ? 1 : 0}`),
  downloadCore: (version: string, activate = true, kind = 'singbox') =>
    request<{ status: string; version: string; kind: string; poll: string }>('/api/cores/download', {
      method: 'POST', body: { version, activate, kind },
    }),
  downloadStatus: (version: string) =>
    request<DownloadProgress | { stage: string }>(`/api/cores/download/status?version=${encodeURIComponent(version)}`),
  listKindAvailable: (kind: string, prerelease = false) =>
    request<{ kind: string; items: AvailableCore[] }>(`/api/cores/kinds/${kind}/available?prerelease=${prerelease ? 1 : 0}`),
  autoMatchCore: (server_id?: string) =>
    request<{ action: string; core?: CoreConfig; version?: string; path?: string }>('/api/cores/auto-match', {
      method: 'POST', body: { server_id: server_id || '' },
    }),

  // multi-engine core kinds
  listCoreKinds: () => request<CoreKindsResponse>('/api/cores/kinds'),
  switchCoreKind: (id: string, kind: string) =>
    request<{ id: string; kind: string }>(`/api/cores/${id}/switch-kind`, { method: 'POST', body: { kind } }),

  // quit
  quit: () => request<{ quitting: boolean }>('/api/quit', { method: 'POST' }),
}