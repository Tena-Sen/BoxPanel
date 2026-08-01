<template>
  <el-tooltip :content="tooltipText" placement="top" :disabled="props.mbps == null || props.mbps <= 0">
    <span :class="['bandwidth', cls]">{{ text }}</span>
  </el-tooltip>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ mbps?: number | null }>()

const text = computed(() => {
  if (props.mbps == null) return '-'
  if (props.mbps >= 1000) return `${(props.mbps / 1000).toFixed(1)}G`
  if (props.mbps >= 1) return `${props.mbps.toFixed(1)}M`
  return `${(props.mbps * 1000).toFixed(0)}K`
})

const cls = computed(() => {
  if (props.mbps == null || props.mbps <= 0) return 'none'
  if (props.mbps >= 50) return 'good'
  if (props.mbps >= 10) return 'med'
  return 'bad'
})

const tooltipText = computed(() => {
  if (props.mbps == null || props.mbps <= 0) return ''
  const mbps = props.mbps.toFixed(1)
  const label = cls.value === 'good' ? '优秀' : cls.value === 'med' ? '一般' : '较慢'
  return `下载带宽: ${mbps} Mbps (${label})`
})
</script>

<style scoped>
.bandwidth {
  font-size: 12px;
  font-weight: 500;
  min-width: 48px;
  text-align: right;
  display: inline-block;
  cursor: default;
}
.bandwidth.good { color: #67c23a; }
.bandwidth.med { color: #e6a23c; }
.bandwidth.bad { color: #f56c6c; }
.bandwidth.none { color: var(--text-mute); }
</style>
