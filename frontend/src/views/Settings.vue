<template>
  <div>
    <h2 class="page-title">{{ t('settings.title') }}</h2>

    <el-row :gutter="16">
      <el-col :xs="24" :sm="12">
        <div class="card">
          <div style="font-weight:600;margin-bottom:12px;">{{ t('settings.general') }}</div>
          <el-form :model="form" label-width="160px" label-position="left">
            <el-form-item :label="t('settings.theme')">
              <el-radio-group v-model="form.theme">
                <el-radio value="dark">☾ {{ t('settings.dark') }}</el-radio>
                <el-radio value="light">☀ {{ t('settings.light') }}</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item :label="t('settings.language')">
              <el-radio-group v-model="form.language">
                <el-radio value="zh-CN">简体中文</el-radio>
                <el-radio value="en">English</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item :label="t('settings.autoRefresh')">
              <el-switch v-model="form.auto_refresh_sub_on_start" />
            </el-form-item>
          </el-form>
        </div>
      </el-col>

      <el-col :xs="24" :sm="12">
        <div class="card">
          <div style="font-weight:600;margin-bottom:12px;">{{ t('settings.network') }}</div>
          <el-form :model="form" label-width="160px" label-position="left">
            <el-form-item :label="t('settings.listenPort')">
              <el-input-number v-model="form.listen_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item :label="t('settings.clashApiPort')">
              <el-input-number v-model="form.clash_api_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item :label="t('settings.latencyUrl')">
              <el-input v-model="form.latency_test_url" placeholder="http://www.gstatic.com/generate_204" />
            </el-form-item>
            <el-form-item :label="t('settings.subUA')">
              <el-input v-model="form.subscription_user_agent" placeholder="clash-meta" />
            </el-form-item>
          </el-form>
        </div>
      </el-col>
    </el-row>

    <!-- 内核管理 -->
    <div class="card" style="margin-top:16px;">
      <div class="toolbar" style="margin-bottom:12px;">
        <span style="font-weight:600;">内核管理</span>
        <el-tag v-if="activeCore" size="small" type="success">当前：{{ kindLabel(activeCore.kind) }} {{ activeCore?.label }} ({{ activeCore?.version }})</el-tag>
        <div class="spacer"></div>
        <el-button size="small" type="primary" @click="onAutoMatch" :loading="autoMatchLoading">自动匹配</el-button>
        <el-dropdown size="small" @command="onDownloadCoreKind">
          <el-button size="small">
            从 GitHub 下载 ▾
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="singbox">sing-box (全协议)</el-dropdown-item>
              <el-dropdown-item command="xray">Xray (vless/vmess/trojan/ss)</el-dropdown-item>
              <el-dropdown-item command="mihomo">mihomo (Clash.Meta)</el-dropdown-item>
              <el-dropdown-item command="hysteria2">Hysteria2 (QUIC 加速)</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button size="small" @click="showAddCore = true">+ 手动添加</el-button>
      </div>
      <el-table :data="cores" stripe>
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag :type="kindTagType(row.kind)" size="small">{{ kindLabel(row.kind) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="label" label="名称" width="160" />
        <el-table-column prop="version" label="版本" width="120" />
        <el-table-column label="路径" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="muted" style="font-family:monospace;font-size:11px;">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300" align="right">
          <template #default="{ row }">
            <el-button v-if="row.id !== activeCore?.id" size="small" type="primary" @click="onActivateCore(row)">使用</el-button>
            <el-tag v-else size="small" type="success">正在使用</el-tag>
            <el-button size="small" @click="onTestCore(row)">探测版本</el-button>
            <el-button v-if="!row.default" size="small" type="danger" @click="onDeleteCore(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="muted" style="font-size:11px;margin-top:8px;">
        支持多内核引擎：sing-box（全协议）/ Xray（vless/vmess/trojan/ss）/ mihomo（ss/vmess/trojan/hy2/tuic）/ Hysteria2（hy2 专用加速）。
        启动核心时使用「当前」内核；configgen 会按内核类型和版本生成对应的配置格式。
      </div>
    </div>

    <div class="card" style="margin-top:16px;">
      <div style="display:flex;align-items:center;gap:16px;">
        <span class="muted">{{ t('settings.about') }} · v{{ version }}</span>
        <span class="muted">Go + Vue 3 + sing-box</span>
        <div class="spacer"></div>
        <el-button type="primary" @click="onSave">{{ t('settings.save') }}</el-button>
      </div>
    </div>

    <!-- 添加内核对话框 -->
    <el-dialog v-model="showAddCore" title="添加内核" width="540px">
      <el-form :model="coreForm" label-width="100px">
        <el-form-item label="内核类型">
          <el-select v-model="coreForm.kind" style="width:100%;">
            <el-option value="singbox" label="sing-box (全协议)" />
            <el-option value="xray" label="Xray (vless/vmess/trojan/ss)" />
            <el-option value="mihomo" label="mihomo (Clash.Meta)" />
            <el-option value="hysteria2" label="Hysteria2 (QUIC 加速)" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="coreForm.label" placeholder="如：sing-box 1.10.7" />
        </el-form-item>
        <el-form-item label="可执行路径">
          <el-input v-model="coreForm.path" placeholder="如：C:\cores\xray\xray.exe" />
        </el-form-item>
        <el-form-item label="版本">
          <el-input v-model="coreForm.version" placeholder="留空自动探测" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddCore = false">取消</el-button>
        <el-button type="primary" @click="onAddCore">添加</el-button>
      </template>
    </el-dialog>

    <!-- 下载内核对话框 -->
    <el-dialog v-model="showDownload" :title="`从 GitHub 下载 ${kindLabel(dlKind)}`" width="640px" :close-on-click-modal="false">
      <div class="toolbar" style="margin-bottom:8px;">
        <el-checkbox v-model="downloadIncludePre">包含预发布版本</el-checkbox>
        <div class="spacer"></div>
        <el-button size="small" @click="loadAvailableCores">↻ 刷新列表</el-button>
      </div>
      <el-table :data="availableCores" max-height="400" @row-dblclick="onPickVersion">
        <el-table-column prop="version" label="版本" width="160" />
        <el-table-column label="类型" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.prerelease" size="small" type="warning">预发布</el-tag>
            <el-tag v-else size="small" type="success">stable</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="本地">
          <template #default="{ row }">
            <el-tag v-if="row.downloaded" size="small" type="info">已下载</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" :disabled="row.downloaded" @click="onPickVersion(row)">下载</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 下载进度对话框 -->
    <el-dialog v-model="showProgress" title="下载中" width="500px" :close-on-click-modal="false" :show-close="false">
      <div v-if="dlProgress">
        <div style="margin-bottom:8px;">下载 {{ dlProgress.version }}</div>
        <div style="margin-bottom:8px;font-family:monospace;color:var(--text-mute);font-size:12px;">
          {{ stageText(dlProgress.stage) }}
          <span v-if="dlProgress.source"> · via {{ dlProgress.source }}</span>
        </div>
        <el-progress
          :percentage="dlProgress.pct || 0"
          :status="dlProgress.stage === 'error' ? 'exception' : (dlProgress.stage === 'done' ? 'success' : '')"
        />
        <div v-if="dlProgress.error" style="margin-top:12px;color:var(--red);font-size:12px;">
          {{ dlProgress.error }}
        </div>
      </div>
      <template #footer>
        <el-button v-if="dlProgress?.stage === 'done' || dlProgress?.stage === 'error'" type="primary" @click="closeProgress">关闭</el-button>
        <el-button v-else @click="cancelWatch">取消监听</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useSettingsStore } from '@/stores/settings'
import { useRuntimeStore } from '@/stores/runtime'
import { api } from '@/api/client'
import type { CoreConfig, CoreKindInfo } from '@/api/types'

const { t, locale } = useI18n()
const settings = useSettingsStore()
const runtime = useRuntimeStore()
const version = '1.0.0'

const form = reactive({
  theme: 'dark',
  language: 'zh-CN',
  listen_port: 20808,
  clash_api_port: 9090,
  latency_test_url: 'http://www.gstatic.com/generate_204',
  subscription_user_agent: 'clash-meta',
  auto_refresh_sub_on_start: true,
})

watch(() => settings.settings, (s) => {
  if (!s) return
  Object.assign(form, {
    theme: s.theme,
    language: s.language,
    listen_port: s.listen_port,
    clash_api_port: s.clash_api_port,
    latency_test_url: s.latency_test_url,
    subscription_user_agent: s.subscription_user_agent,
    auto_refresh_sub_on_start: s.auto_refresh_sub_on_start,
  })
}, { immediate: true, deep: true })

async function onSave() {
  await settings.save(form as any)
  locale.value = form.language
  localStorage.setItem('boxpanel.lang', form.language)
  ElMessage.success('已保存')
}

// ----- 内核管理 -----
const cores = ref<CoreConfig[]>([])
const activeCore = computed(() => cores.value.find((c) => c.id === settings.settings?.active_core_id))
const showAddCore = ref(false)
const coreForm = reactive<CoreConfig>({
  id: '', kind: 'singbox', label: '', version: '', path: '', default: false,
})

async function refreshCores() {
  const r = await api.listCores()
  cores.value = r.cores
  if (r.active_core_id && settings.settings) {
    settings.settings.active_core_id = r.active_core_id
  }
}

async function onAddCore() {
  if (!coreForm.path) {
    ElMessage.warning('路径必填')
    return
  }
  await api.addCore({ ...coreForm, id: '' })
  ElMessage.success('已添加（将自动探测版本）')
  showAddCore.value = false
  Object.assign(coreForm, { id: '', kind: 'singbox', label: '', version: '', path: '', default: false })
  await refreshCores()
}

async function onTestCore(c: CoreConfig) {
  try {
    const r = await api.testCore(c.id)
    ElMessage.success(`已探测：${r.version}`)
    await refreshCores()
  } catch (e: any) {
    ElMessage.error('探测失败：' + (e?.message || String(e)))
  }
}

async function onActivateCore(c: CoreConfig) {
  try {
    await api.activateCore(c.id)
    ElMessage.success(`已切换到 ${c.label}`)
    await settings.load()
    await refreshCores()
  } catch (e: any) {
    ElMessage.error('切换失败：' + (e?.message || String(e)))
  }
}

async function onDeleteCore(c: CoreConfig) {
  try {
    await ElMessageBox.confirm(`删除内核 "${c.label}"？`, '确认', { type: 'warning' })
  } catch { return }
  await api.deleteCore(c.id)
  ElMessage.success('已删除')
  await settings.load()
  await refreshCores()
}

// ----- 自动匹配 -----
const autoMatchLoading = ref(false)
async function onAutoMatch() {
  autoMatchLoading.value = true
  try {
    const r = await api.autoMatchCore()
    if (r.action === 'activate_existing') {
      ElMessage.success(`本地已有兼容内核 v${r.version}，已激活`)
    } else if (r.action === 'downloaded') {
      ElMessage.success(`已下载并激活 v${r.version}`)
    } else {
      ElMessage.info('action: ' + r.action)
    }
    await settings.load()
    await refreshCores()
  } catch (e: any) {
    ElMessage.error('自动匹配失败：' + (e?.message || String(e)))
  } finally {
    autoMatchLoading.value = false
  }
}

// ----- 从 GitHub 下载 -----
const showDownload = ref(false)
const downloadIncludePre = ref(false)
const dlKind = ref('singbox')
const availableCores = ref<Array<{ version: string; tag_name: string; prerelease: boolean; downloaded: boolean; name: string }>>([])
const showProgress = ref(false)
const dlProgress = ref<any>(null)
const dlWatchTimer = ref<any>(null)

async function loadAvailableCores() {
  try {
    if (dlKind.value === 'singbox') {
      const r = await api.listAvailableCores(downloadIncludePre.value)
      availableCores.value = r.items || []
    } else {
      const r = await api.listKindAvailable(dlKind.value, downloadIncludePre.value)
      availableCores.value = (r.items || []).map((i: any) => ({ ...i, downloaded: false }))
    }
  } catch (e: any) {
    ElMessage.error('加载版本列表失败：' + (e?.message || String(e)))
  }
}

function onDownloadCoreKind(kind: string) {
  dlKind.value = kind
  showDownload.value = true
  loadAvailableCores()
}

async function onPickVersion(row: { version: string }) {
  showDownload.value = false
  const version = row.version
  const kind = dlKind.value
  try {
    await api.downloadCore(version, true, kind)
  } catch (e: any) {
    ElMessage.error('启动下载失败：' + (e?.message || String(e)))
    return
  }
  // The status polling key uses "kind:version" for non-singbox cores
  const statusKey = kind === 'singbox' ? version : `${kind}:${version}`
  showProgress.value = true
  dlProgress.value = { stage: 'starting', version }
  if (dlWatchTimer.value) clearInterval(dlWatchTimer.value)
  dlWatchTimer.value = setInterval(async () => {
    try {
      const p: any = await api.downloadStatus(statusKey)
      dlProgress.value = p
      if (p.stage === 'done' || p.stage === 'error') {
        clearInterval(dlWatchTimer.value)
        dlWatchTimer.value = null
        await settings.load()
        await refreshCores()
      }
    } catch {}
  }, 800)
}

function closeProgress() {
  showProgress.value = false
  if (dlWatchTimer.value) {
    clearInterval(dlWatchTimer.value)
    dlWatchTimer.value = null
  }
}

function cancelWatch() {
  // 后端下载仍在跑；只是前端不再轮询
  if (dlWatchTimer.value) {
    clearInterval(dlWatchTimer.value)
    dlWatchTimer.value = null
  }
  showProgress.value = false
}

function stageText(stage: string) {
  return {
    starting: '准备中...',
    fetch_releases: '获取版本列表',
    downloading: '下载中',
    verifying: '校验',
    extracting: '解压',
    done: '完成',
    error: '失败',
  }[stage] || stage
}

onMounted(async () => {
  await refreshCores()
})
onBeforeUnmount(() => {
  if (dlWatchTimer.value) clearInterval(dlWatchTimer.value)
})

// ----- 内核类型标签 -----
const kindLabels: Record<string, string> = {
  singbox: 'sing-box',
  xray: 'Xray',
  mihomo: 'mihomo',
  hysteria2: 'Hysteria2',
}
function kindLabel(kind: string) {
  return kindLabels[kind] || kind || 'sing-box'
}
function kindTagType(kind: string) {
  switch (kind) {
    case 'singbox': return ''       // default blue
    case 'xray': return 'warning'  // orange
    case 'mihomo': return 'danger' // red/pink
    case 'hysteria2': return 'info' // grey
    default: return 'info'
  }
}
</script>