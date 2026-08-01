<template>
  <span :class="['latency', cls]">{{ text }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ ms?: number | null }>()

const text = computed(() => {
  if (props.ms == null) return '-'
  if (props.ms < 1) return '<1ms'
  return `${props.ms}ms`
})

const cls = computed(() => {
  if (props.ms == null || props.ms <= 0) return 'none'
  if (props.ms < 300) return 'good'
  if (props.ms < 800) return 'med'
  return 'bad'
})
</script>