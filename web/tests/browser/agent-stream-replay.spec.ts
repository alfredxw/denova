import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import type { UIMessageChunk } from 'ai'
import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import type { AgentUIMessage } from '../../src/lib/agent-ui'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createAgentChatSession, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent } from '../support/agent-chat'

for (const [product, theme] of [['writing', 'dark'], ['general', 'light']] as const) {
  test(`${product} preserves replayed text and recovers conflicting segments in ${theme}`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, `Replay ${product}`)
    const project = product === 'general'
      ? await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'replay-project-')))
      : undefined
    const session = project ? await createAgentChatSession(request, project.id, 'Replay session') : undefined
    const apiPath = `/api/projects/${project?.id ?? book.projectId}/agent-chat`
    const settings = await (await request.get('/api/settings')).json()
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
      json: { ...settings, effective: { ...settings.effective, theme } },
    }))

    // Keep the HTTP replay open and deliver individual batches through the
    // real parser, projection, reconciliation and recovery hooks.
    await page.addInitScript(() => {
      const fetch = window.fetch.bind(window)
      window.fetch = async (...args) => {
        const response = await fetch(...args)
        if (!response.headers.has('x-test-agent-replay')) return response
        const bytes = new Uint8Array(await response.arrayBuffer())
        let listener: EventListener
        return new Response(new ReadableStream({
          start(controller) {
            controller.enqueue(bytes)
            listener = event => {
              const detail = (event as CustomEvent<string>).detail
              controller.enqueue(new TextEncoder().encode(detail))
              if (detail.includes('data: [DONE]')) {
                window.removeEventListener('test-agent-replay', listener)
                controller.close()
              }
            }
            window.addEventListener('test-agent-replay', listener)
          },
          cancel() { window.removeEventListener('test-agent-replay', listener) },
        }), { headers: { 'content-type': 'text/event-stream' } })
      }
    })
    const metadata = (segment: string) => ({ run_id: 'replay-run', display_segment_id: segment })
    const durableMessage = (segment: string, text: string): AgentUIMessage => ({
      id: `history-${segment}`, role: 'assistant', metadata: metadata(segment), parts: [{ type: 'text', text, state: 'done' }],
    })
    let history = [durableMessage('first', 'Known durable answer.'), durableMessage('second', 'Second durable answer.')]
    let historyReads = 0
    let settled = false
    let releaseActiveCheck!: () => void
    const activeCheck = new Promise<void>(resolve => { releaseActiveCheck = resolve })
    const streamRequests: URL[] = []
    const commands: string[] = []
    page.on('request', req => {
      if (req.method() === 'POST' && /\/chat(?:\/|$)/.test(new URL(req.url()).pathname)) commands.push(req.url())
    })
    await page.route(`**${apiPath}/session/messages?*`, route => {
      historyReads++
      return route.fulfill({ json: { messages: history, page: { has_more: false, total: history.length } } })
    })
    await page.route(`**${apiPath}/chat/active?*`, async route => {
      await activeCheck
      return route.fulfill({ json: settled ? { active: false, cursor: 12 } : {
        active: true, status: 'running', phase: 'running', task_id: 'existing-task', stream_attached: true, cursor: 10,
      } })
    })
    await page.route(`**${apiPath}/chat/stream?*`, route => {
      streamRequests.push(new URL(route.request().url()))
      const chunks: UIMessageChunk[] = [
        { type: 'start', messageId: `replay-${streamRequests.length}`, messageMetadata: { run_id: 'replay-run' } },
        { type: 'text-start', id: 'first', providerMetadata: { agent: metadata('first') } },
        { type: 'text-delta', id: 'first', delta: 'Known' },
        // This visible marker proves the short prefix reached a rendered batch.
        { type: 'message-metadata', messageMetadata: { run_id: 'replay-marker' } },
        { type: 'text-start', id: 'marker' },
        { type: 'text-delta', id: 'marker', delta: `Replay attached ${streamRequests.length}.` },
        { type: 'text-end', id: 'marker' },
        { type: 'message-metadata', messageMetadata: { run_id: 'replay-run' } },
      ]
      return route.fulfill({ headers: { 'content-type': 'text/event-stream', 'x-test-agent-replay': 'true' },
        body: chunks.map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join(''),
      })
    })

    await page.goto('/')
    if (project && session) {
      await openAgentChatWorkbench(page)
      await openAgentChatSession(page, project.id, session.title)
    } else await openWritingAgent(page)
    // Hydrate collapsed history before discovering its still-active run.
    // The historical default must not become a manual disclosure choice.
    try {
      await expect(page.getByText('Second durable answer.', { exact: true })).toBeVisible()
      await expect(page.getByText('Known durable answer.', { exact: true })).toHaveCount(0)
    } finally {
      releaseActiveCheck()
    }
    await expect(page.getByText('Replay attached 1.', { exact: true })).toBeVisible()
    await expect(page.getByText('Known durable answer.', { exact: true })).toHaveCount(1)
    await expect(page.locator('p').filter({ hasText: /^Known$/ })).toHaveCount(0)
    await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toBeVisible()
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('test-agent-replay', {
      detail: `data: ${JSON.stringify({ type: 'text-delta', id: 'first', delta: ' durable answer. Continued.' })}\n\n`,
    })))
    await expect(page.getByText('Known durable answer. Continued.', { exact: true })).toHaveCount(1)

    const readsBeforeConflict = historyReads
    history = [durableMessage('first', 'Known durable answer. Continued.'), durableMessage('second', 'Authoritative recovered answer.')]
    await page.evaluate(agent => window.dispatchEvent(new CustomEvent('test-agent-replay', {
      detail: [
        { type: 'text-start', id: 'second', providerMetadata: { agent } },
        { type: 'text-delta', id: 'second', delta: 'Conflicting replay must not replace history.' },
      ].map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join(''),
    })), metadata('second'))
    await expect(page.getByText('Replay attached 2.', { exact: true })).toBeVisible()
    await expect(page.getByText('Authoritative recovered answer.', { exact: true })).toHaveCount(1)
    await expect(page.getByText('Known durable answer. Continued.', { exact: true })).toHaveCount(1)
    await expect(page.getByText('Conflicting replay must not replace history.', { exact: true })).toHaveCount(0)
    expect(historyReads).toBeGreaterThan(readsBeforeConflict)
    expect(streamRequests).toHaveLength(2)
    expect(streamRequests.map(url => url.searchParams.get('task_id'))).toEqual(['existing-task', 'existing-task'])
    expect(streamRequests[0].searchParams.get('session_id')).toBeTruthy()
    expect(streamRequests[1].searchParams.get('session_id')).toBe(streamRequests[0].searchParams.get('session_id'))
    expect(commands).toEqual([])
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await page.screenshot({ path: test.info().outputPath(`replay-recovered-${product}-${theme}.png`) })

    const readsBeforeFinish = historyReads
    await page.evaluate(() => {
      const warnings: string[] = []
      const observer = new MutationObserver(() => {
        if (document.body.textContent?.includes('正在从持久化状态恢复')) warnings.push('recovery')
      })
      observer.observe(document.body, { childList: true, subtree: true, characterData: true })
      Object.assign(window, { finishRecoveryWarnings: warnings, finishRecoveryObserver: observer })
    })
    settled = true
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('test-agent-replay', {
      detail: 'data: {"type":"text-end","id":"first"}\n\ndata: {"type":"finish"}\n\ndata: [DONE]\n\n',
    })))
    await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
    await expect(page.getByText('Authoritative recovered answer.', { exact: true })).toHaveCount(1)
    expect(historyReads).toBe(readsBeforeFinish)
    expect(await page.evaluate(() => {
      const state = window as unknown as { finishRecoveryWarnings: string[]; finishRecoveryObserver: MutationObserver }
      state.finishRecoveryObserver.disconnect()
      return state.finishRecoveryWarnings
    })).toEqual([])
    await page.screenshot({ path: test.info().outputPath(`replay-finished-${product}-${theme}.png`) })
    expect(commands).toEqual([])
  })
}
