import { expect, type Page } from './fixtures'

export async function openExtensions(page: Page, chinese = true) {
  const mobile = (page.viewportSize()?.width ?? 1280) < 768
  if (mobile) await page.getByRole('button', { name: chinese ? '导航菜单' : 'Navigation', exact: true }).click()
  await page.getByRole('button', { name: chinese ? '资源中心' : 'Resource Center', exact: true }).filter({ visible: true }).click()
  await expect(page.getByTestId('resource-market')).toBeVisible()
  const navigation = page.getByRole('navigation', { name: chinese ? '资源中心导航' : 'Resource Center navigation', exact: true })
  if (mobile) await page.getByRole('button', { name: chinese ? '资源中心导航目录' : 'Resource Center navigation directory', exact: true }).click()
  await navigation.getByRole('button', { name: chinese ? '插件与游戏' : 'Plugins & games', exact: true }).click()
}

export async function selectExtension(page: Page, name: string, chinese = true) {
  const heading = page.getByRole('heading', { name, exact: true }).filter({ visible: true })
  if (await heading.isVisible()) return
  await openExtensions(page, chinese)
  await page.getByRole('button').filter({ hasText: name, visible: true }).click()
  await expect(heading).toBeVisible()
}
