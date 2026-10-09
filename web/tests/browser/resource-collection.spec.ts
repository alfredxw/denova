import { expect, test } from '../support/fixtures'

for (const kind of ['lore.collection', 'game.openings']) for (const locale of ['en-US', 'zh-CN']) for (const theme of ['dark', 'light']) {
  test(`${kind} collection preview ${locale} ${theme}`, async ({ page, request }) => {
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: locale, theme } } })
    await page.addInitScript(locale => { localStorage.setItem('nova:mode', 'market'); localStorage.setItem('nova.locale.configured', locale) }, locale)
    const chinese = locale === 'zh-CN'
    const source = { kind: 'file', filename: 'world.zip' }
    const name = chinese ? '照夜长河' : 'Long River'
    const entry = { id: 'world', name: { [locale]: name }, description: { [locale]: name }, author: 'Fixture', kinds: [kind], format: 'denova.resource-pack', tags: [], source, updated_at: '2026-09-29' }
    const items = Array.from({ length: 300 }, (_, i) => ({ id: `item-${i}`, name: `${chinese ? '资料' : 'Entry'} ${i}`, brief_description: 'World knowledge' }))
    const resource = { id: 'lore', kind, path: 'collection.json', name, digest: '1', item_count: items.length }
    await page.route('**/api/resource-market/catalog', route => route.fulfill({ json: { schema_version: 1, entries: [entry] } }))
    await page.route('**/api/resource-exchange/previews', route => route.fulfill({ json: { preview_id: 'collection', source, candidates: [{ candidate_id: '.', package: { id: 'world', name }, format: 'denova.resource-pack', resources: [resource] }] } }))
    await page.route('**/api/resource-exchange/previews/collection', route => route.fulfill({ json: {} }))
    await page.route('**/api/resource-exchange/previews/collection/files?*', route => {
      const params = new URL(route.request().url()).searchParams
      const item = items.find(item => item.id === params.get('item_id'))
      return route.fulfill({ json: { files: [{ path: 'collection.json', bytes: 200000 }], items, path: params.get('path'), content: item ? JSON.stringify({ ...item, content: `# ${item.name}\n\nA separately readable entry from the same collection file.` }) : '', truncated: false, binary: false } })
    })
    await page.goto('/')
    await page.getByRole('button', { name, exact: true }).click()
    await page.getByTestId('market-entry-detail').getByRole('button', { name: chinese ? '获取资源包' : 'Get this package', exact: true }).click()
    const detail = page.getByRole('dialog', { name: chinese ? '添加资源' : 'Add resources', exact: true })
    await detail.getByRole('button', { name: kind === 'lore.collection' ? (chinese ? '资料' : 'Lore') : (chinese ? '开场白' : 'Openings'), exact: true }).click()
    await expect(detail.getByText(chinese ? '300 项内容 · 一个集合文件' : '300 items · one collection file')).toBeVisible()
    await expect(detail.getByRole('checkbox')).toHaveCount(2)
    await detail.getByRole('button', { name: `${chinese ? '预览' : 'Preview'} ${name}`, exact: true }).click()
    const dialog = page.getByRole('dialog', { name, exact: true })
    await expect(dialog.getByRole('heading', { name: items[0].name, exact: true })).toBeVisible()
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      const search = dialog.getByRole('textbox', { name: chinese ? '搜索集合内容' : 'Search collection contents' })
      await search.fill('299')
      await dialog.getByRole('button', { name: items[299].name, exact: true }).click()
      await expect(dialog.getByRole('heading', { name: items[299].name, exact: true })).toBeVisible()
      expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      await page.screenshot({ path: `test-results/${kind}-collection-${locale}-${theme}-${width}.png`, fullPage: true })
      await search.fill('no-such-item')
      await expect(dialog.getByRole('button', { name: items[299].name, exact: true })).toHaveCount(0)
      await search.fill('')
    }
  })
}
