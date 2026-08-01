<template>
  <div>
    <h2 class="page-title">路由规则</h2>

    <div class="toolbar">
      <el-button type="primary" @click="onAddRule">+ 新建规则</el-button>
      <el-button @click="onForceRefresh">↻ 刷新</el-button>
      <div class="spacer"></div>
      <el-tag size="small" type="info">规则从上到下匹配，命中即生效</el-tag>
    </div>

    <!-- 规则列表 -->
    <div v-if="routing.rules.length === 0" class="card" style="text-align:center;padding:30px;">
      <div class="muted">还没有自定义规则</div>
      <div class="muted" style="font-size:12px;margin-top:6px;">
        内置规则（geosite-cn 直连 / !cn 走代理）始终生效，下方表可管理额外规则。
      </div>
    </div>

    <div
      v-for="(r, idx) in routing.rules"
      :key="r.id"
      class="card"
      style="margin-bottom:8px;display:flex;align-items:center;gap:10px;"
    >
      <div class="muted" style="width:28px;text-align:center;font-family:monospace;">{{ idx + 1 }}</div>
      <el-tag :type="typeTag(r.type)" size="small">{{ typeLabel(r.type) }}</el-tag>
      <div style="flex:1;min-width:0;">
        <div style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:monospace;font-size:13px;">
          {{ r.values.join(', ') }}
        </div>
        <div class="muted" style="font-size:11px;">
          <span v-if="r.invert">非匹配 · </span>{{ r.outbound }}
        </div>
      </div>
      <el-button size="small" :disabled="idx === 0" @click="routing.moveRule(r.id, 'up')">↑</el-button>
      <el-button size="small" :disabled="idx === routing.rules.length - 1" @click="routing.moveRule(r.id, 'down')">↓</el-button>
      <el-button size="small" @click="onEditRule(r)">✎</el-button>
      <el-button size="small" type="danger" @click="onDeleteRule(r)">✕</el-button>
    </div>

    <!-- 规则集 -->
    <div class="toolbar" style="margin-top:24px;">
      <h3 style="margin:0;font-size:16px;">规则集（GeoSite / GeoIP）</h3>
      <el-button size="small" @click="routing.refreshAll()">↻ 全部刷新</el-button>
      <el-button size="small" @click="onAddBuiltin">+ 添加内置源</el-button>
      <el-button size="small" @click="onAddCustom">+ 自定义 URL</el-button>
    </div>
    <div class="card" style="padding:0;overflow:hidden;">
      <el-table :data="statusRows" stripe>
        <el-table-column prop="tag" label="Tag" width="220" />
        <el-table-column prop="type" label="类型" width="100" />
        <el-table-column label="缓存状态" min-width="220">
          <template #default="{ row }">
            <div v-if="row.cached" style="display:flex;align-items:center;gap:6px;">
              <el-tag size="small" type="success">已缓存</el-tag>
              <span class="muted" style="font-size:12px;">{{ formatBytes(row.size) }}</span>
              <span class="muted" style="font-size:11px;" :title="`sha256: ${row.sha256}`">SHA: {{ row.sha256?.slice(0, 8) }}…</span>
            </div>
            <el-tag v-else size="small" type="warning">未下载</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="muted" style="font-family:monospace;font-size:11px;">
              {{ row.url || row.path || '—' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="下次检查" width="180">
          <template #default="{ row }">
            <span class="muted" style="font-size:12px;">{{ row.next_check || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="routing.toggleRuleSet(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" align="right">
          <template #default="{ row }">
            <el-button size="small" :disabled="!row.url" @click="routing.refreshOne(row.id)">↻</el-button>
            <el-button size="small" type="danger" @click="onDeleteRuleSet(row)">✕</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div class="muted" style="margin-top:12px;font-size:12px;">
      💡 修改规则后需重启核心生效。规则集的启用/禁用也会影响内置路由。
    </div>

    <!-- 规则编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑规则' : '新建规则'" width="560px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="匹配类型">
          <el-select v-model="form.type" style="width:100%;">
            <el-option
              v-for="t in ruleTypes"
              :key="t.value"
              :label="t.label"
              :value="t.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="匹配值">
          <el-input
            v-model="valuesText"
            type="textarea"
            :rows="4"
            :placeholder="placeholderFor(form.type)"
          />
          <div class="muted" style="font-size:11px;margin-top:4px;">每行一个</div>
        </el-form-item>
        <el-form-item label="出站">
          <el-select v-model="form.outbound" style="width:100%;">
            <el-option label="走代理 (proxy)" value="proxy" />
            <el-option label="直连 (direct)" value="direct" />
            <el-option label="阻断 (block)" value="block" />
            <el-option
              v-for="g in groups.groups"
              :key="g.id"
              :label="`分组: ${g.name}`"
              :value="`grp-${g.id}`"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="反向匹配">
          <el-switch v-model="form.invert" />
          <span class="muted" style="margin-left:8px;font-size:12px;">命中时不走该出站</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="onSaveRule">保存</el-button>
      </template>
    </el-dialog>

    <!-- 内置源对话框 -->
    <el-dialog v-model="builtinVisible" title="添加内置规则集" width="600px">
      <el-table :data="builtinList" @selection-change="onBuiltinSel" max-height="400">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="tag" label="Tag" />
        <el-table-column label="URL" show-overflow-tooltip>
          <template #default="{ row }">
            <span style="font-family:monospace;font-size:11px;color:var(--text-mute);">{{ row.url }}</span>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="builtinVisible = false">取消</el-button>
        <el-button type="primary" :disabled="builtinSel.length === 0" @click="onConfirmBuiltin">
          添加选中 ({{ builtinSel.length }})
        </el-button>
      </template>
    </el-dialog>

    <!-- 自定义 URL 对话框 -->
    <el-dialog v-model="customVisible" title="添加自定义规则集" width="480px">
      <el-form :model="customForm" label-width="100px">
        <el-form-item label="Tag">
          <el-input v-model="customForm.tag" placeholder="my-ruleset" />
        </el-form-item>
        <el-form-item label="URL">
          <el-input v-model="customForm.url" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="格式">
          <el-radio-group v-model="customForm.format">
            <el-radio value="binary">binary (.srs)</el-radio>
            <el-radio value="source">source (.json)</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="更新间隔(小时)">
          <el-input-number v-model="customForm.update_interval" :min="1" :max="720" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="customVisible = false">取消</el-button>
        <el-button type="primary" @click="onConfirmCustom">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRoutingStore } from '@/stores/routing'
import { useGroupsStore } from '@/stores/groups'
import type { RoutingRule, RuleSet, RuleSetStatus } from '@/api/types'

const routing = useRoutingStore()
const groups = useGroupsStore()

const dialogVisible = ref(false)
const valuesText = ref('')
const form = reactive<RoutingRule>({
  id: '', profile_id: '', order: 0,
  type: 'domain_suffix', values: [], outbound: 'proxy', invert: false,
})

// 规则集状态（独立加载，避免 rules 改动触发 status 重建）
const statusRows = ref<Array<RuleSetStatus & { enabled: boolean }>>([])
async function refreshStatus(force = false) {
  try {
    const sts = await routing.fetchStatus(force)
    const byId = new Map(routing.ruleSets.map((r) => [r.id, r]))
    statusRows.value = sts.map((s) => ({
      ...s,
      enabled: byId.get(s.id)?.enabled ?? true,
    }))
  } catch (e) {
    // ignore
  }
}

// 内置源对话框
const builtinVisible = ref(false)
const builtinList = ref<RuleSet[]>([])
const builtinSel = ref<RuleSet[]>([])
async function onAddBuiltin() {
  try {
    builtinList.value = await routing.fetchBuiltin()
    builtinSel.value = []
    builtinVisible.value = true
  } catch (e: any) {
    ElMessage.error('加载内置源失败：' + (e?.message || String(e)))
  }
}
function onBuiltinSel(rows: RuleSet[]) {
  builtinSel.value = rows
}
async function onConfirmBuiltin() {
  for (const item of builtinSel.value) {
    await routing.add({ ...item, id: '' })
  }
  builtinVisible.value = false
  ElMessage.success(`已添加 ${builtinSel.value.length} 个规则集，开始自动下载…`)
  await routing.load()
  await refreshStatus()
  // 后台触发下载（让用户立即看到进度）
  for (const r of routing.ruleSets) {
    if (r.url) routing.refreshOne(r.id)
  }
}

// 自定义 URL 对话框
const customVisible = ref(false)
const customForm = reactive<RuleSet>({
  id: '', tag: '', type: 'remote', format: 'binary',
  url: '', enabled: true, update_interval: 168,
})
function onAddCustom() {
  Object.assign(customForm, {
    id: '', tag: '', type: 'remote', format: 'binary',
    url: '', enabled: true, update_interval: 168,
  })
  customVisible.value = true
}
async function onConfirmCustom() {
  if (!customForm.tag || !customForm.url) {
    ElMessage.warning('Tag 和 URL 必填')
    return
  }
  await routing.add({ ...customForm })
  customVisible.value = false
  ElMessage.success('已添加')
  await routing.load()
  await refreshStatus()
}

async function onDeleteRuleSet(r: any) {
  try {
    await ElMessageBox.confirm(`删除规则集 "${r.tag}"？`, '确认', { type: 'warning' })
  } catch { return }
  await routing.deleteRuleSet(r.id)
  ElMessage.success('已删除')
  await refreshStatus()
}

function formatBytes(n: number) {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

const ruleTypes = [
  { value: 'domain_suffix', label: '域名后缀（如 google.com 匹配所有子域）' },
  { value: 'domain', label: '完整域名（精确匹配）' },
  { value: 'domain_keyword', label: '域名关键字' },
  { value: 'domain_regex', label: '域名正则' },
  { value: 'ip_cidr', label: 'IP / CIDR（如 192.168.0.0/16）' },
  { value: 'geosite', label: 'GeoSite 类别（如 cn / category-ads）' },
  { value: 'geoip', label: 'GeoIP 类别（如 cn / private）' },
  { value: 'process', label: '进程名（如 chrome.exe）' },
  { value: 'protocol', label: '协议（dns / http / tls）' },
  { value: 'port', label: '端口（如 80 / 8000-9000）' },
]

function typeLabel(t: string) {
  return ruleTypes.find((x) => x.value === t)?.label.split('（')[0] || t
}
function typeTag(t: string) {
  const m: Record<string, string> = {
    domain_suffix: 'primary', domain: 'primary', domain_keyword: 'primary', domain_regex: 'primary',
    ip_cidr: 'success', geoip: 'success', geosite: 'success',
    process: 'warning', protocol: 'info', port: 'info',
  }
  return m[t] || ''
}
function placeholderFor(t: string) {
  const m: Record<string, string> = {
    domain_suffix: 'google.com\nyoutube.com',
    domain: 'www.google.com',
    domain_keyword: 'google',
    ip_cidr: '192.168.0.0/16\n10.0.0.0/8',
    geosite: 'cn\n category-ads',
    geoip: 'cn\n private',
    process: 'chrome.exe\n Cursor.exe',
    protocol: 'dns\n http\n tls',
    port: '80\n 443\n 8000-9000',
  }
  return m[t] || ''
}

function onAddRule() {
  Object.assign(form, {
    id: '', profile_id: '', order: routing.rules.length,
    type: 'domain_suffix', values: [], outbound: 'proxy', invert: false,
  })
  valuesText.value = ''
  dialogVisible.value = true
}

function onEditRule(r: RoutingRule) {
  Object.assign(form, r)
  valuesText.value = r.values.join('\n')
  dialogVisible.value = true
}

async function onDeleteRule(r: RoutingRule) {
  try {
    await ElMessageBox.confirm('删除这条规则？', '确认', { type: 'warning' })
  } catch { return }
  await routing.deleteRule(r.id)
  ElMessage.success('已删除')
}

async function onSaveRule() {
  const values = valuesText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
  if (values.length === 0) {
    ElMessage.warning('请至少填一个匹配值')
    return
  }
  form.values = values
  try {
    if (form.id) {
      await routing.updateRule({ ...form })
    } else {
      await routing.addRule({ ...form })
    }
    dialogVisible.value = false
    ElMessage.success('已保存，重启核心后生效')
  } catch (e: any) {
    ElMessage.error('保存失败：' + (e?.message || String(e)))
  }
}

async function onForceRefresh() {
  await routing.load(true)
  await refreshStatus(true)
}

onMounted(async () => {
  await routing.load()
  await refreshStatus()
})
</script>