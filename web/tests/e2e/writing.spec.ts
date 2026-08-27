import { expect, test } from '@playwright/test'
import { createAndOpenBook, createProjectFile, readProjectFile } from '../support/api'

test('edits and persists a chapter through the Writing workbench', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Writing E2E Book')
  const chapterPath = 'chapters/e2e-chapter.md'
  await createProjectFile(request, book.projectId, chapterPath, '# 起点\n\n最初的段落。')

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '写作', exact: true }).click()
  await page.getByRole('button', { name: /^e2e chapter\b/ }).click()

  const editor = page.locator('.editor-content .ProseMirror')
  await expect(editor).toBeVisible()
  await editor.click()
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
  await page.keyboard.insertText('第一章\n\nPlaywright 已完成真实保存。')
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+S' : 'Control+S')

  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content)
    .toContain('Playwright 已完成真实保存。')
})
