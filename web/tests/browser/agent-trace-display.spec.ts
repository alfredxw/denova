import type { UIMessageChunk } from 'ai'
import { expect, test, type Page } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'

const runId = 'trace-display-run'
const thinkingId = 'trace-thinking'
const reasoning = '**Current thought.**\n\n' + 'Inspect the surrounding passage before changing it. '.repeat(16)
const sse = (chunks: UIMessageChunk[]) => chunks.map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join('')
const gameEvent = (type: string, data: Record<string, unknown>) => `event: ${type}\ndata: ${JSON.stringify({ run_id: runId, ...data })}\n\n`

for (const [product, theme, language] of [['writing', 'dark', 'zh-CN'], ['game', 'light', 'en-US']] as const) {
  test(`${product} persists process display and respects manual disclosures in ${theme}`, async ({ page, request }) => {
    test.setTimeout(90_000)
    await createAndOpenBook(request, `Trace display ${product}`)
    const story = product === 'game' ? await createStartedStory(request, 'Trace display') : undefined
    const settings = await (await request.get('/api/settings')).json()
    expect((await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: settings.revisions.user,
      changes: { theme, language, motion_intensity: 'full', agent_trace_display: 'compact' },
    } })).ok()).toBe(true)
    await page.addInitScript(() => {
      const originalFetch = window.fetch.bind(window)
      window.fetch = async (...args) => {
        const response = await originalFetch(...args)
        if (!response.headers.has('x-test-trace-display')) return response
        const bytes = new Uint8Array(await response.arrayBuffer())
        let listener: EventListener
        return new Response(new ReadableStream({
          start(controller) {
            controller.enqueue(bytes)
            listener = event => controller.enqueue(new TextEncoder().encode((event as CustomEvent<string>).detail))
            window.addEventListener('test-trace-display', listener)
          },
          cancel() { window.removeEventListener('test-trace-display', listener) },
        }), { headers: { 'content-type': 'text/event-stream' } })
      }
    })
    if (product === 'writing') {
      await page.route('**/session/messages?*', route => route.fulfill({ json: { messages: [], page: { has_more: false, total: 0 } } }))
      await page.route('**/chat/active?*', route => route.fulfill({ json: {
        active: true, status: 'running', phase: 'running', task_id: 'trace-task', active_operation_id: runId, stream_attached: true,
      } }))
      await page.route('**/chat/stream?*', route => route.fulfill({
        headers: { 'content-type': 'text/event-stream', 'x-vercel-ai-ui-message-stream': 'v1', 'x-test-trace-display': 'true' },
        body: sse([
          { type: 'start', messageId: runId, messageMetadata: { run_id: runId } },
          { type: 'reasoning-start', id: thinkingId, providerMetadata: { agent: { display_segment_id: thinkingId } } },
          { type: 'reasoning-delta', id: thinkingId, delta: reasoning },
        ]),
      }))
    } else {
      await page.route('**/api/interactive/chat', route => route.fulfill({
        contentType: 'text/event-stream', headers: { 'x-test-trace-display': 'true' },
        body: gameEvent('agent_cycle_started', { id: 'trace-output', command_id: 'trace-command', operation_id: runId, cycle: 1, delivery: 'start_turn', message: 'Inspect the gate' })
          + gameEvent('thinking', { content: reasoning, display_segment_id: thinkingId }),
      }))
    }
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/')
    const sidebar = page.getByLabel(/工作台侧边栏|Workbench sidebar/)
    const openSettings = () => sidebar.getByRole('button', { name: /^(设置|Settings)$/ }).click()
    const preference = page.getByRole('combobox', { name: /Agent 过程展示|Agent process display/ })
    await openSettings()
    await expect(preference).toHaveText(language === 'zh-CN' ? '紧凑' : 'Compact')
    await preference.click()
    await page.getByRole('option', { name: /^(实时展开|Live)$/ }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_trace_display).toBe('live')
    await page.reload()
    await expect(preference).toBeVisible()
    await expect(preference).toHaveText(language === 'zh-CN' ? '实时展开' : 'Live')
    await preference.click()
    await page.screenshot({ path: test.info().outputPath(`trace-setting-${theme}-1440.png`) })
    await page.keyboard.press('Escape')
    await page.setViewportSize({ width: 390, height: 900 })
    await expect(preference).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`trace-setting-${theme}-390.png`) })
    await page.setViewportSize({ width: 1440, height: 900 })
    const openConversation = async () => {
      if (product === 'writing') await openWritingAgent(page)
      else await sidebar.getByRole('button', { name: 'Game', exact: true }).click()
    }
    await openConversation()
    if (story) {
      const snapshot = await (await request.get(`/api/interactive/stories/${story.id}/snapshot?branch=main`)).json()
      await expect(page.getByTestId('story-stage-card').getByText(snapshot.current_turn.narrative, { exact: true })).toBeVisible()
      await page.setViewportSize({ width: 390, height: 900 })
      await page.getByPlaceholder(/What do you do/).fill('Inspect the gate')
      await page.locator('[data-action="send"]').filter({ visible: true }).click()
    }
    const thinking = page.locator('[data-nova-scroll-lock="thinking"]').filter({ visible: true })
    await expect(thinking).toContainText('Current thought.')

    const startTool = (name: string, id: string, input: Record<string, unknown>) => product === 'writing'
      ? sse([
        { type: 'tool-input-start', toolCallId: id, toolName: name },
        { type: 'tool-input-delta', toolCallId: id, inputTextDelta: JSON.stringify(input) },
      ])
      : gameEvent('tool_call', { id, name, args: JSON.stringify(input), display_segment_id: id })
    const finishTool = (name: string, id: string, input: Record<string, unknown>) => product === 'writing'
      ? sse([
        { type: 'tool-input-available', toolCallId: id, toolName: name, input },
        { type: 'tool-output-available', toolCallId: id, output: 'Completed' },
      ])
      : gameEvent('tool_result', { id, name, content: 'Completed', display_segment_id: id })
    const toolRow = (id: string) => page.locator(`[data-nova-chat-row-key*="${id}"]`).filter({ has: page.locator('[data-nova-tool-header]') })
    const writeInput = { path: 'chapters/trace.md', content: 'A quiet path leads to the gate.\n'.repeat(12) }
    await emit(page, (product === 'writing' ? sse([{ type: 'reasoning-end', id: thinkingId }]) : '') + startTool('write', 'trace-write', writeInput))
    const write = toolRow('trace-write')
    await expect(write.locator('[data-nova-tool-header]')).toHaveAttribute('aria-expanded', 'true')
    await expect(write.locator('[data-nova-tool-input-stream]')).toContainText(writeInput.path)
    await expect(thinking).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`trace-live-${theme}.png`) })

    // Verify the actual shrinking panel, not just the final closed attribute.
    const frames = await page.evaluate(detail => new Promise<number[]>(resolve => {
      const heights: number[] = []
      const started = performance.now()
      const panel = document.querySelector('[data-nova-chat-row-key*="trace-write"] .nova-tool-content')!
      const initialHeight = panel.getBoundingClientRect().height
      const sample = () => {
        heights.push(panel.getBoundingClientRect().height)
        if (!panel.isConnected || performance.now() - started > 800) return resolve([initialHeight, ...heights])
        requestAnimationFrame(sample)
      }
      requestAnimationFrame(sample)
      window.dispatchEvent(new CustomEvent('test-trace-display', { detail }))
    }), finishTool('write', 'trace-write', writeInput))
    await test.info().attach('auto-collapse-heights', { body: JSON.stringify(frames), contentType: 'application/json' })
    expect(frames.some(height => height > 1 && height < frames[0] - 1)).toBe(true)
    await expect(write.locator('[data-nova-tool-header]')).toHaveAttribute('aria-expanded', 'false')
    await write.locator('[data-nova-tool-header]').click()
    await expect(write.locator('[data-nova-tool-detail="write"]')).toBeVisible()
    await write.locator('[data-nova-tool-header]').click()

    const editInput = { path: 'chapters/trace.md', edits: [{ old_string: 'quiet', new_string: 'winding' }] }
    await emit(page, startTool('edit', 'trace-edit', editInput))
    const edit = toolRow('trace-edit')
    const editHeader = edit.locator('[data-nova-tool-header]')
    await expect(editHeader).toHaveAttribute('aria-expanded', 'true')
    await editHeader.click()
    await emit(page, product === 'writing'
      ? sse([{ type: 'tool-input-delta', toolCallId: 'trace-edit', inputTextDelta: ' ' }])
      : gameEvent('tool_args_delta', { id: 'trace-edit', name: 'edit', delta: ' ' }))
    await expect(editHeader).toHaveAttribute('aria-expanded', 'false')
    await editHeader.click()
    await emit(page, finishTool('edit', 'trace-edit', editInput))
    await expect(editHeader).toHaveAttribute('aria-expanded', 'true')
    await expect(edit.locator('[data-nova-tool-detail="edit"]')).toBeVisible()
    await emit(page, startTool('bash', 'trace-bash', { command: 'echo ready' }))
    await expect(toolRow('trace-bash').locator('[data-nova-tool-header]')).toHaveAttribute('aria-expanded', 'false')
    await emit(page, finishTool('bash', 'trace-bash', { command: 'echo ready' }))

    await page.setViewportSize({ width: 1440, height: 900 })
    await openSettings()
    await preference.click()
    await page.getByRole('option', { name: /^(紧凑|Compact)$/ }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_trace_display).toBe('compact')
    await openConversation()
    await expect(editHeader).toHaveAttribute('aria-expanded', 'true')
    await emit(page, startTool('write', 'compact-write', writeInput))
    await expect(toolRow('compact-write').locator('[data-nova-tool-header]')).toHaveAttribute('aria-expanded', 'false')
    await expect(page.locator('[data-nova-tool-input-stream]')).toHaveCount(0)
  })
}

async function emit(page: Page, detail: string) {
  await page.evaluate(detail => window.dispatchEvent(new CustomEvent('test-trace-display', { detail })), detail)
}
