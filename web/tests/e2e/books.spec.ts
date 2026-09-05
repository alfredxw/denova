import { expect, test } from '../support/fixtures'
import { getCurrentWorkspace } from '../support/api'
import { openAgentChatWorkbench } from '../support/agent-chat'

test('creates a Book from the bookshelf and keeps it selected after reload', async ({ page, request }) => {
  const title = 'Bookshelf E2E Book'

  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '书籍管理', exact: true }).click()
  await page.getByRole('button', { name: '新建书籍', exact: true }).click()
  await page.getByPlaceholder('书名（必填）').fill(title)
  await page.getByRole('dialog', { name: '新建书籍' }).getByRole('button', { name: '创建', exact: true }).click()

  await expect.poll(async () => (await getCurrentWorkspace(request)).workspace).toContain(title)
  await expect(page.getByText(title, { exact: true }).first()).toBeVisible()

  await page.reload()
  await sidebar.getByRole('button', { name: '书籍管理', exact: true }).click()
  await expect(page.getByText(title, { exact: true }).first()).toBeVisible()
  await expect(page.getByText('当前', { exact: true }).first()).toBeVisible()
})

test('creates a Book from the Agent workbench and opens its project', async ({ page }) => {
  const title = 'Workbench E2E Book'
  await page.goto('/')
  await openAgentChatWorkbench(page)
  await page.getByRole('button', { name: '添加项目', exact: true }).filter({ visible: true }).first().click()
  await page.getByRole('dialog', { name: '添加项目' }).getByRole('button', { name: /创建新书籍/ }).click()
  const dialog = page.getByRole('dialog', { name: '新建书籍' })
  await dialog.getByPlaceholder('书名（必填）').fill(title)
  await dialog.getByRole('button', { name: '创建', exact: true }).click()

  await expect(dialog).not.toBeVisible()
  const project = page.locator('[data-slot="agent-chat-project-toggle"]').filter({ hasText: title })
  await expect(project).toBeVisible()
  await expect(project).toHaveAttribute('aria-expanded', 'true')
  await page.reload()
  await expect(project).toBeVisible()
  await expect(page.locator('[data-nova-app-shell="true"]')).toBeVisible()
})
