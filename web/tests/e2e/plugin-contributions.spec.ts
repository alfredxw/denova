import { openExtensions, selectExtension } from '../support/resource-center'
import { cp, readdir } from 'node:fs/promises'
import path from 'node:path'
import { expect, test, type APIRequestContext, type Page } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

// The plugin scaffold is copied outside the repository and installed through the
// ordinary package API. Its view only receives the public scoped connection.
async function installStarter(request: APIRequestContext) {
  const project = await (await request.post('/api/agent-chat/projects/directory', { data: { name: `Plugin starter ${Date.now()}` } })).json()
  const index = await (await request.get('/api/agent-chat/projects')).json()
  const directory = index.projects.find((item: { id: string }) => item.id === project.id).path
  const source = new URL('../../../internal/platform/assets/starters/plugin/', import.meta.url)
  for (const entry of await readdir(source)) await cp(new URL(entry, source), path.join(directory, entry), { recursive: true, errorOnExist: true, force: false })
  for (const entry of ['runtime.mjs', 'client.mjs', 'client.d.mts']) await cp(new URL(`../../../internal/platform/assets/sdk/${entry}`, import.meta.url), path.join(directory, entry))
  const check = await request.post('/api/platform/manage/packages/preview', { data: { directory } })
  expect(check.ok(), await check.text()).toBe(true)
  const candidate = await check.json()
  const response = await request.post('/api/platform/manage/packages/install', { data: { candidateId: candidate.candidateId, grants: [...candidate.manifest.permissions.required, ...candidate.manifest.permissions.optional] } })
  expect(response.ok(), await response.text()).toBe(true)
  const enabled = await request.patch(`/api/platform/manage/packages/plugin/${candidate.manifest.id}`, { data: { enabled: true } })
  expect(enabled.ok(), await enabled.text()).toBe(true)
  return { project: { ...project, name: index.projects.find((item: { id: string }) => item.id === project.id).name }, release: await response.json() }
}

async function launcher(page: Page, name: string, project: string, chinese: boolean) {
  await selectExtension(page, name, chinese)
  await page.getByRole('button', { name: chinese ? '打开插件' : 'Open plugin', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: chinese ? '项目插件' : 'Project plugins', exact: true })
  await dialog.getByRole('combobox').first().click()
  await page.getByRole('option', { name: project, exact: true }).click()
  return dialog
}

async function closePanel(page: Page, chinese: boolean) {
  await page.getByRole('button', { name: chinese ? '插件面板操作' : 'Plugin panel actions', exact: true }).click()
  await page.getByRole('button', { name: chinese ? '关闭插件面板' : 'Close plugin panel', exact: true }).click()
}

for (const chinese of [true, false]) {
  test(`plugin panel persists Project content and protects close (${chinese ? 'zh dark' : 'en light'})`, async ({ page, request, browserDiagnostics }) => {
    browserDiagnostics.allow(/console\.error: Failed to load resource:.*404.*\/assets\/document/)
    const { project, release } = await installStarter(request)
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: chinese ? 'zh-CN' : 'en-US', theme: chinese ? 'dark' : 'light' } } })
    await page.addInitScript(() => localStorage.setItem('nova:mode', 'extensions'))
    await page.goto('/')
    const pluginName = chinese ? '插件项目' : 'Plugin project'
    const name = chinese ? '草稿工具台' : 'Draft workbench'
    let dialog = await launcher(page, pluginName, project.name, chinese)
    await dialog.getByRole('button', { name, exact: true }).click()
    // Only the active panel is mounted on this first open.
    const view = page.frameLocator('iframe[title="' + (chinese ? '插件界面' : 'Plugin view') + '"]')
    await expect(view.getByRole('status')).toHaveText(chinese ? '已完成' : 'Done')
    await view.getByLabel(chinese ? '文本' : 'Text', { exact: true }).fill('Shared draft')
    await closePanel(page, chinese)
    await expect(page.getByRole('dialog', { name: chinese ? '关闭插件面板' : 'Close plugin panel', exact: true })).toBeVisible()
    await page.getByRole('dialog', { name: chinese ? '关闭插件面板' : 'Close plugin panel', exact: true }).getByRole('button', { name: chinese ? '取消' : 'Cancel', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await view.getByRole('button', { name: chinese ? '保存草稿' : 'Save draft', exact: true }).click()
    await expect(view.getByRole('status')).toHaveText(chinese ? '已完成' : 'Done')
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await page.screenshot({ path: `test-results/plugin-panel-${chinese}-${width}.png` })
      const box = (await page.getByRole('region', { name: chinese ? '插件工作区' : 'Plugin workspace', exact: true }).boundingBox())!
      expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x + box.width).toBeLessThanOrEqual(width)
    }
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.getByRole('button', { name: chinese ? '收起面板，继续运行' : 'Hide panels and keep running', exact: true }).click()
    dialog = await launcher(page, pluginName, project.name, chinese)
    await dialog.getByRole('button', { name, exact: true }).click()
    await expect(page.locator('iframe')).toHaveCount(1)
    await closePanel(page, chinese)
    await expect(page.locator('iframe')).toHaveCount(0)
    const active = await (await request.get('/api/platform/manage/runtimes')).json()
    expect(active.filter((runtime: { context: { source: { package: { id: string } } } }) => runtime.context.source.package.id === release.manifest.id)).toHaveLength(0)
    dialog = await launcher(page, pluginName, project.name, chinese)
    await dialog.getByRole('button', { name, exact: true }).click()
    await expect(view.getByLabel(chinese ? '文本' : 'Text', { exact: true })).toHaveValue('Shared draft')
    await closePanel(page, chinese)
  })
}

test('combined plugin exposes a command and shares panel data with independently scoped tools', async ({ page, request, browserDiagnostics }) => {
  browserDiagnostics.allow(/console\.error: Failed to load resource:.*404.*\/assets\/document/)
  const { project, release } = await installStarter(request)
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'light' } } })
  await page.addInitScript(() => localStorage.setItem('nova:mode', 'extensions'))
  await page.goto('/')
  let dialog = await launcher(page, 'Plugin project', project.name, false)
  await dialog.getByRole('button', { name: 'Echo text', exact: true }).click()
  const command = page.getByRole('dialog', { name: 'Echo text', exact: true })
  await command.getByLabel('Text', { exact: false }).fill('Hello 🌷')
  await command.getByRole('button', { name: 'Run command', exact: true }).click()
  await expect(command.locator('pre')).toContainText('Hello 🌷')
  await expect(command.getByRole('button', { name: 'Run command', exact: true })).toBeEnabled()
  await page.keyboard.press('Escape')
  dialog = await launcher(page, 'Plugin project', project.name, false)
  await dialog.getByRole('button', { name: 'Draft workbench', exact: true }).click()
  const view = page.frameLocator('iframe[title="Plugin view"]')
  await expect(view.getByRole('status')).toHaveText('Done')
  await view.getByLabel('Text', { exact: true }).fill('Shared project draft')
  await view.getByRole('button', { name: 'Save draft', exact: true }).click()
  await expect(view.getByRole('status')).toHaveText('Done')
  const response = await request.post('/api/platform/manage/runtimes/plugin', { data: { pluginId: release.manifest.id, releaseId: release.ref.releaseId, scope: { kind: 'session', projectId: project.id, sessionId: 'independent-contract-check' }, locale: 'en-US', theme: 'light' } })
  expect(response.ok(), await response.text()).toBe(true)
  const other = await response.json()
  const result = await request.post(other.connection.baseUrl + '/tools/example.plugin/read-draft/invoke', { headers: { Authorization: `Bearer ${other.connection.token}` }, data: { input: {} } })
  expect(result.ok(), await result.text()).toBe(true)
  expect((await result.json()).data.text).toBe('Shared project draft')
  await request.post(`/api/platform/manage/runtimes/${other.id}/stop`, { data: {} })
  await closePanel(page, false)
})

test('project entry points preserve panel ownership and transfer content through the host UI', async ({ page, request, browserDiagnostics }) => {
  browserDiagnostics.allow(/console\.error: Failed to load resource:.*404.*\/assets\/document/)
  const { project } = await installStarter(request)
  const book = await createAndOpenBook(request, 'Plugin ownership')
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'light' } } })
  await page.goto('/')
  await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Writing', exact: true }).click()
  await openExtensions(page, false)
  let dialog = await launcher(page, 'Plugin project', book.title, false)
  await expect(dialog).toContainText(book.title)
  await dialog.getByRole('button', { name: 'Draft workbench', exact: true }).click()
  const view = page.frameLocator('iframe[title="Plugin view"]')
  await expect(view.getByRole('status')).toHaveText('Done')
  await view.getByLabel('Text', { exact: true }).fill('Project-bound draft')
  await view.getByRole('button', { name: 'Save draft', exact: true }).click()
  await expect(view.getByRole('status')).toHaveText('Done')
  await page.getByRole('button', { name: 'Hide panels and keep running', exact: true }).click()
  await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Game', exact: true }).click()
  await expect(page.getByTestId('interactive-shell')).toBeVisible()
  await page.getByRole('button', { name: 'Open plugin panels (1)', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Plugin workspace', exact: true })).toContainText(book.title)
  await expect(view.getByLabel('Text', { exact: true })).toHaveValue('Project-bound draft')
  await expect(page.locator('iframe')).toHaveCount(1)
  await closePanel(page, false)
  await openExtensions(page, false)
  dialog = await launcher(page, 'Plugin project', book.title, false)
  const content = dialog.locator('section').filter({ has: page.getByRole('heading', { name: 'Plugin project', exact: true }) })
  await content.getByText('Plugin project content', { exact: true }).click()
  const download = page.waitForEvent('download')
  await content.getByRole('button', { name: 'Export content', exact: true }).click()
  const archive = await download
  const archivePath = test.info().outputPath('plugin-content.zip')
  await archive.saveAs(archivePath)
  await page.keyboard.press('Escape')
  await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Workspace', exact: true }).click()
  await page.getByRole('button', { name: `Project actions for ${project.name}`, exact: true }).click()
  await page.getByRole('menuitem', { name: 'Project plugins', exact: true }).click()
  dialog = page.getByRole('dialog', { name: 'Project plugins', exact: true })
  await expect(dialog).toContainText(project.name)
  const target = dialog.locator('section').filter({ has: page.getByRole('heading', { name: 'Plugin project', exact: true }) })
  await target.getByText('Plugin project content', { exact: true }).click()
  await target.getByLabel('Import content', { exact: true }).setInputFiles(archivePath)
  await expect(target.getByRole('status')).toBeVisible()
  await target.getByRole('switch').click()
  await expect(target.getByRole('switch')).not.toBeChecked()
  await expect(target.getByRole('button', { name: 'Draft workbench', exact: true })).toBeDisabled()
  await target.getByRole('switch').click()
  await expect(target.getByRole('switch')).toBeChecked()
  await target.getByRole('button', { name: 'Draft workbench', exact: true }).click()
  await expect(view.getByLabel('Text', { exact: true })).toHaveValue('Project-bound draft')
  await closePanel(page, false)
})
