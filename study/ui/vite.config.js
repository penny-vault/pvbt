import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/main.js'),
      name: 'PvbtReport',
      formats: ['iife'],
      fileName: () => 'bundle.js',
    },
    outDir: 'dist',
    cssCodeSplit: false,
    cssInlineThreshold: Infinity,
    minify: 'esbuild',
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
      },
    },
  },
})
