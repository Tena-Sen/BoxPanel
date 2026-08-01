// Groups store + Clash 实时状态
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { useRuntimeStore } from './runtime'
import type { Group, ClashProxiesResp } from '@/api/types'
import { ElMessage } from 'element-plus'

export const useGroupsStore = defineStore('groups', {
  state: () => ({
    groups: [] as Group[],
    proxies: {} as Record<string, any>,        // Clash API /proxies 完整快照
    _pollTimer: null as any,
  }),

  getters: {
    // 每个 group 当前的 "now"（活跃成员）
    nowOf: (s) => (groupTag: string): string | undefined => {
      const g = s.proxies[groupTag]
      return g?.now
    },
  },

  actions: {
    async load() {
      this.groups = await api.listGroups()
    },

    async add(g: Partial<Group>) {
      const r = await api.createGroup(g)
      this.groups.push(r)
      return r
    },

    async update(g: Group) {
      const r = await api.updateGroup(g)
      const i = this.groups.findIndex((x) => x.id === r.id)
      if (i >= 0) this.groups[i] = r
      return r
    },

    async remove(id: string) {
      await api.deleteGroup(id)
      this.groups = this.groups.filter((g) => g.id !== id)
    },

    // 拉取 Clash /proxies 实时状态
    async refreshProxies() {
      try {
        const r = await api.getProxies()
        this.proxies = r.proxies || {}
      } catch {
        // core 未运行或 clash 不可达 - 静默
      }
    },

    // 启动轮询（每 3 秒）
    startProxyPolling(intervalMs = 3000) {
      this.stopProxyPolling()
      const tick = () => { this.refreshProxies() }
      tick()
      this._pollTimer = setInterval(tick, intervalMs)
    },

    stopProxyPolling() {
      if (this._pollTimer) {
        clearInterval(this._pollTimer)
        this._pollTimer = null
      }
    },

    // 运行时切换（无需重启）
    async selectMember(groupTag: string, memberTag: string) {
      // 前置检查：内核必须运行才能通过 Clash API 切换
      const rt = useRuntimeStore()
      if (!rt.running) {
        ElMessage.warning('内核未运行，请先启动内核后再切换')
        return false
      }
      try {
        await api.selectProxy(groupTag, memberTag)
        await this.refreshProxies()
        return true
      } catch (e: any) {
        // 503 = 内核未运行，给出友好提示
        if (e?.status === 503 || String(e?.message || '').includes('内核未运行')) {
          ElMessage.warning('内核未运行，请先启动内核后再切换')
        } else if (e?.status === 0) {
          // 网络错误（后端不可达）
          ElMessage.error('后端服务不可用，请检查 BoxPanel 是否运行')
        } else {
          ElMessage.error('切换失败：' + (e?.message || '未知错误'))
        }
        return false
      }
    },
  },
})