import { expect, test } from '@playwright/test'
import { createAndOpenBook, getProjectLoreItems } from '../support/api'

test('creates, autosaves, and reloads a Lore item across Writing and Game', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Lore E2E Book')
  const name = 'E2E 跨模式角色'
  const content = '## 跨模式角色\n\n这条资料在写作与游戏之间共享。'

  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
  await page.getByRole('button', { name: '新建条目', exact: true }).click()

  await page.getByLabel('名称', { exact: true }).fill(name)
  const editor = page.getByRole('textbox', { name: '正文', exact: true })
  await editor.click()
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
  await page.keyboard.insertText(content)
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+S' : 'Control+S')

  await expect(page.getByRole('status', { name: '所有更改均已保存' })).toBeVisible()
  await expect.poll(async () => getProjectLoreItems(request, book.projectId)).toContainEqual(
    expect.objectContaining({ name, content: expect.stringContaining('这条资料在写作与游戏之间共享。') }),
  )

  await sidebar.getByRole('button', { name: '写作', exact: true }).click()
  await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
  await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
  await page.reload()
  await sidebar.getByRole('button', { name: '资料库', exact: true }).click()

  await expect(page.getByText(name, { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('textbox', { name: '正文', exact: true })).toContainText('这条资料在写作与游戏之间共享。')
})
