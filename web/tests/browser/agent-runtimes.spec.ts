import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'
import enRuntime from '../../src/i18n/locales/en-US/agentRuntime'
import zhRuntime from '../../src/i18n/locales/zh-CN/agentRuntime'

for (const engine of ['codex', 'claude'] as const) {
for (const language of ['zh-CN', 'en-US']) {
  test(`${engine} CLI sign-in guidance and connection recheck work in ${language}`, async ({ page, request }) => {
    await createAndOpenBook(request, `${engine} CLI credentials ${language}`)
    const initial = await (await request.get('/api/settings')).json()
    const saved = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: initial.revisions.user,
      changes: { language, agent_runtimes: { ide: { selected: engine } } },
    } })
    expect(saved.ok(), await saved.text()).toBe(true)
    let checked = false
    const catalogResponse = await request.get('/api/agent-runtimes')
    expect(catalogResponse.ok()).toBe(true)
    const catalog = await catalogResponse.json()
    await page.route('**/api/agent-runtimes', async route => {
      await route.fulfill({ json: { items: catalog.items.map((item: { id: string }) => item.id === engine
        ? { ...item, status: 'auth_required' } : item) } })
    })
    await page.route(`**/api/agent-runtimes/${engine}/check`, async route => {
      expect(route.request().method()).toBe('POST')
      checked = true
      await route.fulfill({ json: { id: engine, name_key: `agentRuntime.${engine}`, status: 'ready' } })
    })
    await page.route(`**/api/agent-runtimes/${engine}/models`, async route => {
      expect(checked).toBe(true)
      await route.fulfill({ json: { items: [{ id: 'local-model', display_name: 'Local model', efforts: ['high'] }] } })
    })
    await page.goto('/')
    await page.getByRole('button', { name: 'Agents', exact: true }).click()
    const runtime = page.locator('[data-agent-configuration-section="runtime"]')
    const messages = language === 'zh-CN' ? zhRuntime : enRuntime
    await expect(runtime.getByText(messages['agentRuntime.defaultsOnly'], { exact: true })).toBeVisible()
    const hint = engine === 'claude'
      ? (language === 'zh-CN' ? '请在运行 Denova 的电脑上执行 claude auth login，然后重新检查连接。' : 'Run claude auth login on the computer running Denova, then check the connection again.')
      : language === 'zh-CN'
      ? '请在运行 Denova 的电脑上打开终端，执行 codex login，然后重新检查连接。'
      : 'Run codex login in a terminal on the computer running Denova, then check the connection again.'
    const model = runtime.getByRole('combobox', { name: language === 'zh-CN' ? '引擎模型' : 'Engine model', exact: true })
    await expect(runtime.getByText(hint, { exact: true })).toBeVisible()
    await expect(model).toBeDisabled()
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`cli-login-${language}.png`), animations: 'disabled' })
    await runtime.getByRole('button', { name: language === 'zh-CN' ? '检查连接' : 'Check connection', exact: true }).click()
    await expect(runtime.getByText(hint, { exact: true })).toHaveCount(0)
    await expect(model).toBeEnabled()
    await model.click()
    await page.getByRole('option', { name: 'Local model', exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide[engine])
      .toEqual({ model: 'local-model' })
  })
}

}

for (const theme of ['dark', 'light']) {
  test(`Agent runtime configuration preserves dormant settings in ${theme}`, async ({ page, request }) => {
    test.setTimeout(90_000)
    await createAndOpenBook(request, `Runtime Settings ${theme}`)
    const initial = await (await request.get('/api/settings')).json()
    const seeded = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: initial.revisions.user,
      changes: { theme, agent_runtimes: { ide: { selected: 'native', codex: { model: 'engine-model', effort: 'high' }, claude: { model: 'sonnet', effort: 'medium' } } },
        agent_context: { ide: { compaction_threshold: 0.73 } } },
    } })
    expect(seeded.ok(), await seeded.text()).toBe(true)
    // Exercise the product settings API and UI without signing a real account
    // in. Engine protocol execution has separate actual-CLI integration tests.
    const catalogResponse = await request.get('/api/agent-runtimes')
    expect(catalogResponse.ok()).toBe(true)
    const catalog = await catalogResponse.json()
    await page.route('**/api/agent-runtimes', async (route) => {
      await route.fulfill({ json: { items: catalog.items.map((item: { id: string }) => item.id === 'codex' ? { ...item, status: 'ready' } : item) } })
    })
    await page.route('**/api/agent-runtimes/codex/models', (route) => route.fulfill({ json: { items: [{ id: 'engine-model', display_name: 'Engine model', efforts: ['medium', 'high'] }] } }))
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: 'Agents', exact: true }).click()
    const engine = page.getByRole('combobox', { name: '执行引擎', exact: true })
    await expect(engine).toHaveText('Native')
    await engine.click()
    await page.getByRole('option', { name: 'Codex', exact: true }).click()
    await expect(page.getByRole('combobox', { name: '引擎模型', exact: true })).toHaveText('Engine model')
    const runtime = page.locator('[data-agent-configuration-section="runtime"]')
    await expect(runtime.getByRole('button', { name: '登录', exact: true })).toHaveCount(0)
    await expect(runtime.getByRole('button', { name: '退出登录', exact: true })).toHaveCount(0)
    await runtime.getByRole('button', { name: '运行时', exact: true }).click()
    await expect(engine).toBeHidden()
    await expect(runtime.getByText('Codex · Engine model', { exact: true })).toBeVisible()
    await runtime.getByRole('button', { name: '运行时', exact: true }).click()
    await expect(page.getByRole('combobox', { name: '引擎模型', exact: true })).toHaveText('Engine model')
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes?.ide?.selected).toBe('codex')
    const saved = await (await request.get('/api/settings')).json()
    expect(saved.user.agent_context.ide.compaction_threshold).toBe(0.73)
    expect(saved.user.agent_runtimes.ide.codex).toEqual({ model: 'engine-model', effort: 'high' })
    // Effort edits must keep the model in the atomic engine settings patch.
    const effort = page.getByRole('combobox', { name: '推理强度', exact: true })
    await effort.click()
    await page.getByRole('option', { name: 'medium', exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide.codex)
      .toEqual({ model: 'engine-model', effort: 'medium' })
    await effort.click()
    await page.getByRole('option', { name: '使用模型默认值', exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide.codex)
      .toEqual({ model: 'engine-model' })
    await page.reload()
    await expect(effort).toHaveText('使用模型默认值')
    await effort.click()
    await page.getByRole('option', { name: 'high', exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide.codex)
      .toEqual({ model: 'engine-model', effort: 'high' })
    expect(saved.agent_configuration.ide).toMatchObject({ selected: 'codex', sections: expect.arrayContaining([
      { id: 'shared.input_budget', owner: 'shared', state: 'editable' },
      { id: 'native.context_policy', owner: 'native', state: 'inactive', reason_key: 'agentRuntime.configuration.otherRuntime' },
    ]) })
    const inactive = page.locator('[data-agent-configuration-section="inactive-runtime"]')
    await inactive.getByRole('button', { name: '已保存的其他引擎配置', exact: true }).click()
    await expect(inactive.getByText('Native 的权限、上下文压缩、检查点指引和 Subagents 配置已保留，当前不生效。切回 Native 后恢复使用。')).toBeVisible()
    const threshold = inactive.getByRole('spinbutton').first()
    await expect(threshold).toHaveValue('73')
    await expect(threshold).toBeDisabled()
    await inactive.getByRole('button', { name: '已保存的其他引擎配置', exact: true }).click()
    await page.setViewportSize({ width: 1280, height: 900 })
    await engine.scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`runtime-${theme}-wide.png`), animations: 'disabled' })
    await page.setViewportSize({ width: 390, height: 844 })
    await engine.scrollIntoViewIfNeeded()
    await expect(engine).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`runtime-${theme}-narrow.png`), animations: 'disabled' })
    await engine.click()
    await page.getByRole('option', { name: 'Claude Code', exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide.selected).toBe('claude')
    const claudeSaved = await (await request.get('/api/settings')).json()
    expect(claudeSaved.user.agent_runtimes.ide.claude).toEqual({ model: 'sonnet', effort: 'medium' })
    expect(claudeSaved.user.agent_runtimes.ide.codex).toEqual({ model: 'engine-model', effort: 'high' })
    await engine.click()
    await page.getByRole('option', { name: 'Native', exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes?.ide?.selected).toBe('native')
    const restored = await (await request.get('/api/settings')).json()
    expect(restored.user.agent_runtimes.ide.codex).toEqual({ model: 'engine-model', effort: 'high' })
    expect(restored.user.agent_context.ide.compaction_threshold).toBe(0.73)
  })
}

test('restores parallel external questions and routes each answer or cancellation to its original ID', async ({ page, request }) => {
  await createAndOpenBook(request, 'Runtime questions')
  const questions = ['tone', 'length'].map(id => ({ schema: 'ask.pending.v1', id: `ask-${id}`, tool_call_id: `external-${id}`, agent_operation_id: 'external-fixture', agent_kind: 'ide', status: 'pending', allow_other: true,
    questions: [{ id, question: `Choose ${id}`, options: [] }] }))
  const pending = new Set(questions.map(question => question.id))
  let answered: unknown
  let cancelled: unknown
  await page.route('**/chat/active?*', route => route.fulfill({ json: { active: false, pending_asks: questions.filter(question => pending.has(question.id)) } }))
  await page.route('**/session/asks/ask-tone/answer', route => {
    answered = route.request().postDataJSON()
    pending.delete('ask-tone')
    return route.fulfill({ json: { schema: 'ask.result.v1', id: 'ask-tone', status: 'answered', answers: [{ question_id: 'tone', question: 'Choose tone', custom_input: 'Restrained' }] } })
  })
  await page.route('**/session/asks/ask-length/cancel', route => {
    cancelled = route.request().postDataJSON()
    pending.delete('ask-length')
    return route.fulfill({ json: { schema: 'ask.result.v1', id: 'ask-length', status: 'cancelled', cancel_reason: 'user_cancelled' } })
  })
  await page.goto('/')
  await openWritingAgent(page)
  await expect(page.getByRole('textbox', { name: 'Choose tone', exact: true })).toHaveCount(1)
  await expect(page.getByRole('textbox', { name: 'Choose length', exact: true })).toHaveCount(1)
  await page.reload()
  await openWritingAgent(page)
  const tone = page.locator('section').filter({ has: page.getByRole('textbox', { name: 'Choose tone', exact: true }) }).last()
  await tone.getByRole('textbox', { name: 'Choose tone', exact: true }).fill('Restrained')
  await tone.getByRole('button', { name: '提交', exact: true }).click()
  await expect.poll(() => answered).toMatchObject({ answers: [{ question_id: 'tone', custom_input: 'Restrained' }] })
  // This UI fixture supplies the recovery projection, not canonical history.
  // Reload after resolving one card to restore the independently pending one.
  await page.reload()
  await openWritingAgent(page)
  await expect(page.getByRole('textbox', { name: 'Choose length', exact: true })).toHaveCount(1)
  const length = page.locator('section').filter({ has: page.getByRole('textbox', { name: 'Choose length', exact: true }) }).last()
  await length.getByRole('button', { name: '取消', exact: true }).click()
  await expect.poll(() => cancelled).toMatchObject({ reason: 'user_cancelled' })
  await page.reload()
  await openWritingAgent(page)
  await expect(page.getByRole('textbox', { name: /^Choose / })).toHaveCount(0)
})

for (const engine of ['codex', 'claude'] as const) {
test(`${engine} runtime settings show version and upgrade guidance while unavailable`, async ({ page, request }) => {
  await createAndOpenBook(request, `${engine} unavailable runtime`)
  const initial = await (await request.get('/api/settings')).json()
  const model = `engine-${'long-model-name-'.repeat(12)}`
  const saved = await request.patch('/api/settings', { data: { layer: 'user', base_revision: initial.revisions.user, changes: { language: 'en-US', agent_runtimes: { ide: { selected: engine, [engine]: { model } } } } } })
  expect(saved.ok(), await saved.text()).toBe(true)
  expect((await request.get('/api/agent-runtimes/unknown/models')).status()).toBe(404)
  let incompatible = false
  // The directory is fixed for this UI journey. Reloads must not re-probe host
  // CLIs through a forwarded request that navigation can interrupt.
  const catalogResponse = await request.get('/api/agent-runtimes')
  expect(catalogResponse.ok()).toBe(true)
  const catalog = await catalogResponse.json()
  await page.route('**/api/agent-runtimes', async route => {
    await route.fulfill({ json: { items: catalog.items.map((item: { id: string }) => item.id === engine ? {
      ...item, status: incompatible ? 'incompatible' : 'not_installed',
      reason_key: incompatible ? (engine === 'codex' ? 'agentRuntime.incompatibleVersion' : 'agentRuntime.claudeIncompatibleVersion') : 'agentRuntime.notInstalled',
    } : item) } })
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Agents', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Execution engine', exact: true })).toHaveText(engine === 'codex' ? 'Codex' : 'Claude Code')
  await expect(page.getByRole('combobox', { name: 'Engine model', exact: true })).toBeDisabled()
  await expect(page.getByText('Not installed', { exact: true })).toBeVisible()
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`runtime-english-${width}.png`), animations: 'disabled' })
  }
  incompatible = true
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.reload()
  await page.getByRole('button', { name: 'Agents', exact: true }).click()
  await expect(page.getByText(engine === 'codex' ? 'The Codex CLI version is too old or unrecognized. Minimum supported version: 0.130.0. Run codex update on the computer running Denova (for npm installations, use npm install -g @openai/codex@latest), then click Check connection.' : 'The Claude Code version is too old or unrecognized. Minimum supported version: 2.1.259. Run claude update on the computer running Denova (for Homebrew installations, use brew upgrade claude-code), then click Check connection.', { exact: true })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('runtime-minimum-version-en.png'), animations: 'disabled' })
  const current = await (await request.get('/api/settings')).json()
  const localized = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user, changes: { language: 'zh-CN', theme: 'light' } } })
  expect(localized.ok(), await localized.text()).toBe(true)
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.reload()
  await page.getByRole('button', { name: 'Agents', exact: true }).click()
  await expect(page.getByText(engine === 'codex' ? 'Codex CLI 版本过旧或无法识别，最低支持版本为 0.130.0。请在运行 Denova 的电脑上执行 codex update（npm 安装可执行 npm install -g @openai/codex@latest），更新后点击「检查连接」。' : 'Claude Code 版本过旧或无法识别，最低支持版本为 2.1.259。请在运行 Denova 的电脑上执行 claude update（Homebrew 安装使用 brew upgrade claude-code），更新后点击「检查连接」。', { exact: true })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('runtime-minimum-version-zh.png'), animations: 'disabled' })
})

}
