import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test, type APIRequestContext } from '../support/fixtures'

async function sourceCandidates(request: APIRequestContext, kind: 'plugin' | 'game', id: string) {
  const created = await request.post('/api/agent-chat/projects/directory', { data: { name: id } })
  expect(created.ok(), await created.text()).toBe(true)
  const project = await created.json()
  const initialized = await request.post('/api/platform/manage/development', { data: {
    projectId: project.id, relativePath: '.', kind, templateId: kind === 'plugin' ? 'http-tool' : 'npc-game', id,
    name: { 'zh-CN': `GitHub ${kind} 测试`, 'en-US': `GitHub ${kind} test` },
  } })
  expect(initialized.ok(), await initialized.text()).toBe(true)
  const development = await initialized.json()
  const first = await (await request.get(`/api/platform/manage/development/${development.developmentId}/check`)).json()
  const index = await (await request.get('/api/agent-chat/projects')).json()
  const directory = index.projects.find((item: { id: string }) => item.id === project.id).path
  const file = path.join(directory, kind === 'plugin' ? 'server.mjs' : 'game.mjs')
  await writeFile(file, await readFile(file, 'utf8') + '\n// Source update without a version bump.\n')
  // Add a permission to ensure an update cannot silently grant new capabilities.
  const manifestPath = path.join(directory, `denova.${kind}.json`)
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'))
  manifest.permissions.required.push('tools.write')
  await writeFile(manifestPath, JSON.stringify(manifest))
  const second = await (await request.get(`/api/platform/manage/development/${development.developmentId}/check`)).json()
  expect(second.digest).not.toBe(first.digest)
  expect(second.manifest.version).toBe(first.manifest.version)
  return { first, second, development }
}

for (const kind of ['plugin', 'game'] as const) {
  test(`installs and updates a GitHub ${kind} with one current version`, async ({ page, request }) => {
    const chinese = kind === 'plugin'
    const language = chinese ? 'zh-CN' : 'en-US'
    const theme = chinese ? 'dark' : 'light'
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language, theme } } })
    const id = `test.github-${kind}`
    const { first, second } = await sourceCandidates(request, kind, id)
    const source = { url: 'https://github.com/author/repository', ref: 'main', path: '.', commit: 'a'.repeat(40) }
    const nextSource = { ...source, commit: 'b'.repeat(40) }
    let installedSource = source
    // Only the network-source boundary is simulated. Candidate validation,
    // permission approval, installation and persisted snapshots use the backend.
    await page.route('**/api/platform/manage/packages/github/preview', async route => {
      expect(route.request().postDataJSON()).toEqual({ url: source.url, ref: '', path: '' })
      await route.fulfill({ json: { ...first, source } })
    })
    await page.route(`**/api/platform/manage/packages/${kind}/${id}/update`, async route => {
      if (route.request().method() === 'POST') {
        expect(route.request().postDataJSON()).toEqual({ commit: nextSource.commit })
        await route.fulfill({ json: { ...second, source: nextSource } })
      } else await route.fulfill({ json: { status: installedSource.commit === nextSource.commit ? 'current' : 'available', source: nextSource } })
    })
    await page.route('**/api/platform/manage/catalog', async route => {
      const response = await route.fetch()
      const items = await response.json()
      await route.fulfill({ response, json: items.map((item: { id: string }) => item.id === id ? { ...item, source: installedSource } : item) })
    })
    await page.addInitScript(({ language, theme }) => {
      localStorage.setItem('nova:mode', 'extensions')
      localStorage.setItem('nova.locale.configured', language)
      localStorage.setItem('theme', theme)
    }, { language, theme })
    await page.goto('/')
    const installLabel = chinese ? '安装扩展' : 'Install extension'
    await page.getByRole('button', { name: installLabel, exact: true }).click()
    const dialog = page.getByRole('dialog', { name: installLabel, exact: true })
    await expect(dialog.getByRole('tab', { name: 'GitHub', exact: true })).toHaveAttribute('data-state', 'active')
    const repository = dialog.getByLabel(chinese ? 'GitHub 仓库' : 'GitHub repository', { exact: true })
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: 900 })
      await repository.fill('https://github.com/' + 'long-owner-name/'.repeat(6) + 'long-repository-name')
      await page.screenshot({ path: `test-results/github-source-${language}-${width}.png`, fullPage: true })
      const bounds = (await dialog.boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    }
    await dialog.getByRole('tab', { name: chinese ? '本地文件' : 'Local files' }).click()
    await expect(dialog.locator('#package-directory')).toBeVisible()
    await expect(dialog.locator('#package-archive')).toBeVisible()
    await dialog.getByRole('tab', { name: 'GitHub', exact: true }).click()
    await repository.fill(source.url)
    await dialog.getByRole('button', { name: chinese ? '检查安装包' : 'Check package', exact: true }).click()
    await expect(dialog.getByRole('link', { name: source.url })).toBeVisible()
    for (const permission of await dialog.getByRole('switch').all()) await permission.check()
    const installed = page.waitForResponse(response => response.url().endsWith('/packages/install') && response.request().method() === 'POST')
    await dialog.getByRole('button', { name: installLabel, exact: true }).click()
    expect((await installed).ok()).toBe(true)
    await expect(dialog).not.toBeVisible()
    // The first entry may be another test's extension; choose this installation.
    await page.getByRole('button', { name: new RegExp(chinese ? `GitHub ${kind} 测试` : `GitHub ${kind} test`) }).filter({ visible: true }).first().click()
    const article = page.locator('article').filter({ visible: true })
    await expect(article.getByRole('link', { name: source.url })).toBeVisible()
    await expect(article.getByText(chinese ? /\d+ 个版本/ : /\d+ versions/)).toHaveCount(0)
    await article.getByRole('button', { name: chinese ? '检查更新' : 'Check for updates', exact: true }).click()
    await expect(article.getByRole('status')).toContainText('bbbbbbb')
    await article.getByRole('button', { name: chinese ? '更新' : 'Update', exact: true }).click()
    const review = page.getByRole('dialog', { name: chinese ? '检查扩展更新' : 'Review extension update' })
    const updateButton = review.getByRole('button', { name: chinese ? '更新' : 'Update', exact: true })
    await expect(updateButton).toBeDisabled()
    const unchecked = review.getByRole('switch', { checked: false })
    await expect(unchecked).toHaveCount(1)
    await unchecked.check()
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: 900 })
      await page.screenshot({ path: `test-results/github-update-${language}-${width}.png`, fullPage: true })
      const bounds = (await review.boundingBox())!
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    }
    installedSource = nextSource
    const updated = page.waitForResponse(response => response.url().endsWith('/packages/install') && response.request().method() === 'POST')
    await updateButton.click()
    expect((await updated).ok()).toBe(true)
    await expect(review).not.toBeVisible()
    const catalog = await (await request.get('/api/platform/manage/catalog')).json()
    const item = catalog.find((item: { id: string }) => item.id === id)
    expect(item.currentRelease).toBe(second.digest)
    expect(item.releases.map((release: { digest: string }) => release.digest)).toEqual([first.digest, second.digest])
    await article.getByRole('button', { name: chinese ? '检查更新' : 'Check for updates', exact: true }).click()
    await expect(page.getByText(chinese ? '暂无上游更新' : 'No upstream updates', { exact: true })).toBeVisible()
    await expect(article.getByRole('button', { name: chinese ? '更新' : 'Update', exact: true })).toHaveCount(0)
  })
}

test('opens imported GitHub source in the workbench without installing or building', async ({ page, request }) => {
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'zh-CN', theme: 'dark' } } })
  const { development } = await sourceCandidates(request, 'plugin', 'test.github-import')
  const sources = await (await request.get('/api/platform/manage/development')).json()
  const imported = sources.find((item: { developmentId: string }) => item.developmentId === development.developmentId)
  const before = await (await request.get('/api/platform/manage/catalog')).json()
  let importedCount = 0
  await page.route('**/api/platform/manage/packages/github/import', async route => {
    expect(route.request().postDataJSON()).toEqual({ url: 'https://github.com/author/source', ref: 'beta', path: 'packages/plugin' })
    importedCount++
    await route.fulfill({ json: imported })
  })
  const buildRequests: string[] = []
  page.on('request', request => {
    if (request.method() === 'POST' && request.url().endsWith('/build')) buildRequests.push(request.url())
  })
  await page.addInitScript(() => localStorage.setItem('nova:mode', 'extensions'))
  await page.goto('/')
  await page.getByRole('button', { name: '安装扩展', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '安装扩展', exact: true })
  await dialog.getByLabel('GitHub 仓库', { exact: true }).fill('https://github.com/author/source')
  await dialog.locator('summary').click()
  await dialog.locator('#package-github-ref').fill('beta')
  await dialog.locator('#package-github-path').fill('packages/plugin')
  await dialog.getByRole('button', { name: '导入源码到工作台', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('button', { name: '源码清单', exact: true }).filter({ visible: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '检查并打包', exact: true }).filter({ visible: true })).toBeVisible()
  await expect(page.getByPlaceholder('输入消息，/ 选择命令或 Skills').filter({ visible: true })).toBeVisible()
  await expect(page.getByText('denova.plugin.json', { exact: true }).filter({ visible: true }).first()).toBeVisible()
  expect(importedCount).toBe(1)
  expect(buildRequests).toEqual([])
  expect(await (await request.get('/api/platform/manage/catalog')).json()).toEqual(before)
  const projects = await (await request.get('/api/agent-chat/projects')).json()
  expect(projects.projects.find((item: { id: string }) => item.id === imported.projectId).sessions).toHaveLength(1)
  await page.screenshot({ path: 'test-results/github-import-workbench.png', fullPage: true })
})

test('rejects invalid GitHub sources through management without creating projects', async ({ request }) => {
  const before = await (await request.get('/api/agent-chat/projects')).json()
  for (const action of ['preview', 'import']) {
    const response = await request.post(`/api/platform/manage/packages/github/${action}`, { data: { url: 'file:///outside', ref: '', path: '.' } })
    expect(response.status()).toBe(400)
    expect((await response.json()).messageKey).toBe('platform.errors.GITHUB_URL_INVALID')
  }
  const after = await (await request.get('/api/agent-chat/projects')).json()
  expect(after.projects.map((item: { id: string }) => item.id)).toEqual(before.projects.map((item: { id: string }) => item.id))
})
