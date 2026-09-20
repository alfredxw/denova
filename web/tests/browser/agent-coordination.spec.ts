import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

for (const product of ['writing', 'game'] as const) {
  for (const theme of ['dark', 'light'] as const) {
    test(`${product} renders Agent coordination in ${theme} on wide and narrow screens`, async ({ page, request }) => {
      await createAndOpenBook(request, `Coordination ${product} ${theme}`)
      if (product === 'game') await createStartedStory(request, 'Coordination history')
      const english = theme === 'light'
      const language = english ? 'en-US' : 'zh-CN'
      const settings = await (await request.get('/api/settings')).json()
      await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({ json: {
        ...settings, effective: { ...settings.effective, theme, language },
      } }))
      const ref = { agent: 'researcher', session: 'child-session', run: 'child-run' }
      const longText = 'Review the saved chapter and describe the remaining questions. '.repeat(20)
      const tools = [
        { name: 'send', input: { items: [{ action: 'steer', to: ref, message: longText }] }, output: {
          results: [{ index: 0, outcome: 'accepted', ref, receipt: { command_id: 'steer', cursor: '8' } },
            { index: 1, outcome: 'accepted', ref: { ...ref, run: 'second-run' }, receipt: { command_id: 'another', cursor: '9' } }],
        } },
        { name: 'await', input: { timeout_ms: 0, targets: [{ ref }] }, output: {
          reason: 'attention', results: [{ index: 0, outcome: 'observed', run: { ref, status: 'suspended' }, ready: true, output: { text: longText, incomplete: true } }],
        } },
        { name: 'list_agents', input: { kind: 'definitions' }, output: {
          kind: 'definitions', definitions: [{ agent: 'researcher', description: longText, description_truncated: false }],
        } },
        { name: 'list_agents', input: {}, output: { kind: 'instances', self: { agent: 'parent', session: 'root' }, agents: [] } },
      ]
      const presentation = { call: 'delegation', result: 'delegation' }
      if (product === 'writing') {
        await page.route('**/session/messages?*', route => route.fulfill({ json: {
          messages: tools.map((tool, index) => ({ id: `coordination-${index}`, role: 'assistant', metadata: { tool_presentation: presentation }, parts: [{
            type: 'dynamic-tool', toolName: tool.name, toolCallId: `call-${index}`, state: 'output-available', input: tool.input, output: JSON.stringify(tool.output),
          }] })), page: { has_more: false, total: tools.length },
        } }))
      } else {
        await page.route('**/api/interactive/stories/*/snapshot**', async route => {
          const response = await route.fetch()
          const snapshot = await response.json()
          const turns = snapshot.turns.map((turn: Record<string, unknown>, index: number) => index === snapshot.turns.length - 1 ? { ...turn, display_events: tools.map((tool, index) => ({
            id: `call-${index}`, role: 'tool_call', name: tool.name, args: JSON.stringify(tool.input), result: JSON.stringify(tool.output), status: 'success', tool_presentation: presentation,
          })) } : turn)
          await route.fulfill({ response, json: { ...snapshot, turns } })
        })
      }
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 960 })
        if (width === 1440) {
          await page.goto('/')
          await page.getByLabel(/工作台侧边栏|Workbench sidebar/).getByRole('button', { name: product === 'game' ? /^(游戏|Game)$/ : /^(写作|Writing)$/, exact: true }).click()
        } else await page.reload()
        if (product === 'writing') {
          if (width === 390) await page.getByRole('tab', { name: 'Agent', exact: true }).click()
          else {
            const show = page.getByRole('button', { name: /^(显示创作 Agent|Show Writing Agent)$/, exact: true })
            if (await show.count()) await show.click()
          }
        }
        await page.getByRole('button', { name: /^(执行过程|Execution) · 4 / }).click()
        const headers = page.locator('[data-nova-tool-header]')
        await expect(headers).toHaveCount(4)
        for (const header of await headers.all()) await header.click()
        await expect(page.getByText(english ? 'Paused' : '已暂停', { exact: true })).toBeVisible()
        await expect(page.getByText(english ? 'Available Agent types' : '可用 Agent 类型', { exact: true })).toBeVisible()
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
        await page.screenshot({ path: test.info().outputPath(`${product}-${theme}-${width}.png`), animations: 'disabled' })
      }
    })
  }
}
