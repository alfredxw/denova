import { expect, test } from '../support/fixtures'

for (const theme of ['dark', 'light']) {
  test(`mixed package selection and readable contents in ${theme}`, async ({ page, request }) => {
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme } } })
    await page.addInitScript(() => { localStorage.setItem('nova:mode', 'market'); localStorage.setItem('nova.locale.configured', 'en-US') })
    const source = { kind: 'github', url: 'https://github.com/author/resources', ref: 'main', path: 'packages/writing-and-game' }
    const entry = { id: 'mixed', name: { 'en-US': 'Harbor creative collection with a long package title for writing and interactive stories' }, description: { 'en-US': 'Select the resources you need for your next story.' }, author: 'Author', kinds: ['preset.narrative', 'style.reference', 'game.opening'], tags: ['writing', 'game'], format: 'denova.resource-pack', updated_at: '2026-09-25', source, usage: { 'en-US': 'Use narrative presets in Creative Setups and openings when creating a story.' } }
    const resources = [
      { id: 'narrative', name: 'Quiet mystery', kind: 'preset.narrative', path: 'narrative.json', requires: ['prose'], digest: '1' },
      { id: 'prose', name: 'Prose reference', kind: 'style.reference', path: 'prose.md', digest: '2' },
      { id: 'opening', name: 'Late light', description: 'A quiet harbor at dusk.', kind: 'game.opening', path: 'opening.json', digest: '3' },
      { id: 'dawn', name: 'First light', kind: 'game.opening', path: 'dawn.json', digest: '4' },
      { id: 'harbor', name: 'Harbor', kind: 'lore.item', path: 'harbor.json', digest: '5' },
      { id: 'island', name: 'IslandWorldbuildingWithAVeryLongUnbrokenNameToVerifyWrappingOnNarrowScreens', kind: 'lore.item', path: 'island.json', digest: '6' },
    ]
    const preview = { preview_id: 'detail-preview', source, candidates: [{ candidate_id: 'mixed', package: { id: 'mixed', name: 'Harbor collection', version: '1.0.0' }, format: 'denova.resource-pack', resources }] }
    let previews = 0, deletes = 0
    await page.route('**/api/resource-market/catalog', route => route.fulfill({ json: { schema_version: 1, entries: [entry] } }))
    await page.route('**/api/resource-exchange/previews', async route => { previews++; await route.fulfill({ json: preview }) })
    await page.route('**/api/resource-exchange/previews/detail-preview', async route => { deletes++; await route.fulfill({ json: {} }) })
    await page.route('**/api/resource-exchange/previews/detail-preview/files?*', route => {
      const selected = new URL(route.request().url()).searchParams.get('path')
      return route.fulfill({ json: { files: [{ path: 'opening.json', bytes: 120 }], path: selected, content: selected ? JSON.stringify({ title: 'Late light', content: '# The third light\nA boat emerges from the fog.' }) : undefined, binary: false, truncated: false } })
    })
    await page.goto('/')
    await page.getByRole('button', { name: entry.name['en-US'], exact: true }).click()
    const detail = page.getByTestId('market-entry-detail')
    await expect(detail.getByRole('checkbox', { name: 'Select all Openings', exact: true })).toBeChecked()
    await expect(detail.getByRole('button', { name: 'Openings', exact: true })).toHaveAttribute('aria-expanded', 'false')
    await expect(detail.getByRole('button', { name: 'Lore', exact: true })).toHaveAttribute('aria-expanded', 'false')
    expect(previews).toBe(1)
    await expect(page.getByRole('navigation', { name: 'Market navigation' })).toBeHidden()
    await expect(detail.getByRole('link', { name: 'github.com/author/resources' })).toBeVisible()
    await expect(page.getByTestId('resource-market').getByRole('button', { name: 'Export package', exact: true })).toBeHidden()
    await detail.getByRole('button', { name: 'Lore', exact: true }).focus()
    await detail.getByRole('button', { name: 'Lore', exact: true }).press('ArrowDown')
    await expect(detail.getByRole('button', { name: 'Openings', exact: true })).toBeFocused()
    await detail.getByRole('button', { name: 'Openings', exact: true }).press('Enter')
    await expect(detail.getByRole('group', { name: 'Openings', exact: true }).getByRole('checkbox')).toHaveCount(1)
    const openingPreview = detail.getByRole('button', { name: 'Preview Late light', exact: true })
    await openingPreview.click()
    const previewDialog = page.getByRole('dialog', { name: 'Late light', exact: true })
    await expect(previewDialog.getByRole('heading', { name: 'The third light' })).toBeVisible()
    await previewDialog.getByRole('button', { name: 'Close', exact: true }).click()
    await expect(openingPreview).toBeFocused()
    await expect(detail.getByRole('button', { name: 'Openings', exact: true })).toHaveAttribute('aria-expanded', 'true')
    const search = detail.getByRole('textbox', { name: 'Search name, description or type' })
    await search.fill('Late light')
    await detail.getByRole('checkbox', { name: 'Select all Openings', exact: true }).uncheck()
    await expect(detail.getByRole('button', { name: 'Import 4 selected' })).toBeVisible()
    await detail.getByRole('checkbox', { name: 'Select all Openings', exact: true }).check()
    await expect(detail.getByRole('button', { name: 'Import 6 selected' })).toBeVisible()
    await search.fill('no matching content')
    await expect(detail.getByText('No matching contents. Your selection is unchanged.')).toBeVisible()
    await search.fill('')
    await detail.getByRole('button', { name: 'Clear selection' }).click()
    await detail.getByRole('checkbox', { name: 'Quiet mystery', exact: true }).check()
    await expect(detail.getByRole('checkbox', { name: 'Prose reference', exact: true })).toBeChecked()
    await expect(detail.getByRole('checkbox', { name: 'Prose reference', exact: true })).toBeDisabled()
    await expect(detail.getByRole('checkbox', { name: 'Select all Openings', exact: true })).not.toBeChecked()
    await detail.getByRole('button', { name: 'Lore', exact: true }).click()
    await expect(detail.getByRole('group', { name: 'Lore', exact: true }).getByRole('checkbox')).toHaveCount(1)
    await expect(detail.getByText(resources[5].name, { exact: true })).toBeVisible()
    await expect(detail.getByRole('group', { name: 'Lore', exact: true }).locator('[aria-expanded]:not([aria-haspopup="dialog"])')).toHaveCount(1)
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await detail.getByRole('group', { name: 'Lore', exact: true }).scrollIntoViewIfNeeded()
      expect(await detail.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      const parent = await detail.getByRole('button', { name: 'Lore', exact: true }).boundingBox()
      const child = await detail.getByText('Harbor', { exact: true }).boundingBox()
      expect(child!.x - parent!.x).toBeGreaterThanOrEqual(24)
      await page.screenshot({ path: `test-results/market-detail-${theme}-${width}.png`, fullPage: true })
      const longNamePreview = detail.getByRole('button', { name: `Preview ${resources[5].name}`, exact: true })
      await longNamePreview.click()
      const contentDialog = page.getByRole('dialog', { name: resources[5].name, exact: true })
      await expect(contentDialog.getByRole('heading', { name: 'The third light' })).toBeVisible()
      expect(await contentDialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      await page.screenshot({ path: `test-results/market-preview-${theme}-${width}.png`, fullPage: true })
      await contentDialog.press('Escape')
      await expect(longNamePreview).toBeFocused()
    }
    await detail.getByRole('button', { name: 'Import 2 selected' }).click()
    const dialog = page.getByRole('dialog', { name: 'Import package', exact: true })
    await expect(dialog.getByText(/Quiet mystery/)).toBeVisible()
    await expect(dialog.getByText(/Prose reference/)).toBeVisible()
    await expect(dialog.getByText(/Late light/)).toHaveCount(0)
    await expect(dialog.getByLabel('Source URL')).toHaveCount(0)
    await dialog.getByRole('button', { name: 'Back to selection' }).click()
    await expect(detail.getByRole('button', { name: 'Import 2 selected' })).toBeVisible()
    expect(previews).toBe(1)
    expect(deletes).toBe(0)
    const planRequest = page.waitForRequest('**/api/resource-exchange/plans')
    await page.route('**/api/resource-exchange/plans', route => route.fulfill({ json: { plan_id: 'plan', items: resources.slice(0, 2).map(resource => ({ resource_id: resource.id, name: resource.name, local: { kind: resource.kind }, action: 'create' })), installation: { package: preview.candidates[0].package } } }))
    await detail.getByRole('button', { name: 'Import 2 selected' }).click()
    await dialog.getByRole('button', { name: 'Review plan', exact: true }).click()
    expect((await planRequest).postDataJSON()).toMatchObject({ preview_id: 'detail-preview', candidate_id: 'mixed', resources: ['narrative'] })
    await page.getByRole('dialog', { name: 'Review installation plan' }).getByRole('button', { name: 'Close', exact: true }).click()
    await detail.getByRole('button', { name: 'Back', exact: true }).click()
    await expect.poll(() => deletes).toBe(1)
  })
}
