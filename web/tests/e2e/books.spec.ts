import { expect, test } from '../support/fixtures'
import { getCurrentWorkspace } from '../support/api'

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
