import { selectExtension } from '../support/resource-center'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test, type APIRequestContext } from '../support/fixtures'

async function sourceCandidates(request: APIRequestContext, kind: 'plugin' | 'game', id: string) {
  const created = await request.post('/api/agent-chat/projects/directory', { data: { name: id } })
  expect(created.ok(), await created.text()).toBe(true)
  const project = await created.json()
  const initialized = await request.post('/api/platform/manage/development', { data: {
    projectId: project.id, relativePath: '.', kind, id,
    name: { 'zh-CN': `GitHub ${kind} 测试 · ${id}`, 'en-US': `GitHub ${kind} test · ${id}` },
  } })
  expect(initialized.ok(), await initialized.text()).toBe(true)
  const development = await initialized.json()
  const index = await (await request.get('/api/agent-chat/projects')).json()
  const directory = index.projects.find((item: { id: string }) => item.id === project.id).path
  const firstResponse = await request.post('/api/resource-exchange/previews', { data: { kind: 'directory', directory } })
  expect(firstResponse.ok(), await firstResponse.text()).toBe(true)
  const first = await firstResponse.json()
  const file = path.join(directory, kind === 'plugin' ? 'server.mjs' : 'game.mjs')
  await writeFile(file, await readFile(file, 'utf8') + '\n// Source update without a version bump.\n')
  // Add a permission to ensure an update cannot silently grant new capabilities.
  const manifestPath = path.join(directory, `denova.${kind}.json`)
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'))
  manifest.permissions.required.push('tools.write')
  await writeFile(manifestPath, JSON.stringify(manifest))
  const secondResponse = await request.post('/api/resource-exchange/previews', { data: { kind: 'directory', directory } })
  expect(secondResponse.ok(), await secondResponse.text()).toBe(true)
  const second = await secondResponse.json()
  expect(second.candidates[0].resources[0].digest).not.toBe(first.candidates[0].resources[0].digest)
  expect(second.candidates[0].package.version).toBe(first.candidates[0].package.version)
  return { first, second, development, directory }
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
    const resourceSource = { kind: 'github', ...source }
    // Only source discovery is simulated. Both frozen snapshots, plans,
    // consent, ownership and installation commits use the real backend.
    await page.route('**/api/resource-exchange/previews', async route => {
      expect(route.request().postDataJSON()).toMatchObject({ kind: 'github', url: source.url })
      await route.fulfill({ json: { ...first, source: resourceSource } })
    })
    const checks: string[] = []
    await page.route('**/api/resource-exchange/installations/*/check', async route => {
      checks.push(route.request().url().split('/').at(-2)!)
      await route.fulfill({ json: { ...second, source: { kind: 'github', ...nextSource } } })
    })
    await page.route('**/api/resource-exchange/installations', async route => {
      const response = await route.fetch()
      const items = await response.json()
      await route.fulfill({ response, json: items.map((item: { bindings: Array<{ local: { id: string } }> }) => item.bindings.some(binding => binding.local.id === id) ? { ...item, source: resourceSource } : item) })
    })
    await page.addInitScript(({ language, theme }) => {
      localStorage.setItem('nova:mode', 'extensions')
      localStorage.setItem('nova.locale.configured', language)
      localStorage.setItem('theme', theme)
    }, { language, theme })
    await page.goto('/')
    const addLabel = chinese ? '添加资源' : 'Add resources'
    await page.getByRole('button', { name: addLabel, exact: true }).click()
    const dialog = page.getByRole('dialog', { name: addLabel, exact: true })
    const repository = dialog.getByLabel(chinese ? 'GitHub 仓库' : 'GitHub repository', { exact: true })
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: 900 })
      await repository.fill('https://github.com/' + 'long-owner-name/'.repeat(6) + 'long-repository-name')
      await page.screenshot({ path: `test-results/github-source-${language}-${width}.png`, fullPage: true })
      const bounds = (await dialog.boundingBox())!
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    }
    await repository.fill(source.url)
    await dialog.getByRole('button', { name: chinese ? '下载并预览' : 'Download and preview', exact: true }).click()
    await expect(dialog.getByRole('button', { name: chinese ? '生成安装计划' : 'Review plan', exact: true })).toBeVisible()
    for (const permission of await dialog.locator('[id^="permission-"]').all()) await permission.check()
    await dialog.getByRole('button', { name: chinese ? '生成安装计划' : 'Review plan', exact: true }).click()
    const confirmation = page.getByRole('dialog', { name: chinese ? '确认安装计划' : 'Review installation plan' })
    const installed = page.waitForResponse(response => response.url().endsWith('/apply') && response.request().method() === 'POST')
    await confirmation.getByRole('button', { name: chinese ? '安装扩展' : 'Install extension', exact: true }).click()
    const installedResponse = await installed
    expect(installedResponse.ok()).toBe(true)
    const receipt = await installedResponse.json()
    await expect(confirmation).not.toBeVisible()
    const name = chinese ? `GitHub ${kind} 测试 · ${id}` : `GitHub ${kind} test · ${id}`
    await selectExtension(page, name, chinese)
    const article = page.locator('article').filter({ visible: true })
    await expect(article.getByRole('link', { name: source.url })).toBeVisible()
    await article.getByRole('button', { name: chinese ? '检查并更新已有安装' : 'Check and update installed package', exact: true }).click()
    const review = page.getByRole('dialog', { name: chinese ? '更新内容' : 'Update contents', exact: true })
    await expect(review).toBeVisible()
    const reviewButton = review.getByRole('button', { name: chinese ? '生成安装计划' : 'Review plan', exact: true })
    await expect(reviewButton).toBeDisabled()
    const unchecked = review.locator('[id^="permission-"][data-state="unchecked"]')
    await expect(unchecked).toHaveCount(1)
    await unchecked.check()
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: 900 })
      await page.screenshot({ path: `test-results/github-update-${language}-${width}.png`, fullPage: true })
      const bounds = (await review.boundingBox())!
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    }
    await reviewButton.click()
    const updated = page.waitForResponse(response => response.url().endsWith('/apply') && response.request().method() === 'POST')
    await confirmation.getByRole('button', { name: chinese ? '应用本次更新' : 'Apply this update', exact: true }).click()
    expect((await updated).ok()).toBe(true)
    await expect(confirmation).not.toBeVisible()
    const catalog = await (await request.get('/api/platform/manage/catalog')).json()
    const item = catalog.find((item: { id: string }) => item.id === id)
    const firstExtension = first.candidates[0].resources[0].extension
    const secondExtension = second.candidates[0].resources[0].extension
    expect(item.currentRelease).toBe(secondExtension.digest)
    expect(item.releases.map((release: { digest: string }) => release.digest)).toEqual([firstExtension.digest, secondExtension.digest])
    expect(checks).toEqual([receipt.installation_id])
    const receipts = await (await request.get('/api/resource-exchange/installations')).json()
    expect(receipts.filter((item: { installation_id: string }) => item.installation_id === receipt.installation_id)).toHaveLength(1)

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
  await page.getByRole('button', { name: '添加资源', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '添加资源', exact: true })
  await dialog.getByLabel('GitHub 仓库', { exact: true }).fill('https://github.com/author/source')
  await dialog.locator('summary').click()
  await dialog.locator('#market-ref').fill('beta')
  await dialog.locator('#market-path').fill('packages/plugin')
  await dialog.getByRole('button', { name: '导入源码到工作台', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('button', { name: '更多开发操作', exact: true }).filter({ visible: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '发布到本机', exact: true }).filter({ visible: true })).toBeVisible()
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

test('links mixed package members and reviews updates through the owning installation', async ({ page, request }) => {
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'light' } } })
  const id = 'test.mixed-member'
  const { first } = await sourceCandidates(request, 'plugin', id)
  const native = first.candidates[0]
  const extension = native.resources[0]
  const initialPlan = await request.post('/api/resource-exchange/plans', { data: {
    preview_id: first.preview_id, candidate_id: native.candidate_id, resources: [extension.id],
    grants: { [extension.id]: extension.extension.manifest.permissions.required },
  } })
  expect(initialPlan.ok(), await initialPlan.text()).toBe(true)
  const initial = await request.post(`/api/resource-exchange/plans/${(await initialPlan.json()).plan_id}/apply`, { data: {} })
  expect(initial.ok(), await initial.text()).toBe(true)
  const initialReceipt = await initial.json()
  const image = await request.post('/api/image-presets', { data: { name: 'Mixed package art', prompt: 'Draw a harbor' } })
  expect(image.ok(), await image.text()).toBe(true)
  const exported = await request.post('/api/resource-exchange/export', { data: {
    package: { id: 'test-mixed-package', name: 'A mixed creative package' },
    resources: [initialReceipt.bindings[0].local, { kind: 'preset.image', scope: 'global', id: (await image.json()).id }],
  } })
  expect(exported.ok(), await exported.text()).toBe(true)
  const archive = await exported.body()
  const removed = await request.patch(`/api/platform/manage/packages/plugin/${id}`, { data: { enabled: false, removed: true } })
  expect(removed.ok(), await removed.text()).toBe(true)
  const detached = await request.post(`/api/resource-exchange/installations/${initialReceipt.installation_id}/detach`, { data: {} })
  expect(detached.ok(), await detached.text()).toBe(true)
  const previewResponse = await request.post('/api/resource-exchange/previews', { multipart: { file: { name: 'mixed.zip', mimeType: 'application/zip', buffer: archive } } })
  expect(previewResponse.ok(), await previewResponse.text()).toBe(true)
  const preview = await previewResponse.json()
  const candidate = preview.candidates[0]
  const member = candidate.resources.find((resource: { kind: string }) => resource.kind === 'extension.plugin')
  const planResponse = await request.post('/api/resource-exchange/plans', { data: {
    preview_id: preview.preview_id, candidate_id: candidate.candidate_id,
    resources: candidate.resources.map((resource: { id: string }) => resource.id),
    grants: { [member.id]: member.extension.manifest.permissions.required },
  } })
  expect(planResponse.ok(), await planResponse.text()).toBe(true)
  const applied = await request.post(`/api/resource-exchange/plans/${(await planResponse.json()).plan_id}/apply`, { data: {} })
  expect(applied.ok(), await applied.text()).toBe(true)
  const receipt = await applied.json()
  expect(receipt.bindings.find((binding: { resource_id: string }) => binding.resource_id === member.id).ownership).toBe('owned')
  const source = { kind: 'github', url: 'https://github.com/author/mixed-package', ref: 'main' }
  await page.route('**/api/resource-exchange/installations', async route => {
    const response = await route.fetch()
    const items = await response.json()
    await route.fulfill({ response, json: items.map((item: { installation_id: string }) => item.installation_id === receipt.installation_id ? { ...item, source } : item) })
  })
  const checks: string[] = []
  await page.route('**/api/resource-exchange/installations/*/check', async route => {
    checks.push(route.request().url().split('/').at(-2)!)
    const fresh = await request.post('/api/resource-exchange/previews', { multipart: { file: { name: 'mixed.zip', mimeType: 'application/zip', buffer: archive } } })
    expect(fresh.ok(), await fresh.text()).toBe(true)
    await route.fulfill({ json: { ...await fresh.json(), source } })
  })
  await page.route('**/api/resource-market/catalog', route => route.fulfill({ json: { schema_version: 1, entries: [] } }))
  await page.addInitScript(() => localStorage.setItem('nova:mode', 'market'))
  await page.goto('/')
  await page.getByRole('navigation', { name: 'Resource Center navigation' }).getByRole('button', { name: 'Packages', exact: true }).click()
  const card = page.getByTestId('market-installation-card').filter({ hasText: receipt.package.name })
  await card.getByRole('button', { name: receipt.package.name, exact: true }).click()
  await card.getByRole('button', { name: 'Manage extension', exact: true }).click()
  const article = page.getByRole('article').filter({ visible: true })
  await expect(article.getByRole('heading', { name: `GitHub plugin test · ${id}`, exact: true })).toBeVisible()
  await expect(article.getByText('This extension updates with its package.', { exact: false })).toBeVisible()
  await article.getByRole('button', { name: receipt.package.name, exact: true }).click()
  await expect(card.getByRole('button', { name: receipt.package.name, exact: true })).toHaveAttribute('aria-expanded', 'true')
  await card.getByRole('button', { name: 'Manage extension', exact: true }).click()
  await article.getByRole('button', { name: 'Check and update installed package', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Update contents', exact: true })
  await expect(dialog.getByRole('checkbox', { name: 'Mixed package art', exact: true })).toBeChecked()
  await expect(dialog.getByRole('checkbox', { name: `GitHub plugin test · ${id}`, exact: true })).toBeChecked()
  const planning = page.waitForRequest('**/api/resource-exchange/plans')
  await dialog.getByRole('button', { name: 'Review plan', exact: true }).click()
  expect((await planning).postDataJSON()).toMatchObject({ installation_id: receipt.installation_id, resources: candidate.resources.map((resource: { id: string }) => resource.id) })
  await expect(page.getByRole('dialog', { name: 'Review installation plan', exact: true })).toBeVisible()
  expect(checks).toEqual([receipt.installation_id])
  await page.screenshot({ path: 'test-results/resource-center-mixed-update.png', fullPage: true })
})

test('updates a local extension through its existing receipt and confirms new permissions', async ({ page, request }) => {
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'dark' } } })
  const id = 'test.local-update'
  const { first, second, directory } = await sourceCandidates(request, 'plugin', id)
  const candidate = first.candidates[0]
  const member = candidate.resources[0]
  const planned = await request.post('/api/resource-exchange/plans', { data: {
    preview_id: first.preview_id, candidate_id: candidate.candidate_id, resources: [member.id],
    grants: { [member.id]: [...member.extension.manifest.permissions.required, ...(member.extension.manifest.permissions.optional ?? [])] },
  } })
  expect(planned.ok(), await planned.text()).toBe(true)
  const installed = await request.post(`/api/resource-exchange/plans/${(await planned.json()).plan_id}/apply`, { data: {} })
  expect(installed.ok(), await installed.text()).toBe(true)
  const receipt = await installed.json()
  await page.goto('/')
  await selectExtension(page, `GitHub plugin test · ${id}`, false)
  await page.getByRole('article').getByRole('button', { name: 'Update from a local file', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Update contents', exact: true })
  await expect(dialog.getByLabel('Local file', { exact: true })).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Source type', exact: true }).click()
  await page.getByRole('option', { name: 'Local extension directory (development)', exact: true }).click()
  await dialog.getByLabel('Local package directory', { exact: true }).fill(directory)
  await dialog.getByRole('button', { name: 'Download and preview', exact: true }).click()
  const review = dialog.getByRole('button', { name: 'Review plan', exact: true })
  await expect(review).toBeDisabled()
  const permission = dialog.locator('[id^="permission-"][data-state="unchecked"]')
  await expect(permission).toHaveCount(1)
  await permission.check()
  await review.click()
  const applying = page.waitForResponse(response => response.url().endsWith('/apply') && response.request().method() === 'POST')
  await page.getByRole('dialog', { name: 'Review installation plan', exact: true }).getByRole('button', { name: 'Apply this update', exact: true }).click()
  const updated = await applying
  expect(updated.ok()).toBe(true)
  expect((await updated.json()).installation_id).toBe(receipt.installation_id)
  const catalog = await (await request.get('/api/platform/manage/catalog')).json()
  expect(catalog.find((item: { id: string }) => item.id === id).currentRelease).toBe(second.candidates[0].resources[0].extension.digest)
})
