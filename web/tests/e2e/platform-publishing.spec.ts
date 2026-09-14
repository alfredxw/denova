import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'

for (const kind of ['plugin', 'game'] as const) {
  test(`creates, tests, publishes and exports a neutral ${kind}`, async ({ page, request }) => {
    const chinese = kind === 'game'
    const language = chinese ? 'zh-CN' : 'en-US'
    const theme = chinese ? 'dark' : 'light'
    const name = `Local Publish ${kind}`
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language, theme } } })
    await page.addInitScript(({ language, theme }) => {
      localStorage.setItem('nova:mode', 'extensions')
      localStorage.setItem('nova.locale.configured', language)
      localStorage.setItem('theme', theme)
    }, { language, theme })
    await page.goto('/')
    await page.getByRole('button', { name: chinese ? '创建游戏' : 'Create plugin', exact: true }).click()
    const creation = page.getByRole('dialog', { name: chinese ? '创建扩展' : 'Create extension', exact: true })
    await creation.getByLabel(chinese ? '扩展名称' : 'Extension name').fill(name)
    await creation.locator('summary').filter({ hasText: chinese ? '高级选项' : 'Advanced options' }).click()
    await expect(creation.getByRole('combobox')).toHaveCount(0)
    await creation.getByRole('button', { name: chinese ? '创建并打开工作台' : 'Create and open workbench', exact: true }).click()

    const previewLabel = chinese ? '试运行' : 'Test run'
    const publishLabel = chinese ? '发布到本机' : 'Publish locally'
    await page.getByRole('button', { name: previewLabel, exact: true }).filter({ visible: true }).click()
    const preview = page.getByRole('dialog', { name: previewLabel, exact: true })
    await expect(preview.getByRole('combobox')).toHaveCount(0)
    await preview.getByRole('button', { name: chinese ? '启动试运行' : 'Start test run', exact: true }).click()
    if (kind === 'plugin') {
      await preview.getByLabel('Tool input (JSON)').fill('{"text":"Starter 🧩"}')
      await preview.getByRole('button', { name: 'Run', exact: true }).click()
      await expect(preview.locator('pre')).toContainText('Starter 🧩')
      await preview.getByRole('button', { name: 'Stop', exact: true }).click()
      await preview.getByRole('button', { name: 'Close', exact: true }).first().click()
    } else {
      const frame = page.frameLocator('iframe[title="游戏画面"]')
      await frame.getByRole('button', { name: '测试交互', exact: true }).click()
      await expect(frame.getByRole('status')).toContainText('交互已生效')
      await preview.getByRole('button', { name: '关闭', exact: true }).click()
    }
    await expect(preview).not.toBeVisible()

    const projects = await (await request.get('/api/agent-chat/projects')).json()
    const project = projects.projects.find((item: { name: string }) => item.name === name)
    const manifestPath = path.join(project.path, `denova.${kind}.json`)
    const manifest = JSON.parse(await readFile(manifestPath, 'utf8'))
    expect(manifest.modelSlots).toBeUndefined()
    expect(manifest.settings).toBeUndefined()
    // A declared model requirement must never turn into publication setup.
    manifest.modelSlots = [{ id: 'writer', titleKey: 'writer', kind: 'text', required: true }]
    for (const locale of ['zh-CN', 'en-US']) {
      const localePath = path.join(project.path, 'locales', locale + '.json')
      const content = JSON.parse(await readFile(localePath, 'utf8'))
      await writeFile(localePath, JSON.stringify({ ...content, writer: locale === 'zh-CN' ? '写作模型' : 'Writing model' }))
    }
    await writeFile(manifestPath, JSON.stringify(manifest))
    await writeFile(path.join(project.path, 'developer-notes.txt'), 'Source-only notes')
    await page.getByRole('button', { name: publishLabel, exact: true }).filter({ visible: true }).click()
    const publication = page.getByRole('dialog', { name: publishLabel, exact: true })
    await publication.getByLabel(chinese ? '说明' : 'Description', { exact: true }).fill('A long publication description. '.repeat(12))
    await publication.getByLabel(chinese ? '版本' : 'Version', { exact: true }).fill('0.2.0')
    await expect(publication.getByRole('combobox')).toHaveCount(0)
    await expect(publication.getByRole('switch')).toHaveCount(0)
    const confirmPublication = publication.getByRole('button', { name: chinese ? '确认发布' : 'Confirm publication', exact: true })
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await expect(confirmPublication).toBeInViewport({ ratio: 1 })
      await page.screenshot({ path: `test-results/publication-${kind}-${theme}-${width}.png`, fullPage: true })
      expect(await publication.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
    }
    await confirmPublication.click()
    await expect(page.getByRole('heading', { name, exact: true }).filter({ visible: true })).toBeVisible()
    const catalog = await (await request.get('/api/platform/manage/catalog')).json()
    const installed = catalog.find((item: { id: string }) => item.id === manifest.id)
    const release = installed.releases.find((item: { ref: { releaseId: string } }) => item.ref.releaseId === installed.currentRelease)
    expect(release.manifest.version).toBe('0.2.0')
    expect(installed.grants).toEqual(manifest.permissions.required)
    const entryPath = path.join(project.path, kind === 'plugin' ? 'server.mjs' : 'game.mjs')
    await writeFile(entryPath, await readFile(entryPath, 'utf8') + '\n// Unpublished source edit.\n')
    const exportLink = page.getByRole('link', { name: chinese ? '导出安装包' : 'Export package', exact: true })
    const exported = await request.get((await exportLink.getAttribute('href'))!)
    expect(exported.ok(), await exported.text()).toBe(true)
    expect(exported.headers()['content-type']).toContain('application/zip')
    const importedResponse = await request.post('/api/platform/manage/packages/preview', { headers: { 'Content-Type': 'application/zip' }, data: await exported.body() })
    expect(importedResponse.ok(), await importedResponse.text()).toBe(true)
    const imported = await importedResponse.json()
    expect(imported.digest).toBe(installed.currentRelease)
    expect(imported.files).not.toContain('developer-notes.txt')
    expect(imported.files).not.toContain('DEVELOPMENT.md')
    await request.delete(`/api/platform/manage/candidates/${imported.candidateId}`)
    await page.screenshot({ path: `test-results/published-${kind}-${theme}.png`, fullPage: true })
    await request.patch(`/api/platform/manage/packages/${kind}/${manifest.id}`, { data: { enabled: false } })
  })
}
