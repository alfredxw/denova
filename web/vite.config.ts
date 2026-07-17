import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const backendPort = process.env.DENOVA_BACKEND_PORT || process.env.NOVA_BACKEND_PORT || '8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    globals: true,
    css: true,
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          // Keep size caps on individual groups: a global cap can split tightly coupled SDKs into cyclic chunks.
          minSize: 20 * 1024,
          groups: [
            { name: 'shiki', test: /node_modules[\\/](?:shiki|@shikijs)[\\/]/, priority: 40 },
            { name: 'monaco', test: /node_modules[\\/](?:monaco-editor|@monaco-editor)[\\/]/, priority: 30 },
            { name: 'ai-sdk', test: /node_modules[\\/](?:ai|@ai-sdk)[\\/]/, priority: 20 },
            { name: 'markdown', test: /node_modules[\\/](?:react-markdown|remark-|rehype-|micromark|mdast|hast|unified)[^\\/]*[\\/]/, priority: 10 },
            { name: 'vendor', test: /node_modules[\\/]/, maxSize: 450 * 1024, priority: 1, entriesAware: true },
          ],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: `http://localhost:${backendPort}`,
        changeOrigin: true,
        xfwd: true,
      },
    },
  },
})
