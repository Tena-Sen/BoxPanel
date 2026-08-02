<template>
  <div>
    <h2 class="page-title">{{ t('nav.dashboard') }}</h2>

    <!-- 顶部统计卡片 -->
    <div class="stat-row">
      <div class="stat-card" :class="{ active: runtime.running }">
        <div class="stat-icon" :style="{ background: runtime.running ? 'var(--green-soft)' : 'var(--bg-mute)', color: runtime.running ? 'var(--green)' : 'var(--text-mute)' }">
          {{ runtime.running ? '●' : '○' }}
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ runtime.running ? '运行中' : '已停止' }}</div>
          <div class="stat-label">
            <template v-if="runtime.running">PID {{ runtime.pid }} · {{ formatUptime(state?.uptime_seconds || 0) }}</template>
            <template v-else>点击下方启动</template>
          </div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:var(--accent-soft);color:var(--accent);">◉</div>
        <div class="stat-body">
          <div class="stat-value">{{ state?.server_count || 0 }}</div>
          <div class="stat-label">节点</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:var(--purple-soft);color:var(--purple);">▤</div>
        <div class="stat-body">
          <div class="stat-value">{{ state?.subscription_count || 0 }}</div>
          <div class="stat-label">订阅</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:var(--cyan-soft);color:var(--cyan);">↕</div>
        <div class="stat-body">
          <div class="stat-value">{{ runtime.stats.connections || 0 }}</div>
          <div class="stat-label">活跃连接</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:var(--yellow-soft);color:var(--yellow);">⬇</div>
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
          <div class="control-row">
            <el-button type="primary" :disabled="runtime.running" @click="onStart">
              ▶ 启动
            </el-button>
            <el-button :disabled="!runtime.running" @click="onStop">
              ⏹ 停止
            </el-button>
            <el-button @click="onRestart">↻ 重启</el-button>
            <el-checkbox v-model="autoFallback" :disabled="runtime.running" style="margin-left:auto;">
              自动适配
            </el-checkbox>
          </div>
          <el-divider />
          <!-- 内核信息 -->
          <div class="info-grid">
            <span class="info-label">当前内核</span>
            <span class="info-value">
              <el-tag v-if="activeCoreKind" size="small" :type="kindTagType(activeCoreKind)">{{ kindLabel(activeCoreKind) }}</el-tag>
              <span v-if="activeCoreVersion" class="muted" style="margin-left:6px;">v{{ activeCoreVersion }}</span>
              <span v-if="!activeCoreKind" class="muted">未配置</span>
            </span>
            <span class="info-label">混合端口</span>
            <span class="info-value">{{ settings.settings?.listen_port || 20808 }}</span>
            <span class="info-label">Clash API</span>
            <span class="info-value">
              <span v-if="runtime.clashReachable" class="status-ok">可达</span>
              <span v-else class="status-err">不可达</span>
            </span>
            <span v-if="state?.probe_method" class="info-label">探测方式</span>
            <span v-if="state?.probe_method" class="info-value">
              <el-tag size="small" :type="probeMethodTagType">{{ probeMethodLabel }}</el-tag>
            </span>
          </div>
          <el-divider />
          <!-- 代理模式 -->
          <div class="info-grid">
            <span class="info-label">代理模式</span>
            <span class="info-value">
              <div class="mode-tabs">
                <button
                  v-for="opt in modeOptions"
                  :key="opt.value"
                  :class="['mode-tab', { active: currentMode === opt.value }]"
                  @click="onModeChange(opt.value)"
                >{{ opt.label }}</button>
              </div>
            </span>
          </div>
          <el-divider />
          <!-- 系统代理 / 全局代理 -->
          <div class="info-grid">
            <span class="info-label">系统代理</span>
            <span class="info-value">
              <template v-if="sysProxy?.enabled">
                <el-tag type="success" size="small">{{ sysProxy.server }}</el-tag>
                <el-button link type="danger" size="small" @click="runtime.disableSysProxy()" class="toggle-btn">关闭</el-button>
              </template>
              <template v-else-if="sysProxy?.supported">
                <span class="muted">关闭</span>
                <el-button link type="primary" size="small" @click="runtime.enableSysProxy()" class="toggle-btn">开启</el-button>
              </template>
              <el-tag v-else type="warning" size="small">不支持</el-tag>
            </span>
            <span class="info-label">全局代理</span>
            <span class="info-value">
              <el-switch
                :model-value="isGlobalProxy"
                @change="(v: boolean) => v ? onGlobalProxy() : offGlobalProxy()"
              />
              <span class="info-hint">全局模式 + 系统代理</span>
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

    <!-- 下载兼容内核进度弹窗 -->
    <el-dialog v-model="dlShowProgress" :title="null" width="420px" :close-on-click-modal="false" :show-close="false" class="dl-dialog">
      <div class="dl-content">
        <div class="dl-icon">{{ dlProgress?.stage === 'done' ? '✓' : '⬇' }}</div>
        <div class="dl-title">{{ dlProgress?.stage === 'done' ? '下载完成' : '下载兼容内核' }}</div>
        <div class="dl-stage">{{ stageText(dlProgress?.stage) }}</div>
        <el-progress
          v-if="dlProgress"
          :percentage="dlProgress.pct || 0"
          :stroke-width="8"
          :status="dlProgress.stage === 'error' ? 'exception' : (dlProgress.stage === 'done' ? 'success' : '')"
          :show-text="true"
        />
        <div v-if="dlProgress?.error" class="dl-error">{{ dlProgress.error }}</div>
      </div>
      <template #footer>
        <el-button v-if="dlProgress?.stage === 'done'" type="primary" @click="dlCloseAndStart">启动内核</el-button>
        <el-button v-else-if="dlProgress?.stage === 'error'" @click="dlShowProgress = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
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

// 代理模式切换
const currentMode = ref('normal')
const modeOptions = [
  { label: '规则模式', value: 'normal' },
  { label: '全局模式', value: 'global' },
  { label: 'AI 模式', value: 'ai' },
]

// 同步当前模式
watch(() => runtime.state?.settings?.mode, (m) => {
  currentMode.value = m || 'normal'
}, { immediate: true })

async function onModeChange(mode: string) {
  await settings.save({ mode } as any)
  if (runtime.running) {
    await runtime.restartCore()
  }
}

// 全局代理 = 系统代理 + 全局模式 一键开启
async function onGlobalProxy() {
  await settings.save({ mode: 'global' } as any)
  currentMode.value = 'global'
  if (runtime.running) {
    await runtime.restartCore()
  }
  await runtime.enableSysProxy()
}

async function offGlobalProxy() {
  await settings.save({ mode: 'normal' } as any)
  currentMode.value = 'normal'
  await runtime.disableSysProxy()
  if (runtime.running) {
    await runtime.restartCore()
  }
}

const isGlobalProxy = computed(() => {
  return currentMode.value === 'global' && sysProxy.value?.enabled
})

// ----- 下载兼容内核进度弹窗 -----
const dlShowProgress = ref(false)
const dlProgress = ref<any>(null)
const dlWatchTimer = ref<any>(null)

function stageText(stage: string) {
  return {
    starting: '准备中...',
    fetch_releases: '获取版本列表',
    downloading: '下载中',
    resume_downloading: '断点续传中',
    verifying: '校验完整性',
    resume_verifying: '断点续传校验',
    extracting: '解压安装',
    done: '下载完成',
    error: '下载失败',
  }[stage] || stage
}

async function dlStartPolling(version: string, autoStart = false) {
  dlProgress.value = { stage: 'starting', version }
  dlShowProgress.value = true
  if (dlWatchTimer.value) clearInterval(dlWatchTimer.value)
  dlWatchTimer.value = setInterval(async () => {
    try {
      const p: any = await api.downloadStatus(version)
      dlProgress.value = p
      if (p.stage === 'done' || p.stage === 'error') {
        clearInterval(dlWatchTimer.value)
        dlWatchTimer.value = null
        await settings.load()
        await runtime.refresh()
        // Auto-start after download completes if requested
        if (p.stage === 'done' && autoStart) {
          dlShowProgress.value = false
          await doStart()
        }
      }
    } catch {}
  }, 800)
}

async function dlCloseAndStart() {
  dlShowProgress.value = false
  if (dlWatchTimer.value) {
    clearInterval(dlWatchTimer.value)
    dlWatchTimer.value = null
  }
  await doStart()
}

function dlCancelWatch() {
  if (dlWatchTimer.value) {
    clearInterval(dlWatchTimer.value)
    dlWatchTimer.value = null
  }
  dlShowProgress.value = false
}

onBeforeUnmount(() => {
  if (dlWatchTimer.value) clearInterval(dlWatchTimer.value)
})

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

    // NodeValidator 校验结果：不兼容时提示用户将自动切换（而非硬阻止）
    const nv = pre.node_validation
    if (nv && !nv.valid && nv.errors && nv.errors.length > 0) {
      const errMsgs = nv.errors.map((e: any) => `• ${e.message}`).join('\n')
      const bestCore = pre.core_info?.name || '其他内核'
      // v2rayN 路线：自动选内核，只做提示
      ElMessage({
        message: `当前内核不支持此节点协议，将自动切换兼容内核：\n${errMsgs}\n→ 推荐使用 ${bestCore}`,
        type: 'warning',
        duration: 5000,
      })
    } else if (nv && nv.warnings && nv.warnings.length > 0) {
      const warnMsgs = nv.warnings.map((w: any) => `• ${w.message}`).join('\n')
      ElMessage({ message: warnMsgs, type: 'warning', duration: 5000 })
    }

    if (level === 'warn' || level === 'bad') {
      const reasons = (pre.compatibility.reasons || []).map((r: any) => `• ${r.message}`).join('\n')
      const hasLocal = !!(pre.recommended_id && pre.recommended_id !== pre.current_core_id)
      // v2rayN 路线：有兼容内核时自动切换，不再弹确认
      if (hasLocal) {
        ElMessage({
          message: `自动切换到兼容内核 v${pre.recommended_version}`,
          type: 'info',
          duration: 3000,
        })
      } else if (level === 'bad') {
        // 无本地兼容内核且完全不兼容，提示下载
        const msg = `当前节点与内核 [${pre.current_core}] 不兼容：\n${reasons}\n\n本地无兼容内核，需从 GitHub 下载`
        try {
          await ElMessageBox.confirm(msg, '需要下载兼容内核', {
            type: 'warning',
            confirmButtonText: '立即下载并启动',
            cancelButtonText: '取消',
          })
          try {
            const r = await api.autoMatchCore()
            if (r.action === 'activate_existing') {
              // 本地已有兼容内核，直接激活并启动
              ElMessage.success(`已激活 v${r.version}`)
              await settings.load()
              await runtime.refresh()
              await doStart()
            } else if (r.action === 'downloaded') {
              // 下载完成，弹出进度弹窗，下载完成后自动启动
              dlStartPolling(r.version || '', true)
            } else if (r.action === 'activate_from_cache') {
              // 从缓存激活，直接启动
              ElMessage.success(`已从缓存激活 v${r.version}`)
              await settings.load()
              await runtime.refresh()
              await doStart()
            } else {
              ElMessage.info('action: ' + r.action)
            }
          } catch (e: any) {
            ElMessage.error('下载失败：' + (e?.message || String(e)))
          }
        } catch {
          return
        }
        return
      }
    }
  } catch {}

  await doStart()
}

async function doStart() {
  try {
    const r = await runtime.startCore({ auto_fallback: autoFallback.value })
    const switchMsg = r.auto_switched ? `（已自动切换到 v${r.core_version}）` : ''
    ElMessage.success('已启动 · ' + (r.server || '') + switchMsg)
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
.stat-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 14px;
}
.stat-card {
  background: var(--bg-soft);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px 18px;
  display: flex;
  align-items: center;
  gap: 14px;
  transition: all var(--transition);
  box-shadow: var(--shadow-sm);
}
.stat-card:hover {
  box-shadow: var(--shadow);
  transform: translateY(-1px);
}
.stat-card.active {
  border-color: var(--green);
  box-shadow: 0 0 0 1px var(--green), var(--shadow-sm);
}
.stat-icon {
  font-size: 18px;
  width: 40px;
  height: 40px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: var(--bg-mute);
  color: var(--text-mute);
  transition: all var(--transition);
}
.stat-body {
  min-width: 0;
}
.stat-value {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.02em;
}
.stat-label {
  font-size: 12px;
  color: var(--text-mute);
  margin-top: 2px;
}

/* Control row */
.control-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
  margin-top: 12px;
}

/* Info grid */
.info-grid {
  display: grid;
  grid-template-columns: 90px 1fr;
  gap: 10px 16px;
  font-size: 13px;
  align-items: center;
}
.info-label {
  color: var(--text-mute);
  font-size: 12px;
  white-space: nowrap;
}
.info-value {
  word-break: break-all;
  display: flex;
  align-items: center;
  gap: 6px;
}
.info-hint {
  font-size: 11px;
  color: var(--text-mute);
  margin-left: 4px;
}

/* Status colors */
.status-ok {
  color: var(--green);
  font-weight: 500;
}
.status-err {
  color: var(--red);
  font-weight: 500;
}

/* Toggle button (system proxy on/off) */
.toggle-btn {
  margin-left: 8px;
  font-size: 12px;
}

/* Mode tabs (replaces el-segmented) */
.mode-tabs {
  display: inline-flex;
  background: var(--bg-mute);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 2px;
  gap: 2px;
}
.mode-tab {
  border: none;
  background: transparent;
  color: var(--text-mute);
  font-size: 12px;
  font-weight: 500;
  padding: 4px 14px;
  border-radius: 4px;
  cursor: pointer;
  transition: all var(--transition);
  white-space: nowrap;
  line-height: 1.4;
}
.mode-tab:hover:not(.active) {
  color: var(--text);
  background: var(--accent-soft);
}
.mode-tab.active {
  background: var(--accent);
  color: #fff;
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(108, 140, 255, 0.35);
}

/* Download progress dialog */
.dl-content {
  text-align: center;
  padding: 8px 0 4px;
}
.dl-icon {
  font-size: 36px;
  margin-bottom: 12px;
  line-height: 1;
}
.dl-icon + .dl-title + .dl-stage + .el-progress .el-progress-bar__outer {
  margin-top: 8px;
}
.dl-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 4px;
}
.dl-stage {
  font-size: 13px;
  color: var(--text-mute);
  margin-bottom: 16px;
}
.dl-error {
  margin-top: 12px;
  color: var(--red);
  font-size: 12px;
  text-align: left;
}

/* Card title */
.card-title {
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
}

@media (max-width: 768px) {
  .stat-row {
    grid-template-columns: repeat(2, 1fr);
  }
  .info-grid {
    grid-template-columns: 1fr;
    gap: 6px 0;
  }
  .info-label {
    font-size: 11px;
  }
  .control-row {
    gap: 6px;
  }
}
</style>
