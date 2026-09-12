import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const product of ['writing', 'game'] as const) {
  test(`${product} pauses, reloads without execution, and continues the original task`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, `Pause ${product}`)
    const story = product === 'game' ? await createStartedStory(request, 'Pause one original turn') : undefined
    const marker = story ? 'E2E_GAME_FOLLOW_UP_DELAY' : 'E2E_DELAYED_AGENT_REPLY'
    const initialRequests = (await getModelStatus(request)).request_counts[marker] ?? 0
    let activeURL = ''
    page.on('request', value => {
      const url = new URL(value.url())
      const target = story ? '/api/interactive/chat/active' : `/api/projects/${book.projectId}/agent-chat/chat/active`
      if (value.method() === 'GET' && url.pathname === target) activeURL = value.url()
    })
    await page.goto('/')
    const composer = story ? page.getByPlaceholder(/你要做什么/) : await openWritingAgent(page)
    if (story) await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    const original = `Preserve this original input. ${marker}`
    await composer.fill(original)
    await composer.press('Enter')
    try {
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      await expect.poll(() => activeURL).not.toBe('')
      const readActive = async () => (await request.get(activeURL)).json()
      const running = await readActive()
      expect(running.active_operation_id).toBeTruthy()
      await page.getByRole('button', { name: '暂停任务', exact: true }).click()
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      await expect.poll(async () => (await readActive()).phase).toBe('suspended')
      // The fake provider retains disconnected requests until released. Releasing
      // that abandoned response must not allow a paused runtime to commit it.
      await releaseDelayedRequest(request, marker)
      await page.reload()
      if (story) await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
      else await openWritingAgent(page)
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      const paused = await readActive()
      expect(paused.active_operation_id).toBe(running.active_operation_id)
      expect(paused.phase).toBe('suspended')
      expect((await getModelStatus(request)).request_counts[marker]).toBe(initialRequests + 1)
      await page.getByRole('button', { name: '继续任务', exact: true }).click()
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      expect((await readActive()).active_operation_id).toBe(running.active_operation_id)
      await releaseDelayedRequest(request, marker)
      const output = story ? '石门缓缓开启，暖色灯光照亮了前方的旧车站。' : 'Recovered response completed exactly once.'
      await expect(page.getByText(output, { exact: true })).toHaveCount(1)
      if (story) {
        await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns).toHaveLength(2)
        expect((await getStorySnapshot(request, story.id)).turns[1]?.user).toBe(original)
      }
      await page.reload()
      if (story) await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
      else await openWritingAgent(page)
      await expect(page.getByText(output, { exact: true })).toHaveCount(1)
      await expect(page.getByText(original, { exact: true })).toHaveCount(1)
    } finally {
      await releaseDelayedRequest(request, marker)
    }
  })
}
