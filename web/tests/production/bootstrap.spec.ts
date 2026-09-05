import { expect, test } from '../support/fixtures'

test('production bundle mounts the application without runtime errors', async ({ page }) => {
  const response = await page.goto('/')

  expect(response?.ok()).toBe(true)
  await expect(page.locator('[data-nova-app-shell="true"]')).toBeVisible()
})
