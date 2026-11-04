import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

test('unavailable usage statistics show a disabled, unchecked switch', async ({ page, request }) => {
  await createAndOpenBook(request, 'Unavailable analytics validation')
  await page.route(/\/api\/analytics$/, route => route.fulfill({
    json: { enabled: true, notice_seen: true, available: false },
  }))
  await page.goto('/')
  await page.getByRole('navigation').first().getByRole('button', { name: '设置', exact: true }).click()
  const toggle = page.getByRole('switch', { name: '分享匿名使用统计', exact: true })
  await expect(page.getByText('使用统计当前不可用。', { exact: true })).toBeVisible()
  await expect(toggle).toBeDisabled()
  await expect(toggle).not.toBeChecked()
  await toggle.scrollIntoViewIfNeeded()
  await page.screenshot({ path: test.info().outputPath('analytics-unavailable.png') })
})

for (const { theme, locale, width } of [
  { theme: 'dark', locale: 'zh-CN', width: 1440 },
  { theme: 'light', locale: 'en-US', width: 1440 },
  { theme: 'dark', locale: 'en-US', width: 390 },
  { theme: 'light', locale: 'zh-CN', width: 390 },
] as const) {
  test(`usage notice and settings cover writing/game in ${locale}, ${theme}, ${width}px`, async ({ page, request }) => {
    test.setTimeout(60_000)
    await createAndOpenBook(request, 'Analytics privacy validation')
    await createStartedStory(request, 'Analytics game validation')
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
      json: { ...settings, effective: { ...settings.effective, theme, language: locale } },
    }))
    let preferences = { enabled: true, notice_seen: false, available: true }
    const activities: unknown[] = []
    await page.route(/\/api\/analytics$/, async route => {
      if (route.request().method() === 'PATCH') {
        const body = route.request().postDataJSON()
        expect(Object.keys(body)).toEqual(['enabled'])
        preferences = { ...preferences, enabled: body.enabled, notice_seen: true }
      }
      await route.fulfill({ json: preferences })
    })
    await page.route('**/api/analytics/activity', async route => {
      activities.push(route.request().postDataJSON())
      await route.fulfill({ json: {} })
    })
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/')
    const chinese = locale === 'zh-CN'
    const selectDestination = async (name: string) => {
      if (width < 768) {
        await page.getByRole('button', { name: chinese ? '导航菜单' : 'Navigation', exact: true }).click()
        await page.getByRole('dialog', { name: chinese ? '导航菜单' : 'Navigation', exact: true }).getByRole('button', { name, exact: true }).click()
      } else {
        await page.getByRole('navigation').first().getByRole('button', { name, exact: true }).click()
      }
    }
    const dialog = page.getByRole('dialog', { name: chinese ? '匿名使用统计' : 'Anonymous usage statistics' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText(chinese ? '默认开启' : 'on by default')
    await expect(dialog).toContainText(chinese ? '运行耗时' : 'how long operations take')
    await expect(dialog).toContainText(chinese ? '创作内容、项目名称、文件路径和 API 密钥均不会发送' : 'Statistics exclude creative content, project names, file paths, and API keys')
    expect(activities).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`analytics-notice-${locale}-${theme}-${width}.png`) })
    await dialog.getByRole('button', { name: chinese ? '知道了' : 'Got it', exact: true }).click()
    await expect(dialog).toBeHidden()
    expect(preferences).toEqual({ enabled: true, notice_seen: true, available: true })
    await selectDestination(chinese ? '设置' : 'Settings')
    const toggle = page.getByRole('switch', { name: chinese ? '分享匿名使用统计' : 'Share anonymous usage statistics', exact: true })
    await expect(toggle).toBeChecked()
    await expect(page.getByText(chinese
      ? '帮助改进 Denova'
      : 'Help improve Denova', { exact: false })).toBeVisible()
    await toggle.click()
    await expect(toggle).not.toBeChecked()
    await page.screenshot({ path: test.info().outputPath(`analytics-settings-${locale}-${theme}-${width}.png`) })
    await page.reload()
    await expect(dialog).toBeHidden()
    await expect(toggle).not.toBeChecked()
    await selectDestination(chinese ? '游戏' : 'Game')
    // The settings panel is shared across the two creation destinations.
    await expect(toggle).toBeHidden()
    await selectDestination(chinese ? '设置' : 'Settings')
    await expect(toggle).not.toBeChecked()
    await toggle.click()
    await expect(toggle).toBeChecked()
    expect(preferences.enabled).toBe(true)
  })
}
