/**
 * Shared formatting utilities for report components.
 */

/**
 * Check if a value is missing (null, undefined, or NaN).
 * @param {*} value
 * @returns {boolean}
 */
export function isMissing(value) {
  return value === null || value === undefined || Number.isNaN(value)
}

/**
 * Format a currency value as $1,234.56.
 * @param {number} value
 * @returns {string}
 */
export function formatCurrency(value) {
  if (isMissing(value)) return '--'
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value)
}

/**
 * Format a currency value in compact form ($1.2M, $140k).
 * Useful for chart axes.
 * @param {number} value
 * @returns {string}
 */
export function formatCurrencyCompact(value) {
  if (isMissing(value)) return '--'
  const abs = Math.abs(value)
  const sign = value < 0 ? '-' : ''
  if (abs >= 1_000_000) {
    return `${sign}$${(abs / 1_000_000).toFixed(1)}M`
  }
  if (abs >= 1_000) {
    return `${sign}$${(abs / 1_000).toFixed(0)}k`
  }
  return formatCurrency(value)
}

/**
 * Format a percentage delta with sign (+12.34%).
 * @param {number} value - A ratio (0.1234 = 12.34%)
 * @returns {string}
 */
export function formatPercentDelta(value) {
  if (isMissing(value)) return '--'
  const pct = (value * 100).toFixed(2)
  return value >= 0 ? `+${pct}%` : `${pct}%`
}

/**
 * Return a Tailwind color class based on value sign.
 * @param {number} value
 * @returns {string}
 */
export function valueColorClass(value) {
  if (isMissing(value)) return 'text-muted-light'
  if (value > 0) return 'text-positive'
  if (value < 0) return 'text-negative'
  return 'text-muted-light'
}

/**
 * Format a value according to its type.
 * @param {*} value
 * @param {string} type - One of: percent, currency, ratio, number, date, string
 * @returns {string}
 */
export function formatValue(value, type) {
  if (isMissing(value)) return '--'

  switch (type) {
    case 'percent':
      return `${(value * 100).toFixed(2)}%`
    case 'currency':
      return formatCurrency(value)
    case 'ratio':
      return value.toFixed(2)
    case 'number':
      return new Intl.NumberFormat('en-US', {
        maximumFractionDigits: 2,
      }).format(value)
    case 'date':
      if (typeof value === 'string') return value
      return new Date(value).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
    case 'string':
    default:
      return String(value)
  }
}
