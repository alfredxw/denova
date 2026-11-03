import { createRequire } from 'node:module'
import path from 'node:path'

const { expect } = createRequire(new URL('../../web/package.json', import.meta.url))('@playwright/test')

export async function setNavigationExpanded(page, expanded) {
  const wrapper = page.locator('[data-slot="sidebar-wrapper"]')
  if (await wrapper.getAttribute('data-activity-bar-expanded') === String(expanded)) return
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true }).click({ button: 'right' })
  await page.getByRole('menuitem', { name: expanded ? '展开一级菜单' : '折叠一级菜单', exact: true }).click()
  await expect(wrapper).toHaveAttribute('data-activity-bar-expanded', String(expanded))
  await expect(page.getByRole('menu')).toHaveCount(0)
}

export async function expectWorkspaceFrame(page) {
  await expect.poll(() => page.locator('.nova-workspace-frame').evaluate(element => {
    const rect = element.getBoundingClientRect()
    const style = getComputedStyle(element)
    const controls = navigator.windowControlsOverlay?.getTitlebarAreaRect()
    return rect.top > 0 && rect.right < innerWidth && rect.bottom < innerHeight
      && parseFloat(style.borderRadius) > 0 && parseFloat(style.borderLeftWidth) > 0
      && (!controls?.width || rect.left >= controls.x)
  })).toBe(true)
  if (process.platform === 'darwin') {
    await expect.poll(() => page.getByLabel('工作台侧边栏').getByRole('button').first().evaluate(element =>
      element.getBoundingClientRect().top >= navigator.windowControlsOverlay.getTitlebarAreaRect().height)).toBe(true)
  }
}

/** Check native safe areas, compact navigation, keyboard sizing and restoration. */
export async function verifyNavigation(application, page, artifacts, route) {
  const wrapper = page.locator('[data-slot="sidebar-wrapper"]')
  const navigation = page.getByLabel('工作台侧边栏')
  const handle = navigation.getByRole('separator', { name: '调整一级菜单宽度' })
  const width = () => navigation.evaluate(element => element.getBoundingClientRect().width)

  await setNavigationExpanded(page, true)
  await expectWorkspaceFrame(page)
  await expect(page.locator('[data-activity-bar-toggle]')).toHaveCount(0)
  const expandedWidth = await width()
  await page.screenshot({ path: path.join(artifacts, `${route}-expanded.png`), animations: 'disabled' })

  await setNavigationExpanded(page, false)
  await expectWorkspaceFrame(page)
  await expect.poll(width).toBeLessThan(expandedWidth)
  await expect(handle).toBeVisible()
  const collapsedWidth = await width()
  await page.screenshot({ path: path.join(artifacts, `${route}-collapsed.png`), animations: 'disabled' })
  await setNavigationExpanded(page, true)
  await expect.poll(width).toBe(expandedWidth)

  await handle.focus()
  await page.keyboard.press('Home')
  await expect(wrapper).toHaveAttribute('data-activity-bar-expanded', 'false')
  await expect.poll(width).toBe(collapsedWidth)
  await page.keyboard.press('ArrowRight')
  await expect.poll(width).toBe(expandedWidth)
  for (let i = 0; i < 3; i++) await page.keyboard.press('ArrowRight')
  await expect.poll(width).toBe(expandedWidth + 24)
  for (let i = 0; i < 3; i++) await page.keyboard.press('ArrowLeft')
  await expect.poll(width).toBe(expandedWidth)

  await handle.focus()
  await page.keyboard.press('Enter')
  await expect(wrapper).toHaveAttribute('data-activity-bar-expanded', 'false')
  await page.reload()
  await expect(navigation).toBeVisible()
  await expect(wrapper).toHaveAttribute('data-activity-bar-expanded', 'false')
  await setNavigationExpanded(page, true)
  await expect.poll(width).toBe(expandedWidth)

  if (process.platform === 'darwin') {
    for (const zoom of [0.8, 1.25, 1]) {
      await application.evaluate(({ BrowserWindow }, zoom) => BrowserWindow.getAllWindows()[0].webContents.setZoomFactor(zoom), zoom)
      await setNavigationExpanded(page, false)
      await expectWorkspaceFrame(page)
      await setNavigationExpanded(page, true)
      await expectWorkspaceFrame(page)
    }
  }
  console.log(`Desktop navigation passed: ${route}, context menu, keyboard, reload and zoom`)
}
