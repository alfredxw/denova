import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import { createAgentChatSession, createAndOpenBook, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent } from '../support/agent-chat'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const product of ['writing', 'general'] as const) {
  test(`${product} composer stops a runtime without pause support using abort`, async ({ page, request }) => {
    test.setTimeout(60_000)
    let projectId: string
    if (product === 'writing') {
      projectId = (await createAndOpenBook(request, 'Runtime stop')).projectId
    } else {
      const directory = await mkdtemp(path.join(runtimeRoot, 'runtime-stop-'))
      projectId = (await registerAgentChatProject(request, directory)).id
      await createAgentChatSession(request, projectId, 'Runtime stop')
    }
    // Use the local delayed model for execution while exercising the actual
    // composer with the external engine's advertised control capabilities.
    await page.route('**/conversation-config?*', async route => {
      const response = await route.fetch()
      const body = await response.json()
      await route.fulfill({ response, json: { ...body,
        runtime: { kind: 'codex', codex: { model: 'fixture-model' } },
        runtime_capabilities: { ...body.runtime_capabilities, pause: false, cancel: true, queue: false, goal: false },
      } })
    })
    await page.route('**/api/agent-runtimes/codex/models', route => route.fulfill({ json: {
      items: [{ id: 'fixture-model', display_name: 'Fixture model', efforts: [] }],
    } }))
    await page.goto('/')
    if (product === 'writing') await openWritingAgent(page)
    else {
      await openAgentChatWorkbench(page)
      await openAgentChatSession(page, projectId, 'Runtime stop')
    }
    await expect(page.locator('[data-model-profile-trigger]').filter({ visible: true })).toHaveAttribute('data-current-model', 'Fixture model')
    const marker = 'E2E_COMPOSER_PAUSE'
    await page.getByPlaceholder(/输入消息/).filter({ visible: true }).fill(`Check the footprints. ${marker}`)
    await page.locator('[data-action="send"]').filter({ visible: true }).click()
    try {
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      const submitted = page.waitForRequest(value => value.method() === 'POST' && new URL(value.url()).pathname.endsWith('/chat/commands'))
      await page.locator('[data-action="stop"]').filter({ visible: true }).click()
      const command = await submitted
      expect(command.postDataJSON()).toMatchObject({ type: 'abort', reason: 'user_requested' })
      const response = await command.response()
      expect(response?.status()).toBe(202)
      await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toHaveCount(0)
    } finally {
      await releaseDelayedRequest(request, marker)
    }
  })
}
