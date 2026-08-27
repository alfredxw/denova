import { expect, test } from '@playwright/test'
import { createAndOpenBook, createStory } from '../support/api'

test('submits, streams, and persists a complete Game turn', async ({ page, request }) => {
  await createAndOpenBook(request, 'Game E2E Book')
  const story = await createStory(request, 'Game E2E Story')

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  await composer.fill('推开石门')
  await composer.press('Enter')

  await expect(page.getByText('石门缓缓开启，暖色灯光照亮了前方的旧车站。')).toBeVisible()
  await page.getByRole('button', { name: '获取行动选择' }).click()
  await expect(page.getByText('走进旧车站', { exact: true })).toBeVisible()
  await expect(page.getByText('留在门外观察', { exact: true })).toBeVisible()

  await expect.poll(async () => {
    const response = await request.get(`/api/interactive/stories/${encodeURIComponent(story.id)}/snapshot?branch=main`)
    if (!response.ok()) return []
    const snapshot = await response.json() as { turns?: Array<{ user?: string; narrative?: string }> }
    return snapshot.turns ?? []
  }).toContainEqual(expect.objectContaining({ user: '推开石门', narrative: expect.stringContaining('石门缓缓开启') }))
})
