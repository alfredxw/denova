import { expect, test, type APIRequestContext } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, readProjectFile, saveProjectFile } from '../support/api'

const creatorRef = (projectId: string) => ({ kind: 'project.creator', scope: 'project', project_id: projectId, id: 'creator' })
const openingRef = (projectId: string) => ({ kind: 'game.openings', scope: 'project', project_id: projectId, id: 'all' })

async function exportRules(request: APIRequestContext, projectId: string, includeCreator = true) {
  const response = await request.post('/api/resource-exchange/export', { data: {
    package: { id: 'creator-fixture', name: 'Creative rules fixture' },
    resources: [...(includeCreator ? [creatorRef(projectId)] : []), openingRef(projectId)],
  } })
  expect(response.ok(), await response.text()).toBe(true)
  return response.body()
}

async function previewRules(request: APIRequestContext, buffer: Buffer) {
  const response = await request.post('/api/resource-exchange/previews', { multipart: {
    file: { name: 'creator.zip', mimeType: 'application/zip', buffer },
  } })
  expect(response.ok(), await response.text()).toBe(true)
  return response.json()
}

async function sourceBook(request: APIRequestContext, content: string) {
  const book = await createAndOpenBook(request, 'Creator source')
  await saveProjectFile(request, book.projectId, 'CREATOR.md', content)
  await createProjectFile(request, book.projectId, 'setting/interactive-openings.json', JSON.stringify({ version: 1, presets: [{ id: 'arrival', title: 'Arrival', content: 'A visitor arrives.' }] }))
  return book
}

test('creator package round trip preserves consent, conflicts, backups and local ownership', async ({ request }) => {
  const content = '# Rules\n\n' + 'Use close third-person narration.\n'.repeat(2200) + '\nEND OF RULES\n'
  const source = await sourceBook(request, content)
  const choices = await (await request.get(`/api/resource-exchange/export-resources?project_id=${source.projectId}`)).json()
  expect(choices).toContainEqual(expect.objectContaining({ local: creatorRef(source.projectId) }))
  const preview = await previewRules(request, await exportRules(request, source.projectId))
  const candidate = preview.candidates[0]
  const creator = candidate.resources.find((r: { kind: string }) => r.kind === 'project.creator')
  const opening = candidate.resources.find((r: { kind: string }) => r.kind === 'game.openings')
  const files = await (await request.get(`/api/resource-exchange/previews/${preview.preview_id}/files?${new URLSearchParams({ candidate_id: candidate.candidate_id, resource_id: creator.id, path: creator.path })}`)).json()
  expect(files).toMatchObject({ content, truncated: false, binary: false })
  const target = await createAndOpenBook(request, 'Creator target')
  const original = (await readProjectFile(request, target.projectId, 'CREATOR.md')).content
  const data = { preview_id: preview.preview_id, candidate_id: candidate.candidate_id, project_id: target.projectId, resources: [creator.id, opening.id] }
  expect((await request.post('/api/resource-exchange/plans', { data: { ...data, project_id: '' } })).status()).toBe(400)
  expect((await request.post('/api/resource-exchange/plans', { data })).status()).toBe(409)
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(original)

  const planResponse = await request.post('/api/resource-exchange/plans', { data: { ...data, replace_modified: true } })
  expect(planResponse.ok(), await planResponse.text()).toBe(true)
  const plan = await planResponse.json()
  await saveProjectFile(request, target.projectId, 'CREATOR.md', 'Concurrent author edit')
  expect((await request.post(`/api/resource-exchange/plans/${plan.plan_id}/apply`, { data: {} })).status()).toBe(409)
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe('Concurrent author edit')
  const nextResponse = await request.post('/api/resource-exchange/plans', { data: { ...data, replace_modified: true } })
  expect(nextResponse.ok(), await nextResponse.text()).toBe(true)
  const next = await nextResponse.json()
  const applied = await request.post(`/api/resource-exchange/plans/${next.plan_id}/apply`, { data: {} })
  expect(applied.ok(), await applied.text()).toBe(true)
  const installed = await applied.json()
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(content)
  // Rechecking unchanged installed instructions is safe without replacement consent.
  const unchanged = await request.post('/api/resource-exchange/plans', { data: { ...data, installation_id: installed.installation_id } })
  expect(unchanged.ok(), await unchanged.text()).toBe(true)
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(content)

  const restoreResponse = await request.post(`/api/resource-exchange/backups/${next.plan_id}/restore-plan`, { data: {} })
  expect(restoreResponse.ok(), await restoreResponse.text()).toBe(true)
  const restore = await restoreResponse.json()
  const restored = await request.post(`/api/resource-exchange/plans/${restore.plan_id}/apply`, { data: {} })
  expect(restored.ok(), await restored.text()).toBe(true)
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe('Concurrent author edit')

  const adoption = await request.post('/api/resource-exchange/plans', { data: { ...data, replace_modified: true } })
  expect(adoption.ok(), await adoption.text()).toBe(true)
  const adoptionPlan = await adoption.json()
  const adopted = await request.post(`/api/resource-exchange/plans/${adoptionPlan.plan_id}/apply`, { data: {} })
  expect(adopted.ok(), await adopted.text()).toBe(true)
  const current = await adopted.json()
  const withoutRules = await previewRules(request, await exportRules(request, source.projectId, false))
  const remainder = withoutRules.candidates[0]
  const keepResponse = await request.post('/api/resource-exchange/plans', { data: {
    preview_id: withoutRules.preview_id, candidate_id: remainder.candidate_id,
    installation_id: current.installation_id, project_id: target.projectId,
    resources: remainder.resources.map((r: { id: string }) => r.id),
  } })
  expect(keepResponse.ok(), await keepResponse.text()).toBe(true)
  const keep = await keepResponse.json()
  expect(keep.items).toContainEqual(expect.objectContaining({ local: creatorRef(target.projectId), action: 'keep' }))
  const kept = await request.post(`/api/resource-exchange/plans/${keep.plan_id}/apply`, { data: {} })
  expect(kept.ok(), await kept.text()).toBe(true)
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(content)
  const detached = await request.post(`/api/resource-exchange/installations/${current.installation_id}/detach`, { data: {} })
  expect(detached.ok(), await detached.text()).toBe(true)
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(content)

  await saveProjectFile(request, source.projectId, 'CREATOR.md', 'x'.repeat(256 * 1024))
  const oversized = await previewRules(request, await exportRules(request, source.projectId))
  const rejected = await request.post('/api/resource-exchange/plans', { headers: { 'X-Denova-Locale': 'en-US' }, data: {
    preview_id: oversized.preview_id, candidate_id: oversized.candidates[0].candidate_id,
    project_id: target.projectId, resources: oversized.candidates[0].resources.map((r: { id: string }) => r.id), replace_modified: true,
  } })
  expect(rejected.status()).toBe(400)
  expect((await rejected.json()).error).toContain('context fragment limits')
  expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(content)
})

for (const theme of ['dark', 'light']) {
  test(`creator rules are optional and previewable before import in ${theme}`, async ({ page, request }) => {
    page.setDefaultTimeout(15_000)
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme } } })
    const source = await sourceBook(request, '# Shared creative rules\n\nUse close third-person narration.\n\nEnd of shared rules.')
    const buffer = await exportRules(request, source.projectId)
    const target = await createAndOpenBook(request, 'Creator UI target')
    const original = (await readProjectFile(request, target.projectId, 'CREATOR.md')).content
    await page.setViewportSize({ width: theme === 'dark' ? 390 : 1280, height: 900 })
    await page.addInitScript(() => {
      localStorage.setItem('nova:mode', 'market')
      localStorage.setItem('nova.locale.configured', 'en-US')
    })
    await page.route('**/api/resource-market/catalog', route => route.fulfill({ json: { schema_version: 1, entries: [] } }))
    await page.goto('/')
    await page.getByTestId('resource-market').getByRole('button', { name: 'Add resources', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('combobox').click()
    await page.getByRole('option', { name: 'Local file', exact: true }).click()
    await dialog.getByLabel('Local file', { exact: true }).setInputFiles({ name: 'creator.zip', mimeType: 'application/zip', buffer })
    await dialog.getByRole('button', { name: 'Download and preview', exact: true }).click()
    const creator = dialog.getByRole('checkbox', { name: 'CREATOR.md', exact: true })
    await expect(creator).not.toBeChecked()
    await expect(dialog.getByRole('group', { name: 'Creative instructions (CREATOR.md)', exact: true })).toBeVisible()
    await dialog.getByRole('button', { name: 'Preview CREATOR.md', exact: true }).click()
    const preview = page.getByRole('dialog', { name: 'CREATOR.md', exact: true })
    await expect(preview.getByRole('heading', { name: 'Shared creative rules' })).toBeVisible()
    await expect(preview.getByText('End of shared rules.', { exact: true })).toBeVisible()
    await preview.getByRole('button', { name: 'Close', exact: true }).click()
    await creator.check()
    if (theme === 'light') {
      await dialog.getByRole('radio', { name: 'Import as a new book', exact: true }).check()
      await dialog.getByRole('textbox', { name: 'New book title', exact: true }).fill('Creator package new book')
    } else {
      await dialog.getByRole('combobox', { name: 'Target project', exact: true }).click()
      await page.getByRole('option', { name: target.title, exact: true }).click()
    }
    await dialog.getByRole('checkbox', { name: 'Back up and replace existing resources, including CREATOR.md', exact: true }).check()
    await page.screenshot({ path: `test-results/creator-import-${theme}.png`, fullPage: true })
    await dialog.getByRole('button', { name: theme === 'light' ? 'Create book and review' : 'Review plan', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Review installation plan' })).toBeVisible()
    expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(original)
    await page.getByRole('button', { name: 'Add resources', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    const workspaceResponse = await request.get('/api/workspace/current')
    const workspace = await workspaceResponse.json()
    const importedProject = theme === 'light' ? workspace.project_id : target.projectId
    expect((await readProjectFile(request, importedProject, 'CREATOR.md')).content).toContain('Shared creative rules')
    if (theme === 'light') {
      expect(importedProject).not.toBe(target.projectId)
      expect((await readProjectFile(request, target.projectId, 'CREATOR.md')).content).toBe(original)
    }
    await page.getByTestId('resource-market').getByRole('button', { name: 'Export package', exact: true }).first().click()
    const exporter = page.getByRole('dialog', { name: 'Export package', exact: true })
    await exporter.getByRole('group').filter({ has: page.getByText('Target project', { exact: true }) }).getByRole('combobox').click()
    await page.getByRole('option', { name: theme === 'light' ? 'Creator package new book' : target.title, exact: true }).click()
    await exporter.getByRole('textbox', { name: 'Search name, description or type', exact: true }).fill('CREATOR.md')
    await expect(exporter.getByRole('checkbox', { name: 'CREATOR.md', exact: true })).toBeVisible()
    await exporter.getByRole('button', { name: 'Close', exact: true }).click()
    await page.setViewportSize({ width: 1280, height: 900 })
    const sidebar = page.getByRole('navigation', { name: 'Workbench sidebar', exact: true })
    for (const mode of ['Writing', 'Game']) {
      await sidebar.getByRole('button', { name: mode, exact: true }).click()
      await expect(sidebar.getByRole('button', { name: mode, exact: true })).toHaveAttribute('aria-current', 'page')
      await sidebar.getByRole('button', { name: 'Lore', exact: true }).click()
      await page.getByRole('button', { name: 'CREATOR.md', exact: true }).click()
      await expect(page.getByPlaceholder('Write the highest-priority creative rules for this book...')).toHaveValue(/Shared creative rules/)
    }
  })
}
