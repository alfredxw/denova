import { createServer } from 'node:http'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

// Valid MPEG-1 Layer III mono silence: exercise browser decoding and the real
// backend proxy without a paid provider or any user's credentials/content.
const frame = Buffer.alloc(417)
frame.set([0xff, 0xfb, 0x90, 0xc0])
const mp3 = Buffer.concat(Array.from({ length: 180 }, () => frame))

for (const theme of ['dark', 'light']) {
  test(`game speech settings, filtering and playback in ${theme}`, async ({ page, request }) => {
    test.setTimeout(120_000)
    const inputs: Array<{ input: string; model: string; voice: string; response_format: string }> = []
    let delayNext = false
    let providerCancelled = false
    const server = createServer(async (req, res) => {
      const chunks = []
      for await (const chunk of req) chunks.push(chunk)
      inputs.push(JSON.parse(Buffer.concat(chunks).toString()))
      if (delayNext) { delayNext = false; res.on('close', () => { providerCancelled = true }); return }
      res.writeHead(200, { 'Content-Type': 'audio/mpeg' })
      res.end(mp3)
    })
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
    const address = server.address()
    if (!address || typeof address === 'string') throw new Error('Speech fixture did not bind')
    const endpoint = `http://127.0.0.1:${address.port}/custom/audio/speech`
    const original = await (await request.get('/api/settings', { maxRetries: 2 })).json()
    try {
      await request.patch('/api/settings', { data: { layer: 'user', base_revision: original.revisions.user, changes: { speech: null, image_api_endpoints: [], image_api_profiles: [], default_image_api_profile_id: null, theme, language: 'zh-CN' } } })
      await createAndOpenBook(request, `Speech ${theme}`)
      const story = await createStartedStory(request, '朗读验收故事')
      const snapshot = await (await request.get(`/api/interactive/stories/${story.id}/snapshot?branch=main`)).json()
      const turn = snapshot.turns[0]
      const narrative = '*她轻声说“动作内对白。”* 她说：“你好。”\n\n他回答：「明天见。」'
      const edited = await request.patch(`/api/interactive/stories/${story.id}/turns/${turn.id}/narrative`, { data: { branch_id: 'main', narrative, expected_narrative: turn.narrative } })
      expect(edited.ok(), await edited.text()).toBe(true)
      await page.goto('/')
      const sidebar = page.getByLabel('工作台侧边栏')
      await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
      await page.getByRole('tab', { name: '控制', exact: true }).click()
      const titles = await page.locator('.director-console__scroll h3').allTextContents()
      expect(titles.indexOf('语音朗读')).toBe(titles.indexOf('互动图像') + 1)
      await expect(page.getByRole('switch', { name: '自动朗读新正文', exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '朗读正文', exact: true })).toHaveCount(0)
      await expect(page.getByRole('switch', { name: '自动生成', exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '生成互动图像', exact: true })).toHaveCount(0)
      await page.getByRole('button', { name: '配置图像', exact: true }).scrollIntoViewIfNeeded()
      await page.screenshot({ path: test.info().outputPath(`unconfigured-media-${theme}.png`) })
      await page.getByRole('button', { name: '配置图像', exact: true }).click()
      await expect(page.getByRole('button', { name: '公共配置图像模型', exact: true })).toBeVisible()
      await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
      const beforeImage = await (await request.get('/api/settings')).json()
      const configuredImage = await request.patch('/api/settings', { data: {
        layer: 'user', base_revision: beforeImage.revisions.user,
        changes: {
          default_image_api_profile_id: 'local-image',
          image_api_endpoints: [{ id: 'local-image', provider: 'custom', base_url: 'http://127.0.0.1:8000/v1', protocol: 'openai-images' }],
          image_api_profiles: [{ id: 'local-image', endpoint_id: 'local-image', model: 'test-image' }],
        },
      } })
      expect(configuredImage.ok(), await configuredImage.text()).toBe(true)
      await page.reload()
      await expect(page.getByRole('switch', { name: '自动生成', exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '生成互动图像', exact: true })).toHaveCount(1)
      await page.getByRole('button', { name: '配置语音', exact: true }).click()
      await page.getByLabel('语音接口地址', { exact: true }).fill(endpoint)
      await page.getByLabel('语音模型 ID', { exact: true }).fill('custom-tts')
      await page.getByLabel('声线 ID', { exact: true }).fill('custom-voice')
      await expect.poll(async () => (await (await request.get('/api/settings', { maxRetries: 2 })).json()).user.speech?.voice).toBe('custom-voice')
      await page.getByRole('button', { name: '试听', exact: true }).click()
      const player = page.getByTestId('speech-playback').filter({ visible: true })
      await expect(player).toContainText('正在朗读')
      await player.getByRole('button', { name: '暂停朗读', exact: true }).click()
      await expect(player).toContainText('朗读已暂停')
      expect(inputs).toHaveLength(1)
      expect(inputs[0]).toMatchObject({ model: 'custom-tts', voice: 'custom-voice', response_format: 'mp3' })
      await page.screenshot({ path: test.info().outputPath(`speech-settings-${theme}.png`) })
      await page.setViewportSize({ width: 390, height: 844 })
      await page.getByLabel('语音接口地址', { exact: true }).scrollIntoViewIfNeeded()
      await page.screenshot({ path: test.info().outputPath(`speech-settings-narrow-${theme}.png`) })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.setViewportSize({ width: 1280, height: 900 })
      await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
      await expect(player).toHaveCount(0)
      await page.getByRole('button', { name: '朗读正文', exact: true }).first().click()
      await expect(player).toContainText('正在朗读')
      expect(inputs[1].input).toContain('她轻声说')
      await page.getByRole('combobox', { name: '朗读内容', exact: true }).click()
      await page.getByRole('option', { name: '仅对话', exact: true }).click()
      await page.getByRole('switch', { name: '跳过星号动作描写', exact: true }).click()
      await expect(page.getByRole('switch', { name: '跳过星号动作描写', exact: true })).toBeChecked()
      await expect.poll(async () => {
        const index = await (await request.get('/api/interactive/stories')).json()
        return index.stories.find((item: { id: string }) => item.id === story.id)?.speech_settings
      }).toEqual({ auto_read: false, mode: 'quoted', ignore_asterisks: true })
      await page.getByRole('button', { name: '朗读正文', exact: true }).first().click()
      await expect(player).toContainText('正在朗读')
      expect(inputs.at(-1)?.input).toBe('你好。\n\n明天见。')
      await player.getByRole('button', { name: '暂停朗读', exact: true }).click()
      await page.getByRole('slider', { name: '语速', exact: true }).press('ArrowRight')
      await page.getByRole('slider', { name: '音量', exact: true }).press('ArrowLeft')
      await page.screenshot({ path: test.info().outputPath(`speech-game-${theme}.png`) })
      const beforeReload = inputs.length
      await page.reload()
      await expect(page.getByRole('button', { name: '朗读正文', exact: true }).first()).toBeVisible()
      await expect(player).toHaveCount(0)
      expect(inputs).toHaveLength(beforeReload)
      await page.getByRole('switch', { name: '自动朗读新正文', exact: true }).click()
      await expect(page.getByRole('switch', { name: '自动朗读新正文', exact: true })).toBeChecked()
      await page.getByRole('combobox', { name: '朗读内容', exact: true }).click()
      await page.getByRole('option', { name: '完整正文', exact: true }).click()
      await expect(page.getByRole('combobox', { name: '朗读内容', exact: true })).toHaveText('完整正文')
      await page.getByPlaceholder(/你要做什么/).fill('推开石门')
      await page.locator('[data-action="send"]').filter({ visible: true }).click()
      await expect(player).toContainText('正在朗读', { timeout: 30_000 })
      await expect.poll(() => inputs.length).toBe(beforeReload + 1)
      await sidebar.getByRole('button', { name: '写作', exact: true }).click()
      await expect(player).toHaveCount(0)
      await expect(page.getByRole('button', { name: '朗读正文', exact: true })).toHaveCount(0)
      await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
      delayNext = true
      await page.getByRole('button', { name: '朗读正文', exact: true }).first().click()
      await expect(player).toContainText('正在生成语音')
      await expect.poll(() => delayNext).toBe(false)
      await player.getByRole('button', { name: '停止朗读', exact: true }).click()
      await expect(player).toContainText('朗读已停止')
      await expect.poll(() => providerCancelled).toBe(true)
      if (theme === 'light') {
        await page.getByRole('button', { name: '配置语音', exact: true }).click()
        await page.getByLabel('声线 ID', { exact: true }).fill('custom-voice-with-a-very-long-name-'.repeat(3))
        await expect(page.getByText('所有更改均已保存', { exact: true })).toBeVisible()
        const localized = await (await request.get('/api/settings', { maxRetries: 2 })).json()
        await request.patch('/api/settings', { data: { layer: 'user', base_revision: localized.revisions.user, changes: { language: 'en-US' } } })
        await page.reload()
        await page.getByLabel('Speech endpoint URL', { exact: true }).scrollIntoViewIfNeeded()
        await expect(page.getByRole('button', { name: 'Preview voice', exact: true })).toBeVisible()
        await page.setViewportSize({ width: 390, height: 844 })
        await page.getByLabel('Speech endpoint URL', { exact: true }).scrollIntoViewIfNeeded()
        await page.screenshot({ path: test.info().outputPath('speech-settings-english-narrow.png') })
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      }
    } finally {
      const latest = await (await request.get('/api/settings', { maxRetries: 2 })).json()
      await request.patch('/api/settings', { data: { layer: 'user', base_revision: latest.revisions.user, changes: { speech: original.user.speech ?? null, image_api_endpoints: original.user.image_api_endpoints ?? null, image_api_profiles: original.user.image_api_profiles ?? null, default_image_api_profile_id: original.user.default_image_api_profile_id ?? null, theme: original.user.theme ?? null, language: original.user.language ?? null } } })
      server.closeAllConnections()
      await new Promise<void>(resolve => server.close(() => resolve()))
    }
  })
}
