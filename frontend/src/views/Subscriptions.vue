<template>
  <div>
    <h2 class="page-title">{{ t('subs.title') }}</h2>

    <div class="toolbar">
      <el-button type="primary" @click="showAdd = true">+ {{ t('subs.add') }}</el-button>
      <el-button @click="onRefreshAll">↻ {{ t('subs.refreshAll') }}</el-button>
    </div>

    <el-table :data="subs.subs" stripe>
      <el-table-column prop="name" :label="t('subs.name')" />
      <el-table-column prop="url" :label="t('subs.url')" show-overflow-tooltip>
        <template #default="{ row }">
          <a :href="row.url" target="_blank" rel="noopener">{{ row.url }}</a>
        </template>
      </el-table-column>
      <el-table-column prop="interval_hours" :label="t('subs.interval')" width="120" />
      <el-table-column :label="t('subs.lastRefresh')" width="180">
        <template #default="{ row }">
          <span class="muted">{{ row.last_refresh || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="160">
        <template #default="{ row }">
          <el-tag v-if="row.last_status === 'ok'" type="success">{{ row.server_count || 0 }} servers</el-tag>
          <el-tag v-else-if="row.last_status" type="danger" :title="row.last_status">error</el-tag>
          <el-tag v-else type="info">—</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" align="right">
        <template #default="{ row }">
          <el-button size="small" @click="onRefresh(row)">↻</el-button>
          <el-button size="small" type="danger" @click="onDelete(row)">✕</el-button>
        </template>
      </el-table-column>
    </el-table>

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