import { createServer } from 'node:http'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import type { LoreItem } from '../../src/lib/api'

test('generates the first image directly from the empty material state', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Lore empty generation')
  const route = `/api/projects/${book.projectId}/book/lore/items`
  const created = await request.post(route, {
    data: { id: 'hero', name: '无素材角色', type: 'character', content: '保留正文', enabled: true },
  })
  expect(created.ok()).toBe(true)
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '资料库', exact: true }).click()
  await page.getByRole('tab', { name: '素材 (0)', exact: true }).click()
  await page.getByRole('button', { name: '生成图片', exact: true }).click({ timeout: 5000 })
  const dialog = page.getByRole('dialog', { name: '生成图片', exact: true })
  await dialog.getByRole('combobox', { name: '提示词编写方式', exact: true }).click()
  await page.getByRole('option', { name: '自定义最终提示词', exact: true }).click()
  await dialog.getByLabel('最终提示词', { exact: true }).fill('Draw a character portrait.')
  await dialog.getByRole('button', { name: '生成图片', exact: true }).click()
  await expect(page.getByRole('tab', { name: '素材 (1)', exact: true })).toBeVisible()
  const item = (await (await request.get(route)).json()).items[0] as LoreItem
  expect(item.content).toBe('保留正文')
  expect(item.image).toBeUndefined()
  expect(item.resolved_materials?.[0].source.kind).toBe('generated')
})

for (const theme of ['dark', 'light']) {
  test(`generates speech with configured models and preserves materials on provider failure in ${theme}`, async ({ page, request, browserDiagnostics }, testInfo) => {
    const inputs: Array<{ input: string; model: string; voice: string }> = []
    const frame = Buffer.alloc(417)
    frame.set([0xff, 0xfb, 0x90, 0xc0])
    const mp3 = Buffer.concat(Array.from({ length: 100 }, () => frame))
    let fail = false
    const server = createServer(async (req, res) => {
      const chunks = []
      for await (const chunk of req) chunks.push(chunk)
      inputs.push(JSON.parse(Buffer.concat(chunks).toString()))
      res.writeHead(fail ? 401 : 200, { 'Content-Type': 'audio/mpeg' })
      res.end(fail ? 'private provider diagnostic' : mp3)
    })
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
    const address = server.address()
    if (!address || typeof address === 'string') throw new Error('Speech fixture did not bind')
    const original = await (await request.get('/api/settings')).json()
    const route = `/api/projects/${(await createAndOpenBook(request, 'Lore voice generation')).projectId}/book/lore/items`
    try {
      const configured = await request.patch('/api/settings', { data: {
        layer: 'user', base_revision: original.revisions.user,
        changes: { theme, speech: { endpoint: `http://127.0.0.1:${address.port}/speech`, model: 'lore-tts', voice: 'narrator', api_key: '' } },
      } })
      expect(configured.ok(), await configured.text()).toBe(true)
      await request.post(route, { data: { id: 'hero', name: '声音角色', type: 'character', content: '保留角色设定', enabled: true } })
      await page.goto('/')
      await page.getByLabel('工作台侧边栏').getByRole('button', { name: '资料库', exact: true }).click()
      await page.getByRole('tab', { name: '素材 (0)', exact: true }).click()
      await expect(page.getByRole('button', { name: '生成图片', exact: true })).toBeVisible()
      await page.screenshot({ path: testInfo.outputPath(`empty-generation-${theme}-wide.png`), animations: 'disabled' })
      await page.setViewportSize({ width: 390, height: 844 })
      await page.screenshot({ path: testInfo.outputPath(`empty-generation-${theme}-390.png`), animations: 'disabled' })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.getByRole('button', { name: '生成语音', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: '生成语音', exact: true })
      await dialog.getByLabel('素材名称', { exact: true }).fill('角色开场白')
      await dialog.getByLabel('语音文本', { exact: true }).fill('欢迎来到旧车站。')
      await page.screenshot({ path: testInfo.outputPath(`speech-generation-${theme}-390.png`), animations: 'disabled' })
      await dialog.getByRole('button', { name: '生成语音', exact: true }).click()
      await expect(dialog).toBeHidden()
      await expect(page.getByRole('tab', { name: '素材 (1)', exact: true })).toBeVisible()
      expect(inputs).toEqual([{ model: 'lore-tts', voice: 'narrator', input: '欢迎来到旧车站。', response_format: 'mp3' }])
      const items = async () => (await (await request.get(route)).json()).items as LoreItem[]
      const item = (await items())[0]
      expect(item.content).toBe('保留角色设定')
      expect(item.image).toBeUndefined()
      expect(item.resolved_materials?.[0]).toMatchObject({ name: '角色开场白', description: '欢迎来到旧车站。', mime_type: 'audio/mpeg', source: { kind: 'generated' } })
      await page.reload()
      await page.getByRole('tab', { name: '素材 (1)', exact: true }).click()
      const audio = page.getByRole('tabpanel').locator('audio')
      await audio.evaluate((element: HTMLAudioElement) => element.play())
      await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false)

      fail = true
      browserDiagnostics.allow(/console\.error: Failed to load resource:.*502.*\/speech\/generate/)
      browserDiagnostics.allow(/http\.5xx: POST .*\/speech\/generate returned 502/)
      await page.getByRole('button', { name: '添加素材', exact: true }).click()
      await page.getByRole('menuitem', { name: '生成语音', exact: true }).click()
      await dialog.getByLabel('语音文本', { exact: true }).fill('失败的重试。')
      await dialog.getByRole('button', { name: '生成语音', exact: true }).click()
      await expect(page.getByText(/语音服务鉴权失败，请检查 API 密钥。/)).toBeVisible()
      expect(await items()).toEqual([item])
      await dialog.getByRole('button', { name: '取消', exact: true }).click()

      const latest = await (await request.get('/api/settings')).json()
      await request.patch('/api/settings', { data: { layer: 'user', base_revision: latest.revisions.user, changes: { speech: { endpoint: '', model: '', voice: '', api_key: '' }, image_api_endpoints: [], image_api_profiles: [], default_image_api_profile_id: '' } } })
      await page.reload()
      await page.getByRole('tab', { name: '素材 (1)', exact: true }).click()
      await page.getByRole('button', { name: '添加素材', exact: true }).click()
      await expect(page.getByRole('menuitem', { name: '生成语音', exact: true })).toHaveCount(0)
      await expect(page.getByRole('menuitem', { name: '生成图片', exact: true })).toHaveCount(0)
    } finally {
      const latest = await (await request.get('/api/settings')).json()
      await request.patch('/api/settings', { data: { layer: 'user', base_revision: latest.revisions.user, changes: {
        speech: original.user.speech ?? null,
        image_api_endpoints: original.user.image_api_endpoints ?? null,
        image_api_profiles: original.user.image_api_profiles ?? null,
        default_image_api_profile_id: original.user.default_image_api_profile_id ?? null,
      } } })
      server.closeAllConnections()
      await new Promise<void>(resolve => server.close(() => resolve()))
    }
  })
}
