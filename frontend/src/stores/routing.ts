// Routing rules + rule sets store
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import type { RoutingRule, RuleSet, RuleSetStatus } from '@/api/types'
import { ElMessage } from 'element-plus'

// 默认 profile（与后端 defaultProfile 一致）
const DEFAULT_PROFILE = 'default'

// 缓存 TTL：规则列表 5 秒，状态 10 秒（状态涉及文件扫描，更慢）
const RULES_TTL = 5_000
const STATUS_TTL = 10_000

export const useRoutingStore = defineStore('routing', {
  state: () => ({
    rules: [] as RoutingRule[],
    ruleSets: [] as RuleSet[],
    // 缓存时间戳
    _rulesLoadedAt: 0,
    _statusCache: [] as RuleSetStatus[],
    _statusLoadedAt: 0,
    // 防止并发请求
    _rulesLoading: false,
    _statusLoading: false,
  }),

  actions: {
    async load(force = false) {
      const now = Date.now()
      if (!force && !this._rulesLoading && this._rulesLoadedAt && now - this._rulesLoadedAt < RULES_TTL) {
        return // 缓存未过期，跳过
      }
      if (this._rulesLoading) return // 防并发
      this._rulesLoading = true
      try {
        const [rules, sets] = await Promise.all([
          api.listRoutingRules(DEFAULT_PROFILE),
          api.listRuleSets(),
        ])
        this.rules = rules || []
        this.ruleSets = sets || []
        this._rulesLoadedAt = Date.now()
      } finally {
        this._rulesLoading = false
      }
    },

    async fetchStatus(force = false): Promise<RuleSetStatus[]> {
      const now = Date.now()
      if (!force && !this._statusLoading && this._statusLoadedAt && now - this._statusLoadedAt < STATUS_TTL) {
        return this._statusCache // 缓存未过期，直接返回
      }
      if (this._statusLoading) return this._statusCache // 防并发，返回已有数据
      this._statusLoading = true
      try {
        const sts = await api.ruleSetsStatus()
        this._statusCache = sts || []
        this._statusLoadedAt = Date.now()
        return this._statusCache
      } catch {
        return this._statusCache // 失败时返回旧缓存
      } finally {
        this._statusLoading = false
      }
    },

    async addRule(r: Partial<RoutingRule>) {
      const payload = { ...r, profile_id: r.profile_id || DEFAULT_PROFILE }
      const created = await api.createRoutingRule(payload)
      this.rules.push(created)
      await this._renumber()
      this._rulesLoadedAt = Date.now()
      return created
    },

    async updateRule(r: RoutingRule) {
      const updated = await api.updateRoutingRule(r)
      const i = this.rules.findIndex((x) => x.id === r.id)
      if (i >= 0) this.rules[i] = updated
      this._rulesLoadedAt = Date.now()
      return updated
    },

    async deleteRule(id: string) {
      await api.deleteRoutingRule(id)
      this.rules = this.rules.filter((r) => r.id !== id)
      await this._renumber()
      this._rulesLoadedAt = Date.now()
    },

    async moveRule(id: string, dir: 'up' | 'down') {
      const i = this.rules.findIndex((r) => r.id === id)
      if (i < 0) return
      const j = dir === 'up' ? i - 1 : i + 1
      if (j < 0 || j >= this.rules.length) return
      ;[this.rules[i], this.rules[j]] = [this.rules[j], this.rules[i]]
      const ids = this.rules.map((r) => r.id)
      await api.reorderRules(DEFAULT_PROFILE, ids)
      await this._renumber()
    },

    async _renumber() {
      this.rules.forEach((r, i) => (r.order = i))
    },

    async saveRuleSet(r: Partial<RuleSet>) {
      const saved = await api.saveRuleSet(r)
      const i = this.ruleSets.findIndex((x) => x.id === saved.id)
      if (i >= 0) this.ruleSets[i] = saved
      else this.ruleSets.push(saved)
      this._rulesLoadedAt = Date.now()
      this._statusLoadedAt = 0 // 规则集变更，状态缓存失效
      return saved
    },

    async add(r: Partial<RuleSet>) {
      return this.saveRuleSet(r)
    },

    async deleteRuleSet(id: string) {
      await api.deleteRuleSet(id)
      this.ruleSets = this.ruleSets.filter((r) => r.id !== id)
      this._rulesLoadedAt = Date.now()
      this._statusLoadedAt = 0
    },

    async toggleRuleSet(r: RuleSet) {
      try {
        await this.saveRuleSet({ ...r, enabled: !r.enabled })
      } catch (e: any) {
        ElMessage.error('切换失败：' + (e?.message || String(e)))
      }
    },

    async refreshOne(id: string) {
      try {
        const r = await api.refreshRuleSet(id)
        if (r.ok) ElMessage.success(`已更新：${(r.bytes / 1024).toFixed(1)} KB`)
        else ElMessage.error('失败：' + (r.error || '未知'))
        this._statusLoadedAt = 0 // 刷新后状态缓存失效
        await this.load(true)
      } catch (e: any) {
        ElMessage.error('刷新失败：' + (e?.message || String(e)))
      }
    },

    async refreshAll() {
      try {
        const r = await api.refreshAllRuleSets()
        const results = r.results || []
        const ok = results.filter((x) => x.ok).length
        ElMessage.success(`已刷新 ${ok}/${results.length} 个规则集`)
        this._statusLoadedAt = 0
        await this.load(true)
      } catch (e: any) {
        ElMessage.error('批量刷新失败：' + (e?.message || String(e)))
      }
    },

    async fetchBuiltin() {
      const r = await api.builtinRuleSets()
      return r.items || []
    },
  },
})