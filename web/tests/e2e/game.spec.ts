import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStory, getStoryBranches, getStorySnapshot } from '../support/api'
import { allowGameRegeneration, getModelStatus } from '../support/model'

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
    return (await getStorySnapshot(request, story.id)).turns
  }).toContainEqual(expect.objectContaining({ user: '推开石门', narrative: expect.stringContaining('石门缓缓开启') }))

  await page.reload()
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  await expect(page.getByText('石门缓缓开启，暖色灯光照亮了前方的旧车站。')).toHaveCount(1)
  await page.getByRole('button', { name: '获取行动选择' }).click()
  await page.getByText('走进旧车站', { exact: true }).click()
  await composer.press('Enter')

  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(2)
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns[1]?.user).toBe('走进旧车站')
})

test('creates and switches to a branch from a persisted Game turn', async ({ page, request }) => {
  await createAndOpenBook(request, 'Game Branch E2E Book')
  const story = await createStory(request, 'Game Branch E2E Story')

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await composer.fill('推开石门')
  await composer.press('Enter')
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(1)

  await page.getByRole('button', { name: '从此处创建分支' }).last().click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('剧情线名称').fill('E2E 支线')
  await dialog.getByRole('button', { name: '创建并切换', exact: true }).click()

  await expect.poll(async () => getStoryBranches(request, story.id)).toContainEqual(
    expect.objectContaining({ title: 'E2E 支线', current: true }),
  )
})

test('preserves the settled turn after a failed regeneration and replaces it on retry', async ({ page, request }) => {
  await createAndOpenBook(request, 'Game Regeneration E2E Book')
  const story = await createStory(request, 'Game Regeneration E2E Story')

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await composer.fill('聆听旧车站的广播 E2E_GAME_REGENERATE_FAILURE')
  await composer.press('Enter')
  await expect(page.getByText('第一次生成的钟声从旧车站深处传来。', { exact: true })).toBeVisible()
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(1)

  await page.getByRole('button', { name: '重新生成这一轮' }).last().click()
  await expect.poll(async () => (await getModelStatus(request)).game_regeneration_failure_requests, { timeout: 20_000 })
    .toBeGreaterThan(0)
  await expect(page.getByRole('alert')).toBeVisible({ timeout: 20_000 })
  await expect(composer).toBeEnabled()
  const failedSnapshot = await getStorySnapshot(request, story.id)
  expect(failedSnapshot.turns).toHaveLength(1)
  expect(failedSnapshot.turns[0]?.narrative).toContain('第一次生成的钟声')

  await allowGameRegeneration(request)
  await page.getByRole('button', { name: '重新生成这一轮' }).last().click()
  await expect(page.getByText('重试后，月台广播给出了全新的撤离路线。', { exact: true })).toBeVisible()
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toEqual([
    expect.objectContaining({
      user: '聆听旧车站的广播 E2E_GAME_REGENERATE_FAILURE',
      narrative: expect.stringContaining('重试后，月台广播给出了全新的撤离路线'),
    }),
  ])
})
