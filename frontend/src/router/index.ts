// Router - history 模式（Go 兜底回退到 index.html）
import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { titleKey: 'nav.dashboard' } },
  { path: '/servers', name: 'servers', component: () => import('@/views/Servers.vue'), meta: { titleKey: 'nav.servers' } },
  { path: '/groups', name: 'groups', component: () => import('@/views/Groups.vue'), meta: { titleKey: 'nav.groups' } },
  { path: '/routing', name: 'routing', component: () => import('@/views/Routing.vue'), meta: { titleKey: 'nav.routing' } },
  { path: '/subscriptions', name: 'subscriptions', component: () => import('@/views/Subscriptions.vue'), meta: { titleKey: 'nav.subscriptions' } },
  { path: '/logs', name: 'logs', component: () => import('@/views/Logs.vue'), meta: { titleKey: 'nav.logs' } },
  { path: '/settings', name: 'settings', component: () => import('@/views/Settings.vue'), meta: { titleKey: 'nav.settings' } },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})