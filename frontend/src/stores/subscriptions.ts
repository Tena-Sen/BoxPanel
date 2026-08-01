// Subscriptions store
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { useServersStore } from './servers'
import type { Subscription } from '@/api/types'
import { ElMessage } from 'element-plus'

export const useSubscriptionsStore = defineStore('subscriptions', {
  state: () => ({
    subs: [] as Subscription[],
  }),

  actions: {
    async load() {
      this.subs = await api.listSubs()
    },

    async add(s: Partial<Subscription>) {
      const r = await api.createSub(s)
      this.subs.push(r)
      return r
    },

    async update(s: Subscription) {
      const r = await api.updateSub(s)
      const i = this.subs.findIndex((x) => x.id === r.id)
      if (i >= 0) this.subs[i] = r
      return r
    },

    async remove(id: string) {
      await api.deleteSub(id)
      this.subs = this.subs.filter((s) => s.id !== id)
    },

    async refresh(id: string, replace = false) {
      try {
        const r = await api.refreshSub(id, replace)
        ElMessage.success(`订阅刷新：新增 ${r.added ?? 0}，更新 ${r.updated ?? 0}，移除 ${r.removed ?? 0}`)
        await this.load()
        await useServersStore().load()
      } catch (e: any) {
        ElMessage.error('刷新失败：' + (e?.message || String(e)))
      }
    },
  },
})