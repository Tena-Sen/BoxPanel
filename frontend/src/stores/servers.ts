// Servers store
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { useRuntimeStore } from './runtime'
import type { Server, CompatResult } from '@/api/types'
import { ElMessage } from 'element-plus'

export const useServersStore = defineStore('servers', {
  state: () => ({
    servers: [] as Server[],
    selectedId: '',
    testingIds: new Set<string>(),
    testingBwIds: new Set<string>(),
    compat: [] as CompatResult[], // 服务对每个内核的兼容性
  }),

  getters: {
    current: (s) => s.servers.find((x) => x.id === s.selectedId) || null,
    // 取 server 对当前激活内核的兼容性
    compatFor: (s) => (serverId: string, coreVersion: string): CompatResult | undefined => {
      return s.compat.find((r) => r.server_id === serverId && r.core_version === coreVersion)
    },
  },

  actions: {
    async load() {
      this.servers = await api.listServers()
      const rt = useRuntimeStore()
      this.selectedId = rt.settings?.current_server_id || ''
      this.refreshCompat()
    },

    async refreshCompat() {
      try {
        const r = await api.compatServers()
        this.compat = r.results || []
      } catch {}
    },

    async add(srv: Partial<Server>) {
      const r = await api.createServer(srv)
      this.servers.push(r)
      return r
    },

    async update(srv: Server) {
      const r = await api.updateServer(srv)
      const i = this.servers.findIndex((s) => s.id === r.id)
      if (i >= 0) this.servers[i] = r
      return r
    },

    async remove(id: string) {
      await api.deleteServer(id)
      this.servers = this.servers.filter((s) => s.id !== id)
      if (this.selectedId === id) this.selectedId = ''
    },

    async select(id: string) {
      this.selectedId = id
      try {
        await api.selectServer(id)
      } catch (e) {
        console.warn('select failed', e)
      }
      const rt = useRuntimeStore()
      await rt.refresh()
    },

    async testOne(id: string) {
      this.testingIds.add(id)
      try {
        const r = await api.testLatency(id)
        const i = this.servers.findIndex((s) => s.id === id)
        if (i >= 0) this.servers[i].last_latency_ms = r.latency_ms ?? null
        return r.latency_ms
      } catch {
        return null
      } finally {
        this.testingIds.delete(id)
      }
    },

    async testAll() {
      const results = await api.batchLatency()
      this.servers = this.servers.map((s) => ({
        ...s,
        last_latency_ms: results[s.id] ?? s.last_latency_ms,
      }))
    },

    async testBandwidthOne(id: string) {
      this.testingBwIds.add(id)
      try {
        const r = await api.testBandwidth(id)
        const i = this.servers.findIndex((s) => s.id === id)
        if (i >= 0) this.servers[i].last_bandwidth_mbps = r.mbps ?? null
        return r.mbps
      } catch {
        return null
      } finally {
        this.testingBwIds.delete(id)
      }
    },

    async testBandwidthAll() {
      const results = await api.batchBandwidth()
      this.servers = this.servers.map((s) => {
        const r = results[s.id]
        if (r && typeof r === 'object' && 'mbps' in (r as any)) {
          return { ...s, last_bandwidth_mbps: (r as any).mbps ?? s.last_bandwidth_mbps }
        }
        return s
      })
    },

    async importText(text: string) {
      try {
        const r = await api.importText(text)
        ElMessage.success(`导入：新增 ${r.added}，更新 ${r.updated}`)
        await this.load()
      } catch (e: any) {
        ElMessage.error('导入失败：' + (e?.message || String(e)))
      }
    },
  },
})