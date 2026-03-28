<script setup>
import { computed } from 'vue'

const props = defineProps({
  data: { type: Object, required: true },
})

// Transform fan chart data for PercentileFan component
const percentiles = computed(() => {
  const fc = props.data.fanChart
  if (!fc || !fc.times) return null

  const toPoints = (values) =>
    fc.times.map((tm, idx) => ({ time: tm, value: values[idx] }))

  return {
    p5: toPoints(fc.p5),
    p25: toPoints(fc.p25),
    p50: toPoints(fc.p50),
    p75: toPoints(fc.p75),
    p95: toPoints(fc.p95),
  }
})

const actualSeries = computed(() => {
  const ac = props.data.fanChart?.actual
  if (!ac || !ac.times) return null
  return ac.times.map((tm, idx) => ({ time: tm, value: ac.values[idx] }))
})

const wealthColumns = [
  { key: 'label', header: 'Statistic', type: 'string' },
  { key: 'value', header: 'Value', type: 'currency' },
]

const confidenceColumns = [
  { key: 'metric', header: 'Metric', type: 'string' },
  { key: 'p5', header: 'P5', type: 'ratio' },
  { key: 'p25', header: 'P25', type: 'ratio' },
  { key: 'p50', header: 'P50', type: 'ratio' },
  { key: 'p75', header: 'P75', type: 'ratio' },
  { key: 'p95', header: 'P95', type: 'ratio' },
]

const ruinMetrics = computed(() => {
  const ruin = props.data.ruin
  if (!ruin) return []
  return [
    { label: 'Probability of Ruin', value: ruin.probability, format: 'percent' },
    { label: 'Ruin Threshold', value: ruin.threshold, format: 'percent' },
    { label: 'Median Max Drawdown', value: ruin.medianDrawdown, format: 'percent' },
  ]
})

const rankMetrics = computed(() => {
  const rank = props.data.historicalRank
  if (!rank) return []
  return [
    { label: 'Terminal Value Percentile', value: rank.terminalValuePercentile, format: 'percent' },
    { label: 'TWRR Percentile', value: rank.twrrPercentile, format: 'percent' },
    { label: 'Max Drawdown Percentile', value: rank.maxDrawdownPercentile, format: 'percent' },
    { label: 'Sharpe Percentile', value: rank.sharpePercentile, format: 'percent' },
  ]
})
</script>

<template>
  <div class="max-w-[900px] mx-auto p-10 font-sans text-foreground text-sm leading-relaxed">
    <h1 class="text-[28px] font-semibold mb-8">Monte Carlo Simulation</h1>

    <PercentileFan
      v-if="percentiles"
      title="Equity Curve Distribution"
      :percentiles="percentiles"
      :actual="actualSeries"
      yAxisFormat="currency"
      :height="350"
    />

    <FinancialTable
      title="Terminal Wealth Distribution"
      :columns="wealthColumns"
      :rows="data.terminalWealth || []"
      :sortable="false"
    />

    <FinancialTable
      title="Confidence Intervals"
      :columns="confidenceColumns"
      :rows="data.confidenceIntervals || []"
      :sortable="false"
    />

    <div class="mt-12">
      <h2 class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mb-3">Probability of Ruin</h2>
      <MetricCards :metrics="ruinMetrics" />
    </div>

    <div v-if="data.historicalRank" class="mt-12">
      <h2 class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mb-3">Historical Rank</h2>
      <MetricCards :metrics="rankMetrics" />
    </div>

    <div v-if="data.summary" class="mt-12">
      <h2 class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mb-3">Summary</h2>
      <p class="whitespace-pre-line text-detail text-muted">{{ data.summary }}</p>
    </div>
  </div>
</template>
