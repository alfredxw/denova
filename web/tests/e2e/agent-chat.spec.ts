import { access, mkdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import { createAgentChatSession, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench } from '../support/agent-chat'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

const sessionADelayMarker = 'E2E_SESSION_A_DELAY'
const sessionBDelayMarker = 'E2E_SESSION_B_DELAY'
const queueReloadDelayMarker = 'E2E_QUEUE_RELOAD_DELAY'
const queueReloadFollowUpMarker = 'E2E_QUEUE_RELOAD_FOLLOW_UP'

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
  await composer.fill('Write the deterministic Project proof. E2E_GENERAL_PROJECT_ALPHA_WRITE')
  await composer.press('Enter')
  await expect(page.getByText('General Project write completed: alpha-project-only.', { exact: true }).filter({ visible: true })).toBeVisible()
  await expect.poll(() => readFile(path.join(alphaPath, 'e2e-project-proof.txt'), 'utf8')).toBe('alpha-project-only')
  await expect.poll(() => fileExists(path.join(betaPath, 'e2e-project-proof.txt'))).toBe(false)

  composer = await openAgentChatSession(page, beta.id, betaSession.title)
  await composer.fill('Write the deterministic Project proof. E2E_GENERAL_PROJECT_BETA_WRITE')
  await composer.press('Enter')
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
    await composer.fill(`Hold Session A. ${sessionADelayMarker}`)
    await composer.press('Enter')
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[sessionADelayMarker] ?? 0).toBe(1)

    await composer.fill('Deliver this only after Session A resumes. E2E_SESSION_A_FOLLOW_UP')
    await composer.press('Enter')
    const queue = page.getByRole('region', { name: '排队中的指令' }).filter({ visible: true })
    await expect(queue).toContainText('E2E_SESSION_A_FOLLOW_UP')

    composer = await openAgentChatSession(page, project.id, sessionB.title)
    await composer.fill(`Hold Session B independently. ${sessionBDelayMarker}`)
    await composer.press('Enter')
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

test('restores an accepted Follow Up after reload and delivers it exactly once', async ({ page, request }) => {
  const projectPath = path.resolve('test-results', 'runtime', 'queue-reload-project')
  await mkdir(projectPath, { recursive: true })
  const project = await registerAgentChatProject(request, projectPath)
  const session = await createAgentChatSession(request, project.id, 'Queue Reload Session')

  await page.goto('/')
  await openAgentChatWorkbench(page)
  let composer = await openAgentChatSession(page, project.id, session.title)
  try {
    await composer.fill(`Keep this run active across reload. ${queueReloadDelayMarker}`)
    await composer.press('Enter')
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[queueReloadDelayMarker] ?? 0)
      .toBe(1)

    await composer.fill(`Deliver this after reload. ${queueReloadFollowUpMarker}`)
    await composer.press('Enter')
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
