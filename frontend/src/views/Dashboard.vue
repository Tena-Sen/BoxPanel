<template>
  <div>
    <h2 class="page-title">{{ t('nav.dashboard') }}</h2>

    <!-- 顶部统计卡片 -->
    <div class="stat-row">
      <div class="stat-card" :class="{ active: runtime.running }">
        <div class="stat-icon">{{ runtime.running ? '●' : '○' }}</div>
        <div class="stat-body">
          <div class="stat-value">{{ runtime.running ? '运行中' : '已停止' }}</div>
          <div class="stat-label">
            <template v-if="runtime.running">PID {{ runtime.pid }} · {{ formatUptime(state?.uptime_seconds || 0) }}</template>
            <template v-else>点击下方启动</template>
          </div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">◉</div>
        <div class="stat-body">
          <div class="stat-value">{{ state?.server_count || 0 }}</div>
          <div class="stat-label">节点</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">▤</div>
        <div class="stat-body">
          <div class="stat-value">{{ state?.subscription_count || 0 }}</div>
          <div class="stat-label">订阅</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">↕</div>
        <div class="stat-body">
          <div class="stat-value">{{ runtime.stats.connections || 0 }}</div>
          <div class="stat-label">活跃连接</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">⬇</div>
        <div class="stat-body">
          <div class="stat-value">{{ formatBytes(runtime.stats.cumulative_down_total || 0) }}</div>
          <div class="stat-label">下载总量</div>
        </div>
      </div>
    </div>

    <!-- 中间行：控制 + 节点详情 -->
    <el-row :gutter="16" style="margin-top:16px;">
      <el-col :xs="24" :sm="12">
        <!-- 控制面板 -->
        <div class="card">
          <div class="card-title">内核控制</div>
          <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-top:12px;">
            <el-button type="primary" :disabled="runtime.running" @click="onStart">
              ▶ 启动
            </el-button>
            <el-button :disabled="!runtime.running" @click="onStop">
              ⏹ 停止
            </el-button>
            <el-button @click="onRestart">↻ 重启</el-button>
            <el-checkbox v-model="autoFallback" :disabled="runtime.running" style="margin-left:8px;">
              自动适配
            </el-checkbox>
          </div>
          <el-divider style="margin:12px 0;" />
          <!-- 内核信息 -->
          <div class="info-grid">
            <span class="info-label">当前内核</span>
            <span class="info-value">
              <el-tag v-if="activeCoreKind" size="small" :type="kindTagType(activeCoreKind)">{{ kindLabel(activeCoreKind) }}</el-tag>
              <span v-if="activeCoreVersion" class="muted" style="margin-left:4px;">v{{ activeCoreVersion }}</span>
              <span v-if="!activeCoreKind" class="muted">未配置</span>
            </span>
            <span class="info-label">代理模式</span>
            <span class="info-value">
              <el-tag size="small" :type="modeTagType">{{ modeLabel }}</el-tag>
            </span>
            <span class="info-label">混合端口</span>
            <span class="info-value">{{ settings.settings?.listen_port || 20808 }}</span>
            <span class="info-label">Clash API</span>
            <span class="info-value">
              <span v-if="runtime.clashReachable" style="color:var(--green);">可达</span>
              <span v-else class="muted">不可达</span>
            </span>
            <span v-if="state?.probe_method" class="info-label">探测方式</span>
            <span v-if="state?.probe_method" class="info-value">
              <el-tag size="small" :type="probeMethodTagType">{{ probeMethodLabel }}</el-tag>
            </span>
          </div>
          <el-divider style="margin:12px 0;" />
          <!-- 系统代理 -->
          <div class="info-grid">
            <span class="info-label">系统代理</span>
            <span class="info-value">
              <el-tag v-if="sysProxy?.enabled" type="success" size="small">{{ sysProxy.server }}</el-tag>
              <el-tag v-else-if="sysProxy?.supported" type="info" size="small">关闭</el-tag>
              <el-tag v-else type="warning" size="small">不支持</el-tag>
              <el-button v-if="sysProxy?.supported && !sysProxy?.enabled" link type="primary" size="small" @click="runtime.enableSysProxy()" style="margin-left:6px;">开启</el-button>
              <el-button v-if="sysProxy?.enabled" link type="danger" size="small" @click="runtime.disableSysProxy()" style="margin-left:6px;">关闭</el-button>
            </span>
          </div>
        </div>
      </el-col>

      <el-col :xs="24" :sm="12">
        <!-- 当前节点详情 -->
        <div class="card">
          <div class="card-title">当前节点</div>
          <template v-if="current">
            <div style="margin-top:12px;display:flex;align-items:center;gap:8px;">
              <el-tag size="small">{{ current.protocol }}</el-tag>
              <span style="font-weight:600;font-size:15px;">{{ current.name }}</span>
            </div>
            <div class="info-grid" style="margin-top:12px;">
              <span class="info-label">地址</span>
              <span class="info-value muted">{{ current.server }}:{{ current.server_port }}</span>
              <span class="info-label">延迟</span>
              <span class="info-value"><LatencyBadge :ms="current.last_latency_ms" /></span>
              <span class="info-label">带宽</span>
              <span class="info-value"><BandwidthBadge :mbps="current.last_bandwidth_mbps" /></span>
              <span class="info-label">传输</span>
              <span class="info-value muted">
                {{ current.transport_type && current.transport_type !== 'tcp' ? current.transport_type : 'TCP' }}
                <el-tag v-if="current.tls_enabled" size="small" type="success" style="margin-left:4px;">TLS</el-tag>
                <el-tag v-if="current.reality_enabled" size="small" type="warning" style="margin-left:4px;">Reality</el-tag>
              </span>
            </div>
            <div style="margin-top:8px;">
              <el-button size="small" @click="onTestCurrent">⚡ 测试延迟+带宽</el-button>
            </div>
          </template>
          <template v-else>
            <div class="muted" style="margin-top:12px;">
              未选择节点
              <el-link type="primary" @click="$router.push('/servers')" style="margin-left:8px;">导入节点 →</el-link>
            </div>
          </template>

          <!-- 分组状态 -->
          <template v-if="groups.groups.length > 0">
            <el-divider style="margin:12px 0;" />
            <div class="card-title" style="font-size:13px;">分组状态</div>
            <div
              v-for="g in groups.groups"
              :key="g.id"
              style="display:flex;align-items:center;gap:6px;padding:4px 0;font-size:13px;"
            >
              <el-tag size="small" :type="typeTag(g.type)">{{ g.name }}</el-tag>
              <span class="muted">›</span>
              <span v-if="runtime.running && activeMember(g)" style="color:var(--green);font-weight:500;">
                ✓ {{ activeMember(g) }}
              </span>
              <span v-else-if="runtime.running" class="muted" style="font-size:12px;">等待切换</span>
              <span v-else class="muted" style="font-size:12px;">核心未运行</span>
            </div>
          </template>
        </div>
      </el-col>
    </el-row>

    <!-- 流量图 -->
    <div class="card" style="margin-top:16px;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
        <span class="card-title">实时流量</span>
        <span class="muted" style="font-size:12px;">
          ↑ {{ formatBytes(runtime.stats.up_bps || 0) }}/s ·
          ↓ {{ formatBytes(runtime.stats.down_bps || 0) }}/s ·
          本次 ↑ {{ formatBytes(runtime.stats.up_total || 0) }} ·
          本次 ↓ {{ formatBytes(runtime.stats.down_total || 0) }} ·
          累计 ↑ {{ formatBytes(runtime.stats.cumulative_up_total || 0) }} ·
          累计 ↓ {{ formatBytes(runtime.stats.cumulative_down_total || 0) }}
        </span>
      </div>
      <TrafficChart
        :up-bps="runtime.stats.up_bps || 0"
        :down-bps="runtime.stats.down_bps || 0"
        :up-total="runtime.stats.up_total || 0"
        :down-total="runtime.stats.down_total || 0"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRuntimeStore } from '@/stores/runtime'
import { useServersStore } from '@/stores/servers'
import { useSettingsStore } from '@/stores/settings'
import { useGroupsStore } from '@/stores/groups'
import { api } from '@/api/client'
import TrafficChart from '@/components/TrafficChart.vue'
import LatencyBadge from '@/components/LatencyBadge.vue'
import BandwidthBadge from '@/components/BandwidthBadge.vue'

const { t } = useI18n()
const runtime = useRuntimeStore()
const servers = useServersStore()
const settings = useSettingsStore()
const groups = useGroupsStore()

const state = computed(() => runtime.state)
const sysProxy = computed(() => runtime.state?.sys_proxy)
const current = computed(() => servers.current || runtime.state?.current_server)
const autoFallback = ref(true)

// Active core info
const activeCoreKind = computed(() => {
  const st = settings.settings
  if (!st?.cores || !st.active_core_id) return null
  const c = st.cores.find(c => c.id === st.active_core_id)
  return c?.kind || null
})
const activeCoreVersion = computed(() => {
  const st = settings.settings
  if (!st?.cores || !st.active_core_id) return null
  const c = st.cores.find(c => c.id === st.active_core_id)
  return c?.version || null
})

const modeLabel = computed(() => {
  const m = runtime.state?.settings?.mode
  return ({ normal: '规则模式', ai: 'AI 模式', global: '全局模式' } as any)[m || 'normal'] || '规则模式'
})
const modeTagType = computed(() => {
  const m = runtime.state?.settings?.mode
  return ({ normal: '', ai: 'warning', global: 'danger' } as any)[m || 'normal'] || ''
})

function kindLabel(kind: string) {
  const m: Record<string, string> = { singbox: 'sing-box', xray: 'Xray', mihomo: 'mihomo', hysteria2: 'Hysteria2' }
  return m[kind] || kind || 'sing-box'
}
function kindTagType(kind: string) {
  const m: Record<string, string> = { singbox: '', xray: 'warning', mihomo: 'danger', hysteria2: 'info' }
  return (m[kind] || 'info') as any
}

// 探测方式显示
const probeMethodLabel = computed(() => {
  const m = state.value?.probe_method
  if (!m) return '-'
  return ({ socks5: 'SOCKS5 握手', clash_api: 'Clash API', tcp: 'TCP 端口' } as any)[m] || m
})
const probeMethodTagType = computed(() => {
  const m = state.value?.probe_method
  if (m === 'socks5') return 'success'
  if (m === 'clash_api') return ''
  if (m === 'tcp') return 'warning'
  return 'info'
})

function activeMember(g: any) {
  const tag = groups.nowOf(`grp-${g.id}`)
  if (!tag) return null
  const sid = tag.replace(/^srv-/, '')
  return servers.servers.find((s) => s.id === sid)?.name || tag
}

function typeTag(type: string) {
  return ({ selector: 'primary', url_test: 'success', fallback: 'warning', load_balance: 'info' } as any)[type] || ''
}

onMounted(() => {
  if (groups.groups.length === 0) groups.load()
})

async function onTestCurrent() {
  if (!current.value) return
  const id = current.value.id
  await servers.testOne(id)
  try {
    await servers.testBandwidthOne(id)
  } catch {
    // bandwidth test may fail if core not running
  }
  ElMessage.success('测试完成')
}

async function onStart() {
  if (!servers.selectedId && !runtime.state?.current_server) {
    ElMessage.warning(t('dashboard.noServer'))
    return
  }

  // preflight：检测兼容性
  try {
    const pre = await api.preflight()
    const level = pre.compatibility?.level

    // 显示 NodeValidator 校验警告
    const nv = pre.node_validation
    if (nv && !nv.valid && nv.errors && nv.errors.length > 0) {
      const errMsgs = nv.errors.map((e: any) => `• ${e.message}${e.action ? ' → ' + e.action : ''}`).join('\n')
      try {
        await ElMessageBox.confirm(
          `节点与当前内核不兼容：\n${errMsgs}`,
          '校验失败',
          { type: 'error', confirmButtonText: '仍要启动', cancelButtonText: '取消' },
        )
      } catch { return }
    } else if (nv && nv.warnings && nv.warnings.length > 0) {
      const warnMsgs = nv.warnings.map((w: any) => `• ${w.message}`).join('\n')
      ElMessage({ message: warnMsgs, type: 'warning', duration: 5000 })
    }

    if (level === 'warn' || level === 'bad') {
      const reasons = (pre.compatibility.reasons || []).map((r: any) => `• ${r.message}`).join('\n')
      const hasLocal = !!(pre.recommended_id && pre.recommended_id !== pre.current_core_id)
      const msg = `当前节点与内核 [${pre.current_core}] 兼容性: ${level}\n${reasons}` +
        (hasLocal ? `\n\n推荐本地内核: v${pre.recommended_version}` : `\n\n本地无兼容内核，需从 GitHub 下载`)

      try {
        if (hasLocal) {
          await ElMessageBox.confirm(msg, '兼容性提示', {
            type: 'warning',
            confirmButtonText: '切换并启动',
            cancelButtonText: '仍要启动',
          })
          await api.activateCore(pre.recommended_id)
          await runtime.refresh()
        } else {
          ElMessageBox.confirm(msg, '需要下载兼容内核', {
            type: 'warning',
            confirmButtonText: '立即下载并启动',
            cancelButtonText: '取消',
          }).then(async () => {
            const loading = ElMessage({ message: '正在下载兼容内核...', type: 'info', duration: 0 })
            try {
              const r = await api.autoMatchCore()
              if (r.action === 'downloaded') {
                ElMessage.success(`已下载 v${r.version}`)
              } else if (r.action === 'activate_existing') {
                ElMessage.success(`已激活 v${r.version}`)
              }
              await settings.load()
              await runtime.refresh()
              await doStart()
            } catch (e: any) {
              ElMessage.error('下载失败：' + (e?.message || String(e)))
            } finally {
              loading.close()
            }
          }).catch(() => {})
          return
        }
      } catch {
        return
      }
    }
  } catch {}

  await doStart()
}

async function doStart() {
  try {
    const r = await runtime.startCore({ auto_fallback: autoFallback.value })
    ElMessage.success('已启动 · ' + (r.server || ''))
    if (r.attempts && r.attempts.length > 1) {
      const ok = r.attempts.find((a: any) => a.ok)
      if (ok && ok.core_id) await runtime.refresh()
    }
  } catch (e: any) {
    ElMessage.error('启动失败：' + (e?.message || String(e)))
  }
}

async function onStop() {
  try {
    await runtime.stopCore()
    ElMessage.success('已停止')
  } catch (e: any) {
    ElMessage.error('停止失败：' + (e?.message || String(e)))
  }
}

async function onRestart() {
  try {
    await runtime.restartCore()
    ElMessage.success('已重启')
  } catch (e: any) {
    ElMessage.error('重启失败：' + (e?.message || String(e)))
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}K`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)}M`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)}G`
}

function formatUptime(seconds: number): string {
  if (seconds <= 0) return '-'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}
</script>

<style scoped>
.card-title {
  font-size: 15px;
  font-weight: 600;
}
.stat-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}
.stat-card {
  background: var(--bg-soft);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 14px 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  transition: border-color 0.2s;
}
.stat-card.active {
  border-color: var(--green);
}
.stat-icon {
  font-size: 20px;
  color: var(--text-mute);
  width: 28px;
  text-align: center;
  flex-shrink: 0;
}
.stat-card.active .stat-icon {
  color: var(--green);
}
.stat-body {
  min-width: 0;
}
.stat-value {
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
}
.stat-label {
  font-size: 12px;
  color: var(--text-mute);
  margin-top: 2px;
}
.info-grid {
  display: grid;
  grid-template-columns: 80px 1fr;
  gap: 6px 12px;
  font-size: 13px;
}
.info-label {
  color: var(--text-mute);
}
.info-value {
  word-break: break-all;
}
@media (max-width: 768px) {
  .stat-row {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
