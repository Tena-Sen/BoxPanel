<template>
  <div>
    <h2 class="page-title">{{ t('settings.title') }}</h2>

    <el-row :gutter="16">
      <el-col :xs="24" :sm="12">
        <div class="card settings-section">
          <div class="section-header">
            <span class="section-icon" style="background:var(--accent-soft);color:var(--accent);">⚙</span>
            <span class="section-title">{{ t('settings.general') }}</span>
          </div>
          <el-form :model="form" label-width="160px" label-position="left" class="settings-form">
            <el-form-item :label="t('settings.theme')">
              <el-radio-group v-model="form.theme" class="theme-radio">
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
        <div class="card settings-section">
          <div class="section-header">
            <span class="section-icon" style="background:var(--cyan-soft);color:var(--cyan);">⬡</span>
            <span class="section-title">{{ t('settings.network') }}</span>
          </div>
          <el-form :model="form" label-width="160px" label-position="left" class="settings-form">
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
    <div class="card settings-section core-manager" style="margin-top:16px;">
      <div class="core-header">
        <div class="core-header-left">
          <span class="section-icon" style="background:var(--purple-soft);color:var(--purple);">⬢</span>
          <div>
            <div class="section-title">内核管理</div>
            <div v-if="activeCore" class="active-core-hint">
              当前激活: {{ kindLabel(activeCore.kind) }} {{ activeCore?.label }}
            </div>
          </div>
        </div>
        <div class="core-header-actions">
          <el-button size="small" type="primary" plain @click="onAutoMatch" :loading="autoMatchLoading">
            <span style="margin-right:4px;">⬡</span> 自动匹配
          </el-button>
          <el-dropdown size="small" @command="onDownloadCoreKind">
            <el-button size="small" plain>⬇ 下载 ▾</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="singbox">sing-box (全协议)</el-dropdown-item>
                <el-dropdown-item command="xray">Xray (vless/vmess/trojan/ss)</el-dropdown-item>
                <el-dropdown-item command="mihomo">mihomo (Clash.Meta)</el-dropdown-item>
                <el-dropdown-item command="hysteria2">Hysteria2 (QUIC)</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button size="small" plain @click="showAddCore = true">+ 手动添加</el-button>
        </div>
      </div>
      <div class="table-wrap">
      <el-table :data="cores" stripe class="core-table">
        <el-table-column label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="kindTagType(row.kind)" size="small" effect="plain">{{ kindLabel(row.kind) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="label" label="名称" width="140" />
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column label="协议" min-width="160">
          <template #default="{ row }">
            <div class="proto-list">
              <template v-if="coreInfoByKind(row.kind)">
                <span v-for="p in coreInfoByKind(row.kind)!.supported_protocols.slice(0, 4)" :key="p" class="proto-chip">{{ p }}</span>
                <span v-if="coreInfoByKind(row.kind)!.supported_protocols.length > 4" class="proto-more">+{{ coreInfoByKind(row.kind)!.supported_protocols.length - 4 }}</span>
              </template>
              <span v-else class="muted">-</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Clash API" width="80" align="center">
          <template #default="{ row }">
            <span v-if="coreInfoByKind(row.kind)?.has_clash_api" class="clash-yes">Yes</span>
            <span v-else class="clash-no">No</span>
          </template>
        </el-table-column>
        <el-table-column label="路径" width="" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="muted path-text">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="180" align="right">
          <template #default="{ row }">
            <div class="row-actions">
              <el-button v-if="row.id !== activeCore?.id" size="small" type="primary" text @click="onActivateCore(row)">使用</el-button>
              <el-tag v-else size="small" type="success" effect="plain" class="active-tag">当前</el-tag>
              <el-button size="small" text @click="onTestCore(row)">探测</el-button>
              <el-button v-if="!row.default" size="small" type="danger" text @click="onDeleteCore(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      </div>
      <div class="core-footer">
        支持 sing-box（全协议）/ Xray / mihomo / Hysteria2 多内核引擎。启动时使用「当前」内核，configgen 按类型生成对应配置。
      </div>
    </div>

    <div class="card about-card" style="margin-top:16px;">
      <div class="about-brand">
        <div class="about-logo">BP</div>
        <div>
          <div class="about-name">BoxPanel</div>
          <div class="about-ver">v{{ version }} · Go + Vue 3 + Multi-Core</div>
        </div>
      </div>
      <div class="spacer"></div>
      <a class="about-link" href="https://github.com/Tena-Sen/BoxPanel" target="_blank" rel="noopener">GitHub ↗</a>
      <div style="width:1px;height:28px;background:var(--border);margin:0 4px;"></div>
      <el-button type="primary" @click="onSave">{{ t('settings.save') }}</el-button>
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
    <el-dialog v-model="showProgress" :title="null" width="420px" :close-on-click-modal="false" :show-close="false">
      <div class="dl-content">
        <div class="dl-icon">{{ dlProgress?.stage === 'done' ? '✓' : '⬇' }}</div>
        <div class="dl-title">{{ dlProgress?.stage === 'done' ? '下载完成' : `下载 ${kindLabel(dlKind)}` }}</div>
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
        <el-button v-if="dlProgress?.stage === 'done' || dlProgress?.stage === 'error'" type="primary" @click="closeProgress">关闭</el-button>
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
import type { CoreConfig, CoreKindInfo, CoreInfo } from '@/api/types'

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
const coreInfos = ref<CoreInfo[]>([])   // CoreInfo 元数据注册表
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
  // 同时加载 CoreInfo 注册表
  try {
    const kr = await api.listCoreKinds()
    coreInfos.value = kr.core_info || []
  } catch {}
}

// 按 kind 查找 CoreInfo
function coreInfoByKind(kind: string): CoreInfo | undefined {
  return coreInfos.value.find(ci => ci.kind === kind)
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
    resume_downloading: '断点续传中',
    verifying: '校验',
    resume_verifying: '断点续传校验',
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

<style scoped>
/* Settings section cards */
.settings-section {
  position: relative;
}
.section-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}
.section-icon {
  width: 32px; height: 32px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  font-size: 14px; flex-shrink: 0;
}
.section-title {
  font-weight: 600; font-size: 15px; color: var(--text);
}
.active-core-tag {
  margin-left: 8px;
}

/* ---- Core Manager ---- */
.core-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  gap: 12px;
}
.core-header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.core-header-left .section-title {
  font-size: 15px;
}
.active-core-hint {
  font-size: 12px;
  color: var(--green);
  margin-top: 2px;
}
.core-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.proto-list {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}
.proto-chip {
  font-size: 11px;
  padding: 1px 7px;
  border-radius: 10px;
  background: var(--bg-mute);
  color: var(--text-mute);
  white-space: nowrap;
}
.proto-more {
  font-size: 11px;
  color: var(--text-mute);
  margin-left: 2px;
}
.clash-yes { color: var(--green); font-weight: 500; font-size: 13px; }
.clash-no { color: var(--text-mute); font-size: 13px; }
.path-text {
  font-family: 'Consolas', 'Monaco', ui-monospace, monospace;
  font-size: 11px;
  opacity: 0.6;
}
.core-table :deep(.el-table__body-wrapper) {
  overflow-x: auto;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  justify-content: flex-end;
  white-space: nowrap;
}
.active-tag {
  font-size: 11px !important;
}
.core-footer {
  font-size: 11px;
  color: var(--text-mute);
  margin-top: 10px;
  line-height: 1.6;
  opacity: 0.7;
}
.settings-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

/* About card */
.about-card {
  display: flex;
  align-items: center;
  gap: 16px;
}
.about-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}
.about-logo {
  width: 44px; height: 44px; border-radius: var(--radius);
  background: linear-gradient(135deg, var(--accent), var(--purple));
  color: #fff;
  display: flex; align-items: center; justify-content: center;
  font-weight: 700; font-size: 14px;
  flex-shrink: 0;
  box-shadow: 0 2px 8px rgba(108, 140, 255, 0.3);
}
.about-name {
  font-weight: 700; font-size: 17px; color: var(--text);
  line-height: 1.2;
}
.about-ver {
  font-size: 12px; color: var(--text-mute);
  margin-top: 2px;
}
.about-link {
  color: var(--accent);
  text-decoration: none;
  font-size: 13px;
  font-weight: 500;
  padding: 4px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  transition: all var(--transition);
}
.about-link:hover {
  border-color: var(--accent);
  background: var(--accent-soft);
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

@media (max-width: 768px) {
  :deep(.el-form-item__label) { text-align: left; }
  .section-header { flex-wrap: wrap; }
  .about-card { flex-wrap: wrap; }
  .core-header { flex-direction: column; align-items: flex-start; gap: 12px; }
  .core-header-actions { flex-wrap: wrap; }
  .settings-form :deep(.el-form-item) {
    flex-direction: column;
  }
  .settings-form :deep(.el-form-item__label) {
    text-align: left;
    padding-bottom: 2px;
  }
}
</style>