import { configDefaults, defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { Agent } from 'node:http'
import path from 'path'

const backendPort = process.env.DENOVA_BACKEND_PORT || process.env.NOVA_BACKEND_PORT || '8080'

// Only suites that exercise browser APIs need a fresh DOM and React setup.
// Keep file isolation in both projects so mocks and module state cannot leak.
const domTests = [
  'src/**/*.test.tsx',
  'src/i18n/i18n.test.ts',
  'src/lib/api.test.ts',
  'src/lib/autosave/rebase-with-recovery.test.ts',
  'src/lib/api-client/{autosave-conflicts,client,project-files}.test.ts',
  'src/components/Editor/editorDocument.test.ts',
  'src/components/workbench/TabController.test.ts',
  'src/features/agent-chat/tab-state.test.ts',
  'src/features/workspace-events/client.test.ts',
  'src/features/settings/font-variables.test.ts',
  'src/features/speech/{player,text}.test.ts',
  'src/features/interactive/stores/interactive-store.test.ts',
  'src/features/interactive/components/director-console/persistence.test.ts',
]

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // Isolated browser-test servers must not replace a running dev server's
  // optimized modules while it still holds their previous metadata in memory.
  cacheDir: process.env.DENOVA_TEST_VITE_CACHE_DIR || 'node_modules/.vite',
  optimizeDeps: {
    // Prebundle the renderer and lazy App's hook dependencies together. Late
    // discovery can otherwise replace React's shared chunks during startup.
    include: [
      'react', 'react-dom', 'react-dom/client',
      'react/jsx-runtime', 'react/jsx-dev-runtime', 'react-i18next',
      'monaco-editor',
      '@pierre/diffs', '@pierre/diffs/react', '@pierre/trees', '@pierre/trees/react',
    ],
  },
  test: {
    globals: true,
    exclude: [...configDefaults.exclude, 'tests/**'],
    // Only the review workspace relies on computed CSS visibility in jsdom.
    // Other styles are presentation-only and do not need Vitest processing.
    css: { include: [/review-diff\.css$/] },
    maxWorkers: '50%',
    projects: [
      { extends: true, test: {
        name: 'node', environment: 'node', include: ['**/*.test.ts'],
        exclude: [...configDefaults.exclude, 'tests/**', ...domTests],
      } },
      { extends: true, test: {
        name: 'dom', environment: 'jsdom', include: domTests,
        setupFiles: './src/test/setup.ts',
      } },
    ],
  },
  resolve: {
    dedupe: ['react', 'react-dom'],
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
            // @pierre/diffs has top-level initializers across its internal modules.
            // Keep the package atomic so the vendor size cap cannot create cyclic chunks.
            { name: 'pierre-diffs', test: /node_modules[\\/]@pierre[\\/]diffs[\\/]/, priority: 50 },
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
        target: `http://127.0.0.1:${backendPort}`,
        // Without an agent the proxy sends Connection: close, forcing browsers
        // to reconnect for every API call (including Windows IPv6 fallback).
        agent: new Agent({ keepAlive: true }),
        changeOrigin: true,
        xfwd: true,
        // AgentChat terminals attach over /api/terminal/sessions/:id/attach, so the dev proxy
        // has to forward WebSocket upgrade requests as well.
        ws: true,
      },
    },
  },
})
