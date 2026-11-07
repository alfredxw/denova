import { expect, test, type Page } from '../support/fixtures'
import type { Route } from '@playwright/test'
import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { createAgentChatSession, createAndOpenBook, createProjectFile, readProjectFile, registerAgentChatProject, saveProjectFile } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent } from '../support/agent-chat'

for (const surface of ['writing', 'general'] as const) {
  test(`creates workspace snapshots from the ${surface} composer without starting an Agent turn`, async ({ page, request, browserDiagnostics }, testInfo) => {
    const book = await createAndOpenBook(request, `Snapshot ${surface}`)
    const project = surface === 'general'
      ? await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'snapshot-general-')))
      : { id: book.projectId }
    const filePath = 'snapshot.md'
    await createProjectFile(request, project.id, filePath, 'Persisted workspace content')
    const session = surface === 'general' ? await createAgentChatSession(request, project.id, 'Snapshot session') : null
    const versionPath = `/api/projects/${project.id}/versions`
    const agentRequests: string[] = []
    page.on('request', sent => {
      if (sent.method() !== 'GET' && /\/(?:chat|commands|goal)$/.test(new URL(sent.url()).pathname)) agentRequests.push(sent.url())
    })
    browserDiagnostics.allow(/console\.error: Failed to load resource:.*400 \(Bad Request\).*\/versions/)
    const theme = surface === 'writing' ? 'dark' : 'light'
    const themed = await request.patch('/api/settings', { data: { layer: 'user', changes: { theme } } })
    expect(themed.ok(), await themed.text()).toBe(true)

    await page.goto('/')
    if (session) await openAgentChatWorkbench(page)
    const composer = session
      ? await openAgentChatSession(page, project.id, session.title)
      : await openWritingAgent(page)
    const send = page.locator('[data-action="send"]').filter({ visible: true })
    await composer.fill('/snap')
    await expect(page.getByRole('option', { name: /\/snapshot.*创建工作区版本/ })).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath(`snapshot-${theme}.png`) })
    await page.getByRole('option', { name: /\/snapshot.*创建工作区版本/ }).click()
    await composer.pressSequentially(`Checkpoint ${surface}`)
    const saved = page.waitForResponse(response => new URL(response.url()).pathname === versionPath && response.request().method() === 'POST')
    await expect(send).toBeEnabled()
    await send.press('Enter')
    const response = await saved
    expect(response.ok(), await response.text()).toBe(true)
    expect((await response.json()).version).toMatchObject({ message: `Checkpoint ${surface}`, source: 'manual' })
    await expect(page.getByText(`已保存版本：Checkpoint ${surface}`, { exact: true })).toBeVisible()
    await expect(composer).toBeVisible()
    await expect(composer).toHaveText('')
    expect(agentRequests).toEqual([])

    const clean = page.waitForResponse(response => new URL(response.url()).pathname === versionPath && response.request().method() === 'POST')
    await composer.fill('/snapshot ')
    await expect(send).toBeEnabled()
    await send.press('Enter')
    expect((await clean).status()).toBe(400)
    await expect(page.getByText('暂无变更', { exact: true })).toBeVisible()
    await expect(composer).toHaveText('')
    const history = await (await request.get(versionPath)).json()
    expect(history.versions).toHaveLength(1)
    const englishClean = await request.post(versionPath, { data: {}, headers: { 'X-Denova-Locale': 'en-US' } })
    expect(await englishClean.json()).toMatchObject({ code: 'api.versions.noChanges', error: 'The workspace has no changes to save.' })

    await saveProjectFile(request, project.id, filePath, 'Updated persisted content')
    let snapshotComposer = composer
    if (surface === 'general') {
      await composer.fill('/goal')
      await page.getByRole('option', { name: /\/goal/ }).click()
      snapshotComposer = page.getByPlaceholder('描述目标，并定义可衡量的完成结果')
    }
    const automatic = page.waitForResponse(response => new URL(response.url()).pathname === versionPath && response.request().method() === 'POST')
    await snapshotComposer.fill('/snapshot ')
    await expect(send).toBeEnabled()
    await send.press('Enter')
    const automaticResult = await (await automatic).json()
    expect(automaticResult.version.message).toBeTruthy()
    expect(automaticResult.version.id).not.toBe(history.versions[0].id)
    await expect(composer).toHaveText('')
    expect(agentRequests).toEqual([])

    browserDiagnostics.allow(/console\.error: \[AgentPanel\] Failed to create workspace version/)
    await page.route(`**${versionPath}`, route => route.fulfill({
      status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'Snapshot request rejected' }),
    }), { times: 1 })
    await composer.fill('/snapshot Retry description')
    await expect(send).toBeEnabled()
    await send.press('Enter')
    await expect(page.getByText('Snapshot request rejected', { exact: false }).first()).toBeVisible()
    await expect(composer).toHaveText('/snapshot Retry description')
    expect(agentRequests).toEqual([])
  })
}

async function openVersionHistory(page: Page) {
  const saveVersion = page.getByRole('button', { name: '保存当前版本', exact: true })
  const openHistory = page.getByRole('button', { name: '打开版本历史', exact: true })
  await expect(saveVersion.or(openHistory).filter({ visible: true })).toHaveCount(1)
  if (!(await saveVersion.isVisible())) {
    await openHistory.click()
  }
  await expect(saveVersion).toBeVisible()
  return saveVersion
}

test('saves a version, shows its diff, and restores one file', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Versions E2E Book')
  const filePath = 'chapters/versioned.md'
  const original = '# 版本章节\n\n第一个受保护的版本。'
  const changed = '# 版本章节\n\n这是尚未保存为版本的新内容。'
  await createProjectFile(request, book.projectId, filePath, original)

  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '版本管理', exact: true }).click()
  await (await openVersionHistory(page)).click()
  await expect(page.getByText(/已保存版本：/)).toBeVisible()

  await saveProjectFile(request, book.projectId, filePath, changed)
  await openVersionHistory(page)
  await page.getByRole('button', { name: '刷新版本状态', exact: true }).first().click()
  await page.getByRole('button', { name: /当前变更/ }).click()
  await expect(page.getByText(filePath, { exact: true }).first()).toBeVisible()

  const restoreFile = page.getByRole('button', { name: '恢复此文件', exact: true })
  await restoreFile.focus()
  await restoreFile.press('Enter')
  const dialog = page.getByRole('alertdialog', { name: '确认恢复文件？' })
  await expect(dialog.getByText(filePath, { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: '恢复文件', exact: true }).click()

  await expect(page.getByText('已恢复版本', { exact: true })).toBeVisible()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, filePath)).content).toBe(original)
})

test('opens a large version and navigates files before the complete diff arrives', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Large version navigation')
  const base = `/api/projects/${book.projectId}`
  const operations = Array.from({ length: 60 }, (_, index) => ({
    id: String(index), kind: 'create', type: 'file',
    path: `chapters/ch${String(index).padStart(3, '0')}-late.md`, content: `# Chapter ${index}\n\nComplete content ${index}.`,
  }))
  const created = await request.post(`${base}/files/operations`, { data: { operations } })
  expect(created.ok(), await created.text()).toBe(true)
  const saved = await request.post(`${base}/versions`, { data: { message: 'Large comparison' } })
  expect(saved.ok(), await saved.text()).toBe(true)
  const version = (await saved.json()).version
  const initial = await (await request.get(`${base}/versions/${version.id}/diff?comparison=parent&view=initial`)).json()
  expect(initial.files_complete).toBe(false)
  expect(initial.files).toHaveLength(1)
  expect(initial.changes.length).toBeGreaterThanOrEqual(60)
  const held: Route[] = []
  let hold = true
  const release = async () => {
    hold = false
    await Promise.all(held.splice(0).map(route => route.continue()))
  }
  await page.route('**/versions/*/diff?*', async route => {
    const url = new URL(route.request().url())
    if (hold && !url.searchParams.has('path') && !url.searchParams.has('view')) {
      held.push(route)
      return
    }
    await route.continue()
  })
  try {
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '版本管理', exact: true }).click()
    await expect(page.locator('[data-code-diff-header]').filter({ hasText: initial.files[0].path.split('/').at(-1) })).toBeVisible()
    await page.getByRole('textbox', { name: '筛选文件…', exact: true }).fill('ch059-late')
    await page.getByRole('treeitem', { name: 'ch059-late.md', exact: true }).click({ timeout: 10_000 })
    await expect(page.locator('[data-code-diff-header="chapters/ch059-late.md"]')).toBeVisible()
    await expect(page.getByText('Complete content 59.', { exact: false })).toBeVisible()
    const completed = page.waitForResponse(response => {
      const url = new URL(response.url())
      return url.pathname.endsWith('/diff') && !url.searchParams.has('path') && !url.searchParams.has('view')
    })
    await release()
    expect((await (await completed).json()).files_complete).toBe(true)
    const selectedHeader = page.locator('[data-code-diff-header="chapters/ch059-late.md"]')
    await expect(selectedHeader).toBeVisible()
    await expect(selectedHeader.getByRole('button', { name: '恢复此文件', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '下一个变更文件', exact: true })).toHaveCount(0)
  } finally {
    await release()
  }
})
