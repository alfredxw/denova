import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStory, getStorySnapshot } from '../support/api'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const theme of ['dark', 'light'] as const) {
  test(`new Game opening keeps its composer and saved setup through pause and reload in ${theme}`, async ({ page, request }) => {
    page.setDefaultTimeout(10_000)
    await createAndOpenBook(request, `Opening controls ${theme}`)
    await request.patch('/api/settings', { data: { layer: 'user', changes: { theme, language: 'zh-CN' } } })
    await page.setViewportSize({ width: 1440, height: 960 })
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    await page.getByRole('button', { name: '新建', exact: true }).filter({ visible: true }).click()
    await page.getByRole('radio', { name: '自定义角色', exact: true }).click()
    const name = '旅行者林川'
    const profile = '曾在旧车站工作的旅人，正在寻找失踪的同伴。'
    const marker = 'E2E_COMPOSER_PAUSE'
    const openingText = `${'雨夜里，车站广播传来熟悉的声音。'.repeat(35)}\n\n${'UnbrokenOpeningText'.repeat(16)}\n${marker}`
    await page.getByRole('textbox', { name: '角色名称', exact: true }).fill(name)
    await page.getByRole('textbox', { name: '角色设定', exact: true }).fill(profile)
    await page.getByPlaceholder('可选。写下必须保留的场景、人物处境或第一事件；留空则自动生成。').fill(openingText)
    await page.getByRole('button', { name: '开始故事', exact: true }).click()
    try {
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      const composer = page.getByPlaceholder(/你要做什么/)
      await expect(composer).toBeVisible()
      await expect(page.locator('[data-action="stop"]')).toBeEnabled()
      const opening = page.getByRole('region', { name: '故事开局', exact: true })
      const toggle = opening.getByRole('button', { name: '故事开局', exact: true })
      await expect(opening).toContainText(name)
      await expect(opening).toContainText(profile)
      await expect(opening).toContainText(openingText)
      await expect(toggle).toHaveAttribute('aria-expanded', 'true')
      expect(await opening.evaluate(element => Boolean(element.compareDocumentPosition(document.querySelector('[data-nova-chat-item]')!) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true)
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 960 })
        await toggle.scrollIntoViewIfNeeded()
        await expect(composer).toBeVisible()
        await expect(toggle).toBeVisible()
        expect(await opening.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
        await page.screenshot({ path: test.info().outputPath(`opening-${theme}-${width}.png`) })
      }
      await toggle.click()
      await expect(toggle).toHaveAttribute('aria-expanded', 'false')
      await toggle.press('Enter')
      await expect(toggle).toHaveAttribute('aria-expanded', 'true')
      await page.locator('[data-action="stop"]').click()
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      await releaseDelayedRequest(request, marker)
      await page.reload()
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      await expect(composer).toBeVisible()
      await expect(opening).toContainText(openingText)
      await expect(opening).toHaveCount(1)
      await page.getByRole('button', { name: '取消任务', exact: true }).click()
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeHidden()
      // Aborting a reloaded 0-turn opening settles back into the saved setup form.
      await expect(page.getByRole('button', { name: '开始故事', exact: true })).toBeVisible()
      await expect(page.getByRole('textbox', { name: '角色名称', exact: true })).toHaveValue(name)
      await expect(page.getByPlaceholder('可选。写下必须保留的场景、人物处境或第一事件；留空则自动生成。')).toHaveValue(openingText)
    } finally {
      await releaseDelayedRequest(request, marker)
    }
  })
}

for (const mode of ['preset', 'ai'] as const) {
  test(`completed Game opening retains ${mode} setup before its narrative`, async ({ page, request }) => {
    await createAndOpenBook(request, `Saved opening ${mode}`)
    const story = await createStory(request, `Saved ${mode}`)
    const presetText = '港口的灯逐盏熄灭，旅人在雨中等候最后一班船。'
    const updated = await request.patch(`/api/interactive/stories/${story.id}`, { data: {
      opening: mode === 'preset' ? { mode, preset_id: 'harbor', preset_text: presetText } : { mode },
    } })
    expect(updated.ok(), await updated.text()).toBe(true)
    const started = await request.post('/api/interactive/chat', { data: {
      command_id: `opening-${story.id}`, mode: 'story', story_id: story.id, branch: 'main', start_opening: true,
    } })
    expect(started.ok(), await started.text()).toBe(true)
    await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(1)
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    const opening = page.getByRole('region', { name: '故事开局', exact: true })
    await expect(opening).toContainText(mode === 'preset' ? presetText : 'AI 生成')
    await expect(opening).toContainText('一扇石门挡在旧车站入口。')
    await opening.getByRole('button', { name: '故事开局', exact: true }).scrollIntoViewIfNeeded()
    await expect(opening).toBeVisible()
    expect(await opening.evaluate(element => Boolean(element.compareDocumentPosition(document.querySelector('[data-nova-chat-item]')!) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true)
    await page.reload()
    await expect(opening).toHaveCount(1)
    await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
  })
}
