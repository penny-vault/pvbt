<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import echarts from '../echarts.js'
import { formatCurrencyCompact } from '../format.js'

const props = defineProps({
  percentiles: {
    type: Object,
    required: true,
    // { p5: [{time, value}], p25: [...], p50: [...], p75: [...], p95: [...] }
  },
  actual: {
    type: Array,
    default: null,
    // [{time, value}]
  },
  yAxisFormat: {
    type: String,
    default: 'currency',
    validator: (val) => ['currency', 'percent'].includes(val),
  },
  title: { type: String, default: '' },
  height: { type: Number, default: 350 },
})

const chartRef = ref(null)
let chart = null

const bandColors = {
  outer: 'rgba(55, 138, 221, 0.1)',
  inner: 'rgba(55, 138, 221, 0.25)',
}

function toDataPairs(arr) {
  return arr.map((pt) => [pt.time, pt.value])
}

function buildOption() {
  const series = [
    // P5 baseline (invisible, just for stacking)
    {
      name: 'P5',
      type: 'line',
      data: toDataPairs(props.percentiles.p5),
      lineStyle: { opacity: 0 },
      symbol: 'none',
      stack: 'band',
      areaStyle: { opacity: 0 },
    },
    // P5-P25 band (outer)
    {
      name: 'P5-P25',
      type: 'line',
      data: props.percentiles.p25.map((pt, ii) => [pt.time, pt.value - props.percentiles.p5[ii].value]),
      lineStyle: { opacity: 0 },
      symbol: 'none',
      stack: 'band',
      areaStyle: { color: bandColors.outer },
    },
    // P25-P50 band (inner)
    {
      name: 'P25-P50',
      type: 'line',
      data: props.percentiles.p50.map((pt, ii) => [pt.time, pt.value - props.percentiles.p25[ii].value]),
      lineStyle: { opacity: 0 },
      symbol: 'none',
      stack: 'band',
      areaStyle: { color: bandColors.inner },
    },
    // P50-P75 band (inner)
    {
      name: 'P50-P75',
      type: 'line',
      data: props.percentiles.p75.map((pt, ii) => [pt.time, pt.value - props.percentiles.p50[ii].value]),
      lineStyle: { opacity: 0 },
      symbol: 'none',
      stack: 'band',
      areaStyle: { color: bandColors.inner },
    },
    // P75-P95 band (outer)
    {
      name: 'P75-P95',
      type: 'line',
      data: props.percentiles.p95.map((pt, ii) => [pt.time, pt.value - props.percentiles.p75[ii].value]),
      lineStyle: { opacity: 0 },
      symbol: 'none',
      stack: 'band',
      areaStyle: { color: bandColors.outer },
    },
    // P50 median line (on top)
    {
      name: 'Median (P50)',
      type: 'line',
      data: toDataPairs(props.percentiles.p50),
      lineStyle: { width: 2, color: '#378ADD' },
      itemStyle: { color: '#378ADD' },
      symbol: 'none',
      z: 10,
    },
  ]

  // Optional actual/historical overlay
  if (props.actual && props.actual.length > 0) {
    series.push({
      name: 'Actual',
      type: 'line',
      data: toDataPairs(props.actual),
      lineStyle: { width: 2, color: '#D85A30' },
      itemStyle: { color: '#D85A30' },
      symbol: 'none',
      z: 11,
    })
  }

  return {
    title: props.title ? { text: props.title, textStyle: { fontSize: 14, fontWeight: 600, color: '#1a1a1a' } } : undefined,
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#fff',
      borderColor: '#ddd',
      borderWidth: 1,
      textStyle: { fontSize: 12, color: '#1a1a1a' },
    },
    legend: {
      show: true,
      top: 0,
      data: props.actual ? ['Median (P50)', 'Actual'] : ['Median (P50)'],
      textStyle: { fontSize: 11, color: '#999' },
    },
    grid: {
      left: 80,
      right: 20,
      top: 30,
      bottom: 30,
    },
    xAxis: {
      type: 'time',
      axisLabel: { fontSize: 10, color: '#999' },
      axisLine: { lineStyle: { color: '#ddd' } },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        fontSize: 10,
        color: '#999',
        formatter: props.yAxisFormat === 'currency'
          ? (val) => formatCurrencyCompact(val)
          : (val) => `${(val * 100).toFixed(1)}%`,
      },
      splitLine: { lineStyle: { color: 'rgba(0,0,0,0.06)' } },
    },
    series,
  }
}

onMounted(() => {
  if (chartRef.value) {
    chart = echarts.init(chartRef.value)
    chart.setOption(buildOption())
    window.addEventListener('resize', handleResize)
  }
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
})

watch(() => [props.percentiles, props.actual], () => {
  chart?.setOption(buildOption(), true)
}, { deep: true })

function handleResize() {
  chart?.resize()
}
</script>

<template>
  <div>
    <div ref="chartRef" :style="{ width: '100%', height: `${height}px` }"></div>
  </div>
</template>
