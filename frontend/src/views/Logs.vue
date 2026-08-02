<template>
  <div>
    <div class="toolbar">
      <h2 class="page-title" style="margin:0;">{{ t('logs.title') }}</h2>
      <div class="spacer"></div>
      <el-radio-group v-model="filter" size="small">
        <el-radio-button value="all">全部</el-radio-button>
        <el-radio-button value="warn">仅警告</el-radio-button>
        <el-radio-button value="err">仅错误</el-radio-button>
      </el-radio-group>
      <el-checkbox v-model="autoScroll">{{ t('logs.autoScroll') }}</el-checkbox>
      <el-button @click="runtime.clearLog()" size="small" plain>{{ t('logs.clear') }}</el-button>
    </div>

    <div class="log-view" ref="box">
      <div
        v-for="(l, i) in filteredLogs"
        :key="i"
        :class="['log-line', l.tag]"
      >{{ l.line }}</div>
      <div v-if="filteredLogs.length === 0" class="log-empty">
        <div v-if="runtime.logs.length === 0" class="empty-icon">📜</div>
        <div v-if="runtime.logs.length === 0" class="empty-title">暂无日志</div>
        <div v-else class="empty-title">当前过滤下无日志</div>
        <div class="empty-desc" v-if="runtime.logs.length > 0">共 {{ runtime.logs.length }} 条被隐藏</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRuntimeStore } from '@/stores/runtime'

const { t } = useI18n()
const runtime = useRuntimeStore()
const autoScroll = ref(true)
const filter = ref<'all' | 'warn' | 'err'>('all')
const box = ref<HTMLElement | null>(null)

const filteredLogs = computed(() => {
  if (filter.value === 'all') return runtime.logs
  return runtime.logs.filter((l) => l.tag === filter.value || l.tag === 'dim')
})

watch(() => filteredLogs.value.length, async () => {
  if (!autoScroll.value) return
  await nextTick()
  const el = box.value
  if (el) el.scrollTop = el.scrollHeight
})
</script>

<style scoped>
.log-empty {
  text-align: center;
  padding: 48px 20px;
}
.empty-icon {
  font-size: 36px;
  margin-bottom: 10px;
  opacity: 0.5;
}
.empty-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-mute);
}
.empty-desc {
  font-size: 12px;
  color: var(--text-mute);
  margin-top: 4px;
  opacity: 0.7;
}
@media (max-width: 768px) {
  .toolbar { flex-wrap: wrap; gap: 6px; }
}
</style>