import { expect, test, type Page } from '@playwright/test'
import { createAndOpenBook, createProjectFile, readProjectFile } from '../support/api'

const modelControlURL = `http://127.0.0.1:${process.env.DENOVA_E2E_MODEL_PORT || '18081'}`

async function openWritingAgent(page: Page) {
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '写作', exact: true }).click()
  const composer = page.getByPlaceholder(/输入消息/)
  if (!(await composer.isVisible())) {
    await page.getByRole('button', { name: '显示创作 Agent', exact: true }).click()
  }
  await expect(composer).toBeVisible()
  return composer
}

test('applies an Agent edit and supports review, undo, and redo', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Writing Agent E2E Book')
  const chapterPath = 'chapters/e2e-agent-chapter.md'
  const original = '# Agent 测试\n\nAgent 修改前。'
  await createProjectFile(request, book.projectId, chapterPath, original)

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '写作', exact: true }).click()
  await page.getByRole('button', { name: /^e2e agent chapter\b/ }).click()
  const composer = await openWritingAgent(page)
  await composer.fill('Update the chapter for the deterministic scenario. E2E_EDIT_CHAPTER')
  await composer.press('Enter')

  const summary = page.getByRole('region', { name: '已编辑 1 个文件' })
  await expect(summary).toBeVisible()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content)
    .toContain('Agent 已通过工具完成修改。')

  await summary.getByRole('button', { name: '审阅', exact: true }).click()
  const review = page.getByRole('region', { name: '变更审阅' })
  await expect(review).toBeVisible()
  await review.getByRole('button', { name: '接受本轮', exact: true }).click()

  const undo = review.getByRole('button', { name: '撤销整组', exact: true })
  await expect(undo).toBeEnabled()
  await undo.click()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content).toBe(original)

  const redo = review.getByRole('button', { name: '重做整组', exact: true })
  await expect(redo).toBeEnabled()
  await redo.click()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content)
    .toContain('Agent 已通过工具完成修改。')
})

test('reattaches to an Agent run after the page reloads mid-response', async ({ page, request }) => {
  await createAndOpenBook(request, 'Agent Recovery E2E Book')
  await page.goto('/')
  const composer = await openWritingAgent(page)
  await composer.fill('Wait until the browser reconnects. E2E_DELAYED_AGENT_REPLY')
  await composer.press('Enter')

  await expect.poll(async () => {
    const response = await request.get(`${modelControlURL}/control/status`)
    return response.ok() ? (await response.json() as { delayed_waiting: number }).delayed_waiting : 0
  }).toBe(1)

  let released = false
  try {
    await page.reload()
    const response = await request.post(`${modelControlURL}/control/release`)
    expect(response.ok()).toBe(true)
    released = true
  } finally {
    if (!released) await request.post(`${modelControlURL}/control/release`)
  }

  await openWritingAgent(page)
  await expect(page.getByText('Recovered response completed exactly once.', { exact: true })).toHaveCount(1)
})
