import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

for (const theme of ['dark', 'light']) {
  test(`shared resource imports default to reuse and allow copies in ${theme}`, async ({ page, request }) => {
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme } } })
    await createAndOpenBook(request, 'Shared import destination')
    const skillName = `shared-import-${theme}`
    const skill = await request.post('/api/skills', { data: { scope: 'user', name: skillName, description: 'Shared import fixture' } })
    expect(skill.ok(), await skill.text()).toBe(true)
    const image = await request.post('/api/image-presets', { data: { name: `Shared image ${theme}`, prompt: 'Draw a tree' } })
    expect(image.ok(), await image.text()).toBe(true)
    const imageID = (await image.json()).id
    const exported = await request.post('/api/resource-exchange/export', { data: {
      package: { id: `shared-import-${theme}`, name: 'Shared import package' },
      resources: [{ kind: 'skill', scope: 'user', id: skillName }, { kind: 'preset.image', scope: 'global', id: imageID }],
    } })
    expect(exported.ok(), await exported.text()).toBe(true)
    const archive = await exported.body()
    const previewResponse = await request.post('/api/resource-exchange/previews', { multipart: { file: { name: 'shared.zip', mimeType: 'application/zip', buffer: archive } } })
    expect(previewResponse.ok(), await previewResponse.text()).toBe(true)
    const preview = await previewResponse.json()
    const first = await request.post('/api/resource-exchange/plans', { data: { preview_id: preview.preview_id, candidate_id: preview.candidates[0].candidate_id, resources: preview.candidates[0].resources.map((r: { id: string }) => r.id) } })
    expect(first.ok(), await first.text()).toBe(true)
    const firstPlan = await first.json()
    const installed = await request.post(`/api/resource-exchange/plans/${firstPlan.plan_id}/apply`, { data: {} })
    expect(installed.ok(), await installed.text()).toBe(true)
    await page.addInitScript(() => { localStorage.setItem('nova:mode', 'market'); localStorage.setItem('nova.locale.configured', 'en-US') })
    await page.route('**/api/resource-market/catalog', route => route.fulfill({ json: { schema_version: 1, entries: [] } }))
    await page.goto('/')
    await page.getByTestId('resource-market').getByRole('button', { name: 'Add resources', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('combobox').click()
    await page.getByRole('option', { name: 'Local file', exact: true }).click()
    await dialog.getByLabel('Local file', { exact: true }).setInputFiles({ name: 'shared.zip', mimeType: 'application/zip', buffer: archive })
    await dialog.getByRole('button', { name: 'Download and preview' }).click()
    await expect(dialog.getByRole('combobox', { name: 'Global and user resources' })).toContainText('Reuse existing resources (default)')
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await expect(dialog.getByText('Global and user resources', { exact: true })).toBeVisible()
      expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      await page.screenshot({ path: `test-results/shared-import-${theme}-${width}.png` })
    }
    await dialog.getByRole('button', { name: 'Review plan', exact: true }).click()
    await expect(dialog.getByText('Reuse existing resource', { exact: false })).toHaveCount(2)
    await dialog.getByRole('button', { name: 'Back', exact: true }).click()
    await dialog.getByRole('combobox', { name: 'Global and user resources' }).click()
    await page.getByRole('option', { name: 'Import independent copies', exact: true }).click()
    await dialog.getByRole('button', { name: 'Review plan', exact: true }).click()
    await expect(dialog.getByText(`${skillName}-2`, { exact: true })).toBeVisible()
    await expect(dialog.getByText('Reuse existing resource', { exact: false })).toHaveCount(0)
    await dialog.getByRole('button', { name: 'Add resources', exact: true }).click()
    await expect(dialog).toBeHidden()
  })
}
