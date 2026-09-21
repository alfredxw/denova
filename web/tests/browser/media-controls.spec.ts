import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

for (const theme of ['dark', 'light']) {
  test(`unconfigured media cards stay compact on mobile in ${theme}`, async ({ page, request }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    const settings = await (await request.get('/api/settings')).json()
    await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: settings.revisions.user,
      changes: { speech: null, image_api_endpoints: [], image_api_profiles: [], default_image_api_profile_id: null, theme, language: 'zh-CN' },
    } })
    await createAndOpenBook(request, `Media ${theme}`)
    await createStartedStory(request, '未配置语音与图像模型的故事')
    await page.goto('/')
    await page.getByRole('button', { name: '导航菜单', exact: true }).click()
    await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name: '游戏', exact: true }).click()
    await page.getByRole('button', { name: '显示游戏控制台', exact: true }).click()
    const panel = page.getByRole('dialog', { name: '故事控制台', exact: true })
    await panel.getByRole('tab', { name: '控制', exact: true }).click()
    for (const title of ['互动图像', '语音朗读']) {
      const card = panel.locator('section').filter({ has: page.getByRole('heading', { name: title, exact: true }) })
      await expect(card.getByRole('button')).toHaveCount(1)
      await expect(card.locator('input, [role="switch"], [role="combobox"], [role="slider"]')).toHaveCount(0)
      await expect(card.locator(':scope > *')).toHaveCount(1)
    }
    await panel.getByRole('button', { name: '配置语音', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`media-cards-${theme}-390.png`) })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })
}
