<script setup>
import { computed } from 'vue'
import { formatValue, valueColorClass, isMissing, formatPercentDelta } from '../format.js'

const props = defineProps({
  metrics: {
    type: Array,
    required: true,
    // Each item: { label, value, format, comparison?, comparisonLabel? }
  },
})

const gridCols = computed(() => {
  const count = props.metrics.length
  if (count <= 3) return 'grid-cols-3'
  if (count <= 4) return 'grid-cols-4'
  if (count <= 5) return 'grid-cols-5'
  return 'grid-cols-6'
})
</script>

<template>
  <div :class="['grid gap-2.5 mb-4', gridCols]">
    <div
      v-for="(metric, idx) in metrics"
      :key="idx"
      class="bg-surface rounded-lg p-3 border border-border-light"
    >
      <div class="text-metric-label uppercase text-muted-light">
        {{ metric.label }}
      </div>
      <div
        :class="[
          'text-metric-value font-semibold',
          metric.format !== 'string' ? valueColorClass(metric.value) : 'text-foreground',
        ]"
      >
        {{ isMissing(metric.value) ? 'N/A' : formatValue(metric.value, metric.format) }}
      </div>
      <div
        v-if="metric.comparison !== undefined && metric.comparison !== null"
        :class="['text-detail mt-1', valueColorClass(metric.comparison)]"
      >
        {{ formatPercentDelta(metric.comparison) }}
        <span v-if="metric.comparisonLabel" class="text-muted-light ml-1">
          {{ metric.comparisonLabel }}
        </span>
      </div>
    </div>
  </div>
</template>
