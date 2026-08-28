import { expect, test, type Page } from '@playwright/test'
import { createAndOpenBook, createProjectFile, readProjectFile, saveProjectFile } from '../support/api'

async function openVersionHistory(page: Page) {
  const saveVersion = page.getByRole('button', { name: '保存当前版本', exact: true })
  if (!(await saveVersion.isVisible())) {
    await page.getByRole('button', { name: '打开版本历史', exact: true }).click()
  }
  await expect(saveVersion).toBeVisible()
  return saveVersion
}

test('saves a version, shows its diff, and restores one file', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Versions E2E Book')
  const filePath = 'chapters/versioned.md'
  const original = '# 版本章节\n\n第一个受保护的版本。'
  const changed = '# 版本章节\n\n这是尚未保存为版本的新内容。'
  await createProjectFile(request, book.projectId, filePath, original)

  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '版本管理', exact: true }).click()
  await (await openVersionHistory(page)).click()
  await expect(page.getByText(/已保存版本：/)).toBeVisible()

  await saveProjectFile(request, book.projectId, filePath, changed)
  await openVersionHistory(page)
  await page.getByRole('button', { name: '刷新版本状态', exact: true }).first().click()
  await page.getByRole('button', { name: /当前变更/ }).click()
  await expect(page.getByText(filePath, { exact: true }).first()).toBeVisible()

  const restoreFile = page.getByRole('button', { name: '恢复此文件', exact: true })
  await restoreFile.focus()
  await restoreFile.press('Enter')
  const dialog = page.getByRole('alertdialog', { name: '确认恢复文件？' })
  await expect(dialog.getByText(filePath, { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: '恢复文件', exact: true }).click()

  await expect(page.getByText('已恢复版本', { exact: true })).toBeVisible()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, filePath)).content).toBe(original)
})
