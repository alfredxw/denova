import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench } from '../support/agent-chat'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const projectType of ['book', 'general'] as const) {
  test(`automation uses the ${projectType} Project Agent pause and continuation lifecycle`, async ({ page, request }) => {
    const projectId = projectType === 'book'
      ? (await createAndOpenBook(request, 'Automation Project lifecycle')).projectId
      : (await registerAgentChatProject(request, await mkdtemp(path.resolve('test-results', 'runtime', 'automation-general-')))).id
    const marker = 'E2E_DELAYED_AGENT_REPLY'
    const title = `Automation ${projectType} lifecycle conversation`
    const theme = projectType === 'book' ? 'dark' : 'light'
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
      const response = await route.fetch()
      const settings = await response.json()
      await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, theme } } })
    })
    const created = await request.post('/api/automations', { data: {
      scope: 'workspace', target: { kind: 'workspace', project_id: projectId },
      enabled: false, name: title, template: 'custom_prompt',
      prompt: `Explain the next scene without editing files. ${marker}`,
      session_strategy: 'per_task',
    } })
    expect(created.ok(), await created.text()).toBe(true)
    const definition = await created.json()
    expect(definition.target.project_id).toBe(projectId)
    const startURL = `/api/automations/${encodeURIComponent(definition.catalog_id || definition.id)}/run`
    const command = { command_id: 'automation-project-lifecycle' }
    const started = await request.post(startURL, { data: command })
    expect(started.ok(), await started.text()).toBe(true)
    const { run } = await started.json()
    const activeURL = `/api/projects/${projectId}/agent-chat/chat/active?session_id=${encodeURIComponent(run.session_id)}`
    const readActive = async () => (await request.get(activeURL)).json()
    const readRecord = async () => {
      const catalog = await (await request.get(`/api/automations?project_id=${projectId}`)).json()
      return catalog.tasks.find((task: { id: string }) => task.id === definition.id)?.recent_runs?.find((item: { id: string }) => item.id === run.id)
    }
    try {
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      const attached = page.waitForResponse(response =>
        new URL(response.url()).pathname === `/api/projects/${projectId}/agent-chat/chat/stream`
        && response.status() === 200,
      )
      await page.goto('/')
      await openAgentChatWorkbench(page)
      await openAgentChatSession(page, projectId, title)
      await attached
      // Stop aborts during recovery. Exercise pause only after the existing
      // display stream has attached and the composer leaves recovery mode.
      await expect(page.getByText('正在从持久化状态恢复已接受的 Agent 运行…', { exact: true })).toHaveCount(0)
      const running = await readActive()
      expect(running.active_operation_id).toBeTruthy()
      const pauseCommand = page.waitForRequest(request => request.method() === 'POST'
        && new URL(request.url()).pathname === `/api/projects/${projectId}/agent-chat/chat/commands`)
      await page.locator('[data-action="stop"]').filter({ visible: true }).click()
      expect((await pauseCommand).postDataJSON()).toMatchObject({ type: 'suspend' })
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      await expect.poll(async () => (await readActive()).phase).toBe('suspended')
      await releaseDelayedRequest(request, marker)
      await page.reload()
      await openAgentChatWorkbench(page)
      await openAgentChatSession(page, projectId, title)
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
      // A paused Agent remains resumable. Trigger bookkeeping must never report
      // it as a failed execution or require another trigger to continue.
      expect((await readRecord()).status).toBe('suspended')
      await page.getByLabel('工作台侧边栏').getByRole('button', { name: '自动化', exact: true }).click()
      await page.getByTitle(`${title}\n已停用`, { exact: true }).click()
      await expect(page.getByText('已暂停', { exact: true })).toBeVisible()
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await page.getByText('已暂停', { exact: true }).scrollIntoViewIfNeeded()
      await page.screenshot({ path: test.info().outputPath(`automation-${theme}-wide.png`) })
      await page.setViewportSize({ width: 390, height: 844 })
      await expect(page.getByText('已暂停', { exact: true })).toBeVisible()
      await page.getByText('已暂停', { exact: true }).scrollIntoViewIfNeeded()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.screenshot({ path: test.info().outputPath(`automation-${theme}-narrow.png`) })
      await page.setViewportSize({ width: 1280, height: 960 })
      await page.getByRole('button', { name: '打开会话', exact: true }).click()
      // Opening from another destination rehydrates the conversation before
      // its runtime controls render, especially after the full browser suite.
      await expect(page.getByPlaceholder(/输入消息/).filter({ visible: true })).toBeVisible({ timeout: 30_000 })
      await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()

      const replay = await request.post(startURL, { data: command })
      expect(replay.ok(), await replay.text()).toBe(true)
      expect((await replay.json()).run.id).toBe(run.id)
      expect((await readActive()).active_operation_id).toBe(running.active_operation_id)
      await page.getByRole('button', { name: '继续任务', exact: true }).click()
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      expect((await readActive()).active_operation_id).toBe(running.active_operation_id)
      await page.locator('[data-action="stop"]').filter({ visible: true }).click()
      await expect.poll(async () => (await readRecord()).status).toBe('suspended')
      await releaseDelayedRequest(request, marker)
      // A recovered display task can settle before its GET reaches the server.
      // Hold this attachment until settlement to exercise canonical rehydration
      // deterministically, including on faster developer machines.
      await page.route(`**/api/projects/${projectId}/agent-chat/chat/stream?**`, async (route) => {
        const staleTaskId = new URL(route.request().url()).searchParams.get('task_id')
        expect(staleTaskId).toBeTruthy()
        await expect.poll(async () => (await readRecord()).status).toBe('success')
        // Pending delivery may already have started the next task. Only this
        // attachment's task must be gone; the conversation need not be idle.
        await expect.poll(async () => (await readActive()).task_id).not.toBe(staleTaskId)
        await route.continue()
      }, { times: 1 })
      const staleAttachment = page.waitForRequest(request => request.method() === 'GET'
        && new URL(request.url()).pathname === `/api/projects/${projectId}/agent-chat/chat/stream`,
      )
      const staleStream = page.waitForResponse(response =>
        new URL(response.url()).pathname === `/api/projects/${projectId}/agent-chat/chat/stream`
        && response.status() === 409,
      )
      await page.getByRole('button', { name: '继续任务', exact: true }).click()
      // Keep the model blocked until the browser has actually requested the
      // stream; otherwise hydration can see an already completed operation.
      await staleAttachment
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      expect((await readActive()).active_operation_id).toBe(running.active_operation_id)
      // A second trigger must not occupy the shared AgentChat admission lock
      // while this conversation is busy. Its frozen delivery can retry later.
      const waitingCommand = { command_id: 'automation-next-delivery' }
      const busyDelivery = await request.post(startURL, { data: waitingCommand, timeout: 2_000 })
      expect(busyDelivery.ok(), await busyDelivery.text()).toBe(true)
      expect((await busyDelivery.json()).run.delivery_status).toBe('pending')
      const pendingCatalog = await (await request.get(`/api/automations?project_id=${projectId}`)).json()
      const waitingRun = pendingCatalog.tasks.find((task: { id: string }) => task.id === definition.id).recent_runs.find((item: { id: string }) => item.id !== run.id)
      expect(waitingRun.delivery_status).toBe('pending')
      expect((await readActive()).active_operation_id).toBe(running.active_operation_id)
      await releaseDelayedRequest(request, marker)
      expect(await (await staleStream).json()).toMatchObject({ code: 'agent_runtime.rehydrate_required' })
      await expect(page.getByText('Recovered response completed exactly once.', { exact: true })).toHaveCount(1)
      await expect.poll(async () => (await readRecord()).status).toBe('success')
      const settledReplay = await request.post(startURL, { data: command })
      expect(settledReplay.ok(), await settledReplay.text()).toBe(true)
      expect((await settledReplay.json()).run.runtime_operation_id).toBe(running.active_operation_id)
      const next = await request.post(startURL, { data: waitingCommand })
      expect(next.ok(), await next.text()).toBe(true)
      const nextRun = (await next.json()).run
      expect(nextRun.id).toBe(waitingRun.id)
      expect(nextRun.session_id).toBe(run.session_id)
      expect(nextRun.runtime_operation_id).not.toBe(running.active_operation_id)
      await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
      const removed = await request.delete(`/api/automations/${encodeURIComponent(definition.catalog_id || definition.id)}`)
      expect(removed.ok(), await removed.text()).toBe(true)
      expect((await readActive()).active_operation_id).toBe(nextRun.runtime_operation_id)
      const aborted = await request.post(`/api/projects/${projectId}/agent-chat/chat/commands`, { data: {
        session_id: run.session_id, type: 'abort', command_id: 'abort-from-project-agent', target_operation_id: nextRun.runtime_operation_id,
      } })
      expect(aborted.ok(), await aborted.text()).toBe(true)
      await expect.poll(async () => (await readActive()).phase).toBe('idle')

    } finally {
      await releaseDelayedRequest(request, marker)
    }
  })
}
