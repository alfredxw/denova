import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('keeps exactly one primary destination active across the normal workbench routes', async ({ page, request }) => {
  await createAndOpenBook(request, 'Browser Navigation Book')
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
    const response = await route.fetch()
    const settings = await response.json()
    await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, labs: { ...settings.effective?.labs, developer_mode: true } } } })
  })
  // Exercise the developer destination's empty state without enabling trace collection in the test backend.
  await page.route(/\/api\/agent-runs(?:\?|$)/, (route) => route.fulfill({ json: { runs: [], issues: [] } }))
  await page.goto('/')

  const sidebar = page.getByLabel('工作台侧边栏')
  await expect(sidebar).toBeVisible()
  for (const destination of ['写作', '游戏', '资料库', '方案预设', '工作台', '书籍管理', '版本管理', 'Skills', 'Agents', '自动化', '轨迹', '设置']) {
    await sidebar.getByRole('button', { name: destination, exact: true }).click()
    await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
    await expect(sidebar.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
    await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
    await expect(page.getByRole('button', { name: /^关闭(?:设置|书籍管理|版本管理|自动化| Agents)?$/ })).toHaveCount(0)
  }
})

test('exposes the same primary destinations in English on a narrow viewport', async ({ page, request }) => {
  await createAndOpenBook(request, 'Mobile Navigation Book')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.addInitScript(() => {
    window.localStorage.setItem('nova.locale.configured', 'en-US')
  })
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
    const response = await route.fetch()
    const settings = await response.json() as { effective?: Record<string, unknown> }
    await route.fulfill({
      response,
      json: { ...settings, effective: { ...settings.effective, language: 'en-US' } },
    })
  })
  await page.goto('/')

  await page.getByRole('button', { name: 'Navigation', exact: true }).click()
  const navigation = page.getByRole('dialog')
  await expect(navigation.getByRole('button', { name: 'Writing', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Game', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Lore', exact: true })).toBeVisible()
  await navigation.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(navigation).toBeHidden()
  const categories = page.locator('.nova-mobile-topbar').getByRole('button', { name: 'Categories', exact: true })
  await categories.click()
  const categoryDrawer = page.getByRole('dialog', { name: 'Categories', exact: true })
  await expect(categoryDrawer).toHaveAttribute('data-side', 'right')
  await categoryDrawer.getByRole('button', { name: 'Appearance', exact: true }).click()
  await expect(categoryDrawer).toBeHidden()
  await expect(categories).toBeFocused()
})
