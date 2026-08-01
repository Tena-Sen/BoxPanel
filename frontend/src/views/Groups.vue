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

    <div v-if="groups.groups.length === 0" class="card" style="text-align:center;padding:40px;">
      <div class="muted" style="margin-bottom:12px;">还没有分组</div>
      <el-button type="primary" @click="onCreate">创建第一个分组</el-button>
      <div class="muted" style="margin-top:12px;font-size:12px;">
        提示：分组是 v2rayN 风格的核心能力。在分组内可手动切换或自动选最低延迟。
      </div>
    </div>

    <div v-for="g in groups.groups" :key="g.id" class="card" style="margin-bottom:12px;">
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;">
        <span style="font-weight:600;font-size:16px;">{{ g.name }}</span>
        <el-tag size="small" :type="typeTag(g.type)">{{ typeLabel(g.type) }}</el-tag>
        <span v-if="g.url" class="muted" style="font-size:12px;">{{ g.url }}</span>
        <div class="spacer"></div>
        <el-button size="small" @click="onEdit(g)">编辑</el-button>
        <el-button size="small" type="danger" @click="onDelete(g)">删除</el-button>
      </div>

      <div v-if="g.server_ids.length === 0" class="muted" style="padding:12px;">空分组 - 点编辑添加节点</div>
      <div v-else style="display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:8px;">
        <div
          v-for="sid in g.server_ids"
          :key="sid"
          :class="['member-card', { active: groups.nowOf(groupTag(g)) === srvTag(sid) }]"
          @click="onSwitch(g, sid)"
        >
          <div style="display:flex;align-items:center;gap:6px;">
            <el-icon v-if="groups.nowOf(groupTag(g)) === srvTag(sid)" style="color:var(--green);">
              <Check />
            </el-icon>
            <span style="flex:1;font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
              {{ serverName(sid) }}
            </span>
          </div>
          <div class="muted" style="font-size:12px;margin-top:4px;">
            {{ serverMeta(sid) }}
          </div>
          <div style="margin-top:6px;">
            <LatencyBadge :ms="serverLatency(sid)" />
          </div>
        </div>
      </div>

      <div v-if="g.type === 'url_test' || g.type === 'fallback'" class="muted" style="font-size:12px;margin-top:8px;">
        {{ g.type === 'url_test' ? '自动选择最低延迟的成员' : '按顺序尝试，第一个可用即用' }}
        <span v-if="g.interval"> · 测速间隔 {{ g.interval }}s</span>
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
  // 切换节点不需要启动核心——Clash API 改 selector 立即生效
  // 核心未运行时 Clash API 不可用，selectMember 会自然报错提示
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
.member-card {
  background: var(--bg-mute);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 0.15s;
}
.member-card:hover {
  border-color: var(--accent);
}
.member-card.active {
  border-color: var(--green);
  background: color-mix(in srgb, var(--green) 8%, var(--bg-mute));
}
</style>