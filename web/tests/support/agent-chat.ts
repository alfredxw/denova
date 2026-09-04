import { expect, type Locator, type Page } from './fixtures'

/** Opens one durable AgentChat conversation without relying on project display names. */
export async function openAgentChatSession(page: Page, projectId: string, sessionTitle: string): Promise<Locator> {
  const project = page.locator(
    `[data-slot="agent-chat-project-toggle"][data-project-id="${projectId}"]`,
  )
  await expect(project).toBeVisible()
  if (await project.getAttribute('aria-expanded') !== 'true') await project.click()

  const session = page.locator('[data-slot="agent-chat-conversation-row"]')
    .filter({ hasText: sessionTitle })
  await expect(session).toBeVisible()
  await session.click()
  await expect(session).toHaveAttribute('aria-current', 'page')

  const composer = page.getByPlaceholder(/输入消息/).filter({ visible: true })
  await expect(composer).toBeVisible()
  return composer
}

/** Submits after the selected conversation's configuration is ready. */
export async function submitAgentChatMessage(page: Page, composer: Locator, message: string): Promise<void> {
  await composer.fill(message)
  await page.locator('[data-action="send"]').filter({ visible: true }).click()
}

export async function openAgentChatWorkbench(page: Page): Promise<void> {
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '工作台', exact: true }).click()
}

/** Opens the Writing Agent after route transitions and persisted panel visibility settle. */
export async function openWritingAgent(page: Page): Promise<Locator> {
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '写作', exact: true }).click()

  const composer = page.getByTestId('right').getByPlaceholder(/输入消息/).filter({ visible: true })
  const showAgent = page.getByRole('button', { name: '显示创作 Agent', exact: true }).filter({ visible: true })
  await expect(composer.or(showAgent)).toHaveCount(1)
  if (await showAgent.count()) await showAgent.click()
  await expect(composer).toHaveCount(1)
  return composer
}
