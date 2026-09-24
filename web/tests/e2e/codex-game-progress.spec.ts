import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot } from '../support/api'
import { getModelStatus, modelControlURL, releaseDelayedRequest } from '../support/model'

test.skip(!process.env.DENOVA_TEST_CODEX_EXE, 'Installed Codex acceptance is opt-in')

for (const theme of ['dark', 'light'] as const) {
  test(`Codex Game shows waiting progress and accepts prose before submission in ${theme}`, async ({ page, request }) => {
    await createAndOpenBook(request, `Game progress ${theme}`)
    const story = await createStartedStory(request, 'Game progress')
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({ json: {
      ...settings, effective: { ...settings.effective, theme },
    } }))
    // Reproduce the reported journal: a settled older turn retains a running
    // tool card. It must never hide a later turn's waiting indicator.
    await page.route('**/api/interactive/stories/*/snapshot**', async route => {
      const response = await route.fetch()
      const snapshot = await response.json()
      snapshot.turns[0].display_events = [{
        id: 'previous-tool', role: 'tool_call', name: 'prepare_interactive_turn',
        status: 'running', run_id: snapshot.turns[0].run_id,
      }, { role: 'narrative' }]
      if (theme === 'light') snapshot.turns[0].display_events.splice(1, 0, {
        id: 'previous-tool', role: 'tool_result', name: 'prepare_interactive_turn',
        status: 'success', content: 'The previous check completed.',
      })
      await route.fulfill({ response, json: snapshot })
    })
    const width = theme === 'dark' ? 1440 : 390
    await page.setViewportSize({ width, height: 960 })
    await page.goto('/')
    if (width < 800) {
      await page.getByRole('button', { name: '导航菜单', exact: true }).click()
      await page.getByRole('dialog').getByRole('button', { name: '游戏', exact: true }).click()
    } else {
      await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    }
    const composer = page.getByPlaceholder(/你要做什么/)
    const marker = `E2E_GAME_TURN_ORDER_${theme.toUpperCase()}`
    await composer.fill(marker)
    const streamResponse = page.waitForResponse(response => response.url().endsWith('/api/interactive/chat') && response.request().method() === 'POST')
    await page.locator('[data-action="send"]').filter({ visible: true }).click()
    try {
      for (const gate of ['E2E_GAME_TURN_ORDER_START', 'E2E_GAME_TURN_ORDER_AFTER_TOOL']) {
        await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[gate] ?? 0).toBe(1)
        await expect(page.getByRole('status').filter({ has: page.locator('.shimmer') })).toBeVisible()
        if (theme === 'light') await expect(page.getByText(/^正在执行/)).toHaveCount(0)
        expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(1)
        await page.screenshot({ path: test.info().outputPath(`${gate}-${theme}.png`) })
        await releaseDelayedRequest(request, gate)
      }
      const prose = 'The gate opens after the guard leaves.'
      await expect(page.getByText(prose, { exact: true })).toBeVisible()
      await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
      const body = await (await streamResponse).text()
      expect(body.indexOf('event: chunk')).toBeGreaterThan(-1)
      expect(body.indexOf('event: tool_call')).toBeGreaterThan(body.indexOf('event: chunk'))
      const calls = await (await request.get(`${modelControlURL}/control/runtime-requests?marker=${marker}`)).json()
      const feedback = calls.flatMap((call: { input?: Array<{ type: string; call_id?: string; output?: string }> }) => call.input ?? [])
        .find((item: { type: string; call_id?: string }) => item.type === 'function_call_output' && item.call_id === 'call-early-submit')
      expect(feedback?.output).toContain('before')
      expect(feedback?.output).not.toContain('"ready": true')
      await page.reload()
      await expect(page.getByText(prose, { exact: true })).toBeVisible()
      expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(2)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`settled-${theme}.png`) })
    } finally {
      await releaseDelayedRequest(request, 'E2E_GAME_TURN_ORDER_START')
      await releaseDelayedRequest(request, 'E2E_GAME_TURN_ORDER_AFTER_TOOL')
    }
  })
}
