import { defineConfig, devices } from '@playwright/test'
import workbenchConfig from './playwright.config'

const frontendPort = process.env.DENOVA_PRODUCTION_FRONTEND_PORT || '14173'
const baseURL = `http://127.0.0.1:${frontendPort}`
const backendPort = process.env.DENOVA_E2E_BACKEND_PORT || '18080'
const workbenchServers = Array.isArray(workbenchConfig.webServer) ? workbenchConfig.webServer : []

export default defineConfig({
  testDir: './tests',
  testMatch: ['production/**/*.spec.ts', 'e2e/books.spec.ts', 'browser/workspace-navigation.spec.ts'],
  outputDir: './test-results/production-artifacts',
  workers: 1,
  retries: 0,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    locale: 'zh-CN',
    colorScheme: 'dark',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  // Reuse the isolated model and backend so production checks exercise real routes.
  webServer: [...workbenchServers.slice(0, 2), {
    command: `pnpm preview --host 127.0.0.1 --port ${frontendPort} --strictPort`,
    url: baseURL,
    reuseExistingServer: false,
    env: { DENOVA_BACKEND_PORT: backendPort },
  }],
})
