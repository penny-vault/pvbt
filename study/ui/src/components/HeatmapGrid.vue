<script setup>
import { computed } from 'vue'
import { isMissing } from '../format.js'

const props = defineProps({
  rows: {
    type: Array,
    required: true,
    // Each: { label, values: number[] }
  },
  columns: {
    type: Array,
    required: true,
    // Array of column header strings
  },
  format: {
    type: String,
    default: 'percent',
    validator: (val) => ['percent', 'number'].includes(val),
  },
  showTotals: { type: Boolean, default: false },
  title: { type: String, default: '' },
})

// Find the maximum absolute value for color scaling
const maxAbs = computed(() => {
  let peak = 0
  for (const row of props.rows) {
    for (const val of row.values) {
      if (!isMissing(val) && Math.abs(val) > peak) {
        peak = Math.abs(val)
      }
    }
  }
  return peak || 1
})

function formatCell(value) {
  if (isMissing(value)) return ''
  if (props.format === 'percent') return `${(value * 100).toFixed(1)}%`
  return value.toLocaleString()
}

function cellStyle(value) {
  if (isMissing(value)) return {}
  const intensity = Math.min(Math.abs(value) / maxAbs.value, 1)
  const alpha = 0.15 + intensity * 0.45
  if (value > 0) return { backgroundColor: `rgba(29, 158, 117, ${alpha})`, color: '#1a1a1a' }
  if (value < 0) return { backgroundColor: `rgba(226, 75, 74, ${alpha})`, color: '#1a1a1a' }
  return {}
}

function rowTotal(row) {
  if (props.format === 'percent') {
    let compound = 1
    for (const val of row.values) {
      if (!isMissing(val)) compound *= (1 + val)
    }
    return compound - 1
  }
  return row.values.reduce((sum, val) => sum + (isMissing(val) ? 0 : val), 0)
}
</script>

<template>
  <div>
    <h3 v-if="title" class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mt-12 mb-3">
      {{ title }}
    </h3>
    <div class="overflow-x-auto">
      <table class="border-collapse text-table">
        <thead>
          <tr>
            <th class="px-2 py-1 text-left font-semibold text-muted bg-surface border-b border-border sticky left-0 bg-surface z-10"></th>
            <th
              v-for="col in columns"
              :key="col"
              class="px-2 py-1 text-right font-semibold text-muted bg-surface border-b border-border min-w-[56px]"
            >
              {{ col }}
            </th>
            <th v-if="showTotals" class="px-2 py-1 text-right font-semibold text-muted bg-surface border-b border-border min-w-[56px]">
              Total
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.label">
            <td class="px-2 py-1 text-left font-medium text-muted border-b border-border-light sticky left-0 bg-white z-10">
              {{ row.label }}
            </td>
            <td
              v-for="(val, ci) in row.values"
              :key="ci"
              class="px-2 py-1 text-right border-b border-border-light min-w-[56px]"
              :style="cellStyle(val)"
            >
              {{ formatCell(val) }}
            </td>
            <td
              v-if="showTotals"
              class="px-2 py-1 text-right border-b border-border-light font-medium min-w-[56px]"
              :style="cellStyle(rowTotal(row))"
            >
              {{ formatCell(rowTotal(row)) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
