import { expect, test } from '../support/fixtures'

for (const theme of ['dark', 'light']) {
  test(`market sidebar stays within its pane in ${theme}`, async ({ page, request }) => {
    await request.patch('/api/settings', {
      data: { layer: 'user', changes: { language: 'en-US', theme } },
    })
    await page.addInitScript(() => {
      localStorage.setItem('nova:mode', 'market')
      localStorage.setItem('nova.locale.configured', 'en-US')
    })
    await page.route('**/api/resource-market/catalog', route =>
      route.fulfill({ json: { schema_version: 1, entries: [] } }),
    )
    await page.goto('/')
    const navigation = page.getByRole('navigation', { name: 'Resource Center navigation', exact: true })
    const registry = page.getByRole('button', { name: 'denova-index', exact: true })
    await expect(registry).toBeVisible()
    const title = page.getByRole('heading', { name: 'Discover resources', exact: true })
    expect(Math.abs((await registry.boundingBox())!.y - (await title.boundingBox())!.y)).toBeLessThan(8)
    for (const destination of ['Packages', 'Plugins & games', 'Updates']) {
      await navigation.getByRole('button', { name: destination, exact: true }).click()
      await expect(navigation.getByRole('button', { name: 'Skills', exact: true })).toBeVisible()
    }
    const packages = (await navigation.getByRole('button', { name: 'Packages', exact: true }).boundingBox())!
    const extensions = (await navigation.getByRole('button', { name: 'Plugins & games', exact: true }).boundingBox())!
    const updates = (await navigation.getByRole('button', { name: 'Updates', exact: true }).boundingBox())!
    expect(Math.abs((updates.y - extensions.y) - (extensions.y - packages.y))).toBeLessThan(2)
    await navigation.getByRole('button', { name: 'Skills', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Skills', exact: true })).toBeVisible()
    await registry.click()
    await expect(page.locator('[data-settings-section="market"] input')).toBeInViewport()
    await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Resource Center', exact: true }).click()
    for (const width of [1440, 1100, 390]) {
      await page.setViewportSize({ width, height: 600 })
      if (width === 390) {
        await page.getByRole('button', { name: 'Resource Center navigation directory', exact: true }).click()
      }
      await expect(navigation).toBeVisible()
      // Check the actual scroll extents, including inset separators and the footer.
      expect(await navigation.evaluate(element =>
        [element, ...element.querySelectorAll('[data-sidebar="content"]')].every(
          node => node.scrollWidth <= node.clientWidth,
        ),
      )).toBe(true)
      await expect(navigation.getByRole('link', { name: 'Submit a package' })).toBeVisible()
      await page.screenshot({ path: `test-results/market-sidebar-${theme}-${width}.png` })
    }
  })
}
