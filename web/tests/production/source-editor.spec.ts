import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createProjectFile } from '../support/api'

test('loads the source editor and its worker without public network access', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Offline Source Editor')
  await createProjectFile(request, book.projectId, 'chapters/offline.md', '# Offline\n\nLocal editor content.\n')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await page.getByRole('button', { name: '文件', exact: true }).click()
  await page.getByRole('dialog', { name: '项目', exact: true }).getByRole('button', { name: /^offline/ }).click()
  await page.getByRole('button', { name: '编辑工具', exact: true }).click()
  const workerStarted = page.waitForEvent('worker')
  await page.getByRole('menuitemcheckbox', { name: '源码', exact: true }).click()
  await expect(page.locator('.monaco-editor:visible')).toBeVisible()
  await expect(page.locator('.monaco-editor .view-lines')).toContainText('Local editor content.')
  const worker = await workerStarted
  expect(new URL(worker.url()).origin).toBe(new URL(page.url()).origin)
})
