import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, createStory } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { getCapturedModelRequest } from '../support/model'

test('tunes story rules in the existing console without changing shared styles', async ({ page, request, browserDiagnostics }, testInfo) => {
  test.setTimeout(180_000)
  await createAndOpenBook(request, 'Story narrative adjustments')
  const style = {
    id: `story-rules-${Date.now()}`, name: '可随时调整的叙事风格',
    slots: [
      { id: 'voice', name: '对白约束', target: 'system', enabled: true, content: 'SHARED_VOICE_RULE: Use concrete dialogue.' },
      { id: 'detail', name: '细节描写', target: 'turn_context', enabled: false, content: 'SHARED_DETAIL_RULE: Use sensory details.' },
      ...Array.from({ length: 210 }, (_, index) => ({ id: `extra-${index}`, name: `长名称规则 ${index} ${'用于窄屏检查'.repeat(8)}`, target: 'system', enabled: false, content: 'Optional rule.' })),
    ],
  }
  const created = await request.post('/api/interactive/tellers', { data: style })
  expect(created.ok(), await created.text()).toBe(true)
  const original = await created.json()
  const settings = await (await request.get('/api/settings')).json()
  expect((await request.patch('/api/settings', { data: { layer: 'user', base_revision: settings.revisions.user, changes: {
    theme: 'dark', language: 'zh-CN', ide_story_teller_id: style.id,
  } } })).ok()).toBe(true)
  const other = await createStory(request, 'Unchanged other story')
  const story = await createStartedStory(request, 'Adjustable story')
  for (const id of [other.id, story.id]) expect((await request.patch(`/api/interactive/stories/${id}`, { data: { story_teller_id: style.id } })).ok()).toBe(true)
  const readStory = async (id = story.id) => (await (await request.get('/api/interactive/stories')).json()).stories.find((item: { id: string }) => item.id === id)
  await page.setViewportSize({ width: 1680, height: 1000 })
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  await page.getByRole('tab', { name: '控制', exact: true }).click()
  const card = page.locator('[data-control-section="narrative"]')
  await expect(page.locator('.director-console').getByRole('tab')).toHaveCount(3)
  await expect(card.getByRole('combobox', { name: '叙事风格' })).toContainText(style.name)
  await card.getByRole('button', { name: '叙事风格', exact: true }).click()
  await expect(card.getByRole('combobox')).toBeVisible()
  await expect(card.getByRole('switch', { name: '对白约束', exact: true })).toBeHidden()
  await card.getByRole('button', { name: '叙事风格', exact: true }).click()
  await card.getByRole('switch', { name: '对白约束', exact: true }).uncheck()
  await card.getByRole('switch', { name: '细节描写', exact: true }).check()
  await card.getByRole('button', { name: '细节描写', exact: true }).click()
  const content = card.getByRole('textbox', { name: '细节描写的规则内容' })
  browserDiagnostics.allow(/^console\.error: Failed to load resource:.*400.*\/api\/interactive\/stories/)
  browserDiagnostics.allow(/^console\.error: \[story-narrative-style\] Failed to save story rules/)
  await expect.poll(async () => (await readStory()).narrative_style_overrides?.slots.detail?.enabled).toBe(true)
  await page.route(`**/api/interactive/stories/${story.id}`, async route => {
    if (route.request().method() === 'PATCH') await route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'E2E save rejected' }) })
    else await route.continue()
  })
  await content.fill('LOCAL_DETAIL_RULE: Describe rain on the window.')
  await expect(card.getByRole('status')).toHaveAttribute('aria-label', /保存失败/)
  await expect(content).toHaveValue('LOCAL_DETAIL_RULE: Describe rain on the window.')
  await page.unroute(`**/api/interactive/stories/${story.id}`)

  // Leaving Controls flushes the editor instead of discarding its delayed save.
  await page.getByRole('tab', { name: '总览', exact: true }).click()
  await expect.poll(async () => (await readStory()).narrative_style_overrides).toEqual({ style_id: style.id, slots: {
    voice: { enabled: false }, detail: { enabled: true, content: 'LOCAL_DETAIL_RULE: Describe rain on the window.' },
  } })
  await page.reload()
  await page.getByRole('tab', { name: '控制', exact: true }).click()
  await expect(card.getByRole('switch', { name: '对白约束', exact: true })).not.toBeChecked()
  await expect(card.getByRole('switch', { name: '细节描写', exact: true })).toBeChecked()
  expect((await readStory(other.id)).narrative_style_overrides).toBeUndefined()
  expect(await (await request.get(`/api/interactive/tellers/${style.id}`)).json()).toEqual(original)

  for (const theme of ['dark', 'light']) {
    expect((await request.patch('/api/settings', { data: { layer: 'user', changes: { theme } } })).ok()).toBe(true)
    await page.reload()
    for (const width of [1680, 390]) {
      await page.setViewportSize({ width, height: 1000 })
      const showConsole = page.getByRole('button', { name: '显示控制台', exact: true })
      if (await showConsole.isVisible()) await showConsole.click()
      await page.getByRole('tab', { name: '控制', exact: true }).click()
      await card.getByRole('textbox', { name: '搜索条目' }).fill('长名称规则 20')
      await expect(card.getByRole('switch').nth(1)).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      const bounds = (await card.boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
      await page.screenshot({ path: testInfo.outputPath(`story-rules-${theme}-${width}.png`) })
      await card.getByRole('textbox', { name: '搜索条目' }).fill('不存在的规则')
      await expect(card.getByText('没有匹配的条目')).toBeVisible()
      await card.getByRole('textbox', { name: '搜索条目' }).fill('')
    }
    await page.setViewportSize({ width: 1680, height: 1000 })
  }
  await page.reload()
  await page.getByRole('tab', { name: '控制', exact: true }).click()
  await card.getByRole('checkbox', { name: '仅看已启用' }).check()
  await expect(card.getByRole('switch')).toHaveCount(2)
  await card.getByRole('button', { name: '叙事风格操作' }).click()
  await page.getByRole('menuitem', { name: '另存为叙事风格' }).click()
  const dialog = page.getByRole('dialog', { name: '另存为叙事风格' })
  await dialog.getByLabel('新风格名称').fill('Saved story adjustments')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  const copied = (await (await request.get('/api/interactive/tellers')).json()).tellers.find((item: { name: string }) => item.name === 'Saved story adjustments')
  expect(copied.slots[0].enabled).toBe(false)
  expect(copied.slots[1]).toMatchObject({ enabled: true, content: 'LOCAL_DETAIL_RULE: Describe rain on the window.' })

  await submitAgentChatMessage(page, page.getByPlaceholder(/你要做什么/), 'E2E_STORY_RULES_GAME')
  await expect.poll(() => getCapturedModelRequest(request, 'E2E_STORY_RULES_GAME')).not.toBeNull()
  const model = JSON.stringify((await getCapturedModelRequest(request, 'E2E_STORY_RULES_GAME'))!.messages)
  expect(model).toContain('LOCAL_DETAIL_RULE')
  expect(model).not.toContain('SHARED_VOICE_RULE')
  expect(model).not.toContain('SHARED_DETAIL_RULE')
  await expect.poll(async () => (await readStory()).turn_count).toBeGreaterThanOrEqual(2)
  await card.getByRole('button', { name: '叙事风格操作' }).click()
  await page.getByRole('menuitem', { name: '恢复预设' }).click()
  await expect.poll(async () => (await readStory()).narrative_style_overrides).toBeUndefined()
  await submitAgentChatMessage(page, page.getByPlaceholder(/你要做什么/), 'E2E_STORY_RULES_RESET')
  await expect.poll(() => getCapturedModelRequest(request, 'E2E_STORY_RULES_RESET')).not.toBeNull()
  const resetModel = JSON.stringify((await getCapturedModelRequest(request, 'E2E_STORY_RULES_RESET'))!.messages)
  expect(resetModel).toContain('SHARED_VOICE_RULE')
  expect(resetModel).not.toContain('LOCAL_DETAIL_RULE')
  await expect.poll(async () => (await readStory()).turn_count).toBeGreaterThanOrEqual(3)
  await card.getByRole('checkbox', { name: '仅看已启用' }).uncheck()
  await card.getByRole('switch', { name: '对白约束', exact: true }).uncheck()
  await card.getByRole('combobox').click()
  await page.getByRole('option', { name: '稳健叙事', exact: true }).click()
  await expect.poll(async () => (await readStory()).story_teller_id).toBe('classic')
  expect((await readStory()).narrative_style_overrides).toBeUndefined()
  await card.getByRole('combobox').click()
  await page.getByRole('option', { name: style.name, exact: true }).click()
  await expect(card.getByRole('switch', { name: '对白约束', exact: true })).toBeChecked()
  const composer = await openWritingAgent(page)
  await submitAgentChatMessage(page, composer, 'E2E_STORY_RULES_WRITING')
  await expect.poll(() => getCapturedModelRequest(request, 'E2E_STORY_RULES_WRITING')).not.toBeNull()
  const writingModel = JSON.stringify((await getCapturedModelRequest(request, 'E2E_STORY_RULES_WRITING'))!.messages)
  expect(writingModel).toContain('SHARED_VOICE_RULE')
  expect(writingModel).not.toContain('LOCAL_DETAIL_RULE')
})
