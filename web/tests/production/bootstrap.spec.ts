import { expect, test } from '@playwright/test'

test('production bundle mounts the application without runtime errors', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.stack || error.message))

  const response = await page.goto('/')

  expect(response?.ok()).toBe(true)
  expect(pageErrors).toEqual([])
  await expect(page.locator('[data-nova-app-shell="true"]')).toBeVisible()
  expect(pageErrors).toEqual([])
})
