<script setup>
import { computed } from 'vue'

const props = defineProps({
  data: { type: Object, required: true },
})

const rankingColumns = [
  { key: 'runName', header: 'Run', type: 'string' },
  { key: 'scenarioName', header: 'Scenario', type: 'string' },
  { key: 'maxDrawdown', header: 'Max Drawdown', type: 'percent' },
  { key: 'totalReturn', header: 'Total Return', type: 'percent' },
  { key: 'worstDay', header: 'Worst Day', type: 'percent' },
]

const rankingRows = computed(() =>
  (props.data.rankings || []).filter(r => !r.errorMsg)
)
</script>

<template>
  <div class="max-w-[900px] mx-auto p-10 font-sans text-foreground text-sm leading-relaxed">
    <h1 class="text-[28px] font-semibold mb-2">Stress Test Analysis</h1>

    <FinancialTable
      title="Scenario Rankings by Max Drawdown"
      :columns="rankingColumns"
      :rows="rankingRows"
    />

    <div v-for="scenario in data.scenarios" :key="scenario.name" class="mt-12">
      <h2 class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mb-3">
        {{ scenario.name }}
      </h2>
      <p class="text-detail text-muted mb-3">{{ scenario.dateRange }}</p>
      <div v-for="rm in scenario.runMetrics" :key="rm.runName" class="mb-4">
        <MetricCards
          v-if="rm.hasData"
          :metrics="[
            { label: rm.runName + ': Max Drawdown', value: rm.maxDrawdown, format: 'percent' },
            { label: rm.runName + ': Total Return', value: rm.totalReturn, format: 'percent' },
            { label: rm.runName + ': Worst Day', value: rm.worstDay, format: 'percent' },
          ]"
        />
        <p v-else class="text-muted-light text-detail">{{ rm.runName }}: {{ rm.errorMsg || 'No data' }}</p>
      </div>
    </div>

    <div v-if="data.summary" class="mt-12">
      <h2 class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mb-3">Summary</h2>
      <p class="whitespace-pre-line text-detail text-muted">{{ data.summary }}</p>
    </div>
  </div>
</template>
