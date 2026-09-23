import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

for (const product of ['writing', 'game'] as const) {
  // Both products exercise both themes and locales; theme and locale do not
  // change the compaction protocol, so avoid their full Cartesian product.
  for (const [theme, language] of [['dark', 'zh-CN'], ['light', 'en-US']] as const) {
    test(`${product} restores runtime compaction and plan cards in ${theme} ${language}`, async ({ page, request }) => {
      await createAndOpenBook(request, `Compaction ${product} ${theme} ${language}`)
      if (product === 'game') await createStartedStory(request, 'Runtime context')
      const settings = await (await request.get('/api/settings')).json()
      await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({
        json: { ...settings, effective: { ...settings.effective, theme, language } },
      }))
      const event = { id: 'runtime-compact', role: 'context_compaction', status: 'success', phase: 'model_step', runtime_managed: true }
      const taskText = language === 'zh-CN' ? '核对已保存章节与验收要求，确认长文本在窄屏中完整换行。'.repeat(4) : 'Verify the saved chapter and acceptance criteria, including readable long task text on narrow screens. '.repeat(4)
      const plan = { schema: 'agent.todo.v1', items: [{ id: '1', text: taskText, status: 'in_progress' }] }
      const planEvent = { id: 'runtime-plan', role: 'todo_updated', content: JSON.stringify(plan) }
      if (product === 'writing') {
        await page.route('**/session/messages?*', route => route.fulfill({ json: {
          messages: [{ id: event.id, role: 'assistant', parts: [{ type: 'data-agent-context-compaction', id: event.id, data: event }] }, { id: planEvent.id, role: 'assistant', parts: [{ type: 'data-agent-todo', id: planEvent.id, data: plan }] }],
          page: { has_more: false, total: 2 },
        } }))
      } else {
        await page.route('**/api/interactive/stories/*/snapshot**', async route => {
          const response = await route.fetch()
          const snapshot = await response.json()
          await route.fulfill({ response, json: { ...snapshot, pending_display_events: [event, planEvent] } })
        })
      }
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 900 })
        if (width === 1440) await page.goto('/')
        else await page.reload()
        if (width === 1440) {
          const sidebar = page.getByLabel(/工作台侧边栏|Workbench sidebar/)
          await sidebar.getByRole('button', { name: product === 'game' ? /^(游戏|Game)$/ : /^(写作|Writing)$/, exact: true }).click()
        }
        if (product === 'writing') {
          if (width === 390) await page.getByRole('tab', { name: 'Agent', exact: true }).click()
          else {
            const showAgent = page.getByRole('button', { name: /^(显示创作 Agent|Show Writing Agent)$/, exact: true })
            const editor = page.getByTestId('right').locator('[contenteditable="true"]').filter({ visible: true })
            await expect(showAgent.or(editor)).toHaveCount(1)
            if (await showAgent.count()) await showAgent.click()
          }
        }
        const text = language === 'zh-CN'
          ? '运行时已压缩工作上下文，会话历史完整保留。'
          : 'The runtime compacted its working context. Conversation history is preserved.'
        await expect(page.getByText(text, { exact: true })).toHaveCount(1)
        await expect(page.getByText(text, { exact: true })).toBeVisible()
        await expect(page.getByText(language === 'zh-CN' ? '自动压缩' : 'Automatic compaction', { exact: true })).toBeVisible()
        await expect(page.getByText(/没有压缩摘要内容|No compaction summary content/)).toHaveCount(0)
        await expect(page.locator('li').getByText(taskText.trim(), { exact: true })).toBeVisible()
        await expect(page.getByText('0/1', { exact: true })).toBeVisible()
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
        await page.screenshot({ path: test.info().outputPath(`${product}-${theme}-${language}-${width}.png`), animations: 'disabled' })
      }
    })
  }
}
