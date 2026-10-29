import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { getCapturedModelRequest } from '../support/model'

// Put the prompts after a large extension to catch truncation-before-parsing.
const preset = {
  extensions: { tavern_helper: { scripts: [{ content: 'IGNORED_EXTENSION'.repeat(75000) }] } },
  temperature: 0.1,
  prompts: [
    { identifier: 'unused', name: '未启用规则', content: 'UNSELECTED_RULE' },
    { identifier: 'voice', name: '对白约束', content: '{{setvar::voice::E2E_NARRATIVE_IMPORT At least 40% direct dialogue.}}' },
    { identifier: 'style', name: '白描', content: '{{setvar::style::Concrete gestures; no narrator mind reading.}}' },
    { identifier: 'apply', name: '应用写法', content: '{{getvar::voice}}{{getvar::style}}' },
    { identifier: 'history', name: '聊天历史', marker: true },
    ...Array.from({ length: 200 }, (_, index) => ({ identifier: `reference-${index}`, name: `未选参考 ${index}`, content: 'UNSELECTED_RULE' })),
  ],
  prompt_order: [{ character_id: 100001, order: [
    { identifier: 'voice', enabled: true }, { identifier: 'unused', enabled: false },
    { identifier: 'style', enabled: false }, { identifier: 'apply', enabled: true }, { identifier: 'history', enabled: true },
  ] }],
}

for (const mode of ['writing', 'game'] as const) for (const method of ['direct', 'ai'] as const) {
  test(`imports an editable native style via ${method} from ${mode}`, async ({ page, request, browserDiagnostics }, testInfo) => {
    await createAndOpenBook(request, `Narrative import ${mode}`)
    const story = mode === 'game' ? await createStartedStory(request, 'Narrative import game') : null
    const initialStories = (await (await request.get('/api/interactive/stories')).json()).stories
    const before = await (await request.get('/api/settings')).json()
    expect((await request.patch('/api/settings', { data: { layer: 'user', base_revision: before.revisions.user, changes: {
      theme: mode === 'writing' ? 'dark' : 'light', language: 'zh-CN',
    } } })).ok()).toBe(true)
    const initialCatalog = (await (await request.get('/api/interactive/tellers')).json()).tellers
    await page.setViewportSize({ width: 1680, height: 1000 })
    await page.goto('/')
    const sidebar = page.getByLabel('工作台侧边栏')
    await sidebar.getByRole('button', { name: mode === 'writing' ? '写作' : '游戏', exact: true }).click()
    await sidebar.getByRole('button', { name: '创作方案', exact: true }).click()
    await page.getByRole('button', { name: '添加叙事风格', exact: true }).click()
    await page.getByRole('menuitem', { name: '导入 SillyTavern 预设', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '导入 SillyTavern 预设', exact: true })
    if (mode === 'game') await page.setViewportSize({ width: 390, height: 844 })
    const submit = dialog.getByRole('button', { name: method === 'direct' ? '直接导入' : 'AI 转换', exact: true })
    const upload = dialog.getByLabel('预设 JSON 文件')
    // Invalid input leaves a useful error and cannot start a model request.
    await upload.setInputFiles({ name: 'invalid.json', mimeType: 'application/json', buffer: Buffer.from('{}') })
    await expect(dialog.getByRole('alert')).toContainText('无法读取此预设')
    await expect(submit).toBeDisabled()
    await upload.setInputFiles({ name: 'Source preset.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(preset)) })
    await expect(dialog.getByRole('alert')).toHaveCount(0)
    await expect(dialog.getByRole('checkbox')).toHaveCount(204)
    await expect(dialog.getByRole('checkbox', { name: '对白约束', exact: true })).toBeChecked()
    await expect(dialog.getByRole('checkbox', { name: '白描', exact: true })).toBeChecked()
    await expect(dialog.getByText('聊天历史', { exact: true })).toHaveCount(0)
    if (method === 'direct') {
      await expect(dialog.getByRole('checkbox', { checked: true })).toHaveCount(204)
      await dialog.getByRole('textbox', { name: '搜索条目', exact: true }).fill('白描')
      await dialog.getByRole('button', { name: '取消全选', exact: true }).click()
      await expect(submit).toBeDisabled()
      await dialog.getByRole('button', { name: '仅选预设启用', exact: true }).click()
      await expect(dialog.getByRole('checkbox', { name: '白描', exact: true })).not.toBeChecked()
      await dialog.getByRole('button', { name: '全选', exact: true }).click()
      await dialog.getByRole('textbox', { name: '搜索条目', exact: true }).fill('')
      await expect(dialog.getByRole('checkbox', { checked: true })).toHaveCount(204)
      await expect(dialog.getByText('启用 2 项', { exact: false })).toBeVisible()
    } else {
      await dialog.getByRole('button', { name: '仅选预设启用', exact: true }).click()
      await dialog.getByRole('checkbox', { name: '白描', exact: true }).check()
    }
    await dialog.getByRole('button', { name: '白描', exact: true }).click()
    await expect(dialog.getByText('{{setvar::style::Concrete gestures; no narrator mind reading.}}', { exact: true })).toBeVisible()
    const importedName = `Imported ${method} ${mode}`
    await dialog.getByLabel('新风格名称').fill(importedName)
    const bounds = await dialog.boundingBox()
    expect(bounds!.x).toBeGreaterThanOrEqual(0)
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width)
    expect(bounds!.y).toBeGreaterThanOrEqual(0)
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(page.viewportSize()!.height)
    await dialog.getByRole('textbox', { name: '搜索条目', exact: true }).fill('没有此条目')
    await expect(dialog.getByText('没有匹配的条目', { exact: true })).toBeVisible()
    await expect(submit).toBeEnabled()
    await dialog.getByRole('textbox', { name: '搜索条目', exact: true }).fill('')
    await page.screenshot({ path: testInfo.outputPath(`import-${method}-${mode}.png`) })
    const agentRequests: string[] = []
    page.on('request', req => {
      if (req.method() === 'POST' && /\/(agent-chat\/chat|interactive\/chat)$/.test(new URL(req.url()).pathname)) agentRequests.push(req.url())
    })
    if (method === 'direct' && mode === 'writing') {
      browserDiagnostics.allow(/^console\.error: Failed to load resource:.*400.*\/api\/interactive\/tellers/)
      await page.route('**/api/interactive/tellers', async route => {
        if (route.request().method() === 'POST') await route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'E2E rejected create' }) })
        else await route.continue()
      })
      await submit.click()
      await expect(dialog.getByRole('alert')).toContainText('导入失败，所选内容已保留')
      await expect(dialog.getByLabel('新风格名称')).toHaveValue(importedName)
      await expect(dialog.getByRole('checkbox', { checked: true })).toHaveCount(204)
      expect((await (await request.get('/api/interactive/tellers')).json()).tellers).toHaveLength(initialCatalog.length)
      await page.unroute('**/api/interactive/tellers')
    }
    await submit.click()
    await expect(dialog).toHaveCount(0)
    const editor = page.getByTestId('teller-editor')
    await expect(editor.getByRole('textbox').first()).toHaveValue(importedName)
    const catalog = (await (await request.get('/api/interactive/tellers')).json()).tellers
    expect(catalog).toHaveLength(initialCatalog.length + 1)
    const imported = catalog.find((item: { name: string }) => item.name === importedName)
    expect(imported.custom).toBe(true)
    expect(imported.slots.map((slot: { content: string }) => slot.content).join('\n')).toContain('At least 40%')
    expect(JSON.stringify(imported)).not.toContain('IGNORED_EXTENSION')
    if (method === 'direct') {
      const ordered = ['voice', 'unused', 'style', 'apply', ...Array.from({ length: 200 }, (_, index) => `reference-${index}`)]
      expect(imported.slots).toEqual(ordered.map((id, index) => {
        const source = preset.prompts.find(prompt => prompt.identifier === id)!
        return { id: `prompt-${index + 1}`, name: source.name, content: source.content, target: 'system', enabled: id === 'voice' || id === 'apply' }
      }))
      expect(agentRequests).toEqual([])
    } else expect(JSON.stringify(imported)).not.toMatch(/UNSELECTED_RULE|\{\{/)
    for (const original of initialCatalog) expect(catalog.find((item: { id: string }) => item.id === original.id)).toEqual(original)
    // Import does not activate the new style or change model parameters.
    const after = await (await request.get('/api/settings')).json()
    expect(after.user.ide_story_teller_id).toEqual(before.user.ide_story_teller_id)
    expect(after.user.model_profiles).toEqual(before.user.model_profiles)
    if (story) {
      const stories = (await (await request.get('/api/interactive/stories')).json()).stories
      expect(stories.find((item: { id: string }) => item.id === story.id).story_teller_id)
        .toBe(initialStories.find((item: { id: string }) => item.id === story.id).story_teller_id)
    }
    // Native editing persists without a converter or a source file at runtime.
    await editor.getByRole('textbox').first().fill(`${importedName} edited`)
    await expect.poll(async () => {
      const items = (await (await request.get('/api/interactive/tellers')).json()).tellers
      return items.find((item: { id: string }) => item.id === imported.id)?.name
    }).toBe(`${importedName} edited`)
    await page.reload()
    if (mode === 'game') await page.getByRole('button', { name: '创作方案目录', exact: true }).click()
    await expect(page.getByText(`${importedName} edited`, { exact: true }).first()).toBeVisible()

    // Selecting the saved native resource reaches the actual model in both products.
    if (story) {
      expect((await request.patch(`/api/interactive/stories/${story.id}`, { data: { story_teller_id: imported.id } })).ok()).toBe(true)
    } else {
      const settings = await (await request.get('/api/settings')).json()
      expect((await request.patch('/api/settings', { data: { layer: 'user', base_revision: settings.revisions.user, changes: { ide_story_teller_id: imported.id } } })).ok()).toBe(true)
    }
    await page.setViewportSize({ width: 1680, height: 1000 })
    await page.reload()
    if (story) await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
    const composer = story ? page.getByPlaceholder(/你要做什么/) : await openWritingAgent(page)
    await expect(composer).toBeVisible()
    const marker = `${method === 'direct' ? 'E2E_DIRECT_USE' : 'E2E_IMPORTED_USE'}_${mode.toUpperCase()}`
    await submitAgentChatMessage(page, composer, marker)
    await expect.poll(async () => getCapturedModelRequest(request, marker)).not.toBeNull()
    const modelRequest = await getCapturedModelRequest(request, marker)
    const system = modelRequest!.messages.filter(message => message.role === 'system').map(message => message.content).join('\n')
    const context = modelRequest!.messages.filter(message => message.role !== 'system').map(message => message.content).join('\n')
    if (method === 'direct') {
      expect(system).toContain('{{setvar::voice::E2E_NARRATIVE_IMPORT At least 40% direct dialogue.}}')
      expect(system).not.toContain('UNSELECTED_RULE')
      expect(system).not.toContain('Concrete gestures; no narrator mind reading.')
    } else {
      expect(system).toContain('E2E_IMPORTED_DIALOGUE: At least 40%')
      expect(story ? context : system).toContain('E2E_IMPORTED_VIEWPOINT: Convey emotion through concrete gestures')
    }
  })
}
