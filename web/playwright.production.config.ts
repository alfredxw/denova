import { defineConfig, devices } from '@playwright/test'

const frontendPort = process.env.DENOVA_PRODUCTION_FRONTEND_PORT || '14173'
const baseURL = `http://127.0.0.1:${frontendPort}`

export default defineConfig({
  testDir: './tests/production',
  outputDir: './test-results/production-artifacts',
  workers: 1,
  retries: 0,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    locale: 'en-US',
    colorScheme: 'dark',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: `pnpm preview --host 127.0.0.1 --port ${frontendPort} --strictPort`,
    url: baseURL,
    reuseExistingServer: false,
  },
})
