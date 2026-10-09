import { createRequire } from 'node:module'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import type { LoreItem } from '../../src/lib/api'

for (const theme of ['dark', 'light']) {
  test(`book defaults prefill once and save explicit choices in ${theme}`, async ({ page, request }) => {
    test.setTimeout(90_000)
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'zh-CN', theme } } })
    const globalBefore = (await (await request.get('/api/settings')).json()).user
    const book = await createAndOpenBook(request, `Defaults ${theme}`)
    const settingsURL = `/api/projects/${book.projectId}/settings`
    const loreURL = `/api/projects/${book.projectId}/book/lore/items`
    const created = await request.post(loreURL, { data: { id: 'world', name: '默认世界', type: 'world', content: 'A world for the test.' } })
    expect(created.ok(), await created.text()).toBe(true)
    const sharp = createRequire(import.meta.url)('sharp') as typeof import('sharp').default
    const longName = 'LongBackgroundName'.repeat(8)
    const uploaded = await request.post(`${loreURL}/world/materials/upload`, { multipart: { file: {
      name: `${longName}.png`, mimeType: 'image/png', buffer: await sharp({ create: { width: 320, height: 180, channels: 3, background: '#735c49' } }).png().toBuffer(),
    } } })
    expect(uploaded.ok(), await uploaded.text()).toBe(true)
    const items = (await (await request.get(loreURL)).json()).items as LoreItem[]
    const asset = items.find(item => item.id === 'world')!.resolved_materials![0]
    const defaults = { narrative_style_id: 'classic', event_package_ids: [], default_background: { mode: 'image', item_id: 'world', asset_id: asset.id } }
    const saved = await request.patch(settingsURL, { data: { layer: 'workspace', changes: { game_creation_defaults: defaults } } })
    expect(saved.ok(), await saved.text()).toBe(true)
    await page.addInitScript(() => localStorage.setItem('nova:mode', 'interactive'))
    await page.goto('/')
    await page.getByRole('button', { name: '新建', exact: true }).filter({ visible: true }).click()
    await expect(page.getByRole('heading', { name: '新故事线', exact: true })).toBeVisible()
    await expect(page.getByText('已预填本书默认资源；这里的调整只影响当前故事。')).toBeVisible()
    const background = page.getByRole('button', { name: '当前背景', exact: true })
    await expect(background).toHaveAttribute('title', `${longName}.png`)
    await page.getByRole('button', { name: '保存资源搭配为本书默认', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '本书的新故事默认配置', exact: true })
    await expect(dialog.getByRole('checkbox', { name: /^默认背景/ })).not.toBeChecked()
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`defaults-${theme}-${width}.png`) })
    }
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await page.setViewportSize({ width: 1440, height: 900 })
    await background.click()
    const picker = page.getByRole('dialog', { name: '当前背景', exact: true })
    await picker.getByRole('textbox').fill('missing-image')
    await expect(picker.getByText('暂无匹配图片，请先在资料库中绑定本地图片素材。')).toBeVisible()
    await picker.getByRole('button', { name: '无背景', exact: true }).click()
    // Reopening a resource picker must not restore the initial recommendation.
    await background.click()
    await picker.getByRole('button', { name: '无背景', exact: true }).click()
    await expect(background).toHaveText('无背景')
    await page.getByRole('button', { name: '保存资源搭配为本书默认', exact: true }).click()
    await dialog.getByRole('checkbox', { name: /^默认背景/ }).check()
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(dialog).toBeHidden()
    expect((await (await request.get(settingsURL)).json()).workspace.game_creation_defaults.default_background).toEqual({ mode: 'none' })
    expect((await (await request.get('/api/settings')).json()).user).toEqual(globalBefore)
    await expect(background).toHaveText('无背景')
    // Creating through the API exercises the same default resolution as the form.
    const storyResponse = await request.post('/api/interactive/stories', { data: { title: 'Captured defaults', state_schema_policy: { mode: 'fixed_template' } } })
    expect(storyResponse.ok(), await storyResponse.text()).toBe(true)
    const story = await storyResponse.json()
    expect(story.module_refs.narrative_style_id).toBe('classic')
    expect(story.module_refs.event_packages_disabled).toBe(true)
    expect(story.presentation_settings.default_background).toBeUndefined()
    await request.patch(settingsURL, { data: { layer: 'workspace', changes: { game_creation_defaults: { narrative_style_id: '' } } } })
    const stories = (await (await request.get('/api/interactive/stories')).json()).stories
    expect(stories.find((item: { id: string }) => item.id === story.id).module_refs).toEqual(story.module_refs)
    const other = await createAndOpenBook(request, 'Independent defaults')
    expect((await (await request.get(`/api/projects/${other.projectId}/settings`)).json()).workspace?.game_creation_defaults).toBeUndefined()
  })
}

for (const [locale, theme] of [['zh-CN', 'dark'], ['en-US', 'light']]) {
  test(`package adoption review preserves existing choices in ${locale}`, async ({ page, request }) => {
    const zh = locale === 'zh-CN'
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language: locale, theme } } })
    const book = await createAndOpenBook(request, 'Import recommendations')
    await request.patch(`/api/projects/${book.projectId}/settings`, { data: { layer: 'workspace', changes: { game_creation_defaults: { narrative_style_id: 'classic' } } } })
    const name = 'Recommended world'
    const source = { kind: 'github', url: 'https://github.com/example/world' }
    const entry = { id: 'world', name: { [locale]: name }, description: { [locale]: name }, author: 'Fixture', kinds: ['lore.collection', 'preset.narrative'], format: 'denova.resource-pack', tags: [], source, updated_at: '2026-09-30' }
    const resources = [
      { id: 'lore', kind: 'lore.collection', name: 'LongWorldName'.repeat(12), path: 'lore.json', digest: '1', item_count: 1 },
      { id: 'style', kind: 'preset.narrative', name: 'Package narrative', path: 'style.json', digest: '2' },
    ]
    const candidate = { candidate_id: '.', package: { id: 'world', name }, format: 'denova.resource-pack', resources, game_defaults: { narrative_style_id: 'style', default_background: { resource_id: 'lore', item_id: 'world', asset_path: 'background.png' } } }
    await page.route('**/api/resource-market/catalog', route => route.fulfill({ json: { schema_version: 1, entries: [entry] } }))
    await page.route('**/api/resource-exchange/previews', route => route.fulfill({ json: { preview_id: 'defaults', source, candidates: [candidate] } }))
    await page.route('**/api/resource-exchange/previews/defaults', route => route.fulfill({ json: {} }))
    await page.addInitScript(locale => { localStorage.setItem('nova:mode', 'market'); localStorage.setItem('nova.locale.configured', locale) }, locale)
    await page.goto('/')
    await page.getByRole('button', { name, exact: true }).click()
    await page.getByTestId('market-entry-detail').getByRole('button', { name: zh ? '获取资源包' : 'Get this package', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: zh ? '添加资源' : 'Add resources', exact: true })
    await dialog.getByRole('combobox', { name: zh ? '目标作品' : 'Target project' }).click()
    await page.getByRole('option', { name: book.title, exact: true }).click()
    const master = dialog.getByRole('checkbox', { name: zh ? '用作本书的新故事默认配置' : 'Use as new-story defaults for this book', exact: true })
    await expect(master).toBeEnabled()
    await expect(master).not.toBeChecked()
    await master.check()
    const narrative = dialog.getByRole('checkbox', { name: /^(叙事风格|Narrative style) ·/ })
    await expect(narrative).not.toBeChecked()
    await expect(dialog.getByRole('checkbox', { name: /^(默认背景|Default background) ·/ })).toBeChecked()
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await master.scrollIntoViewIfNeeded()
      expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`import-defaults-${locale}-${width}.png`) })
    }
    await narrative.check()
    await expect(dialog.getByText(zh ? '将替换：本书已有配置' : 'Will replace: Existing book setting', { exact: true })).toBeVisible()
    // Reviewing recommendations alone must not save any Project configuration.
    await dialog.getByRole('button', { name: zh ? '取消' : 'Cancel', exact: true }).click()
    expect((await (await request.get(`/api/projects/${book.projectId}/settings`)).json()).workspace.game_creation_defaults).toEqual({ narrative_style_id: 'classic' })
  })
}
