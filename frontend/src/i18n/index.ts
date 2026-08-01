// i18n 配置
import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN'
import en from './en'

export const i18n = createI18n({
  legacy: false,
  locale: localStorage.getItem('boxpanel.lang') || 'zh-CN',
  fallbackLocale: 'en',
  messages: { 'zh-CN': zhCN, en },
})