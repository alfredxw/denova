import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '@playwright/test'
import { getCurrentWorkspace, getProjectLoreItems, getStorySnapshot } from '../support/api'

const legacyWorkspace = path.resolve('test-results/runtime/denova/projects/Legacy E2E Book')
const legacyLorePath = path.join(legacyWorkspace, '.nova', 'lore', 'items.json')
const legacyStoryID = 'st_legacy_v033_e2e'

test('opens and continues the released v0.3.3 Book, Lore, and Game data', async ({ page, request }) => {
  const originalLore = await readFile(legacyLorePath, 'utf8')

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
