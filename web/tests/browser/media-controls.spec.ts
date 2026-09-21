import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot } from '../support/api'

for (const theme of ['dark', 'light']) {
  test(`configured game media keeps history actions and settings access in ${theme}`, async ({ page, request }) => {
    const settings = await (await request.get('/api/settings')).json()
    const configured = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: settings.revisions.user,
      changes: {
        theme, language: 'zh-CN',
        speech: { endpoint: 'http://127.0.0.1:8000/v1/audio/speech', model: 'test-speech', voice: 'test-voice' },
        default_image_api_profile_id: 'local-image',
        image_api_endpoints: [{ id: 'local-image', provider: 'custom', base_url: 'http://127.0.0.1:8000/v1', protocol: 'openai-images' }],
        image_api_profiles: [{ id: 'local-image', endpoint_id: 'local-image', model: 'test-image' }],
      },
    } })
    expect(configured.ok(), await configured.text()).toBe(true)
    await createAndOpenBook(request, `Media history ${theme}`)
    const story = await createStartedStory(request, '历史回合媒体操作')
    const continued = await request.post('/api/interactive/chat', { data: {
      command_id: `media-history-${story.id}`, mode: 'story', story_id: story.id, branch: 'main', message: '推开石门',
    } })
    expect(continued.ok(), await continued.text()).toBe(true)
    await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(2)
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    await expect(page.getByRole('button', { name: '生成互动图像', exact: true })).toHaveCount(2)
    await expect(page.getByRole('button', { name: '朗读正文', exact: true })).toHaveCount(2)
    await page.getByRole('tab', { name: '控制', exact: true }).click()
    await expect(page.getByRole('switch', { name: '自动生成', exact: true })).toBeVisible()
    await page.locator('header').filter({ has: page.getByRole('heading', { name: '互动图像', exact: true }) }).getByRole('button', { name: '配置模型', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`configured-media-${theme}-wide.png`) })
    await page.locator('header').filter({ has: page.getByRole('heading', { name: '互动图像', exact: true }) }).getByRole('button', { name: '配置模型', exact: true }).click()
    await expect(page.getByRole('button', { name: '公共配置图像模型', exact: true })).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.getByRole('button', { name: '导航菜单', exact: true }).click()
    await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name: '游戏', exact: true }).click()
    await page.getByRole('button', { name: '显示游戏控制台', exact: true }).click()
    const panel = page.getByRole('dialog', { name: '故事控制台', exact: true })
    await panel.getByRole('tab', { name: '控制', exact: true }).click()
    await panel.locator('header').filter({ has: page.getByRole('heading', { name: '互动图像', exact: true }) }).getByRole('button', { name: '配置模型', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`configured-media-${theme}-390.png`) })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await panel.locator('header').filter({ has: page.getByRole('heading', { name: '互动图像', exact: true }) }).getByRole('button', { name: '配置模型', exact: true }).click()
    await expect(page.getByRole('button', { name: '公共配置图像模型', exact: true })).toBeVisible()
  })

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
    await panel.locator('header').filter({ has: page.getByRole('heading', { name: '语音朗读', exact: true }) }).getByRole('button', { name: '配置模型', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`media-cards-${theme}-390.png`) })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })
}
