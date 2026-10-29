import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import zh from '../../src/i18n/locales/zh-CN/agents'
import en from '../../src/i18n/locales/en-US/agents'

for (const [language, theme, labels] of [['zh-CN', 'dark', zh], ['en-US', 'light', en]] as const) {
  test(`Agent editors adapt to available pane width in ${language} ${theme}`, async ({ page, request }) => {
    await createAndOpenBook(request, `Agent design ${language}`)
    const settings = await (await request.get('/api/settings')).json()
    const saved = await request.patch('/api/settings', { data: { layer: 'user', base_revision: settings.revisions.user,
      changes: { language, theme, agent_prompts: { ide: { system_prompt: '' } } },
    } })
    expect(saved.ok(), await saved.text()).toBe(true)
    await page.goto('/')
    await page.getByRole('button', { name: 'Agents', exact: true }).click()
    const custom = page.getByRole('textbox', { name: labels['agents.prompt.source.custom'], exact: true })
    await expect(custom).toBeVisible()
    await expect(page.getByRole('textbox', { name: labels['agents.prompt.source.flow'], exact: true })).toHaveCount(0)
    await expect(page.getByRole('radio', { name: labels['agents.layer.user'], exact: true })).toBeChecked()
    for (const width of [1440, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 844 })
      await expect(custom).toBeInViewport()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`behavior-${width}.png`), animations: 'disabled' })
      if (width === 320) {
        await page.getByRole('tab', { name: labels['agents.tabs.behavior'], exact: true }).press('End')
        const contextTab = page.getByRole('tab', { name: labels['agents.tabs.context'], exact: true })
        await expect(contextTab).toBeFocused()
        // Native scrolling rounds fractional CSS pixels at the clip boundary.
        await expect(contextTab).toBeInViewport({ ratio: 0.99 })
        await contextTab.press('Home')
        await expect(custom).toBeVisible()
      }
    }
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.getByRole('tab', { name: labels['agents.tabs.runtime'], exact: true }).click()
    await page.getByRole('button', { name: labels['agents.configAgent.button'], exact: true }).click()
    const engine = page.getByRole('combobox', { name: language === 'zh-CN' ? '执行引擎' : 'Execution engine', exact: true })
    await expect(engine).toBeVisible()
    expect((await engine.boundingBox())!.width).toBeGreaterThan(160)
    await expect(engine).toHaveText('Native')
    await page.screenshot({ path: test.info().outputPath('runtime-with-assistant.png'), animations: 'disabled' })
    await page.getByRole('tab', { name: labels['agents.tabs.runtime'], exact: true }).press('ArrowRight')
    await expect(page.getByRole('tab', { name: labels['agents.tabs.capabilities'], exact: true })).toBeFocused()
    await expect(page.getByRole('tab', { name: labels['agents.tabs.capabilities'], exact: true })).toHaveAttribute('aria-selected', 'true')
  })
}

test('Agent scopes restore inherited rules and creation follows the selected game contract', async ({ page, request }) => {
  test.setTimeout(60_000)
  const book = await createAndOpenBook(request, 'Agent inheritance')
  const settings = await (await request.get('/api/settings')).json()
  expect((await request.patch('/api/settings', { data: { layer: 'user', base_revision: settings.revisions.user,
    changes: { language: 'zh-CN', agent_prompts: { ide: { system_prompt: 'Inherited creative rule' } } },
  } })).ok()).toBe(true)
  await page.goto('/')
  await page.getByRole('button', { name: 'Agents', exact: true }).click()
  await page.getByRole('radio', { name: '当前工作区', exact: true }).click()
  await expect(page.getByRole('radio', { name: '当前工作区', exact: true })).toBeChecked()
  const custom = page.getByRole('textbox', { name: '用户自定义', exact: true })
  await expect(custom).toHaveValue('Inherited creative rule')
  await custom.fill('Workspace creative rule')
  const prompts = async () => (await (await request.get(`/api/projects/${book.projectId}/settings`)).json()).workspace.agent_prompts?.ide
  await expect.poll(prompts).toMatchObject({ system_prompt: 'Workspace creative rule' })
  await page.getByRole('button', { name: '恢复用户自定义的继承设置', exact: true }).click()
  await expect(custom).toHaveValue('Inherited creative rule')
  await expect.poll(async () => (await prompts())?.system_prompt ?? '').toBe('')
  await page.getByRole('button', { name: /^游戏 Agent 故事舞台/ }).click()
  await page.getByRole('button', { name: '新建自定义 Agent', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('combobox', { name: '基于哪个 Agent' })).toHaveText('游戏 Agent')
  const longName = '负责维护长篇互动故事中人物关系与世界设定一致性的叙事助手'
  await dialog.getByRole('textbox', { name: '显示名称' }).fill(longName)
  await dialog.getByRole('button', { name: '新建自定义 Agent', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1, name: longName })).toBeVisible()
  await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.custom_agents?.find((agent: { name: string }) => agent.name === longName)?.contract).toBe('game.narrator.v1')
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.getByRole('button', { name: '配置管理', exact: true }).click()
  const name = page.getByRole('textbox', { name: '显示名称', exact: true })
  const description = page.getByRole('textbox', { name: '描述', exact: true })
  await expect(name).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('custom-with-assistant.png'), animations: 'disabled' })
  // A narrow editor must use one full-width column even in a desktop viewport.
  await expect.poll(async () => Math.abs((await name.boundingBox())!.width - (await description.boundingBox())!.width)).toBeLessThan(2)
  await page.getByRole('tab', { name: '上下文', exact: true }).click()
  await page.getByRole('button', { name: '添加上下文', exact: true }).click()
  const contextName = page.getByRole('textbox', { name: '名称', exact: true })
  const contextPurpose = page.getByRole('textbox', { name: '模型可见用途（英文）', exact: true })
  await expect(contextName).toBeVisible()
  await expect.poll(async () => Math.abs((await contextName.boundingBox())!.width - (await contextPurpose.boundingBox())!.width)).toBeLessThan(2)
  const contextSlot = page.getByRole('combobox', { name: '注入位置', exact: true })
  await expect.poll(async () => Math.abs((await contextName.boundingBox())!.width - (await contextSlot.boundingBox())!.width)).toBeLessThan(2)
  await page.screenshot({ path: test.info().outputPath('context-with-assistant.png'), animations: 'disabled' })
  await page.getByRole('button', { name: '配置管理', exact: true }).click()
  await page.getByRole('tab', { name: '行为', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('long-custom-game.png'), animations: 'disabled' })
  await page.setViewportSize({ width: 1440, height: 844 })
  await page.getByRole('button', { name: /^写作 Agent 章节续写/ }).click()
  await page.getByRole('tab', { name: '能力', exact: true }).click()
  await page.getByRole('button', { name: '新增 SubAgent', exact: true }).click()
  const subAgentDialog = page.getByRole('dialog')
  const subAgentID = subAgentDialog.getByRole('textbox', { name: 'ID', exact: true })
  const subAgentName = subAgentDialog.getByRole('textbox', { name: '显示名称', exact: true })
  await expect(subAgentID).toBeVisible()
  await expect.poll(async () => Math.abs((await subAgentID.boundingBox())!.y - (await subAgentName.boundingBox())!.y)).toBeLessThan(2)
  await page.screenshot({ path: test.info().outputPath('sub-agent-wide.png'), animations: 'disabled' })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect.poll(async () => (await subAgentName.boundingBox())!.y - (await subAgentID.boundingBox())!.y).toBeGreaterThan(40)
  await page.screenshot({ path: test.info().outputPath('sub-agent-narrow.png'), animations: 'disabled' })
  await subAgentDialog.getByRole('button', { name: '关闭', exact: true }).click()
})
