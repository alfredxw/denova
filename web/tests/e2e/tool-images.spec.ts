import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { createRequire } from 'node:module'
import { unlink, writeFile } from 'node:fs/promises'
import path from 'node:path'

test('reads local images through tools and restores captured pixels in Writing and Game', async ({ page, request }, testInfo) => {
  const sharp = createRequire(import.meta.url)('sharp') as typeof import('sharp').default
  const image = await sharp({ create: { width: 96, height: 64, channels: 3, background: '#165cbd' } }).png().toBuffer()
  const book = await createAndOpenBook(request, 'Tool Image E2E Book')
  await createStartedStory(request, 'Tool Image E2E Story')
  const source = path.join(book.workspace, 'e2e-tool-image.png')
  await page.goto('/')

  for (const surface of ['writing', 'game'] as const) {
    await writeFile(source, image)
    const settings = await request.patch('/api/settings', { data: { layer: 'user', changes: { theme: surface === 'writing' ? 'dark' : 'light' } } })
    expect(settings.ok()).toBe(true)
    await page.reload()
    const open = async () => {
      if (surface === 'writing') return openWritingAgent(page)
      await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
      const composer = page.getByPlaceholder(/你要做什么/)
      await expect(composer).toBeVisible()
      return composer
    }
    let composer = await open()
    const reply = surface === 'writing' ? 'Tool image reached the model.' : '工具读取的图片已呈现，旧车站地图上的路线清晰可见。'
    await submitAgentChatMessage(page, composer, 'Read e2e-tool-image.png and describe it. E2E_TOOL_IMAGE_READ')
    await expect(page.getByText(reply, { exact: true })).toHaveCount(1)
    await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)

    // No image was uploaded. The next turn can only see the tool's snapshot.
    await unlink(source)
    await page.reload()
    composer = await open()
    await submitAgentChatMessage(page, composer, 'Continue using the previously read image. E2E_TOOL_IMAGE_READ')
    await expect(page.getByText(reply, { exact: true })).toHaveCount(2)
    await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
    await page.screenshot({ path: testInfo.outputPath(`${surface}-tool-image.png`) })
  }
})
