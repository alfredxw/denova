import type { UIMessageChunk } from 'ai'
import { writeFile } from 'node:fs/promises'
import type { AgentUIMessage } from '../../src/lib/agent-ui'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'

test('large tool loops do not clone completed results on subsequent stream chunks', async ({ page, request, context }) => {
  await createAndOpenBook(request, 'Large stream memory regression')
  // Count copied result characters rather than relying on a machine-specific
  // RSS threshold. This reproduces the SDK snapshot amplification in a browser.
  await page.addInitScript(() => {
    const original = window.structuredClone
    let copiedResultCharacters = 0
    Object.defineProperty(window, '__copiedAgentResultCharacters', { get: () => copiedResultCharacters })
    window.structuredClone = ((value: unknown, options?: StructuredSerializeOptions) => {
      const message = value as { parts?: Array<{ type?: string; output?: unknown }> } | null
      if (Array.isArray(message?.parts)) {
        for (const part of message.parts) {
          if (part.type === 'dynamic-tool' && typeof part.output === 'string') copiedResultCharacters += part.output.length
        }
      }
      return original(value, options)
    }) as typeof window.structuredClone
  })
  const chunks: UIMessageChunk[] = [{ type: 'start', messageId: 'large-run', messageMetadata: { run_id: 'large-run' } }]
  const history: AgentUIMessage[] = []
  const result = 'Chapter evidence. '.repeat(2400)
  for (let i = 0; i < 200; i++) {
    const toolCallId = `read-${i}`
    chunks.push(
      { type: 'tool-input-start', toolCallId, toolName: 'read' },
      { type: 'tool-input-delta', toolCallId, inputTextDelta: JSON.stringify({ path: `chapter-${i}.md` }) },
      { type: 'tool-input-available', toolCallId, toolName: 'read', input: { path: `chapter-${i}.md` } },
      { type: 'tool-output-available', toolCallId, output: result },
    )
    history.push({ id: toolCallId, role: 'assistant', metadata: { run_id: 'large-run' }, parts: [{
      type: 'dynamic-tool', toolName: 'read', toolCallId, state: 'output-available', input: { path: `chapter-${i}.md` }, output: result,
    }] })
  }
  chunks.push({ type: 'text-start', id: 'answer' }, { type: 'text-delta', id: 'answer', delta: 'All 200 chapters read.' }, { type: 'text-end', id: 'answer' }, { type: 'finish' })
  history.push({ id: 'answer', role: 'assistant', metadata: { run_id: 'large-run' }, parts: [{ type: 'text', text: 'All 200 chapters read.' }] })
  let submitted = false
  await page.route(/\/api\/(?:projects\/[^/]+\/agent-chat\/)?chat$/, route => {
    submitted = true
    return route.fulfill({
      headers: { 'content-type': 'text/event-stream', 'x-vercel-ai-ui-message-stream': 'v1' },
      body: chunks.map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join('') + 'data: [DONE]\n\n',
    })
  })
  // A settled stream reloads its canonical page. Keep the mocked durable lane
  // consistent with the mocked stream so recovery cannot erase its results.
  await page.route('**/session/messages?*', route => submitted
    ? route.fulfill({ json: { messages: history, page: { has_more: false, total: history.length } } })
    : route.continue())
  await page.goto('/')
  const composer = await openWritingAgent(page)
  const cdp = await context.newCDPSession(page)
  await cdp.send('Performance.enable')
  await submitAgentChatMessage(page, composer, 'Read the chapter collection.')
  await expect(page.getByText('All 200 chapters read.', { exact: true })).toBeVisible()
  await expect(page.getByText('200 次工具调用', { exact: false })).toBeVisible()
  const copied = await page.evaluate(() => (window as unknown as { __copiedAgentResultCharacters: number }).__copiedAgentResultCharacters)
  expect(copied).toBeLessThan(result.length * 200 * 2)
  await page.screenshot({ path: test.info().outputPath('large-tool-loop.png') })
  const metrics = await cdp.send('Performance.getMetrics')
  const measurements = test.info().outputPath('stream-memory.json')
  await writeFile(measurements, JSON.stringify({
    results: 200, resultCharacters: result.length * 200, copiedResultCharacters: copied,
    heapUsedBytes: metrics.metrics.find(metric => metric.name === 'JSHeapUsedSize')?.value,
  }, null, 2))
  await test.info().attach('stream-memory.json', { contentType: 'application/json', path: measurements })
})
