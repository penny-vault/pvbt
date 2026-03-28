/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{vue,js}'],
  theme: {
    extend: {
      colors: {
        positive: '#1D9E75',
        negative: '#E24B4A',
        warning: '#BA7517',
        accent: '#378ADD',
        'accent-secondary': '#534AB7',
        'accent-tertiary': '#D85A30',
        foreground: '#1a1a1a',
        muted: '#666666',
        'muted-light': '#999999',
        surface: '#fafafa',
        border: '#dddddd',
        'border-light': '#eeeeee',
      },
      fontFamily: {
        sans: ['-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
      },
      fontSize: {
        'metric-label': ['11px', { letterSpacing: '1.5px', lineHeight: '1.4' }],
        'metric-value': ['20px', { lineHeight: '1.2' }],
        'section-heading': ['20px', { lineHeight: '1.3' }],
        'table': ['13px', { lineHeight: '1.5' }],
        'detail': ['12px', { lineHeight: '1.5' }],
      },
    },
  },
  plugins: [],
}
