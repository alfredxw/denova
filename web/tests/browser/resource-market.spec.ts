import { expect, test } from '../support/fixtures'

test('resource errors are localized by the API before reaching the UI', async ({
  request,
}) => {
  for (const [locale, message] of [
    ['zh-CN', '操作未完成'],
    ['en-US', 'The operation could not complete'],
  ]) {
    const response = await request.post('/api/resource-exchange/previews', {
      headers: { 'X-Denova-Locale': locale },
      data: { kind: 'github', url: 'http://invalid.example/repository' },
    })
    expect(response.status()).toBe(400)
    const body = await response.json()
    expect(body.error).toContain(message)
    expect(body.error).not.toContain('market.errors.')
  }
})

for (const theme of ['dark', 'light']) {
  test(`export picker reflects builtin edits and restoration in ${theme}`, async ({ page, request }) => {
    await request.patch('/api/settings', {
      data: { layer: 'user', changes: { language: 'en-US', theme } },
    })
    await page.addInitScript(() => {
      localStorage.setItem('nova:mode', 'market')
      localStorage.setItem('nova.locale.configured', 'en-US')
    })
    await page.route('**/api/resource-market/catalog', (route) =>
      route.fulfill({ json: { schema_version: 1, entries: [] } }),
    )
    const presetsResponse = await request.get('/api/image-presets')
    expect(presetsResponse.ok(), await presetsResponse.text()).toBe(true)
    const { presets } = await presetsResponse.json()
    const builtin = presets.find((preset: { custom: boolean }) => !preset.custom)
    expect(builtin).toBeDefined()
    const choicesResponse = await request.get('/api/resource-exchange/export-resources')
    expect(choicesResponse.ok(), await choicesResponse.text()).toBe(true)
    const choices = await choicesResponse.json()
    expect(choices.some((item: { local: { scope: string } }) => item.local.scope === 'builtin')).toBe(false)
    await page.goto('/')
    const market = page.getByTestId('resource-market')
    const exporter = page.getByRole('dialog', { name: 'Export package', exact: true })
    await market.getByRole('button', { name: 'Export package', exact: true }).click()
    await exporter.getByRole('textbox', { name: 'Search name, description or type' }).fill(builtin.name)
    await expect(exporter.locator('[aria-busy]')).toHaveAttribute('aria-busy', 'false')
    await expect(exporter.getByRole('checkbox', { name: builtin.name, exact: true })).toHaveCount(0)
    await exporter.getByRole('button', { name: 'Close', exact: true }).click()

    const editedName = `Edited builtin for export ${theme}`
    try {
      const editedResponse = await request.patch(`/api/image-presets/${builtin.id}`, {
        data: { ...builtin, name: editedName, base_revision: builtin.revision },
      })
      expect(editedResponse.ok(), await editedResponse.text()).toBe(true)
      await market.getByRole('button', { name: 'Export package', exact: true }).click()
      await exporter.getByRole('textbox', { name: 'Search name, description or type' }).fill(editedName)
      await expect(exporter.getByRole('checkbox', { name: editedName, exact: true })).toBeVisible()
      await page.screenshot({ path: `test-results/export-picker-${theme}.png` })
      await exporter.getByRole('button', { name: 'Close', exact: true }).click()
    } finally {
      const restoredResponse = await request.delete(`/api/image-presets/${builtin.id}`)
      expect(restoredResponse.ok(), await restoredResponse.text()).toBe(true)
    }
    await market.getByRole('button', { name: 'Export package', exact: true }).click()
    await exporter.getByRole('textbox', { name: 'Search name, description or type' }).fill(builtin.name)
    await expect(exporter.locator('[aria-busy]')).toHaveAttribute('aria-busy', 'false')
    await expect(exporter.getByRole('checkbox', { name: builtin.name, exact: true })).toHaveCount(0)
    await exporter.getByRole('button', { name: 'Close', exact: true }).click()
  })

  test(`resource market discovery and import in ${theme}`, async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000)
    await request.patch('/api/settings', {
      data: { layer: 'user', changes: { language: 'en-US', theme } },
    })
    await page.addInitScript(() => {
      localStorage.setItem('nova:mode', 'market')
      localStorage.setItem('nova.locale.configured', 'en-US')
    })
    const entries = Array.from({ length: 51 }, (_, i) => ({
      id: `fixture-${i}`,
      name: { 'en-US': `Creative resource ${i}` },
      description: {
        'en-US':
          i === 0
            ? 'Long description '.repeat(80)
            : 'A writing resource for the test library.',
      },
      author: 'Fixture author',
      format:
        i === 50 ? 'extension.game' : i === 49 ? 'extension.plugin' : i === 48 ? 'denova.resource-pack' : 'skill',
      kinds:
        i === 48
          ? ['preset.narrative', 'style.reference']
          : [i === 50 ? 'extension.game' : i === 49 ? 'extension.plugin' : 'skill'],
      tags: ['writing'],
      updated_at: '2026-09-24',
      source: { kind: 'github', url: 'https://github.com/author/repository' },
    }))
    let fetches = 0
    await page.route('**/api/resource-market/catalog', (route) => {
      fetches++
      return route.fulfill({ json: { schema_version: 1, entries } })
    })
    await page.goto('/')
    const market = page.getByTestId('resource-market')
    await expect(
      market.getByRole('heading', { name: 'Marketplace', exact: true }),
    ).toBeVisible()
    await expect(
      market.getByRole('button', { name: /^Creative resource/ }),
    ).toHaveCount(50)
    await page.setViewportSize({ width: 1440, height: 900 })
    const navigation = page.getByRole('navigation', {
      name: 'Market navigation',
    })
    // Mixed packages are discoverable under each type of content they include.
    for (const category of ['Creative Setups', 'Style references']) {
      await navigation.getByRole('button', { name: category, exact: true }).click()
      await expect(market.getByTestId('market-resource-card')).toHaveCount(1)
      await expect(market.getByRole('button', { name: 'Creative resource 48', exact: true })).toBeVisible()
    }
    await navigation
      .getByRole('button', { name: 'Content type', exact: true })
      .click()
    await expect(
      navigation.getByRole('button', { name: 'Plugins', exact: true }),
    ).toBeHidden()
    await navigation
      .getByRole('button', { name: 'Content type', exact: true })
      .click()
    await navigation
      .getByRole('button', { name: 'Plugins', exact: true })
      .click()
    await expect(market.getByTestId('market-resource-card')).toHaveCount(1)
    await navigation
      .getByRole('button', { name: 'Discover', exact: true })
      .click()
    await market
      .getByRole('button', { name: 'Collapse sidebar', exact: true })
      .click()
    await expect(navigation).toBeHidden()
    await market
      .getByRole('button', { name: 'Expand sidebar', exact: true })
      .click()
    await expect(navigation).toBeVisible()
    const cards = market.getByTestId('market-resource-card')
    expect((await cards.first().boundingBox())!.height).toBeLessThan(200)
    await market.getByRole('radio', { name: 'List view' }).click()
    expect((await cards.first().boundingBox())!.height).toBeLessThan(110)
    await market.getByRole('radio', { name: 'Grid view' }).click()
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await expect(
        market.getByRole('button', { name: 'Import package', exact: true }),
      ).toBeVisible()
      expect(
        await market.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true)
      await page.screenshot({
        path: `test-results/market-${theme}-${width}.png`,
        fullPage: true,
      })
    }
    await page
      .getByRole('button', {
        name: 'Market navigation directory',
        exact: true,
      })
      .click()
    await navigation.getByRole('button', { name: 'Games', exact: true }).click()
    await expect(navigation).toBeHidden()
    await expect(market.getByTestId('market-resource-card')).toHaveCount(1)
    await page
      .getByRole('button', {
        name: 'Market navigation directory',
        exact: true,
      })
      .click()
    await navigation
      .getByRole('button', { name: 'Discover', exact: true })
      .click()
    await market
      .getByRole('textbox', { name: 'Search name, description or author' })
      .fill('no matching package')
    await expect(
      market.getByText('No packages found', { exact: true }),
    ).toBeVisible()
    await market
      .getByRole('button', { name: 'Clear filters', exact: true })
      .click()
    await market
      .getByRole('textbox', { name: 'Search name, description or author' })
      .fill('resource 50')
    await expect(
      market.getByRole('button', { name: /^Creative resource/ }),
    ).toHaveCount(1)
    await page.route('**/api/resource-exchange/previews', (route) => route.fulfill({ json: { preview_id: 'catalog-preview', candidates: [{ candidate_id: 'catalog-candidate', package: { id: 'catalog', name: 'Creative resource 50' }, format: 'extension.game', resources: [] }], source: entries[50].source } }))
    await page.route('**/api/resource-exchange/previews/catalog-preview', (route) => route.fulfill({ json: {} }))
    const beforeDetails = fetches
    await market
      .getByRole('button', { name: 'Creative resource 50', exact: true })
      .click()
    await expect(
      market.getByRole('heading', { name: 'Creative resource 50' }),
    ).toBeVisible()
    await market.getByRole('button', { name: 'Back', exact: true }).click()
    await expect(
      market.getByRole('textbox', {
        name: 'Search name, description or author',
      }),
    ).toHaveValue('resource 50')
    expect(fetches).toBe(beforeDetails)
    await page.unroute('**/api/resource-exchange/previews')
    // Export a user definition, then reimport through the shared UI.
    const createdResponse = await request.post('/api/image-presets', {
      data: { name: `Market image ${theme}`, prompt: 'Draw a quiet harbor.' },
    })
    expect(createdResponse.ok(), await createdResponse.text()).toBe(true)
    const created = await createdResponse.json()
    const choicesResponse = await request.get(
      '/api/resource-exchange/export-resources',
    )
    expect(choicesResponse.ok(), await choicesResponse.text()).toBe(true)
    const choices = await choicesResponse.json()
    const image = choices.find(
      (item: { local: { kind: string; id: string } }) =>
        item.local.kind === 'preset.image' && item.local.id === created.id,
    )
    const archiveResponse = await request.post(
      '/api/resource-exchange/export',
      {
        data: {
          package: {
            id: `market-fixture-${theme}`,
            name: `Market fixture ${theme}`,
          },
          resources: [image.local],
        },
      },
    )
    expect(archiveResponse.ok(), await archiveResponse.text()).toBe(true)
    await market
      .getByRole('button', { name: 'Import package', exact: true })
      .click()
    const dialog = page.getByRole('dialog', {
      name: 'Import package',
      exact: true,
    })
    await dialog.getByRole('combobox').click()
    await page.getByRole('option', { name: 'Local file', exact: true }).click()
    await dialog.getByLabel('Local file', { exact: true }).setInputFiles({
      name: 'fixture.zip',
      mimeType: 'application/zip',
      buffer: await archiveResponse.body(),
    })
    await dialog.getByRole('button', { name: 'Download and preview' }).click()
    const previewTrigger = dialog.getByRole('button', { name: `Preview Market image ${theme}`, exact: true })
    await previewTrigger.click()
    const resourcePreview = page.getByRole('dialog', { name: `Market image ${theme}`, exact: true })
    await expect(resourcePreview.getByText('Draw a quiet harbor.', { exact: true })).toBeVisible()
    await resourcePreview.getByText('View source', { exact: true }).click()
    await expect(resourcePreview.locator('pre')).toContainText('name')
    await resourcePreview.getByRole('button', { name: 'Close', exact: true }).click()
    await expect(previewTrigger).toBeFocused()
    await expect(dialog.getByRole('checkbox', { name: `Market image ${theme}`, exact: true })).toBeChecked()
    await dialog
      .getByRole('button', { name: 'Review plan', exact: true })
      .click()
    const confirmation = page.getByRole('dialog', {
      name: 'Review installation plan',
      exact: true,
    })
    await expect(
      confirmation.getByText(`Market fixture ${theme}`, { exact: true }),
    ).toBeVisible()
    await confirmation
      .getByRole('button', { name: 'Install', exact: true })
      .click()
    await expect(confirmation).toBeHidden()
    await expect(
      market.getByRole('heading', { name: 'Acquired', exact: true }),
    ).toBeVisible()
    await market
      .getByRole('button', { name: new RegExp(`Market fixture ${theme}`) })
      .click()
    await expect(
      market.getByRole('button', { name: 'Stop tracking source', exact: true }),
    ).toBeVisible()
    // Save and reuse an export selection, reviewing a frozen archive before download.
    await market
      .getByRole('button', { name: 'Export package', exact: true })
      .last()
      .click()
    const exporter = page.getByRole('dialog', {
      name: 'Export package',
      exact: true,
    })
    const imageGroup = exporter.getByRole('group', { name: 'Image setup', exact: true })
    const imageGroupTrigger = imageGroup.getByRole('button', { name: 'Image setup', exact: true })
    await expect(imageGroupTrigger).toHaveAttribute('aria-expanded', 'false')
    await imageGroupTrigger.click()
    await expect(imageGroup.getByRole('checkbox', { name: `Market image ${theme}`, exact: true, checked: true })).toHaveCount(1)
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await imageGroup.scrollIntoViewIfNeeded()
      expect(await exporter.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      const parent = await imageGroup.getByRole('checkbox', { name: 'Select all Image setup', exact: true }).boundingBox()
      const child = await imageGroup.getByRole('checkbox', { name: `Market image ${theme}`, exact: true }).first().boundingBox()
      expect(child!.x - parent!.x).toBeGreaterThanOrEqual(24)
      await page.screenshot({ path: `test-results/market-export-${theme}-${width}.png`, fullPage: true })
    }
    await exporter.getByLabel('Save this selection for future exports').check()
    await exporter
      .getByRole('button', { name: 'Review export', exact: true })
      .click()
    await expect(exporter.getByText(/1 resources/)).toBeVisible()
    const downloadEvent = page.waitForEvent('download')
    await exporter
      .getByRole('button', { name: 'Download ZIP', exact: true })
      .click()
    expect((await downloadEvent).suggestedFilename()).toBe(
      `market-fixture-${theme}.zip`,
    )
    await expect(exporter).toBeHidden()
    await market
      .getByRole('button', { name: 'Installation backups', exact: true })
      .click()
    const backup = page.getByRole('dialog', {
      name: 'Installation backups',
      exact: true,
    })
    await backup
      .getByRole('button', { name: 'Review restoration', exact: true })
      .click()
    await backup
      .getByRole('button', { name: 'Restore backup', exact: true })
      .click()
    await expect(backup).toBeHidden()
    await expect(
      market.getByRole('button', {
        name: new RegExp(`Market fixture ${theme}`),
      }),
    ).toHaveCount(0)
    const deletedResponse = await request.delete(`/api/image-presets/${created.id}`)
    expect(deletedResponse.ok(), await deletedResponse.text()).toBe(true)
  })
}
