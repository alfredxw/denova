import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import type { LoreItem } from '../../src/lib/api'
import { readFile } from 'node:fs/promises'
import path from 'node:path'

const portrait = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=',
  'base64',
)

for (const theme of ['dark', 'light']) {
  test(`remote lore images preserve references across reuse and navigation in ${theme}`, async ({
    page,
    request,
    browserDiagnostics,
  }, testInfo) => {
    const settings = await (await request.get('/api/settings')).json()
    const configured = await request.patch('/api/settings', {
      data: {
        layer: 'user',
        base_revision: settings.revisions.user,
        changes: { theme, language: 'zh-CN' },
      },
    })
    expect(configured.ok()).toBe(true)
    const book = await createAndOpenBook(request, `Remote lore ${theme}`)
    const itemsURL = `/api/projects/${book.projectId}/book/lore/items`
    for (const id of ['hero', 'scene']) {
      const created = await request.post(itemsURL, {
        data: {
          id,
          name: id === 'hero' ? '网络参考角色' : '场景',
          type: 'character',
          content: 'Preserve lore text',
          enabled: true,
        },
      })
      expect(created.ok()).toBe(true)
    }
    const readItems = async (): Promise<LoreItem[]> =>
      (await (await request.get(itemsURL)).json()).items
    const readHero = async () => (await readItems()).find((item) => item.id === 'hero')!
    const url = 'https://images.example.com/portrait.png?token=keep%2Fexact'
    const replacement = 'https://images.example.com/replacement.png'
    await page.route('https://images.example.com/**', (route) =>
      route.fulfill({
        contentType: 'image/png',
        headers: { 'Cache-Control': 'no-store' },
        body: portrait,
      }),
    )
    await page.goto('/')
    const sidebar = page.getByLabel('工作台侧边栏')
    await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
    await page.getByRole('button', { name: /^网络参考角色/ }).click()
    await page.getByRole('tab', { name: '素材 (0)', exact: true }).click()
    await expect(page.getByText('还没有关联素材', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '添加素材', exact: true }).click()
    await page.getByRole('menuitem', { name: '来自网络', exact: true }).click()
    let dialog = page.getByRole('dialog', { name: '来自网络', exact: true })
    await dialog.getByLabel('图片链接', { exact: true }).fill('javascript:alert(1)')
    await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeDisabled()
    await dialog.getByLabel('图片链接', { exact: true }).fill(url)
    const name = ('网络肖像 ' + 'Long reference '.repeat(8)).trim()
    await dialog.getByLabel('素材名称', { exact: true }).fill(name)
    await dialog
      .getByLabel('素材说明（可选）', { exact: true })
      .fill('保持角色脸型；仅作创作参考。'.repeat(8))
    await expect(dialog.getByRole('img', { name: '网络图片预览' })).toBeVisible()
    await expect(
      dialog.getByRole('checkbox', { name: '保存到项目', exact: true }),
    ).not.toBeChecked()
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.getByRole('tab', { name: '素材 (1)', exact: true })).toBeVisible()
    const original = (await readHero()).resolved_materials![0]
    expect(original.url).toBe(url)
    expect(original.path).toBeUndefined()
    const linked = await request.post(`${itemsURL}/scene/materials`, {
      data: { op: 'link', asset_id: original.id, name: 'Shared reference' },
    })
    expect(linked.ok()).toBe(true)
    await page.getByRole('button', { name: /^素材操作：网络肖像/ }).click()
    await page.getByRole('menuitem', { name: '设为封面', exact: true }).click()
    await expect.poll(async () => (await readHero()).image?.image_url).toBe(url)
    await page.getByRole('button', { name: /^查看素材：网络肖像/ }).click()
    dialog = page.getByRole('dialog', { name, exact: true })
    await expect(
      dialog.getByText('需要 AI 读图时，请先保存到项目。', { exact: true }),
    ).toBeVisible()
    await expect(
      dialog.getByRole('link', { name: 'images.example.com', exact: true }),
    ).toHaveAttribute('href', url)
    await page.screenshot({
      path: testInfo.outputPath(`remote-${theme}-wide.png`),
      animations: 'disabled',
    })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.screenshot({
      path: testInfo.outputPath(`remote-${theme}-narrow.png`),
      animations: 'disabled',
    })
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
    await dialog.getByRole('button', { name: '替换链接', exact: true }).click()
    dialog = page.getByRole('dialog', { name: '替换链接', exact: true })
    await dialog.getByLabel('图片链接', { exact: true }).fill(replacement)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect.poll(async () => (await readHero()).image?.image_url).toBe(replacement)
    expect(
      (await readItems()).find((item) => item.id === 'scene')!.resolved_materials![0].url,
    ).toBe(url)
    expect((await readHero()).resolved_materials![0].name).toBe(name)
    await page.setViewportSize({ width: 1280, height: 900 })
    await sidebar.getByRole('button', { name: '写作', exact: true }).click()
    await Promise.all([
      page.waitForResponse(
        (response) =>
          response.url().includes('/api/interactive/stories') &&
          response.request().method() === 'GET' &&
          response.ok(),
      ),
      sidebar.getByRole('button', { name: '游戏', exact: true }).click(),
    ])
    await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
    await page.reload()
    await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
    await page.getByRole('button', { name: /^网络参考角色/ }).click()
    await page.getByRole('tab', { name: '素材 (1)', exact: true }).click()
    await expect(page.locator(`img[src="${replacement}"]`).first()).toBeVisible()
    // A failed image remains editable, and retry loads the same URL again.
    await page.unroute('https://images.example.com/**')
    browserDiagnostics.allow(/console\.error: Failed to load resource:.*404/)
    await page.route('https://images.example.com/**', (route) =>
      route.fulfill({ status: 404, headers: { 'Cache-Control': 'no-store' }, body: '' }),
    )
    await page.reload()
    await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
    await page.getByRole('button', { name: /^网络参考角色/ }).click()
    await page.getByRole('tab', { name: '素材 (1)', exact: true }).click()
    await page.getByRole('button', { name: /^查看素材：网络肖像/ }).click()
    dialog = page.getByRole('dialog', { name, exact: true })
    await expect(dialog.getByText('图片无法加载', { exact: true })).toBeVisible()
    await page.unroute('https://images.example.com/**')
    await page.route('https://images.example.com/**', (route) =>
      route.fulfill({
        contentType: 'image/png',
        headers: { 'Cache-Control': 'no-store' },
        body: portrait,
      }),
    )
    await dialog.getByRole('button', { name: '重试', exact: true }).click()
    await expect(dialog.getByRole('img', { name, exact: true })).toBeVisible()
    expect((await readHero()).content).toBe('Preserve lore text')
    const persisted = JSON.parse(
      await readFile(path.join(book.workspace, 'setting/lore/items.json'), 'utf8'),
    )
    expect(persisted.assets.every((asset: { path?: string }) => !asset.path)).toBe(true)
  })
}
