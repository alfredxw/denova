import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('keeps exactly one primary destination active across the normal workbench routes', async ({ page, request }) => {
  // This journey hydrates the primary destinations; each assertion keeps its normal
  // deadline, while the total budget includes cold route loads on CI runners.
  test.setTimeout(90_000)
  await createAndOpenBook(request, 'Browser Navigation Book')
  const settings = await (await request.get('/api/settings')).json()
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
    json: { ...settings, effective: { ...settings.effective, labs: { ...settings.effective?.labs, developer_mode: true } } },
  }))
  // Exercise the developer destination's empty state without enabling trace collection in the test backend.
  await page.route(/\/api\/agent-runs(?:\?|$)/, (route) => route.fulfill({ json: { runs: [], issues: [] } }))
  await page.route('**/api/resource-market/catalog', (route) => route.fulfill({ json: { schema_version: 1, entries: [] } }))
  await page.goto('/')

  const sidebar = page.getByLabel('工作台侧边栏')
  await expect(sidebar).toBeVisible()
  for (const destination of ['写作', '游戏', '资料库', '创作方案', '资源中心', '工作台', '书籍管理', '版本管理', 'Skills', 'Agents', '自动化', '轨迹', '设置']) {
    await test.step(destination, async () => {
      await sidebar.getByRole('button', { name: destination, exact: true }).click()
      await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
      await expect(sidebar.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
      await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
      await expect(page.getByRole('button', { name: /^关闭(?:设置|书籍管理|版本管理|自动化| Agents)?$/ })).toHaveCount(0)
    })
  }
})

test('exposes the same primary destinations in English on a narrow viewport', async ({ page, request }) => {
  await createAndOpenBook(request, 'Mobile Navigation Book')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.addInitScript(() => {
    window.localStorage.setItem('nova.locale.configured', 'en-US')
  })
  const settings = await (await request.get('/api/settings')).json()
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
    json: { ...settings, effective: { ...settings.effective, language: 'en-US' } },
  }))
  await page.goto('/')

  await page.getByRole('button', { name: 'Navigation', exact: true }).click()
  const navigation = page.getByRole('dialog')
  await expect(navigation.getByRole('button', { name: 'Writing', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Game', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Lore', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Creative Setups', exact: true })).toBeVisible()
  await expect(navigation.getByRole('button', { name: 'Resource Center', exact: true })).toBeVisible()
  await navigation.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(navigation).toBeHidden()
  const categories = page.locator('.nova-mobile-topbar').getByRole('button', { name: 'Categories', exact: true })
  await categories.click()
  const categoryDrawer = page.getByRole('dialog', { name: 'Categories', exact: true })
  await expect(categoryDrawer).toHaveAttribute('data-side', 'right')
  await categoryDrawer.getByRole('button', { name: 'General', exact: true }).click()
  await expect(categoryDrawer).toBeHidden()
  await expect(categories).toBeFocused()
  const content = page.locator('[data-nova-settings-content]')
  await expect(content.getByRole('heading', { name: 'General', exact: true })).toBeInViewport()
  await expect(content.getByRole('heading', { name: 'Language Models', exact: true })).toBeAttached()
  await categories.click()
  await categoryDrawer.getByRole('textbox', { name: 'Search settings', exact: true }).fill('Temperature')
  await categoryDrawer.getByRole('button', { name: /Temperature/ }).click()
  await expect(categoryDrawer).toBeHidden()
  const temperature = content.getByRole('spinbutton', { name: 'Temperature', exact: true }).first()
  await expect(temperature).toBeInViewport()
  await expect(content.getByRole('heading', { name: 'General', exact: true })).toBeAttached()
  await categories.click()
  await expect(categoryDrawer.getByRole('button', { name: 'Language Models', exact: true })).toHaveAttribute('aria-current', 'page')
  await categoryDrawer.getByRole('button', { name: 'General', exact: true }).click()
  await content.hover()
  await page.mouse.wheel(0, 100_000)
  await categories.click()
  await expect(categoryDrawer.getByRole('button', { name: 'Advanced', exact: true })).toHaveAttribute('aria-current', 'page')
})
