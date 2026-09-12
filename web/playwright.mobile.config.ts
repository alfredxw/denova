import { defineConfig, devices } from '@playwright/test'
import base from './playwright.config'

// Optional touch-browser coverage; install Chromium and WebKit before running.
export default defineConfig({
  ...base,
  projects: [
    { name: 'mobile-chromium', testMatch: /browser\/mobile-(?:workspace|reading)\.spec\.ts/, use: { ...devices['Pixel 5'] } },
    { name: 'mobile-webkit', testMatch: /browser\/mobile-(?:workspace|reading)\.spec\.ts/, use: { ...devices['iPhone 13'] } },
  ],
})
