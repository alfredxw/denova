import type { UIMessageChunk } from 'ai'
import type { AgentUIMessage } from '../../src/lib/agent-ui'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'

for (const theme of ['dark', 'light']) {
  test(`thinking word animation follows explicit full motion in ${theme}`, async ({ page, request }) => {
    test.setTimeout(90_000)
    await createAndOpenBook(request, `Thinking motion ${theme}`)
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async route => {
      const response = await route.fetch()
      const body = await response.json()
      return route.fulfill({ response, json: { ...body, effective: { ...body.effective, theme } } })
    })
    await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: settings.revisions.user, changes: { motion_intensity: 'system' },
    } })
    await page.addInitScript(() => {
      const originalFetch = window.fetch.bind(window)
      window.fetch = async (...args) => {
        const response = await originalFetch(...args)
        if (!response.headers.has('x-test-thinking-motion')) return response
        const bytes = new Uint8Array(await response.arrayBuffer())
        let listener: EventListener
        return new Response(new ReadableStream({
          start(controller) {
            controller.enqueue(bytes)
            listener = event => {
              const detail = (event as CustomEvent<string>).detail
              controller.enqueue(new TextEncoder().encode(detail))
              if (detail.includes('data: [DONE]')) {
                window.removeEventListener('test-thinking-motion', listener)
                controller.close()
              }
            }
            window.addEventListener('test-thinking-motion', listener)
          },
          cancel() { window.removeEventListener('test-thinking-motion', listener) },
        }), { headers: { 'content-type': 'text/event-stream' } })
      }
    })
    const reasoning = '**Motion evidence.**\n\n' + 'Long thinking remains readable while tools are toggled. '.repeat(180)
    const chunks: UIMessageChunk[] = [
      { type: 'start', messageId: 'motion-run', messageMetadata: { run_id: 'motion-run' } },
      { type: 'tool-input-start', toolCallId: 'motion-tool', toolName: 'bash' },
      { type: 'tool-input-available', toolCallId: 'motion-tool', toolName: 'bash', input: { command: 'echo motion-probe' } },
      { type: 'tool-output-available', toolCallId: 'motion-tool', output: 'motion-probe' },
      { type: 'reasoning-start', id: 'motion-thinking', providerMetadata: { agent: { display_segment_id: 'motion-thinking' } } },
      { type: 'reasoning-delta', id: 'motion-thinking', delta: reasoning },
    ]
    const stream = (route: import('@playwright/test').Route) => route.fulfill({
      headers: { 'content-type': 'text/event-stream', 'x-vercel-ai-ui-message-stream': 'v1', 'x-test-thinking-motion': 'true' },
      body: chunks.map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join(''),
    })
    // Resume one deterministic Native display stream without submitting commands.
    await page.route('**/chat/stream?*', stream)
    let settled = false
    let history: AgentUIMessage[] = []
    await page.route('**/session/messages?*', route => route.fulfill({ json: {
      messages: history, page: { has_more: false, total: history.length },
    } }))
    await page.route('**/chat/active?*', route => route.fulfill({ json: settled ? { active: false } : {
      active: true, status: 'running', phase: 'running', task_id: 'motion-task', active_operation_id: 'motion-run', stream_attached: true,
    } }))
    await page.goto('/')
    await openWritingAgent(page)
    const thinking = page.locator('[data-nova-scroll-lock="thinking"]').filter({ visible: true })
    await expect(thinking).toHaveCount(0)
    await page.getByRole('button', { name: '展开思考', exact: true }).click()
    await expect(thinking.locator('[data-streamdown="strong"]')).toHaveText('Motion evidence.')
    await expect(thinking.locator('[data-sd-animate]')).toHaveCount(0)
    const tool = page.locator('[data-nova-tool-header]').filter({ visible: true })
    await tool.click()
    await expect(tool).toHaveAttribute('aria-expanded', 'true')
    await tool.click()
    await expect(tool).toHaveAttribute('aria-expanded', 'false')
    await thinking.press('PageUp')

    const sidebar = page.getByLabel('工作台侧边栏')
    for (const [option, value] of [['完整', 'full'], ['跟随系统', 'system']] as const) {
      await sidebar.getByRole('button', { name: '设置', exact: true }).click()
      const row = page.locator('.nova-settings-row').filter({ hasText: '动效强度' })
      await row.getByRole('combobox').click()
      await page.getByRole('option', { name: option, exact: true }).click()
      await expect(page.locator('html')).toHaveAttribute('data-nova-motion', value)
      await openWritingAgent(page)
      await expect(thinking).toBeVisible()
      if (value === 'full') await expect.poll(() => thinking.locator('[data-sd-animate]').count()).toBeGreaterThan(0)
      else await expect(thinking.locator('[data-sd-animate]')).toHaveCount(0)
      await expect(thinking.locator('[data-streamdown="strong"]')).toHaveText('Motion evidence.')
    }
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('test-thinking-motion', {
      detail: 'data: {"type":"reasoning-delta","id":"motion-thinking","delta":" Still streaming."}\n\n',
    })))
    await expect(thinking).toContainText('Still streaming.')
    await expect(thinking.locator('[data-sd-animate]')).toHaveCount(0)
    settled = true
    history = [{ id: 'motion-run', role: 'assistant', metadata: { run_id: 'motion-run' }, parts: [
      { type: 'dynamic-tool', toolCallId: 'motion-tool', toolName: 'bash', state: 'output-available', input: { command: 'echo motion-probe' }, output: 'motion-probe' },
      { type: 'reasoning', text: reasoning + ' Still streaming.', state: 'done', providerMetadata: { agent: { display_segment_id: 'motion-thinking' } } },
      { type: 'text', text: 'The result stays below the process.', state: 'done', providerMetadata: { agent: { display_segment_id: 'motion-final', display_phase: 'final' } } },
    ] }]
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('test-thinking-motion', {
      detail: [
        { type: 'reasoning-end', id: 'motion-thinking' },
        { type: 'text-start', id: 'motion-final', providerMetadata: { agent: { display_segment_id: 'motion-final', display_phase: 'final' } } },
        { type: 'text-delta', id: 'motion-final', delta: 'The result stays below the process.' },
        { type: 'text-end', id: 'motion-final' },
        { type: 'finish' },
      ].map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join('') + 'data: [DONE]\n\n',
    })))
    await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
    await expect(thinking).toBeVisible()
    const process = page.locator('[data-agent-execution-process]').filter({ visible: true }).getByRole('button')
    await expect(process).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByText('The result stays below the process.', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '收起思考', exact: true }).click()
    await expect(thinking).toHaveCount(0)
    await expect(process).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await page.screenshot({ path: test.info().outputPath(`thinking-motion-${theme}.png`) })
  })
}

test('collapsing a process keeps pending approvals and failed tools visible', async ({ page, request }) => {
  await createAndOpenBook(request, 'Process attention')
  await page.route('**/session/messages?*', route => route.fulfill({ json: {
    messages: [{ id: 'attention-run', role: 'assistant', metadata: { run_id: 'attention-run' }, parts: [
      { type: 'dynamic-tool', toolName: 'bash', toolCallId: 'done-call', state: 'output-available', input: { command: 'echo completed' }, output: 'Completed hidden operation' },
      { type: 'dynamic-tool', toolName: 'bash', toolCallId: 'failed-call', state: 'output-error', input: { command: 'echo failed' }, errorText: 'Visible failed operation' },
      { type: 'dynamic-tool', toolName: 'bash', toolCallId: 'approval-call', state: 'input-available', input: { command: 'echo approval' } },
      { type: 'data-agent-ask', id: 'pending-approval', data: {
        schema: 'ask.pending.v1', id: 'pending-approval', tool_call_id: 'approval-call', agent_kind: 'writer', kind: 'tool_approval', status: 'pending', questions: [],
        approval: { tool_name: 'bash', command: 'echo approval', mode: 'ask', risk: 'low', rule_id: 'command-review', args_hash: 'approval-args' },
      } },
    ] }], page: { has_more: false, total: 1 },
  } }))
  await page.goto('/')
  await openWritingAgent(page)
  const process = page.locator('[data-agent-execution-process]').getByRole('button')
  await expect(process).toHaveAttribute('aria-expanded', 'true')
  await process.click()
  await expect(process).toHaveAttribute('aria-expanded', 'false')
  await expect(page.locator('[data-tool-approval-panel]')).toBeVisible()
  await expect(page.locator('[data-nova-tool-header]')).toHaveCount(2)
  await expect(page.getByText('Visible failed operation', { exact: true })).toBeVisible()
  await expect(page.getByText('Completed hidden operation', { exact: true })).toHaveCount(0)
})
