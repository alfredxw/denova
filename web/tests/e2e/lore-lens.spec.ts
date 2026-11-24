import { expect, test, type APIRequestContext } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, createStartedStory, readProjectFile } from '../support/api'

async function getStorySnapshot(request: APIRequestContext, storyId: string) {
  const response = await request.get(`/api/interactive/stories/${encodeURIComponent(storyId)}/snapshot`)
  expect(response.ok(), await response.text()).toBe(true)
  return response.json() as Promise<{ current_turn: { id: string; narrative: string } }>
}

test('highlights Lore entities non-destructively in Writing and Game with hover cards and quick-create', async ({ page, request }) => {
  test.setTimeout(180_000)

  const book = await createAndOpenBook(request, 'Lore lens verification')
  const chapterPath = 'chapters/ch00001-港口风云.md'
  const chapterMarkdown = [
    '# 港口风云',
    '',
    '温照野站在栈桥尽头，温照野低声说：“潮汐快涨了。”',
    '',
    '江湖人称星海剑客，远处的青岚剑宗灯火通明。',
    '',
    '```',
    '温照野',
    '```',
    '',
  ].join('\n')

  await createProjectFile(request, book.projectId, chapterPath, chapterMarkdown)

  const createLoreRes = await request.post(`/api/projects/${encodeURIComponent(book.projectId)}/book/lore/items`, {
    data: {
      id: 'wen-zhaoye',
      name: '温照野',
      type: 'character',
      enabled: true,
      importance: 'major',
      brief_description: '港城巡夜司执事，擅长潮汐剑术。',
      keywords: ['星海剑客'],
      content: '常驻港城旧码头。',
    },
  })
  expect(createLoreRes.ok(), await createLoreRes.text()).toBe(true)

  const story = await createStartedStory(request, 'Lore lens story')
  const snap = await getStorySnapshot(request, story.id)
  const gameNarrative = '夜色沉沉，温照野提灯走向栈桥，身旁传来低语：“温照野来了。”'
  const patchTurnRes = await request.patch(`/api/interactive/stories/${encodeURIComponent(story.id)}/turns/${encodeURIComponent(snap.current_turn.id)}/narrative`, {
    data: {
      branch_id: 'main',
      expected_narrative: snap.current_turn.narrative,
      narrative: gameNarrative,
    },
  })
  expect(patchTurnRes.ok(), await patchTurnRes.text()).toBe(true)

  // 1. Writing editor: non-destructive highlight, per-paragraph dedup, code-block exclusion, hover card & jump.
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '写作', exact: true }).click()
  await page.getByText('港口风云', { exact: false }).first().click()

  const editor = page.locator('.ProseMirror:visible').first()
  await expect(editor).toContainText('温照野站在栈桥尽头')

  const writingMentions = editor.locator('[data-lore-lens-id="wen-zhaoye"]')
  // Per-paragraph deduplication + code block exclusion => exactly 1 mention in the chapter.
  await expect(writingMentions).toHaveCount(1)
  await expect(writingMentions.first()).toHaveText('温照野')

  // Keyword alias is disabled by default.
  await expect(editor.locator('[data-lore-lens-id]', { hasText: '星海剑客' })).toHaveCount(0)

  // Hovering opens the singleton preview card with brief description.
  await writingMentions.first().hover()
  const previewCard = page.getByTestId('lore-lens-card')
  await expect(previewCard).toBeVisible()
  await expect(previewCard).toContainText('温照野')
  await expect(previewCard).toContainText('港城巡夜司执事，擅长潮汐剑术。')

  // Save chapter and verify the underlying Markdown is byte-identical (zero syntax pollution).
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const savedAfterLens = await readProjectFile(request, book.projectId, chapterPath)
  expect(savedAfterLens.content).toBe(chapterMarkdown)

  // Toggle "Also match keyword aliases" on in Editor Settings => keyword alias lights up in paragraph 2.
  await page.getByRole('button', { name: '编辑器设置', exact: true }).click()
  const keywordSwitch = page.getByRole('switch', { name: '同时匹配关键词别名' })
  await keywordSwitch.click()
  await expect(editor.locator('[data-lore-lens-id="wen-zhaoye"]', { hasText: '星海剑客' })).toHaveCount(1)
  // Toggle Lore Lens off => mentions clear completely.
  const lensSwitch = page.getByRole('switch', { name: '资料实体透视' })
  await lensSwitch.click()
  await expect(editor.locator('[data-lore-lens-id]')).toHaveCount(0)
  // Re-enable Lore Lens for navigation + quick-create check.
  await lensSwitch.click()
  await expect(editor.locator('[data-lore-lens-id="wen-zhaoye"]')).toHaveCount(2)
  await page.getByRole('button', { name: '关闭', exact: true }).click()

  // Select an unlisted entity ("青岚剑宗") and create it via SelectionToolbar -> QuickCreateLoreDialog.
  await editor.evaluate((el) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
    let node: Node | null = walker.nextNode()
    while (node) {
      const idx = node.textContent?.indexOf('青岚剑宗') ?? -1
      if (idx >= 0) {
        const range = document.createRange()
        range.setStart(node, idx)
        range.setEnd(node, idx + 4)
        const selection = window.getSelection()
        selection?.removeAllRanges()
        selection?.addRange(range)
        break
      }
      node = walker.nextNode()
    }
    ;(el as HTMLElement).focus()
    document.dispatchEvent(new Event('selectionchange'))
  })
  const saveAsLoreBtn = page.getByRole('button', { name: '存为资料', exact: true })
  await expect(saveAsLoreBtn).toBeVisible()
  await saveAsLoreBtn.click()

  const createDialog = page.getByTestId('quick-create-lore-dialog')
  await expect(createDialog).toBeVisible()
  await createDialog.getByRole('textbox').last().fill('东海名门剑派。')
  await createDialog.getByRole('button', { name: '创建资料', exact: true }).click()
  await expect(createDialog).toBeHidden()

  // Newly created Lore entry lights up immediately in the active chapter.
  const newMention = editor.locator('[data-lore-lens-id]', { hasText: '青岚剑宗' })
  await expect(newMention).toHaveCount(1)

  // Clicking "Open Lore" from hover card navigates to Lore item.
  await writingMentions.first().hover()
  await expect(previewCard).toBeVisible()
  await previewCard.getByRole('button', { name: '打开资料', exact: true }).click()
  await expect(page.getByText('常驻港城旧码头。', { exact: false }).first()).toBeVisible()

  // 2. Game mode: settled turn highlights Lore entity, clicking opens card, presentation toggle turns it off.
  await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
  await expect(page.getByText('夜色沉沉')).toBeVisible()

  const stageCard = page.getByTestId('story-stage-card')
  const gameMentions = stageCard.locator('[data-lore-lens-id="wen-zhaoye"]')
  // Per-paragraph deduplication in Game too: 1 mention even though name appears twice in same paragraph.
  await expect(gameMentions).toHaveCount(1)
  await gameMentions.first().click()
  await expect(previewCard).toBeVisible()
  await expect(previewCard).toContainText('港城巡夜司执事，擅长潮汐剑术。')
  await page.keyboard.press('Escape')
  await expect(previewCard).toBeHidden()

  // Toggle Lore Lens off in Game Director Console -> Controls -> Stage presentation.
  await page.getByRole('tab', { name: '控制', exact: true }).click()
  const gameLensSwitch = page.getByRole('switch', { name: '资料实体透视' })
  await expect(gameLensSwitch).toBeChecked()
  await gameLensSwitch.click()
  await expect(stageCard.locator('[data-lore-lens-id]')).toHaveCount(0)
  await gameLensSwitch.click()
  await expect(stageCard.locator('[data-lore-lens-id="wen-zhaoye"]')).toHaveCount(1)

  // 3. Narrow viewport check: preview card remains clamped inside viewport bounds.
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(gameMentions).toHaveCount(1)
  await gameMentions.first().click()
  await expect(previewCard).toBeVisible()
  const cardBox = await previewCard.boundingBox()
  expect(cardBox).not.toBeNull()
  expect(cardBox!.x).toBeGreaterThanOrEqual(0)
  expect(cardBox!.x + cardBox!.width).toBeLessThanOrEqual(390)
  await page.keyboard.press('Escape')
  await expect(previewCard).toBeHidden()
})
