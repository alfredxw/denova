import { defineConfig, devices } from '@playwright/test'

const backendPort = process.env.DENOVA_E2E_BACKEND_PORT || '18080'
const frontendPort = process.env.DENOVA_E2E_FRONTEND_PORT || '15173'
const modelPort = process.env.DENOVA_E2E_MODEL_PORT || '18081'
const packaged = Boolean(process.env.DENOVA_E2E_PACKAGE_DIR)
const baseURL = `http://127.0.0.1:${packaged ? backendPort : frontendPort}`

export default defineConfig({
  testDir: './tests',
  outputDir: './test-results/artifacts',
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  reporter: process.env.CI
    ? [['line'], ['html', { open: 'never' }]]
    : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    locale: 'zh-CN',
    colorScheme: 'dark',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  expect: {
    timeout: 10_000,
  },
  projects: [
    {
      name: 'browser',
      testMatch: /browser\/.*\.spec\.ts/,
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'e2e',
      testMatch: /e2e\/.*\.spec\.ts/,
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: 'node ./scripts/e2e-model-server.mjs',
      url: `http://127.0.0.1:${modelPort}/health`,
      reuseExistingServer: false,
      env: {
        DENOVA_E2E_MODEL_PORT: modelPort,
      },
    },
    {
      command: 'node ./scripts/start-e2e-backend.mjs',
      url: `http://127.0.0.1:${backendPort}/api/status`,
      reuseExistingServer: false,
      env: {
        DENOVA_E2E_BACKEND_PORT: backendPort,
        DENOVA_E2E_MODEL_PORT: modelPort,
      },
    },
    ...(!packaged ? [{
      command: `pnpm dev --host 127.0.0.1 --port ${frontendPort} --strictPort`,
      url: baseURL,
      reuseExistingServer: false,
      env: {
        DENOVA_BACKEND_PORT: backendPort,
        DENOVA_FRONTEND_PORT: frontendPort,
      },
    }] : []),
  ],
})
