<script setup>
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import echarts from '../echarts.js'
import { formatCurrencyCompact } from '../format.js'

const props = defineProps({
  series: {
    type: Array,
    required: true,
    // Each: { name, times, values, style? } where style is 'solid'|'dashed'|'area'
  },
  yAxisFormat: {
    type: String,
    default: 'currency',
    validator: (val) => ['currency', 'percent'].includes(val),
  },
  title: { type: String, default: '' },
  height: { type: Number, default: 300 },
})

const chartRef = ref(null)
let chart = null

const seriesColors = ['#378ADD', '#534AB7', '#D85A30', '#1D9E75', '#BA7517']

function buildOption() {
  const seriesData = props.series.map((ser, idx) => {
    const base = {
      name: ser.name,
      type: 'line',
      data: ser.times.map((tm, ii) => [tm, ser.values[ii]]),
      symbol: 'none',
      smooth: false,
      lineStyle: {
        width: 2,
        color: seriesColors[idx % seriesColors.length],
      },
      itemStyle: {
        color: seriesColors[idx % seriesColors.length],
      },
    }

    if (ser.style === 'dashed') {
      base.lineStyle.type = 'dashed'
    } else if (ser.style === 'area') {
      base.areaStyle = {
        opacity: 0.3,
        color: seriesColors[idx % seriesColors.length],
      }
    }

    return base
  })

  return {
    title: props.title ? { text: props.title, textStyle: { fontSize: 14, fontWeight: 600, color: '#1a1a1a' } } : undefined,
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#fff',
      borderColor: '#ddd',
      borderWidth: 1,
      textStyle: { fontSize: 12, color: '#1a1a1a' },
      axisPointer: { type: 'cross', crossStyle: { color: '#999' } },
    },
    legend: {
      show: props.series.length > 1,
      top: 0,
      textStyle: { fontSize: 11, color: '#999' },
    },
    grid: {
      left: 80,
      right: 20,
      top: props.series.length > 1 ? 30 : 10,
      bottom: 60,
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
    dataZoom: [
      { type: 'inside', start: 0, end: 100 },
      { type: 'slider', start: 0, end: 100, height: 20, bottom: 10 },
    ],
    series: seriesData,
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

watch(() => props.series, () => {
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
