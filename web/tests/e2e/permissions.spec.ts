import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import {
  createAgentChatSession,
  registerAgentChatProject,
  setAgentChatApprovalMode,
} from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench } from '../support/agent-chat'
import { getModelStatus } from '../support/model'

test('enforces Ask, Write, and Full access on a real external filesystem read', async ({ page, request }) => {
  const projectPath = path.resolve('test-results', 'runtime', 'permission-project')
  await mkdir(projectPath, { recursive: true })
  const project = await registerAgentChatProject(request, projectPath)
  const [askSession, writeSession, fullSession] = await Promise.all([
    createAgentChatSession(request, project.id, 'Permission Ask Session'),
    createAgentChatSession(request, project.id, 'Permission Write Session'),
    createAgentChatSession(request, project.id, 'Permission Full Session'),
  ])
  const modelStatus = await getModelStatus(request)
  await mkdir(path.dirname(modelStatus.external_secret_path), { recursive: true })
  await writeFile(modelStatus.external_secret_path, 'DENOVA_E2E_EXTERNAL_SECRET\n', 'utf8')
  await Promise.all([
    setAgentChatApprovalMode(request, project.id, askSession.id, 'ask'),
    setAgentChatApprovalMode(request, project.id, writeSession.id, 'write'),
    setAgentChatApprovalMode(request, project.id, fullSession.id, 'full_access'),
  ])

  await page.goto('/')
  await openAgentChatWorkbench(page)

  let composer = await openAgentChatSession(page, project.id, askSession.title)
  await expect(page.getByRole('button', { name: 'Agent 安全模式: Ask' }).filter({ visible: true })).toBeVisible()
  await composer.fill('Read the external E2E file. E2E_EXTERNAL_READ_ASK')
  await composer.press('Enter')
  let approval = page.getByRole('region', { name: '需要你的确认' }).filter({ visible: true })
  const displayedExternalPath = modelStatus.external_secret_path.replaceAll('\\', '/')
  await expect(approval).toContainText(displayedExternalPath)
  await approval.getByRole('button', { name: '仅允许本次' }).click()
  await expect(page.getByText('External read completed in Ask mode.', { exact: true }).filter({ visible: true })).toBeVisible()

  composer = await openAgentChatSession(page, project.id, writeSession.title)
  await expect(page.getByRole('button', { name: 'Agent 安全模式: Write' }).filter({ visible: true })).toBeVisible()
  await composer.fill('Read the external E2E file. E2E_EXTERNAL_READ_WRITE')
  await composer.press('Enter')
  approval = page.getByRole('region', { name: '需要你的确认' }).filter({ visible: true })
  await expect(approval).toContainText(displayedExternalPath)
  await approval.getByRole('button', { name: '拒绝' }).click()
  await expect(page.getByText('External read was denied in Write mode.', { exact: true }).filter({ visible: true })).toBeVisible()

  composer = await openAgentChatSession(page, project.id, fullSession.title)
  await expect(page.getByRole('button', { name: 'Agent 安全模式: Full access' }).filter({ visible: true })).toBeVisible()
  await composer.fill('Read the external E2E file. E2E_EXTERNAL_READ_FULL_ACCESS')
  await composer.press('Enter')
  await expect(page.getByText('External read completed in Full access mode.', { exact: true }).filter({ visible: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '需要你的确认' }).filter({ visible: true })).toHaveCount(0)
})
