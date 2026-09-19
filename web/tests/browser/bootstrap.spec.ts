import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('mounts the application without runtime errors', async ({ page }) => {
  const response = await page.goto('/')

  expect(response?.ok()).toBe(true)
  // The Suspense fallback also carries data-nova-app-shell. Wait for the
  // hydrated workbench so errors during the lazy App import cannot pass.
  await expect(page.getByLabel('工作台侧边栏')).toBeVisible()
  await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
})

test('restores writing and game after a full page reload', async ({ page, request }) => {
  await createAndOpenBook(request, 'Startup Reload Book')
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await expect(sidebar).toBeVisible()

  for (const destination of ['写作', '游戏']) {
    const button = sidebar.getByRole('button', { name: destination, exact: true })
    await button.click()
    await expect(button).toHaveAttribute('aria-current', 'page')
    await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)

    await page.reload()
    await expect(sidebar).toBeVisible()
    await expect(button).toHaveAttribute('aria-current', 'page')
    await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
    await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
  }
})
