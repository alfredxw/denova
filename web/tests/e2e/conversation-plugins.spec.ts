import { cp, readFile, readdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { test, expect, type APIRequestContext } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, createAgentChatSession, getStorySnapshot, readProjectFile } from '../support/api'
import { openWritingAgent, openAgentChatWorkbench, openAgentChatSession, submitAgentChatMessage } from '../support/agent-chat'

async function installTool(request: APIRequestContext, name = 'Conversation plugin source') {
  const response = await request.post('/api/agent-chat/projects/directory', { data: { name } })
  expect(response.ok(), await response.text()).toBe(true)
  const project = await response.json()
  const index = await (await request.get('/api/agent-chat/projects')).json()
  const directory = index.projects.find((item: { id: string }) => item.id === project.id).path
  for (const fixture of ['runtime', 'tool']) {
    const root = new URL('../../../internal/platform/testdata/' + fixture + '/', import.meta.url)
    for (const name of await readdir(root)) await cp(new URL(name, root), path.join(directory, name), { recursive: true })
  }
  const manifestPath = path.join(directory, 'denova.plugin.json')
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'))
  manifest.id = 'test.conversation-tools'
  manifest.name = { 'zh-CN': '写作与游戏共用的长名称插件'.repeat(6), 'en-US': 'Shared writing and game plugin with a long name '.repeat(6) }
  await writeFile(manifestPath, JSON.stringify(manifest))
  const candidates = await request.post('/api/platform/manage/packages/preview', { data: { directory } })
  expect(candidates.ok(), await candidates.text()).toBe(true)
  const candidate = await candidates.json()
  const installed = await request.post('/api/platform/manage/packages/install', { data: { candidateId: candidate.candidateId, grants: manifest.permissions.required } })
  expect(installed.ok(), await installed.text()).toBe(true)
  return { project, directory, release: await installed.json() }
}

test.beforeEach(async ({ request }) => {
  // Each journey owns its enabled plugins within the isolated test backend.
  const catalog = await (await request.get('/api/platform/manage/catalog')).json()
  for (const item of catalog.filter((entry: { kind: string; enabled: boolean }) => entry.kind === 'plugin' && entry.enabled)) {
    const response = await request.patch(`/api/platform/manage/packages/plugin/${item.id}`, { data: { enabled: false } })
    expect(response.ok(), await response.text()).toBe(true)
  }
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'zh-CN', theme: 'dark' } } })
})

test('enabled plugin tools work in writing, workbench and game without conversation setup', async ({ page, request }) => {
  test.slow()
  const { project } = await installTool(request)
  const book = await createAndOpenBook(request, 'Shared plugin integration')
  await page.goto('/')
  const composer = await openWritingAgent(page)
  await submitAgentChatMessage(page, composer, 'Use the plugin result to write a chapter. E2E_PLUGIN_CHAIN E2E_PLUGIN_WRITE')
  await expect(page.getByText('Plugin result adopted: 3.', { exact: true }).filter({ visible: true })).toBeVisible()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, 'chapters/plugin-result.md')).content).toBe('# Plugin result\n\nPlugin result adopted: 3.')
  await expect(page.getByRole('region', { name: '已编辑 1 个文件', exact: true })).toBeVisible()

  const session = await createAgentChatSession(request, project.id, 'Shared plugin workbench')
  await openAgentChatWorkbench(page)
  const workbenchComposer = await openAgentChatSession(page, project.id, session.title)
  await submitAgentChatMessage(page, workbenchComposer, 'Use the plugin result. E2E_PLUGIN_CHAIN')
  await expect(page.getByText('Plugin result adopted: 3.', { exact: true }).filter({ visible: true })).toBeVisible()

  const story = await createStartedStory(request, 'Shared plugin game')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const gameComposer = page.getByPlaceholder(/你要做什么/).filter({ visible: true })
  await gameComposer.fill('Use the plugin result. E2E_PLUGIN_CHAIN')
  await page.locator('[data-action="send"]').filter({ visible: true }).click()
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns.at(-1)?.narrative).toContain('Plugin result adopted: 3.')

  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '扩展', exact: true }).click()
  await expect(page.getByText('启用后，写作、工作台和内置游戏 Agent 均可调用本插件的工具，无需单独配置会话。')).toBeVisible()
  const article = page.getByRole('article').filter({ visible: true })
  await expect(article.getByText('下次任务开始时生效', { exact: true })).toBeVisible()
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    await page.screenshot({ path: `test-results/shared-plugin-dark-${width}.png`, fullPage: true })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  }
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'light' } } })
  await page.evaluate(() => { localStorage.setItem('theme', 'light'); localStorage.setItem('nova.locale.configured', 'en-US') })
  await page.reload()
  await expect(page.getByText('When enabled, this plugin provides tools to Writing, Workbench, and built-in Game Agents without per-conversation setup.')).toBeVisible()
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    await page.screenshot({ path: `test-results/shared-plugin-light-${width}.png`, fullPage: true })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  }
})

test('existing sessions use shared settings and stop receiving disabled tools', async ({ page, request }) => {
  const { project } = await installTool(request, 'Shared configuration source')
  const session = await createAgentChatSession(request, project.id, 'Existing shared configuration session')
  await page.goto('/')
  await openAgentChatWorkbench(page)
  const composer = await openAgentChatSession(page, project.id, session.title)
  await submitAgentChatMessage(page, composer, 'Use the plugin result. E2E_PLUGIN_CHAIN')
  await expect(page.getByText('Plugin result adopted: 3.', { exact: true }).filter({ visible: true })).toBeVisible()
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '扩展', exact: true }).click()
  const setting = page.getByRole('switch', { name: '启用测试行为', exact: true })
  await expect(setting).toBeVisible()
  await setting.check()
  await page.getByRole('button', { name: '保存设置', exact: true }).click()
  await expect(page.getByRole('button', { name: '保存设置', exact: true })).toBeDisabled()
  await openAgentChatWorkbench(page)
  await submitAgentChatMessage(page, composer, 'Use the changed shared settings. E2E_PLUGIN_CHAIN')
  await expect(page.getByText('Plugin result adopted: 2.', { exact: true }).filter({ visible: true })).toBeVisible()
  const endpoint = '/api/platform/manage/packages/plugin/test.conversation-tools'
  const disabled = await request.patch(endpoint, { data: { enabled: false } })
  expect(disabled.ok(), await disabled.text()).toBe(true)
  await page.reload()
  await openAgentChatWorkbench(page)
  const reloadedComposer = await openAgentChatSession(page, project.id, session.title)
  await submitAgentChatMessage(page, reloadedComposer, 'Check plugin availability. E2E_PLUGIN_CHAIN')
  await expect(page.getByText('No plugin tools are enabled.', { exact: true }).filter({ visible: true })).toBeVisible()
})
