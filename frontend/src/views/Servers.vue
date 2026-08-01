<template>
  <div>
    <h2 class="page-title">{{ t('servers.title') }}</h2>

    <div class="toolbar">
      <el-button type="primary" @click="showImport = true">📥 {{ t('servers.import') }}</el-button>
      <el-button @click="onTestAll" :loading="testingAll">⚡ 全部测试</el-button>
      <div class="spacer"></div>
      <!-- 搜索框 -->
      <el-input
        v-model="searchQuery"
        placeholder="搜索节点名/地址/协议..."
        clearable
        class="search-input"
        size="default"
      >
        <template #prefix>🔍</template>
      </el-input>
      <!-- 协议筛选 -->
      <el-select v-model="protocolFilter" placeholder="全部协议" clearable size="default" class="proto-filter">
        <el-option v-for="p in protocols" :key="p" :label="p" :value="p" />
      </el-select>
      <span class="muted" style="margin-left:8px;">{{ filteredServers.length }} / {{ servers.servers.length }}</span>
    </div>

    <div v-if="servers.servers.length === 0" class="card" style="text-align:center;padding:40px;">
      <div class="muted" style="margin-bottom:12px;">{{ t('servers.empty') }}</div>
      <el-button type="primary" @click="showImport = true">{{ t('servers.import') }}</el-button>
    </div>

    <div
      v-for="srv in filteredServers"
      :key="srv.id"
      :class="['card', 'server-card', { selected: srv.id === servers.selectedId }]"
      style="margin-bottom:8px;display:flex;align-items:center;gap:12px;cursor:pointer;"
      @click="onSelect(srv)"
    >
      <el-tag size="small" type="info">{{ srv.protocol }}</el-tag>
      <div style="flex:1;min-width:0;">
        <div style="font-weight:500;">{{ srv.name }}</div>
        <div class="muted" style="font-size:12px;">
          {{ srv.server }}:{{ srv.server_port }}
          <span v-if="srv.transport_type && srv.transport_type !== 'tcp'"> · {{ srv.transport_type }}</span>
          <el-tag v-if="srv.tls_enabled" size="small" type="success" style="margin-left:4px;">TLS</el-tag>
          <el-tag v-if="srv.reality_enabled" size="small" type="warning" style="margin-left:4px;">Reality</el-tag>
        </div>
        <!-- 兼容性徽章 -->
        <div v-if="compatChipsFor(srv).length > 0" style="margin-top:4px;display:flex;gap:4px;flex-wrap:wrap;">
          <el-tooltip
            v-for="c in compatChipsFor(srv)"
            :key="c.ver + c.level"
            :content="c.title"
            placement="top"
          >
            <span :class="['compat-chip', 'compat-' + c.level]">{{ c.label }}</span>
          </el-tooltip>
        </div>
        <!-- NodeValidator 兼容性警告 -->
        <div v-if="!nodeValidateFor(srv).ok" style="margin-top:4px;display:flex;gap:4px;flex-wrap:wrap;">
          <el-tooltip
            v-for="(err, idx) in nodeValidateFor(srv).errors"
            :key="idx"
            :content="err"
            placement="top"
          >
            <span class="compat-chip compat-bad">&#x26A0; {{ err }}</span>
          </el-tooltip>
        </div>
      </div>
      <!-- 延迟 + 带宽 一行展示 -->
      <div class="test-badges">
        <LatencyBadge :ms="srv.last_latency_ms" />
        <BandwidthBadge :mbps="srv.last_bandwidth_mbps" />
      </div>
      <!-- 单节点测试按钮：同时测延迟+带宽 -->
      <el-button
        size="small"
        :loading="servers.testingIds.has(srv.id) || servers.testingBwIds.has(srv.id)"
        @click.stop="onTestOne(srv)"
      >⚡</el-button>
      <el-button size="small" @click.stop="onEdit(srv)">✎</el-button>
      <el-button size="small" type="danger" @click.stop="onDelete(srv)">✕</el-button>
    </div>

    <!-- 导入对话框 -->
    <el-dialog v-model="showImport" :title="t('servers.import')" width="640px">
      <el-input
        v-model="importText"
        type="textarea"
        :rows="12"
        :placeholder="t('servers.importPlaceholder')"
      />
      <template #footer>
        <el-button @click="showImport = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="onImport">{{ t('servers.import') }}</el-button>
      </template>
    </el-dialog>

    <!-- 编辑对话框 -->
    <el-dialog v-model="showEdit" title="编辑节点" width="520px">
      <el-form :model="editForm" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="editForm.name" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="editForm.server" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="editForm.server_port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="原始链接">
          <el-input v-model="editForm.raw_link" type="textarea" :rows="3" />
        </el-form-item>
        <div class="muted" style="font-size:12px;margin-top:-12px;">
          提示：改协议/认证字段请重新导入。原始链接改了会重置。
        </div>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="onSaveEdit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useServersStore } from '@/stores/servers'
import { useRuntimeStore } from '@/stores/runtime'
import LatencyBadge from '@/components/LatencyBadge.vue'
import BandwidthBadge from '@/components/BandwidthBadge.vue'
import type { Server, CompatLevel, CoreInfo } from '@/api/types'
import { api } from '@/api/client'

const { t } = useI18n()
const servers = useServersStore()
const runtime = useRuntimeStore()

// CoreInfo 校验：判断当前激活内核是否支持该节点的协议/传输
const coreInfos = ref<CoreInfo[]>([])
const activeCoreKind = computed(() => {
  const st = runtime.settings
  if (!st?.cores || !st.active_core_id) return null
  const c = st.cores.find(c => c.id === st.active_core_id)
  return c?.kind || null
})
const activeCoreInfo = computed(() => {
  if (!activeCoreKind.value) return null
  return coreInfos.value.find(ci => ci.kind === activeCoreKind.value) || null
})

// 前端轻量 NodeValidator：用 CoreInfo 的 supported_protocols 和 unsupported_transports
function nodeValidateFor(srv: Server): { ok: boolean; errors: string[] } {
  const info = activeCoreInfo.value
  if (!info) return { ok: true, errors: [] }
  const errors: string[] = []
  // 协议检查
  if (info.supported_protocols && info.supported_protocols.length > 0 && !info.supported_protocols.includes(srv.protocol)) {
    errors.push(`${info.name} 不支持 ${srv.protocol}`)
  }
  // 传输检查
  const t = srv.transport_type
  if (t && t !== 'tcp' && t !== 'raw' && info.unsupported_transports?.includes(t)) {
    errors.push(`${info.name} 不支持 ${t} 传输`)
  }
  // sing-box SS 传输限制
  if (info.ss_only_transports && info.ss_only_transports.length > 0 && srv.protocol === 'shadowsocks') {
    const normalized = (!t || t === 'tcp') ? 'raw' : t
    if (!info.ss_only_transports.includes(normalized)) {
      errors.push(`sing-box SS 仅支持 raw/ws 传输`)
    }
  }
  return { ok: errors.length === 0, errors }
}

async function loadCoreInfos() {
  try {
    const r = await api.listCoreKinds()
    coreInfos.value = r.core_info || []
  } catch {}
}

onMounted(() => { loadCoreInfos() })

const showImport = ref(false)
const importText = ref('')
const importing = ref(false)
const testingAll = ref(false)

// 搜索 & 筛选
const searchQuery = ref('')
const protocolFilter = ref('')

const protocols = ['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'anytls', 'http', 'socks']

const filteredServers = computed(() => {
  let list = servers.servers
  if (protocolFilter.value) {
    list = list.filter(s => s.protocol === protocolFilter.value)
  }
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter(s =>
      s.name.toLowerCase().includes(q) ||
      s.server.toLowerCase().includes(q) ||
      s.protocol.toLowerCase().includes(q) ||
      (s.server + ':' + s.server_port).includes(q)
    )
  }
  return list
})

const showEdit = ref(false)
const editForm = reactive<Server>({
  id: '', protocol: '', name: '', server: '', server_port: 0,
})

// 兼容性徽章
function compatChipsFor(srv: Server) {
  const out: Array<{ ver: string; level: CompatLevel; label: string; title: string }> = []
  const activeVer = runtime.settings?.cores?.find((c) => c.id === runtime.settings?.active_core_id)?.version
  for (const c of servers.compat) {
    if (c.server_id !== srv.id) continue
    const level = c.level
    const label = `✓ ${c.core_version}`
    const title = c.reasons?.length
      ? c.reasons.map((r) => `${r.code}: ${r.message}`).join('\n')
      : `完全兼容 ${c.core_version}`
    if (level === 'bad') continue
    if (activeVer && c.core_version === activeVer) {
      out.unshift({ ver: c.core_version, level, label: `▶ ${c.core_version}`, title })
    } else {
      out.push({ ver: c.core_version, level, label, title })
    }
  }
  return out
}

async function onSelect(srv: Server) {
  await servers.select(srv.id)
  ElMessage.success('已选择：' + srv.name)
}

// 单节点测试：同时测延迟 + 带宽
async function onTestOne(srv: Server) {
  // 先测延迟
  await servers.testOne(srv.id)
  // 再测带宽
  try {
    const mbps = await servers.testBandwidthOne(srv.id)
    if (mbps != null && mbps > 0) {
      const lat = servers.servers.find(s => s.id === srv.id)?.last_latency_ms
      const latStr = lat != null ? `${lat}ms` : '-'
      ElMessage.success(`${srv.name}: ${latStr} / ${mbps.toFixed(1)} Mbps`)
    }
  } catch {
    // 带宽测试失败也正常（可能内核未运行），不弹错误
  }
}

// 全部测试：先批量测延迟，再测当前节点带宽
async function onTestAll() {
  testingAll.value = true
  try {
    await servers.testAll()
    // 延迟测完后再测当前活跃节点带宽
    await servers.testBandwidthAll()
    const current = servers.current
    if (current?.last_bandwidth_mbps && current.last_bandwidth_mbps > 0) {
      ElMessage.success(`全部测试完成 · 当前节点带宽: ${current.last_bandwidth_mbps.toFixed(1)} Mbps`)
    } else {
      ElMessage.success('延迟测试完成（带宽需启动内核后测试）')
    }
  } finally {
    testingAll.value = false
  }
}

async function onDelete(srv: Server) {
  try {
    await ElMessageBox.confirm(
      t('servers.confirmDelete', { name: srv.name }),
      '确认',
      { type: 'warning' },
    )
  } catch { return }
  await servers.remove(srv.id)
  ElMessage.success('已删除')
}

async function onImport() {
  if (!importText.value.trim()) {
    ElMessage.warning('内容为空')
    return
  }
  importing.value = true
  try {
    await servers.importText(importText.value)
    showImport.value = false
    importText.value = ''
    if (!servers.selectedId && servers.servers.length > 0) {
      await servers.select(servers.servers[0].id)
    }
  } finally {
    importing.value = false
  }
}

function onEdit(srv: Server) {
  Object.assign(editForm, srv)
  showEdit.value = true
}

async function onSaveEdit() {
  if (!editForm.name.trim()) {
    ElMessage.warning('名称必填')
    return
  }
  await servers.update({ ...editForm })
  showEdit.value = false
  ElMessage.success('已保存')
}
</script>

<style scoped>
.server-card.selected {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 8%, var(--bg-soft));
}
.test-badges {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 120px;
  justify-content: flex-end;
}
.search-input {
  width: 220px;
  margin-right: 8px;
}
.proto-filter {
  width: 130px;
}
@media (max-width: 768px) {
  .server-card {
    flex-wrap: wrap !important;
  }
  .test-badges {
    min-width: unset;
    width: 100%;
    justify-content: flex-start;
  }
  .search-input {
    width: 100%;
    margin-right: 0;
    margin-bottom: 4px;
  }
  .proto-filter {
    width: 100%;
  }
}
</style>
