<template>
  <div>
    <h2 class="page-title">{{ t('subs.title') }}</h2>

    <div class="toolbar">
      <el-button type="primary" @click="showAdd = true">+ {{ t('subs.add') }}</el-button>
      <el-button @click="onRefreshAll">↻ {{ t('subs.refreshAll') }}</el-button>
    </div>

    <div v-if="subs.subs.length === 0" class="empty-state">
      <div class="empty-icon">📡</div>
      <div class="empty-title">还没有订阅</div>
      <div class="empty-desc">添加订阅源可自动导入和更新节点列表</div>
      <el-button type="primary" @click="showAdd = true" style="margin-top:16px;">+ 添加订阅</el-button>
    </div>

    <div v-else class="card" style="padding:0;overflow:hidden;">
      <div class="table-wrap">
      <el-table :data="subs.subs" stripe>
        <el-table-column prop="name" :label="t('subs.name')" width="160">
          <template #default="{ row }">
            <span style="font-weight:500;">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="url" :label="t('subs.url')" show-overflow-tooltip>
          <template #default="{ row }">
            <a :href="row.url" target="_blank" rel="noopener" class="sub-link">{{ row.url }}</a>
          </template>
        </el-table-column>
        <el-table-column prop="interval_hours" :label="t('subs.interval')" width="120" align="center">
          <template #default="{ row }">
            <span class="muted">{{ row.interval_hours }}h</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('subs.lastRefresh')" width="180">
          <template #default="{ row }">
            <span class="muted">{{ row.last_refresh || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="160" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.last_status === 'ok'" type="success" size="small">{{ row.server_count || 0 }} nodes</el-tag>
            <el-tag v-else-if="row.last_status" type="danger" size="small" :title="row.last_status">error</el-tag>
            <el-tag v-else type="info" size="small">—</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" align="right">
          <template #default="{ row }">
            <el-button size="small" text @click="onRefresh(row)">↻ 刷新</el-button>
            <el-button size="small" text type="danger" @click="onDelete(row)">✕</el-button>
          </template>
        </el-table-column>
      </el-table>
      </div>
    </div>

    <el-dialog v-model="showAdd" :title="t('subs.add')" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item :label="t('subs.name')">
          <el-input v-model="form.name" placeholder="my-sub" />
        </el-form-item>
        <el-form-item :label="t('subs.url')">
          <el-input v-model="form.url" placeholder="https://..." />
        </el-form-item>
        <el-form-item :label="t('subs.ua')">
          <el-input v-model="form.user_agent" placeholder="clash-meta" />
        </el-form-item>
        <el-form-item :label="t('subs.interval')">
          <el-input-number v-model="form.interval_hours" :min="0" :max="168" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAdd = false">取消</el-button>
        <el-button type="primary" @click="onAdd">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useSubscriptionsStore } from '@/stores/subscriptions'
import type { Subscription } from '@/api/types'

const { t } = useI18n()
const subs = useSubscriptionsStore()

const showAdd = ref(false)
const form = reactive({ name: '', url: '', user_agent: '', interval_hours: 24 })

async function onAdd() {
  if (!form.url) {
    ElMessage.warning('URL 必填')
    return
  }
  await subs.add(form)
  showAdd.value = false
  Object.assign(form, { name: '', url: '', user_agent: '', interval_hours: 24 })
  ElMessage.success('已添加')
}

async function onRefresh(row: Subscription) {
  await subs.refresh(row.id)
}

async function onRefreshAll() {
  for (const s of subs.subs) {
    await subs.refresh(s.id)
  }
}

async function onDelete(row: Subscription) {
  try {
    await ElMessageBox.confirm(t('subs.confirmDelete', { name: row.name }), '确认', { type: 'warning' })
  } catch { return }
  await subs.remove(row.id)
  ElMessage.success('已删除')
}
</script>

<style scoped>
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
}
.sub-link {
  font-size: 12px;
  font-family: ui-monospace, Consolas, monospace;
}
@media (max-width: 768px) {
  .toolbar { flex-wrap: wrap; }
}
</style>