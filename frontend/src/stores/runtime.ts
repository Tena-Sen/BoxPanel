// Runtime store: core running state, logs SSE, stats polling
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import type { AppState, Stats } from '@/api/types'

const MAX_LOG = 2000

export const useRuntimeStore = defineStore('runtime', {
  state: () => ({
    state: null as AppState | null,
    stats: { running: false, up_total: 0, down_total: 0, up_bps: 0, down_bps: 0 } as Stats,
    logs: [] as Array<{ line: string; tag: 'info' | 'warn' | 'err' | 'dim' }>,
    _sse: null as EventSource | null,
    _pollStats: null as any,
  }),

  getters: {
    running: (s) => s.state?.running ?? false,
    pid: (s) => s.state?.pid ?? 0,
    clashReachable: (s) => s.state?.clash_reachable ?? false,
    sysProxy: (s) => s.state?.sys_proxy,
    settings: (s) => s.state?.settings,
  },

  actions: {
    async loadState() {
      this.state = await api.getState()
      return this.state
    },

    async refresh() {
      this.state = await api.getState()
    },

    async startCore(opts?: { auto_fallback?: boolean }) {
      const r = await api.startCore(opts)
      await this.refresh()
      return r
    },

    async stopCore() {
      const r = await api.stopCore()
      await this.refresh()
      return r
    },

    async restartCore() {
      await api.restartCore()
      await this.refresh()
    },

    async enableSysProxy() {
      const s = this.state?.settings
      const server = `127.0.0.1:${s?.listen_port || 20808}`
      const bypass = '<local>;127.*;10.*;172.16.*;192.168.*'
      await api.enableSysProxy(server, bypass)
      await this.refresh()
    },

    async disableSysProxy() {
      await api.disableSysProxy()
      await this.refresh()
    },

    // ---- Logs (SSE) ----
    connectLog() {
      if (this._sse) return
      const es = new EventSource('/api/logs')
      this._sse = es
      es.addEventListener('log', (e) => this._appendLog((e as MessageEvent).data))
      es.addEventListener('clashlog', (e) => this._appendLog((e as MessageEvent).data))
      es.addEventListener('exit', (e) => {
        this._appendLog(`■ sing-box 退出 code=${(e as MessageEvent).data}`)
      })
      es.addEventListener('error', () => {
        // SSE auto-reconnects; just ignore transient errors
      })
    },

    disconnectLog() {
      this._sse?.close()
      this._sse = null
    },

    _appendLog(line: string) {
      const tag = this._classify(line)
      this.logs.push({ line, tag })
      if (this.logs.length > MAX_LOG) {
        this.logs.splice(0, this.logs.length - MAX_LOG)
      }
    },

    clearLog() {
      this.logs.splice(0)
    },

    _classify(line: string): 'info' | 'warn' | 'err' | 'dim' {
      const u = line.toUpperCase()
      if (u.includes('FATAL') || u.includes('ERROR')) return 'err'
      if (u.includes('WARN')) return 'warn'
      if (u.includes('DEBUG') || u.includes('TRACE')) return 'dim'
      return 'info'
    },

    // ---- Stats polling ----
    startStatsPolling(intervalMs = 2000) {
      if (this._pollStats) return
      const tick = async () => {
        try {
          this.stats = await api.getStats()
        } catch {}
      }
      tick()
      this._pollStats = setInterval(tick, intervalMs)
    },

    stopStatsPolling() {
      if (this._pollStats) {
        clearInterval(this._pollStats)
        this._pollStats = null
      }
    },
  },
})