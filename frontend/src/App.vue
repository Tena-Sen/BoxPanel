<template>
  <div class="app-shell">
    <header class="app-header">
      <div class="app-brand">
        <div class="app-brand-logo">BP</div>
        <div>{{ t('app.title') }}</div>
      </div>
      <div class="app-status">
        <span class="muted">v{{ version }}</span>
        <div :class="['status-dot', runtime.running && 'running']"></div>
        <span>{{ runtime.running ? t('app.running') : t('app.stopped') }}</span>
        <el-button text size="small" @click="settings.toggleTheme()">
          {{ settings.settings?.theme === 'dark' ? '☾' : '☀' }}
        </el-button>
      </div>
    </header>

    <aside class="app-sidebar">
      <div class="nav-items">
        <router-link
          v-for="(r, i) in navItems"
          :key="i"
          :to="r.path"
          custom
          v-slot="{ navigate, isActive }"
        >
          <div :class="['nav-item', isActive && 'active']" @click="navigate">
            <span style="width:20px;text-align:center;">{{ r.icon }}</span>
            <span class="nav-label">{{ t(r.titleKey) }}</span>
          </div>
        </router-link>
      </div>
      <div class="nav-footer">
        <div class="nav-item nav-quit" @click="onQuit">
          <span style="width:20px;text-align:center;">⏻</span>
          <span class="nav-label">{{ t('nav.quit', '退出') }}</span>
        </div>
      </div>
    </aside>

    <main class="app-main">
      <router-view v-slot="{ Component }">
        <component :is="Component" />
      </router-view>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRuntimeStore } from '@/stores/runtime'
import { useSettingsStore } from '@/stores/settings'
import { useServersStore } from '@/stores/servers'
import { useSubscriptionsStore } from '@/stores/subscriptions'
import { api } from '@/api/client'
import { ElMessageBox } from 'element-plus'

const { t } = useI18n()
const runtime = useRuntimeStore()
const settings = useSettingsStore()
const servers = useServersStore()
const subs = useSubscriptionsStore()

const version = '1.0.0'

const navItems = [
  { path: '/',             icon: '🏠', titleKey: 'nav.dashboard' },
  { path: '/servers',      icon: '🌐', titleKey: 'nav.servers' },
  { path: '/groups',       icon: '📂', titleKey: 'nav.groups' },
  { path: '/routing',      icon: '🛣', titleKey: 'nav.routing' },
  { path: '/subscriptions',icon: '📡', titleKey: 'nav.subscriptions' },
  { path: '/logs',         icon: '📜', titleKey: 'nav.logs' },
  { path: '/settings',     icon: '⚙', titleKey: 'nav.settings' },
]

onMounted(async () => {
  try {
    await settings.load()
    await runtime.loadState()
    await servers.load()
    await subs.load()
    const { useGroupsStore } = await import('@/stores/groups')
    const groups = useGroupsStore()
    await groups.load()
    groups.startProxyPolling(3000)
    runtime.connectLog()
    runtime.startStatsPolling(2000)
  } catch (e: any) {
    console.error('init error', e)
  }
})

async function onQuit() {
  try {
    await ElMessageBox.confirm('确定要退出 BoxPanel 吗？内核将自动停止。', '退出确认', {
      confirmButtonText: '退出',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return // 取消
  }
  try {
    await api.quit()
  } catch {
    // 服务端已退出，fetch 会报错，忽略
  }
  // 服务端退出后页面会自动断连，用户可关闭浏览器标签
}

onBeforeUnmount(() => {
  runtime.disconnectLog()
  runtime.stopStatsPolling()
})
</script>
