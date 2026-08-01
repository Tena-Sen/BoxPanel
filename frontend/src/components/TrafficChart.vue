<template>
  <v-chart :option="option" :autoresize="true" style="height: 240px;" />
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import {
  GridComponent, TooltipComponent, LegendComponent, DataZoomComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent])

const props = defineProps<{
  upBps: number
  downBps: number
  upTotal: number
  downTotal: number
}>()

const samples = ref<Array<{ t: number; up: number; down: number }>>([])
let lastUp = 0
let lastDown = 0
let lastT = 0

function pushSample() {
  const now = Date.now()
  // 第一次：记录基准
  if (lastT === 0) {
    lastUp = props.upTotal
    lastDown = props.downTotal
    lastT = now
    return
  }
  const dt = (now - lastT) / 1000
  if (dt < 0.5) return
  const upRate = Math.max(0, (props.upTotal - lastUp) / dt)
  const downRate = Math.max(0, (props.downTotal - lastDown) / dt)
  lastUp = props.upTotal
  lastDown = props.downTotal
  lastT = now
  samples.value.push({ t: now, up: upRate, down: downRate })
  if (samples.value.length > 120) samples.value.shift()
}

let timer: any
onMounted(() => { timer = setInterval(pushSample, 1000) })
onBeforeUnmount(() => clearInterval(timer))
watch(() => [props.upTotal, props.downTotal], pushSample, { deep: false })

const option = computed(() => {
  const xs = samples.value.map((s) => new Date(s.t).toLocaleTimeString())
  return {
    backgroundColor: 'transparent',
    textStyle: { color: '#cdd6f4' },
    grid: { left: 50, right: 20, top: 30, bottom: 30 },
    legend: { textStyle: { color: '#cdd6f4' }, top: 5 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: xs, axisLine: { lineStyle: { color: '#45475a' } } },
    yAxis: {
      type: 'value', axisLine: { lineStyle: { color: '#45475a' } },
      splitLine: { lineStyle: { color: '#313244' } },
      axisLabel: { formatter: (v: number) => formatBytes(v) + '/s' },
    },
    series: [
      { name: 'Down', type: 'line', smooth: true, showSymbol: false,
        data: samples.value.map((s) => s.down), lineStyle: { color: '#89b4fa' }, areaStyle: { opacity: 0.2 } },
      { name: 'Up', type: 'line', smooth: true, showSymbol: false,
        data: samples.value.map((s) => s.up), lineStyle: { color: '#a6e3a1' }, areaStyle: { opacity: 0.2 } },
    ],
  }
})

function formatBytes(n: number): string {
  if (n < 1024) return `${n.toFixed(0)}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}K`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)}M`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)}G`
}
</script>