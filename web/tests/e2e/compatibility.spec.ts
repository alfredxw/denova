import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import { getCurrentWorkspace, getProjectLoreItems, getStorySnapshot } from '../support/api'

const legacyWorkspace = path.resolve('test-results/runtime/denova/projects/Legacy E2E Book')
const legacyLorePath = path.join(legacyWorkspace, '.nova', 'lore', 'items.json')
const legacyWritingSessionPath = path.join(
  legacyWorkspace,
  '.nova',
  'sessions',
  'v033-writing-main-e2e.jsonl',
)
const legacyStoryID = 'st_legacy_v033_e2e'

test('opens and continues the released v0.3.3 Writing, Book, Lore, and Game data', async ({ page, request }) => {
  const originalLore = await readFile(legacyLorePath, 'utf8')
  const originalWritingSession = await readFile(legacyWritingSessionPath, 'utf8')

  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '书籍管理', exact: true }).click()
  await page.getByRole('main').getByRole('button', { name: /Legacy E2E Book Denova v0\.3\.3/ }).click()

  await expect.poll(async () => getCurrentWorkspace(request)).toMatchObject({
    workspace: legacyWorkspace,
    has_state: true,
  })
  const current = await getCurrentWorkspace(request)
  expect(current.project_id).toBeTruthy()

  await sidebar.getByRole('button', { name: '写作', exact: true }).click()
  await page.getByRole('button', { name: /^legacy chapter\b/ }).click()
  await expect(page.locator('.editor-content .ProseMirror')).toContainText('这是 v0.3.3 保留的正文。')

  const writingComposer = page.getByPlaceholder(/输入消息/)
  if (!(await writingComposer.isVisible())) {
    await page.getByRole('button', { name: '显示创作 Agent', exact: true }).click()
  }
  await expect(writingComposer).toBeVisible()
  await expect(page.getByText('旧会话问题：第一章保留了什么？', { exact: true })).toBeVisible()
  await expect(page.getByText('第一章保留了 v0.3.3 的正文。', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: '会话历史', exact: true }).click()
  await expect(page.getByRole('option', { name: '切换到会话 v0.3.3 写作会话' })).toBeVisible()
  await expect(page.getByRole('option', { name: '切换到会话 v0.3.3 备用会话' })).toBeVisible()
  await page.keyboard.press('Escape')

  await writingComposer.fill('请继续旧会话。E2E_V033_WRITING_CONTINUE')
  await writingComposer.press('Enter')
  await expect(page.getByText('v0.3.3 写作历史续聊成功。', { exact: true })).toBeVisible()
  expect(await readFile(legacyWritingSessionPath, 'utf8')).toBe(originalWritingSession)

  await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
  await expect(page.getByText('林川', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('旧资料库正文', { exact: true }).first()).toBeVisible()
  await expect.poll(async () => getProjectLoreItems(request, current.project_id)).toContainEqual(
    expect.objectContaining({ id: 'hero', name: '林川', content: '旧资料库正文' }),
  )
  expect(await readFile(legacyLorePath, 'utf8')).toBe(originalLore)

  await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
  await expect(page.getByText('这是 v0.3.3 保存的游戏正文。', { exact: true })).toBeVisible()
  await expect.poll(async () => getStorySnapshot(request, legacyStoryID)).toEqual({
    turns: [expect.objectContaining({ user: '查看旧车站', narrative: '这是 v0.3.3 保存的游戏正文。' })],
  })

  const composer = page.getByPlaceholder(/你要做什么/)
  await composer.fill('继续探索旧车站')
  await composer.press('Enter')
  await expect.poll(async () => getStorySnapshot(request, legacyStoryID)).toMatchObject({
    turns: [
      { user: '查看旧车站', narrative: '这是 v0.3.3 保存的游戏正文。' },
      { user: '继续探索旧车站', narrative: expect.stringContaining('石门缓缓开启') },
    ],
  })
})
