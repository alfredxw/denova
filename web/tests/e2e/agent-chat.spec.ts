import { access, mkdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test, type Page } from '../support/fixtures'
import { createAgentChatSession, registerAgentChatProject, setAgentChatApprovalMode } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, submitAgentChatMessage } from '../support/agent-chat'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

const sessionADelayMarker = 'E2E_SESSION_A_DELAY'
const sessionBDelayMarker = 'E2E_SESSION_B_DELAY'
const queueReloadDelayMarker = 'E2E_QUEUE_RELOAD_DELAY'
const queueReloadFollowUpMarker = 'E2E_QUEUE_RELOAD_FOLLOW_UP'
const multiAgentDisplayMarker = 'E2E_MULTI_AGENT_DISPLAY'
const multiAgentStreamGateMarker = 'E2E_MULTI_AGENT_STREAM_GATE'
const multiAgentExpectations = [
  { marker: 'E2E_MULTI_AGENT_ALPHA', output: 'Alpha stream started.', reasoning: 'Alpha reasoning one. Alpha reasoning two.' },
  { marker: 'E2E_MULTI_AGENT_BETA', output: 'Beta stream started.', reasoning: 'Beta reasoning one. Beta reasoning two.' },
  { marker: 'E2E_MULTI_AGENT_GAMMA', output: 'Gamma stream started.', reasoning: 'Gamma reasoning one. Gamma reasoning two.' },
]

test('runs General Agent tools in ordinary directories without crossing Project boundaries', async ({ page, request }) => {
  const alphaPath = path.resolve('test-results', 'runtime', 'general-project-alpha')
  const betaPath = path.resolve('test-results', 'runtime', 'general-project-beta')
  await Promise.all([mkdir(alphaPath, { recursive: true }), mkdir(betaPath, { recursive: true })])
  const [alpha, beta] = await Promise.all([
    registerAgentChatProject(request, alphaPath),
    registerAgentChatProject(request, betaPath),
  ])
  expect(alpha.type).toBe('general')
  expect(beta.type).toBe('general')
  const [alphaSession, betaSession] = await Promise.all([
    createAgentChatSession(request, alpha.id, 'General Alpha Session'),
    createAgentChatSession(request, beta.id, 'General Beta Session'),
  ])

  await page.goto('/')
  await openAgentChatWorkbench(page)
  let composer = await openAgentChatSession(page, alpha.id, alphaSession.title)
  await submitAgentChatMessage(page, composer, 'Write the deterministic Project proof. E2E_GENERAL_PROJECT_ALPHA_WRITE')
  await expect(page.getByText('General Project write completed: alpha-project-only.', { exact: true }).filter({ visible: true })).toBeVisible()
  await expect.poll(() => readFile(path.join(alphaPath, 'e2e-project-proof.txt'), 'utf8')).toBe('alpha-project-only')
  await expect.poll(() => fileExists(path.join(betaPath, 'e2e-project-proof.txt'))).toBe(false)

  composer = await openAgentChatSession(page, beta.id, betaSession.title)
  await submitAgentChatMessage(page, composer, 'Write the deterministic Project proof. E2E_GENERAL_PROJECT_BETA_WRITE')
  await expect(page.getByText('General Project write completed: beta-project-only.', { exact: true }).filter({ visible: true })).toBeVisible()
  await expect.poll(() => readFile(path.join(betaPath, 'e2e-project-proof.txt'), 'utf8')).toBe('beta-project-only')
  await expect(readFile(path.join(alphaPath, 'e2e-project-proof.txt'), 'utf8')).resolves.toBe('alpha-project-only')
})

test('keeps concurrent sessions independent and delivers Follow Up to its exact session', async ({ page, request }) => {
  const projectPath = path.resolve('test-results', 'runtime', 'parallel-session-project')
  await mkdir(projectPath, { recursive: true })
  const project = await registerAgentChatProject(request, projectPath)
  const [sessionA, sessionB] = await Promise.all([
    createAgentChatSession(request, project.id, 'Parallel Session A'),
    createAgentChatSession(request, project.id, 'Parallel Session B'),
  ])

  await page.goto('/')
  await openAgentChatWorkbench(page)
  try {
    let composer = await openAgentChatSession(page, project.id, sessionA.title)
    await submitAgentChatMessage(page, composer, `Hold Session A. ${sessionADelayMarker}`)
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[sessionADelayMarker] ?? 0).toBe(1)

    await submitAgentChatMessage(page, composer, 'Deliver this only after Session A resumes. E2E_SESSION_A_FOLLOW_UP')
    const queue = page.getByRole('region', { name: '排队中的指令' }).filter({ visible: true })
    await expect(queue).toContainText('E2E_SESSION_A_FOLLOW_UP')

    composer = await openAgentChatSession(page, project.id, sessionB.title)
    await submitAgentChatMessage(page, composer, `Hold Session B independently. ${sessionBDelayMarker}`)
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting).toBe(2)

    await releaseDelayedRequest(request, sessionBDelayMarker)
    await expect(page.getByText('Session B response completed independently.', { exact: true }).filter({ visible: true })).toBeVisible()
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[sessionADelayMarker] ?? 0).toBe(1)

    await openAgentChatSession(page, project.id, sessionA.title)
    await releaseDelayedRequest(request, sessionADelayMarker)
    await expect(page.getByText('Session A initial response completed.', { exact: true }).filter({ visible: true })).toBeVisible()
    await expect(page.getByText('Session A follow-up reached only Session A.', { exact: true }).filter({ visible: true })).toHaveCount(1)
    await expect(page.getByText('Session B response completed independently.', { exact: true }).filter({ visible: true })).toHaveCount(0)

    await openAgentChatSession(page, project.id, sessionB.title)
    await expect(page.getByText('Session B response completed independently.', { exact: true }).filter({ visible: true })).toHaveCount(1)
    await expect(page.getByText('Session A follow-up reached only Session A.', { exact: true }).filter({ visible: true })).toHaveCount(0)
  } finally {
    await Promise.allSettled([
      releaseDelayedRequest(request, sessionADelayMarker),
      releaseDelayedRequest(request, sessionBDelayMarker),
    ])
  }
})

test('keeps three interleaved SubAgent streams responsive, isolated, and restorable', async ({ page, request }) => {
  // Three complete isolation passes and two reloads need the slow-test budget
  // on CI; each individual interaction keeps the normal assertion timeout.
  test.slow()
  const projectPath = path.resolve('test-results', 'runtime', 'multi-agent-display-project')
  await mkdir(projectPath, { recursive: true })
  const project = await registerAgentChatProject(request, projectPath)
  const session = await createAgentChatSession(request, project.id, 'Multi-Agent Display Session')
  await setAgentChatApprovalMode(request, project.id, session.id, 'full_access')

  await page.goto('/')
  await openAgentChatWorkbench(page)
  let composer = await openAgentChatSession(page, project.id, session.title)
  let released = false
  let streamsReleased = false
  try {
    await submitAgentChatMessage(page, composer, `Delegate this test to three agents and wait for all results. ${multiAgentDisplayMarker}`)
    await expect.poll(async () => (
      (await getModelStatus(request)).delayed_waiting_by_marker[multiAgentStreamGateMarker] ?? 0
    )).toBe(3)

    const activeProcess = page.locator('[data-agent-execution-process]').last()
    // Attached tasks synchronize before the next model call, so no explicit
    // task_wait call is needed while the children are still streaming.
    await expect(activeProcess.getByText('委派任务', { exact: true })).toBeVisible()
    await releaseDelayedRequest(request, multiAgentStreamGateMarker)
    streamsReleased = true
    await expect.poll(async () => {
      const status = await getModelStatus(request)
      return multiAgentExpectations.map(item => status.delayed_waiting_by_marker[item.marker] ?? 0)
    }).toEqual([1, 1, 1])

    await expectIsolatedSubAgentSessions(page)

    await page.reload()
    await openAgentChatWorkbench(page)
    composer = await openAgentChatSession(page, project.id, session.title)
    await expectIsolatedSubAgentSessions(page)

    await Promise.all(multiAgentExpectations.map(item => releaseDelayedRequest(request, item.marker)))
    released = true
    await expect.poll(async () => {
      const status = await getModelStatus(request)
      return multiAgentExpectations.map(item => status.delayed_waiting_by_marker[item.marker] ?? 0)
    }).toEqual([0, 0, 0])
    await expect(page.getByText('All three delegated results completed.', { exact: true }).filter({ visible: true })).toHaveCount(1)
    const process = page.locator('[data-agent-execution-process]').filter({ hasText: 'SubAgent' }).last()
    await expect(process.locator('[data-slot="collapsible-trigger"]').first()).toContainText('执行过程')
    await expect(composer).toBeVisible()

    await page.reload()
    await openAgentChatWorkbench(page)
    composer = await openAgentChatSession(page, project.id, session.title)
    await expectIsolatedSubAgentSessions(page)
  } finally {
    if (!streamsReleased) {
      await releaseDelayedRequest(request, multiAgentStreamGateMarker)
    }
    if (!released) {
      await Promise.allSettled(multiAgentExpectations.map(item => releaseDelayedRequest(request, item.marker)))
    }
  }
})

test('restores an accepted Follow Up after reload and delivers it exactly once', async ({ page, request }) => {
  const projectPath = path.resolve('test-results', 'runtime', 'queue-reload-project')
  await mkdir(projectPath, { recursive: true })
  const project = await registerAgentChatProject(request, projectPath)
  const session = await createAgentChatSession(request, project.id, 'Queue Reload Session')

  await page.goto('/')
  await openAgentChatWorkbench(page)
  let composer = await openAgentChatSession(page, project.id, session.title)
  try {
    await submitAgentChatMessage(page, composer, `Keep this run active across reload. ${queueReloadDelayMarker}`)
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[queueReloadDelayMarker] ?? 0)
      .toBe(1)

    await submitAgentChatMessage(page, composer, `Deliver this after reload. ${queueReloadFollowUpMarker}`)
    let queue = page.getByRole('region', { name: '排队中的指令' }).filter({ visible: true })
    await expect(queue).toContainText(queueReloadFollowUpMarker)

    await page.reload()
    await openAgentChatWorkbench(page)
    composer = await openAgentChatSession(page, project.id, session.title)
    await expect(composer).toBeVisible()
    queue = page.getByRole('region', { name: '排队中的指令' }).filter({ visible: true })
    await expect(queue).toContainText(queueReloadFollowUpMarker)

    await releaseDelayedRequest(request, queueReloadDelayMarker)
    await expect(page.getByText('Reloaded queue initial response completed.', { exact: true }).filter({ visible: true })).toHaveCount(1)
    await expect(page.getByText('Reloaded queued follow-up completed exactly once.', { exact: true }).filter({ visible: true })).toHaveCount(1)
    await expect.poll(async () => (await getModelStatus(request)).request_counts[queueReloadFollowUpMarker] ?? 0).toBe(1)
  } finally {
    await releaseDelayedRequest(request, queueReloadDelayMarker)
  }
})

async function fileExists(filePath: string): Promise<boolean> {
  try {
    await access(filePath)
    return true
  } catch {
    return false
  }
}

async function expectIsolatedSubAgentSessions(page: Page): Promise<void> {
  const process = page.locator('[data-agent-execution-process]').filter({ hasText: 'SubAgent' }).last()
  await expect(process).toBeVisible()
  const trigger = process.locator('[data-slot="collapsible-trigger"]').first()
  if (await trigger.getAttribute('aria-expanded') !== 'true') await trigger.click()

  const cards = process.getByRole('button', { name: 'general-purpose 输出', exact: true })
  await expect(cards).toHaveCount(3)
  for (const item of multiAgentExpectations) await expect(process).not.toContainText(item.output)
  const seen = new Set<string>()
  for (let index = 0; index < 3; index += 1) {
    await cards.nth(index).click()
    const panel = page.getByRole('region', { name: 'general-purpose 子会话', exact: true }).filter({ visible: true })
    await expect(panel).toBeVisible()
    const panelText = await panel.innerText()
    const matches = multiAgentExpectations.filter(item => panelText.includes(item.output))
    expect(matches, `SubAgent detail ${index + 1} should contain exactly one child stream`).toHaveLength(1)
    const matched = matches[0]
    seen.add(matched.marker)

    const collapsedThinking = panel.getByRole('button', { name: '展开思考', exact: true })
    const expandedThinking = panel.getByRole('button', { name: '收起思考', exact: true })
    await expect(collapsedThinking.or(expandedThinking)).toHaveCount(1)
    if (await collapsedThinking.count()) await collapsedThinking.click()
    const thinking = panel.getByRole('region', { name: '思考内容', exact: true })
    await expect(thinking).toHaveCount(1)
    await expect(thinking).toContainText(matched.reasoning)
    for (const other of multiAgentExpectations.filter(item => item.marker !== matched.marker)) {
      await expect(panel).not.toContainText(other.output)
      await expect(panel).not.toContainText(other.reasoning)
    }
    await expect(panel.getByRole('button', { name: '关闭 SubAgent 详情', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: '关闭 general-purpose', exact: true }).click()
  }
  expect([...seen].sort()).toEqual(multiAgentExpectations.map(item => item.marker).sort())
}
