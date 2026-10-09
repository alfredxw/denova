import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createAgentChatSession, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const mode of ['Writing', 'General'] as const) {
  test(`${mode} starts an independent conversation with /new and preserves the original`, async ({ page, request }) => {
    const writing = mode === 'Writing'
    const settings = await request.patch('/api/settings', { data: {
      layer: 'user', changes: { theme: writing ? 'dark' : 'light' },
    } })
    expect(settings.ok(), await settings.text()).toBe(true)
    const project = writing
      ? await createAndOpenBook(request, 'New conversation E2E')
      : { projectId: (await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'new-conversation-')))).id }
    const initial = writing ? null : await createAgentChatSession(request, project.projectId, 'Original General E2E')
    await page.goto('/')
    if (!writing) await openAgentChatWorkbench(page)
    let composer = writing
      ? await openWritingAgent(page)
      : await openAgentChatSession(page, project.projectId, initial!.title)
    const base = `/api/projects/${encodeURIComponent(project.projectId)}`
    const chatPath = `${base}/agent-chat/chat`
    const originalMessage = writing ? 'Original Writing E2E' : 'Hold the original. E2E_SESSION_A_DELAY'
    const originalRequest = page.waitForRequest(candidate => candidate.method() === 'POST' && new URL(candidate.url()).pathname === chatPath)
    await submitAgentChatMessage(page, composer, originalMessage)
    const originalID = (await originalRequest).postDataJSON().session_id as string
    const history = async (id: string) => {
      const response = await request.get(`${base}/agent-chat/session/messages?session_id=${encodeURIComponent(id)}&limit=100`)
      expect(response.ok(), await response.text()).toBe(true)
      return response.json()
    }
    const refreshProjectSnapshot = () => page.evaluate(({ writing, projectId }) => {
      if (writing) {
        window.dispatchEvent(new CustomEvent('nova:agent-chat-project-updated', { detail: { projectId } }))
      } else {
        window.dispatchEvent(new Event('focus'))
      }
    }, { writing, projectId: project.projectId })
    const goal = async (id: string) => {
      const response = await request.get(`${base}/conversation-goal?mode=agent_chat&session_id=${encodeURIComponent(id)}`)
      expect(response.ok(), await response.text()).toBe(true)
      return response.json()
    }
    let releaseSubmission: (() => void) | undefined
    let releaseProjectSnapshot: (() => void) | undefined
    try {
      if (writing) {
        await expect(page.getByText('Deterministic E2E response completed.', { exact: true }).filter({ visible: true })).toBeVisible()
        await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
        const response = await request.post(`${base}/conversation-goal`, { data: {
          binding: { mode: 'agent_chat', session_id: originalID },
          action: 'set', objective: 'Preserve the original objective.',
        } })
        expect(response.ok(), await response.text()).toBe(true)
      } else {
        await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker.E2E_SESSION_A_DELAY ?? 0).toBe(1)
      }
      const originalHistory = await history(originalID)
      const originalGoal = await goal(originalID)
      const submittedCommands: string[] = []
      page.on('request', candidate => {
        if (candidate.method() === 'POST' && new URL(candidate.url()).pathname.startsWith(`${base}/agent-chat/`)) {
          const body = candidate.postDataJSON()
          submittedCommands.push(body.message ?? body.command ?? '')
        }
      })
      await composer.fill('/new')
      await expect(page.getByRole('option').filter({ hasText: '/new' })).toBeVisible()
      await expect(page.getByRole('option').filter({ hasText: '/clear' })).toHaveCount(0)
      await page.screenshot({ path: test.info().outputPath(`${mode.toLowerCase()}-new-command.png`) })
      await page.locator('[data-action="send"]').filter({ visible: true }).click()
      composer = page.getByPlaceholder(/输入消息/).filter({ visible: true })
      await expect(composer).toHaveText('')
      await expect(page.getByText(originalMessage, { exact: true }).filter({ visible: true })).toHaveCount(0)
      const submissionGate = new Promise<void>(resolve => { releaseSubmission = resolve })
      await page.route(`**${chatPath}`, async route => {
        await submissionGate
        await route.continue()
      }, { times: 1 })
      const freshRequest = page.waitForRequest(candidate => candidate.method() === 'POST' && new URL(candidate.url()).pathname === chatPath)
      await submitAgentChatMessage(page, composer, 'Start independently. E2E_NEW_CONVERSATION_FRESH')
      const freshID = (await freshRequest).postDataJSON().session_id as string
      expect(freshID).not.toBe(originalID)
      {
        // A refresh before durable acceptance must keep the pending draft mounted.
        const snapshot = page.waitForResponse(response => new URL(response.url()).pathname === '/api/agent-chat/projects')
        await refreshProjectSnapshot()
        await (await snapshot).finished()
        await expect(composer).toBeVisible()
        let snapshotCaptured: (() => void) | undefined
        const capturedSnapshot = new Promise<void>(resolve => { snapshotCaptured = resolve })
        const snapshotGate = new Promise<void>(resolve => { releaseProjectSnapshot = resolve })
        const oldProjects = await (await request.get('/api/agent-chat/projects')).json()
        let holdSnapshot = true
        await page.route('**/api/agent-chat/projects', async route => {
          if (!holdSnapshot) return route.continue()
          holdSnapshot = false
          snapshotCaptured?.()
          await snapshotGate
          await route.fulfill({ json: oldProjects })
        })
        await refreshProjectSnapshot()
        await capturedSnapshot
        // Deliver a pre-creation snapshot after the server confirms the first turn.
        const acceptance = page.waitForResponse(response => new URL(response.url()).pathname === chatPath)
        releaseSubmission?.()
        expect((await acceptance).ok()).toBe(true)
        releaseProjectSnapshot?.()
      }
      await expect(page.getByText('Deterministic E2E response completed.', { exact: true }).filter({ visible: true })).toBeVisible()
      expect(await goal(freshID)).toEqual({ goal: null })
      expect(await goal(originalID)).toEqual(originalGoal)
      expect(await history(originalID)).toEqual(originalHistory)
      expect(JSON.stringify(await history(freshID))).not.toContain(originalMessage)
      expect(submittedCommands).not.toContain('/new')
      expect(submittedCommands).not.toContain('new')
      const retired = await request.post(`${base}/agent-chat/command`, { data: { session_id: originalID, command: 'clear' } })
      expect(retired.status()).toBe(400)
      expect(await history(originalID)).toEqual(originalHistory)
      if (!writing) {
        await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker.E2E_SESSION_A_DELAY ?? 0).toBe(1)
        await releaseDelayedRequest(request, 'E2E_SESSION_A_DELAY')
      }
      await page.reload()
      if (writing) {
        await openWritingAgent(page)
        await page.getByRole('button', { name: '会话历史', exact: true }).filter({ visible: true }).click()
        await page.getByRole('option').filter({ hasText: originalMessage }).click()
      } else {
        await openAgentChatWorkbench(page)
        await openAgentChatSession(page, project.projectId, initial!.title)
      }
      await expect(page.getByText(originalMessage, { exact: true }).filter({ visible: true })).toBeVisible()
      await expect(page.getByText(writing ? 'Deterministic E2E response completed.' : 'Session A initial response completed.', { exact: true }).filter({ visible: true })).toBeVisible()
    } finally {
      releaseSubmission?.()
      releaseProjectSnapshot?.()
      if (!writing) await releaseDelayedRequest(request, 'E2E_SESSION_A_DELAY')
    }
  })
}
