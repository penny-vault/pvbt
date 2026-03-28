<script setup>
import { ref, computed } from 'vue'
import {
  useVueTable,
  getCoreRowModel,
  getSortedRowModel,
  createColumnHelper,
} from '@tanstack/vue-table'
import { formatValue, valueColorClass, isMissing } from '../format.js'

const props = defineProps({
  columns: {
    type: Array,
    required: true,
    // Each: { key, header, type, align? }
  },
  rows: {
    type: Array,
    required: true,
  },
  sortable: {
    type: Boolean,
    default: true,
  },
  title: {
    type: String,
    default: '',
  },
})

const sorting = ref([])

const columnDefs = computed(() =>
  props.columns.map((col) => ({
    accessorKey: col.key,
    header: col.header,
    meta: { type: col.type, align: col.align },
    sortingFn: col.type === 'string' || col.type === 'date' ? 'alphanumeric' : 'basic',
  }))
)

const table = useVueTable({
  get data() { return props.rows },
  get columns() { return columnDefs.value },
  state: {
    get sorting() { return sorting.value },
  },
  onSortingChange: (updater) => {
    sorting.value = typeof updater === 'function' ? updater(sorting.value) : updater
  },
  getCoreRowModel: getCoreRowModel(),
  getSortedRowModel: props.sortable ? getSortedRowModel() : undefined,
})

function cellAlign(colMeta) {
  if (colMeta?.align) return colMeta.align
  const numericTypes = ['percent', 'currency', 'ratio', 'number']
  return numericTypes.includes(colMeta?.type) ? 'right' : 'left'
}

function cellColorClass(value, colMeta) {
  if (isMissing(value)) return 'text-muted-light'
  const coloredTypes = ['percent', 'currency', 'ratio', 'number']
  if (coloredTypes.includes(colMeta?.type)) return valueColorClass(value)
  return 'text-foreground'
}
</script>

<template>
  <div>
    <h3 v-if="title" class="text-section-heading font-semibold border-b-2 border-foreground pb-1.5 mt-12 mb-3">
      {{ title }}
    </h3>
    <table class="w-full border-collapse text-table">
      <thead>
        <tr v-for="headerGroup in table.getHeaderGroups()" :key="headerGroup.id">
          <th
            v-for="header in headerGroup.headers"
            :key="header.id"
            :class="[
              'px-2 py-1.5 bg-surface border-b border-border font-semibold',
              cellAlign(header.column.columnDef.meta) === 'right' ? 'text-right' : 'text-left',
              sortable ? 'cursor-pointer select-none hover:bg-border-light' : '',
            ]"
            @click="sortable ? header.column.getToggleSortingHandler()?.($event) : null"
          >
            {{ header.column.columnDef.header }}
            <span v-if="sortable" class="text-muted-light ml-1">
              {{ header.column.getIsSorted() === 'asc' ? '\u25B2' : header.column.getIsSorted() === 'desc' ? '\u25BC' : '' }}
            </span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="(row, rowIdx) in table.getRowModel().rows"
          :key="row.id"
          :class="rowIdx % 2 === 0 ? 'bg-white' : 'bg-surface'"
          class="hover:bg-border-light/50"
        >
          <td
            v-for="cell in row.getVisibleCells()"
            :key="cell.id"
            :class="[
              'px-2 py-1.5 border-b border-border-light',
              cellAlign(cell.column.columnDef.meta) === 'right' ? 'text-right' : 'text-left',
              cellColorClass(cell.getValue(), cell.column.columnDef.meta),
            ]"
          >
            {{ isMissing(cell.getValue()) ? 'N/A' : formatValue(cell.getValue(), cell.column.columnDef.meta?.type) }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
