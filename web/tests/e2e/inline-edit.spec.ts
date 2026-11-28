import { expect, test, type APIRequestContext, type Locator } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, createStartedStory, readProjectFile } from '../support/api'

const shortcutModifier = process.platform === 'darwin' ? 'Meta' : 'Control'

async function selectTextInEditor(editor: Locator, target: string) {
  const matched = await editor.evaluate((el, needle) => {
    const blocks = Array.from(el.querySelectorAll('p, h1, h2, h3, li, blockquote'))
    const containers = blocks.length > 0 ? blocks : [el]
    for (const block of containers) {
      const full = block.textContent ?? ''
      const startOffset = full.indexOf(needle)
      if (startOffset < 0) continue
      const endOffset = startOffset + needle.length

      const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT)
      let currentOffset = 0
      let startNode: Node | null = null
      let startInNode = 0
      let endNode: Node | null = null
      let endInNode = 0

      let node: Node | null = walker.nextNode()
      while (node) {
        const len = node.textContent?.length ?? 0
        if (!startNode && currentOffset + len >= startOffset) {
          startNode = node
          startInNode = startOffset - currentOffset
        }
        if (currentOffset + len >= endOffset) {
          endNode = node
          endInNode = endOffset - currentOffset
          break
        }
        currentOffset += len
        node = walker.nextNode()
      }

      if (startNode && endNode) {
        const range = document.createRange()
        range.setStart(startNode, startInNode)
        range.setEnd(endNode, endInNode)
        const selection = window.getSelection()
        selection?.removeAllRanges()
        selection?.addRange(range)
        ;(el as HTMLElement).focus()
        document.dispatchEvent(new Event('selectionchange'))
        return true
      }
    }
    return false
  }, target)
  expect(matched, `expected to find text "${target}" in editor`).toBe(true)
}

async function getStorySnapshot(request: APIRequestContext, storyId: string) {
  const response = await request.get(`/api/interactive/stories/${encodeURIComponent(storyId)}/snapshot`)
  expect(response.ok(), await response.text()).toBe(true)
  return response.json() as Promise<{
    current_turn: { id: string; narrative: string }
    branches?: Array<{ id: string }>
    state?: unknown
  }>
}

test('supports inline AI prose rewrite with CJK word diff, atomic undo, and zero chat pollution across Writing, Lore, and Game', async ({ page, request }) => {
  test.setTimeout(180_000)

  const book = await createAndOpenBook(request, 'Inline Edit E2E')
  const chapterPath = 'chapters/ch00001-港口夜雾.md'
  const originalSentence = '温照野站在栈桥尽头，低声说：“潮汐快涨了。”'
  const chapterMarkdown = `# 港口夜雾\n\n${originalSentence}\n\n远处传来钟声。\n`

  await createProjectFile(request, book.projectId, chapterPath, chapterMarkdown)

  const createLoreRes = await request.post(`/api/projects/${encodeURIComponent(book.projectId)}/book/lore/items`, {
    data: {
      id: 'wen-zhaoye',
      name: '温照野',
      type: 'character',
      enabled: true,
      importance: 'major',
      brief_description: '港城巡夜司执事，擅长潮汐剑术。',
      content: '常驻港城旧码头。',
    },
  })
  expect(createLoreRes.ok(), await createLoreRes.text()).toBe(true)

  const story = await createStartedStory(request, 'Inline Edit Game Story')
  const initialSnap = await getStorySnapshot(request, story.id)
  const gameOriginal = '夜色沉沉，温照野走向栈桥，开口说：“潮声不对。”'
  const patchTurnRes = await request.patch(`/api/interactive/stories/${encodeURIComponent(story.id)}/turns/${encodeURIComponent(initialSnap.current_turn.id)}/narrative`, {
    data: {
      branch_id: 'main',
      expected_narrative: initialSnap.current_turn.narrative,
      narrative: gameOriginal,
    },
  })
  expect(patchTurnRes.ok(), await patchTurnRes.text()).toBe(true)

  // 1. Writing editor: SelectionToolbar -> AI Edit -> CJK word diff -> Discard -> Cmd+J -> Refine -> Accept -> Single Undo/Redo
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '写作', exact: true }).click()
  await page.getByText('港口夜雾', { exact: false }).first().click()

  const editor = page.locator('.ProseMirror:visible').first()
  await expect(editor).toContainText(originalSentence)

  await selectTextInEditor(editor, originalSentence)
  const aiEditBtn = page.getByRole('button', { name: 'AI 改写', exact: true })
  await expect(aiEditBtn).toBeVisible()
  await aiEditBtn.click()

  const popover = page.getByTestId('inline-edit-popover')
  await expect(popover).toBeVisible()
  // Matched Lore entity from the selected sentence is auto-detected.
  await expect(popover).toContainText('资料 · 1')
  // Target range stays highlighted non-destructively (composes cleanly with Lore Lens & dialogue decorations).
  await expect(editor.locator('[data-inline-edit-target="true"]').first()).toBeVisible()

  await popover.getByLabel('改写指令').fill('增强压迫感 E2E_INLINE_WRITING_REWRITE')
  await popover.getByRole('button', { name: '生成', exact: true }).click()

  const preview = popover.getByTestId('inline-edit-preview')
  await expect(preview).toContainText('按住剑柄')
  // CJK word-level diff preserves unchanged prefix and highlights only changed words.
  await expect(preview.locator('del.nova-inline-edit-del', { hasText: '低声' })).toBeVisible()
  await expect(preview.locator('ins.nova-inline-edit-add', { hasText: '按住剑柄' })).toBeVisible()

  // Capture desktop dark-theme visual check of the inline diff popover.
  await page.screenshot({ path: test.info().outputPath('inline-edit-writing-dark.png') })

  // Follow-up refinement inside the same popover + toggle Diff/Result view.
  await popover.getByLabel('继续微调（如：再短一半、别改最后一句）…').fill('更克制 E2E_INLINE_WRITING_REFINE')
  await popover.getByRole('button', { name: '微调', exact: true }).click()
  await expect(preview.locator('ins.nova-inline-edit-add', { hasText: '冷' })).toBeVisible()
  await popover.getByRole('button', { name: '结果', exact: true }).click()
  await expect(preview).toContainText('冷声道')

  // Discard with Escape leaves the document completely unchanged.
  await popover.getByRole('button', { name: '放弃', exact: true }).click()
  await expect(popover).toHaveCount(0)
  await expect(editor).toContainText(originalSentence)

  // Trigger via Cmd/Ctrl+J, generate, and Accept replacement.
  await selectTextInEditor(editor, originalSentence)
  await page.keyboard.press(`${shortcutModifier}+KeyJ`)
  await expect(popover).toBeVisible()
  await popover.getByLabel('改写指令').fill('增强压迫感 E2E_INLINE_WRITING_REWRITE')
  await popover.getByRole('button', { name: '生成', exact: true }).click()
  await expect(preview).toContainText('按住剑柄')
  await popover.getByRole('button', { name: '替换', exact: true }).click()

  await expect(popover).toHaveCount(0)
  await expect(editor).toContainText('温照野站在栈桥尽头，按住剑柄说：“潮汐要涨了。”')
  await expect(editor).toContainText('远处传来钟声。')

  // Single Cmd/Ctrl+Z atomically reverts the entire AI rewrite back to the original sentence.
  await page.keyboard.press(`${shortcutModifier}+KeyZ`)
  await expect(editor).toContainText(originalSentence)
  // Single Redo restores the AI rewrite.
  await page.keyboard.press(`${shortcutModifier}+Shift+KeyZ`)
  await expect(editor).toContainText('温照野站在栈桥尽头，按住剑柄说：“潮汐要涨了。”')

  // Save chapter and verify disk persistence + zero Agent Chat / ChangeSet pollution.
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content)
    .toContain('温照野站在栈桥尽头，按住剑柄说：“潮汐要涨了。”')

  const sessionMessagesRes = await request.get('/api/session/messages')
  expect(sessionMessagesRes.ok()).toBe(true)
  const sessionMessagesPayload = await sessionMessagesRes.json() as { messages?: unknown[] }
  expect(sessionMessagesPayload.messages ?? []).toHaveLength(0)

  const changeGroupsRes = await request.get(`/api/projects/${encodeURIComponent(book.projectId)}/changes/groups?status=pending`)
  expect(changeGroupsRes.ok()).toBe(true)
  const changeGroupsPayload = await changeGroupsRes.json() as { groups?: unknown[] }
  expect(changeGroupsPayload.groups ?? []).toHaveLength(0)

  // 2. Lore content editor: select prose -> AI Edit -> accept -> persisted.
  await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
  await page.getByRole('button', { name: /温照野/ }).first().click()
  const loreEditor = page.locator('.ProseMirror:visible').first()
  await expect(loreEditor).toContainText('常驻港城旧码头。')

  await selectTextInEditor(loreEditor, '常驻港城旧码头。')
  await page.getByRole('button', { name: 'AI 改写', exact: true }).click()
  await expect(popover).toBeVisible()
  await popover.getByLabel('改写指令').fill('补充职责 E2E_INLINE_LORE_REWRITE')
  await popover.getByRole('button', { name: '生成', exact: true }).click()
  await expect(popover.getByTestId('inline-edit-preview')).toContainText('暗中掌管巡夜司潮汐渡口')
  await popover.getByRole('button', { name: '替换', exact: true }).click()
  await expect(loreEditor).toContainText('常驻港城旧码头，暗中掌管巡夜司潮汐渡口。')

  // 3. Game turn reply revision dialog: AI Edit embedded inside dialog without touching Game state.
  await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
  await expect(page.getByText('温照野走向栈桥', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: '编辑 AI 回复', exact: true }).first().click()

  const replyDialog = page.getByRole('dialog', { name: '编辑 AI 回复' })
  await expect(replyDialog).toBeVisible()
  const replyTextarea = replyDialog.locator('textarea')
  await replyTextarea.evaluate((el) => {
    const textarea = el as HTMLTextAreaElement
    const start = textarea.value.indexOf('温照野走向栈桥')
    textarea.focus()
    textarea.setSelectionRange(start, textarea.value.length)
  })
  await replyDialog.getByRole('button', { name: 'AI 改写', exact: true }).click()

  const dialogPopover = replyDialog.getByTestId('inline-edit-popover')
  await expect(dialogPopover).toBeVisible()
  await expect(dialogPopover).toContainText('资料 · 1')
  await dialogPopover.getByLabel('改写指令').fill('润色回合 E2E_INLINE_GAME_REWRITE')
  await dialogPopover.getByRole('button', { name: '生成', exact: true }).click()
  await expect(dialogPopover.getByTestId('inline-edit-preview').locator('ins.nova-inline-edit-add', { hasText: '压低声音道' })).toBeVisible()
  await expect(dialogPopover.getByTestId('inline-edit-preview').locator('ins.nova-inline-edit-add', { hasText: '变了' })).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('inline-edit-game-dialog.png') })

  await dialogPopover.getByRole('button', { name: '替换', exact: true }).click()
  await expect(replyTextarea).toHaveValue('夜色沉沉，温照野提灯踏上湿滑栈桥，压低声音道：“潮声变了。”')
  await replyDialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(replyDialog).toHaveCount(0)

  const afterSnap = await getStorySnapshot(request, story.id)
  expect(afterSnap.current_turn.narrative).toBe('夜色沉沉，温照野提灯踏上湿滑栈桥，压低声音道：“潮声变了。”')
  expect(afterSnap.state).toEqual(initialSnap.state)

  // 4. Light theme & narrow viewport sanity check on Writing inline edit popover.
  await page.emulateMedia({ colorScheme: 'light' })
  await page.evaluate(() => {
    document.documentElement.setAttribute('data-theme', 'light')
  })
  await sidebar.getByRole('button', { name: '写作', exact: true }).click()
  await selectTextInEditor(editor, '远处传来钟声。')
  await page.getByRole('button', { name: 'AI 改写', exact: true }).click()
  await expect(popover).toBeVisible()
  await popover.getByLabel('改写指令').fill('扩写 E2E_INLINE_WRITING_INSERT')
  await popover.getByRole('button', { name: '生成', exact: true }).click()
  await expect(popover.getByTestId('inline-edit-preview')).toContainText('海雾顺着石阶漫上来')
  await page.screenshot({ path: test.info().outputPath('inline-edit-writing-light.png') })
  await popover.getByRole('button', { name: '放弃', exact: true }).click()

  await page.setViewportSize({ width: 390, height: 844 })
  await selectTextInEditor(editor, '远处传来钟声。')
  await page.keyboard.press(`${shortcutModifier}+KeyJ`)
  await expect(popover).toBeVisible()
  const box = await popover.boundingBox()
  expect(box && box.x >= 0 && box.x + box.width <= 390).toBe(true)
  await page.screenshot({ path: test.info().outputPath('inline-edit-writing-mobile.png') })
  await popover.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(popover).toHaveCount(0)
})
