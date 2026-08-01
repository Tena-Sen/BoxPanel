// Settings store - persisted to localStorage for theme/language
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { useRuntimeStore } from './runtime'
import type { Settings } from '@/api/types'
import { ElMessage } from 'element-plus'

export const useSettingsStore = defineStore('settings', {
  state: () => ({
    settings: null as Settings | null,
  }),

  actions: {
    async load() {
      this.settings = await api.getSettings()
      this.applyTheme()
      return this.settings
    },

    async save(patch: Partial<Settings>) {
      try {
        const r = await api.saveSettings(patch)
        this.settings = r
        this.applyTheme()
        const rt = useRuntimeStore()
        await rt.refresh()
      } catch (e: any) {
        ElMessage.error('保存失败：' + (e?.message || String(e)))
      }
    },

    applyTheme() {
      const theme = this.settings?.theme || 'dark'
      document.documentElement.classList.toggle('light', theme === 'light')
    },

    toggleTheme() {
      this.save({ theme: this.settings?.theme === 'dark' ? 'light' : 'dark' } as any)
    },
  },
})