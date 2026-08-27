import { configDefaults, defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const backendPort = process.env.DENOVA_BACKEND_PORT || process.env.NOVA_BACKEND_PORT || '8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  optimizeDeps: {
    include: ['@pierre/diffs', '@pierre/diffs/react', '@pierre/trees', '@pierre/trees/react'],
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    globals: true,
    exclude: [...configDefaults.exclude, 'tests/**'],
    // Only the review workspace relies on computed CSS visibility in jsdom.
    // Other styles are presentation-only and do not need Vitest processing.
    css: { include: [/review-diff\.css$/] },
    // jsdom suites are CPU and memory intensive. A proportional cap stays
    // adaptive across developer and CI machines.
    maxWorkers: '25%',
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
        // AgentChat terminals attach over /api/terminal/sessions/:id/attach, so the dev proxy
        // has to forward WebSocket upgrade requests as well.
        ws: true,
      },
    },
  },
})
