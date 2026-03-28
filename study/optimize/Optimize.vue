<script setup>
const props = defineProps({
  data: { type: Object, required: true },
})

const rankingColumns = [
  { key: 'rank', header: 'Rank', type: 'number' },
  { key: 'parameters', header: 'Parameters', type: 'string' },
  { key: 'meanOOS', header: 'Mean OOS', type: 'ratio' },
  { key: 'meanIS', header: 'Mean IS', type: 'ratio' },
  { key: 'oosStdDev', header: 'OOS StdDev', type: 'ratio' },
]

const foldColumns = [
  { key: 'foldName', header: 'Fold', type: 'string' },
  { key: 'isScore', header: 'IS Score', type: 'ratio' },
  { key: 'oosScore', header: 'OOS Score', type: 'ratio' },
]

const overfitColumns = [
  { key: 'parameters', header: 'Parameters', type: 'string' },
  { key: 'meanIS', header: 'Mean IS', type: 'ratio' },
  { key: 'meanOOS', header: 'Mean OOS', type: 'ratio' },
  { key: 'degradation', header: 'Degradation', type: 'percent' },
]
</script>

<template>
  <div class="max-w-[900px] mx-auto p-10 font-sans text-foreground text-sm leading-relaxed">
    <h1 class="text-[28px] font-semibold mb-8">Parameter Optimization</h1>

    <FinancialTable
      :title="'Rankings by Mean OOS ' + (data.objectiveName || '')"
      :columns="rankingColumns"
      :rows="data.rankings || []"
    />

    <FinancialTable
      v-if="data.bestDetail"
      :title="'Best Combination: ' + data.bestDetail.parameters"
      :columns="foldColumns"
      :rows="data.bestDetail.folds || []"
      :sortable="false"
    />

    <FinancialTable
      :title="'Overfitting Check: IS vs OOS ' + (data.objectiveName || '')"
      :columns="overfitColumns"
      :rows="data.overfitting || []"
    />

    <div v-if="data.equityCurves && data.equityCurves.length > 0" class="mt-12">
      <TimeSeriesChart
        title="Top Combinations OOS Equity Curves"
        :series="data.equityCurves.filter(s => s.times && s.times.length > 0)"
        yAxisFormat="currency"
        :height="350"
      />
    </div>
  </div>
</template>
