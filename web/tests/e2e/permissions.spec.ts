import { mkdir, mkdtemp, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import {
  createAgentChatSession,
  registerAgentChatProject,
  setAgentChatApprovalMode,
} from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, submitAgentChatMessage } from '../support/agent-chat'
import { getModelStatus } from '../support/model'

// Each policy has independent setup. A failure in one must not prevent the
// other policies from exercising the real filesystem.
for (const scenario of [
  { mode: 'ask', label: 'Ask', marker: 'ASK', reply: 'External read completed in Ask mode.' },
  { mode: 'write', label: 'Write', marker: 'WRITE', reply: 'External read was denied in Write mode.' },
  { mode: 'full_access', label: 'Full access', marker: 'FULL_ACCESS', reply: 'External read completed in Full access mode.' },
] as const) {
  test(`enforces ${scenario.label} permissions on real external reads`, async ({ page, request }) => {
    const projectPath = await mkdtemp(path.resolve('test-results', 'runtime', 'permission-project-'))
    const project = await registerAgentChatProject(request, projectPath)
    const session = await createAgentChatSession(request, project.id, `Permission ${scenario.label} Session`)
    const modelStatus = await getModelStatus(request)
    await mkdir(path.dirname(modelStatus.external_secret_path), { recursive: true })
    await writeFile(modelStatus.external_secret_path, 'DENOVA_E2E_EXTERNAL_SECRET\n', 'utf8')
    await setAgentChatApprovalMode(request, project.id, session.id, scenario.mode)

    await page.goto('/')
    await openAgentChatWorkbench(page)
    const composer = await openAgentChatSession(page, project.id, session.title)
    await expect(page.getByRole('button', { name: `Agent 安全模式: ${scenario.label}` }).filter({ visible: true })).toBeVisible()
    await submitAgentChatMessage(page, composer, `Read the external E2E file. E2E_EXTERNAL_READ_${scenario.marker}`)
    const approval = page.getByRole('region', { name: '需要你的确认' }).filter({ visible: true })
    const displayedExternalPath = modelStatus.external_secret_path.replaceAll('\\', '/')

    switch (scenario.mode) {
      case 'ask': {
        const pendingApproval = approval.filter({ has: page.getByRole('button', { name: '仅允许本次' }) })
        const answeredApprovals = new Set<string>()
        for (let index = 0; index < 3; index += 1) {
          await expect(pendingApproval).toContainText(displayedExternalPath)
          const [answer] = await Promise.all([
            page.waitForResponse(response => response.request().method() === 'POST'
              && response.url().includes('/agent-chat/session/asks/') && response.url().endsWith('/answer')),
            pendingApproval.getByRole('button', { name: '仅允许本次' }).click(),
          ])
          const failureDetails = answer.status() === 200 ? '' : await answer.text()
          expect(answer.status(), `Approval ${index + 1}: ${failureDetails}`).toBe(200)
          expect(answeredApprovals.has(answer.url()), 'Each approval must belong to a new tool call').toBe(false)
          answeredApprovals.add(answer.url())
        }
        break
      }
      case 'write':
        await expect(approval).toContainText(displayedExternalPath)
        await approval.getByRole('button', { name: '拒绝' }).click()
        break
      case 'full_access':
        break
    }
    await expect(page.getByText(scenario.reply, { exact: true }).filter({ visible: true })).toBeVisible()
    if (scenario.mode === 'full_access') await expect(approval).toHaveCount(0)
  })
}
