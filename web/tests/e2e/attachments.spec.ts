import { expect, test, type Page } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'
import { createRequire } from 'node:module'

test('uploads, previews, sends, and restores image attachments in Writing and Game', async ({ page, request }) => {
  const sharp = createRequire(import.meta.url)('sharp') as typeof import('sharp').default
  // Identical pixels can have very different lossless encodings. This valid
  // image used to be rejected as >590K text tokens before reaching the model.
  const image = await sharp({ create: { width: 768, height: 768, channels: 3, background: '#165cbd' } })
    .png({ compressionLevel: 0 }).toBuffer()
  expect(image.length).toBeGreaterThan(1_200_000)
  await createAndOpenBook(request, 'Attachment E2E Book')
  await createStartedStory(request, 'Attachment E2E Story')
  await page.goto('/')

  let composer = await openWritingAgent(page)
  await attachImage(page, 'right', 'writing-e2e.png', image)
  await expect(page.getByTestId('right').getByRole('button', { name: '预览 writing-e2e.png' })).toBeVisible()
  await composer.fill('Inspect this image. E2E_WRITING_IMAGE_ATTACHMENT')
  await composer.press('Enter')
  await expect(page.getByTestId('right').getByText('Writing image attachment reached the model.', { exact: true })).toBeVisible()
  await expect(page.getByTestId('right').getByTestId('sent-message-attachments')).toHaveCount(1)
  await previewImage(page, 'right', 'writing-e2e.png')

  composer = await openGame(page)
  await attachImage(page, 'story-stage', 'game-e2e.png', image)
  await expect(page.getByTestId('story-stage').getByRole('button', { name: '预览 game-e2e.png' })).toBeVisible()
  await composer.fill('Follow the image signal. E2E_GAME_IMAGE_ATTACHMENT')
  await composer.press('Enter')
  await expect(page.getByText('图像中的蓝色信标亮起，旧车站的侧门随之打开。', { exact: true })).toBeVisible()
  await expect(page.getByTestId('story-stage').getByTestId('sent-message-attachments')).toHaveCount(1)
  await previewImage(page, 'story-stage', 'game-e2e.png')

  await page.reload()
  composer = await openGame(page)
  await expect(page.getByTestId('story-stage').getByTestId('sent-message-attachments')).toHaveCount(1)
  await previewImage(page, 'story-stage', 'game-e2e.png')

  await composer.fill('Continue using the original image. E2E_GAME_IMAGE_ATTACHMENT')
  await composer.press('Enter')
  await expect(page.getByText('图像中的蓝色信标亮起，旧车站的侧门随之打开。', { exact: true })).toHaveCount(2)

  composer = await openWritingAgent(page)
  await expect(page.getByTestId('right').getByText('Writing image attachment reached the model.', { exact: true })).toHaveCount(1)
  await expect(page.getByTestId('right').getByTestId('sent-message-attachments')).toHaveCount(1)
  await previewImage(page, 'right', 'writing-e2e.png')
  await composer.fill('Continue using the original image. E2E_WRITING_IMAGE_ATTACHMENT')
  await composer.press('Enter')
  await expect(page.getByTestId('right').getByText('Writing image attachment reached the model.', { exact: true })).toHaveCount(2)
})

async function openGame(page: Page) {
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  return composer
}

async function attachImage(page: Page, surface: 'right' | 'story-stage', name: string, image: Buffer) {
  await page.getByTestId(surface).getByLabel('添加文件', { exact: true }).setInputFiles({
    name,
    mimeType: 'image/png',
    buffer: image,
  })
}

async function previewImage(page: Page, surface: 'right' | 'story-stage', name: string) {
  await page.getByTestId(surface).getByRole('button', { name: `预览 ${name}` }).click()
  const dialog = page.getByRole('dialog', { name })
  await expect(dialog.getByRole('img', { name })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
}
