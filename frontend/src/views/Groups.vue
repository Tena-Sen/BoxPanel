<template>
  <div>
    <h2 class="page-title">代理分组</h2>

    <el-alert
      v-if="!runtime.running"
      type="info"
      :closable="false"
      style="margin-bottom:12px;"
      show-icon
    >
      <template #title>
        <span>核心未运行。点节点卡片会自动启动并切换。</span>
      </template>
    </el-alert>

    <div class="toolbar">
      <el-button type="primary" @click="onCreate">+ 新建分组</el-button>
      <el-button @click="groups.load()">↻ 刷新</el-button>
      <div class="spacer"></div>
      <el-tag v-if="!runtime.running" type="warning" size="small">核心未运行</el-tag>
      <el-tag v-else-if="!runtime.clashReachable" type="info" size="small">Clash API 未就绪</el-tag>
      <el-tag v-else type="success" size="small">实时可用</el-tag>
    </div>

    <!-- 空状态 -->
    <div v-if="groups.groups.length === 0" class="empty-state">
      <div class="empty-icon">📂</div>
      <div class="empty-title">还没有分组</div>
      <div class="empty-desc">分组是 v2rayN 风格的核心能力。在分组内可手动切换或自动选最低延迟。</div>
      <el-button type="primary" @click="onCreate" style="margin-top:16px;">+ 创建第一个分组</el-button>
    </div>

    <!-- 分组列表 -->
    <div v-for="g in groups.groups" :key="g.id" class="card group-card">
      <div class="group-header">
        <span class="group-name">{{ g.name }}</span>
        <el-tag size="small" :type="typeTag(g.type)">{{ typeLabel(g.type) }}</el-tag>
        <span v-if="g.url" class="muted" style="font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:300px;">{{ g.url }}</span>
        <div class="spacer"></div>
        <el-button size="small" text @click="onEdit(g)">✎ 编辑</el-button>
        <el-button size="small" text type="danger" @click="onDelete(g)">✕ 删除</el-button>
      </div>

      <div v-if="g.server_ids.length === 0" class="empty-members">
        <span class="muted">空分组</span>
        <el-button size="small" link type="primary" @click="onEdit(g)" style="margin-left:8px;">添加节点</el-button>
      </div>
      <div v-else class="members-grid">
        <div
          v-for="sid in g.server_ids"
          :key="sid"
          :class="['member-card', { active: groups.nowOf(groupTag(g)) === srvTag(sid), disabled: g.type !== 'selector' }]"
          @click="onSwitch(g, sid)"
        >
          <div class="member-top">
            <span v-if="groups.nowOf(groupTag(g)) === srvTag(sid)" class="member-active-dot"></span>
            <span class="member-name">{{ serverName(sid) }}</span>
          </div>
          <div class="member-meta">{{ serverMeta(sid) }}</div>
          <div style="margin-top:6px;">
            <LatencyBadge :ms="serverLatency(sid)" />
          </div>
        </div>
      </div>

      <div v-if="g.type === 'url_test' || g.type === 'fallback'" class="group-hint">
        {{ g.type === 'url_test' ? '自动选择最低延迟的成员' : '按顺序尝试，第一个可用即用' }}
        <span v-if="g.interval"> · 测速间隔 {{ g.interval }}s</span>
        <span class="hint-non-switchable">不支持手动切换</span>
      </div>
    </div>

    <!-- 创建/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑分组' : '新建分组'" width="560px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="我的分组" />
        </el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio value="selector">手动选择</el-radio>
            <el-radio value="url_test">自动选最优</el-radio>
            <el-radio value="fallback">故障转移</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.type === 'url_test' || form.type === 'fallback'" label="测速 URL">
          <el-input v-model="form.url" placeholder="http://www.gstatic.com/generate_204" />
        </el-form-item>
        <el-form-item v-if="form.type === 'url_test'" label="测速间隔(秒)">
          <el-input-number v-model="form.interval" :min="60" :max="3600" />
        </el-form-item>
        <el-form-item label="成员节点">
          <el-select
            v-model="form.server_ids"
            multiple
            filterable
            placeholder="选择节点"
            style="width:100%;"
            :max-collapse-tags="3"
          >
            <el-option
              v-for="s in servers.servers"
              :key="s.id"
              :label="`${s.name} (${s.server}:${s.server_port})`"
              :value="s.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="onSave">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Check } from '@element-plus/icons-vue'
import { useGroupsStore } from '@/stores/groups'
import { useServersStore } from '@/stores/servers'
import { useRuntimeStore } from '@/stores/runtime'
import LatencyBadge from '@/components/LatencyBadge.vue'
import type { Group } from '@/api/types'

const groups = useGroupsStore()
const servers = useServersStore()
const runtime = useRuntimeStore()

const dialogVisible = ref(false)
const form = reactive<Group>({
  id: '',
  name: '',
  type: 'selector',
  url: '',
  interval: 300,
  server_ids: [],
})

const groupTag = (g: Group) => `grp-${g.id}`
const srvTag = (sid: string) => `srv-${sid}`

function typeLabel(t: string) {
  return { selector: '手动', url_test: '自动', fallback: '故障转移', load_balance: '负载均衡' }[t] || t
}
function typeTag(t: string) {
  return ({ selector: 'primary', url_test: 'success', fallback: 'warning', load_balance: 'info' } as any)[t] || ''
}

function serverName(sid: string) {
  return servers.servers.find((s) => s.id === sid)?.name || sid
}
function serverMeta(sid: string) {
  const s = servers.servers.find((x) => x.id === sid)
  if (!s) return ''
  return `${s.protocol} · ${s.server}:${s.server_port}`
}
function serverLatency(sid: string) {
  return servers.servers.find((s) => s.id === sid)?.last_latency_ms
}

async function onCreate() {
  Object.assign(form, { id: '', name: '', type: 'selector', url: '', interval: 300, server_ids: [] })
  dialogVisible.value = true
}

async function onEdit(g: Group) {
  Object.assign(form, g)
  dialogVisible.value = true
}

async function onDelete(g: Group) {
  try {
    await ElMessageBox.confirm(`删除分组 "${g.name}"？`, '确认', { type: 'warning' })
  } catch { return }
  await groups.remove(g.id)
  ElMessage.success('已删除')
}

async function onSave() {
  if (!form.name.trim()) {
    ElMessage.warning('请填名称')
    return
  }
  if (form.server_ids.length === 0) {
    ElMessage.warning('请至少选一个节点')
    return
  }
  try {
    if (form.id) {
      await groups.update({ ...form })
    } else {
      await groups.add({ ...form })
    }
    dialogVisible.value = false
    ElMessage.success('已保存')
  } catch (e: any) {
    ElMessage.error('保存失败：' + (e?.message || String(e)))
  }
}

async function onSwitch(g: Group, sid: string) {
  // 只有 selector 类型支持手动切换
  if (g.type !== 'selector') {
    ElMessage.warning(`${typeLabel(g.type)}分组不支持手动切换，仅「手动选择」类型可切换`)
    return
  }
  await groups.selectMember(groupTag(g), srvTag(sid))
}

onMounted(async () => {
  await groups.load()
  groups.startProxyPolling(3000)
})
onBeforeUnmount(() => {
  groups.stopProxyPolling()
})
</script>

<style scoped>
/* Empty state */
.empty-state {
  text-align: center;
  padding: 48px 20px;
  background: var(--bg-soft);
  border: 1px dashed var(--border);
  border-radius: var(--radius);
}
.empty-icon {
  font-size: 40px;
  margin-bottom: 12px;
  opacity: 0.6;
}
.empty-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 6px;
}
.empty-desc {
  font-size: 13px;
  color: var(--text-mute);
  max-width: 360px;
  margin: 0 auto;
}

/* Group card */
.group-card {
  margin-bottom: 12px;
  transition: all var(--transition);
}
.group-card:hover {
  box-shadow: var(--shadow);
}
.group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.group-name {
  font-weight: 600;
  font-size: 16px;
  color: var(--text);
}
.empty-members {
  padding: 16px;
  text-align: center;
  color: var(--text-mute);
  font-size: 13px;
  background: var(--bg-mute);
  border-radius: var(--radius-sm);
}
.members-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 8px;
}
.member-card {
  background: var(--bg-mute);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 12px 14px;
  cursor: pointer;
  transition: all var(--transition);
}
.member-card:hover {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent-soft);
}
.member-card.active {
  border-color: var(--green);
  background: color-mix(in srgb, var(--green) 8%, var(--bg-mute));
  box-shadow: 0 0 0 1px var(--green-soft);
}
.member-top {
  display: flex;
  align-items: center;
  gap: 8px;
}
.member-active-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--green);
  box-shadow: 0 0 6px var(--green);
  flex-shrink: 0;
}
.member-name {
  flex: 1;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.member-meta {
  font-size: 12px;
  color: var(--text-mute);
  margin-top: 4px;
}
.group-hint {
  font-size: 12px;
  color: var(--text-mute);
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}
.hint-non-switchable {
  color: var(--yellow);
  font-weight: 500;
  margin-left: 8px;
}
.member-card.disabled {
  opacity: 0.55;
  cursor: default;
}
.member-card.disabled:hover {
  border-color: var(--border);
  box-shadow: none;
}

@media (max-width: 768px) {
  .toolbar { flex-wrap: wrap; }
  .members-grid {
    grid-template-columns: 1fr;
  }
  .member-card { padding: 10px; }
  .group-header { flex-wrap: wrap; }
}
</style>