import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

for (const product of ['writing', 'game'] as const) {
  for (const theme of ['dark', 'light'] as const) {
    for (const width of [390, 1440]) {
      test(`${product} paused verification remains usable in ${theme} at ${width}px`, async ({ page, request }) => {
        await createAndOpenBook(request, `Recovery ${product} ${theme} ${width}`)
        if (product === 'game') await createStartedStory(request, 'A preserved story')
        else {
          const created = await request.post('/api/sessions', { data: { title: 'Preserved Writing session' } })
          expect(created.ok()).toBe(true)
          const { id } = await created.json()
          expect((await request.post('/api/sessions/switch', { data: { id } })).ok()).toBe(true)
        }
        const english = theme === 'light'
        const language = english ? 'en-US' : 'zh-CN'
        const settings = await (await request.get('/api/settings')).json()
        await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({ json: {
          ...settings, effective: { ...settings.effective, theme, language },
        } }))
        await page.addInitScript(({ language }) => {
          localStorage.setItem('nova.locale.configured', language)
          localStorage.setItem('nova:onboarding:v1', JSON.stringify({ version: 1, skipped: true }))
        }, { language })
        await page.setViewportSize({ width, height: 960 })
        let cancelled = false
        const pendingAsk = {
          schema: 'ask.pending.v1', id: 'verify-preserved', status: 'pending', tool_call_id: 'original-execution',
          agent_kind: product === 'writing' ? 'ide' : 'interactive_story', allow_other: false,
          verification: { execution_id: 'original-execution', tool: 'publish_document', arguments: {
            title: 'A long document title / 一份需要保留的长标题 '.repeat(5),
            destination: 'chapter-'.repeat(60),
          } },
          questions: [{ id: 'effect', question: 'Diagnostic host text', options: [
            { id: 'executed', label: 'Executed' }, { id: 'not_executed', label: 'Not executed' }, { id: 'unknown', label: 'Unknown' },
          ] }],
        }
        let activeReads = 0
        await page.route(/\/api\/(?:projects\/[^/]+\/agent-chat\/|interactive\/)?chat\/active(?:\?|$)/, route => {
          activeReads++
          return route.fulfill({ json: cancelled ? { active: false, phase: 'idle' } : {
            active: false, phase: 'suspended', cursor: 7, recovery_paused: true, runtime_recoverable: true,
            active_operation_id: 'original-run', active_command_id: 'original-input', pending_ask: pendingAsk,
            recovery_actions: [
              { kind: 'resume', action_id: '7', operation_id: 'original-run', command_id: 'original-input' },
              { kind: 'abort', action_id: '7', operation_id: 'original-run', command_id: 'original-input' },
            ],
          } })
        })
        const resolutions: Record<string, unknown>[] = []
        await page.route(/\/api\/(?:projects\/[^/]+\/agent-chat\/session|session|interactive\/chat)\/asks\/verify-preserved\/(?:answer|cancel)$/, route => {
          const body = route.request().postDataJSON() as Record<string, unknown>
          resolutions.push(body)
          cancelled = body.reason === 'task_aborted'
          return route.fulfill({ json: { schema: 'ask.result.v1', id: 'verify-preserved', status: cancelled ? 'cancelled' : 'pending' } })
        })
        const executionRequests: string[] = []
        page.on('request', value => {
          if (value.method() === 'POST' && /\/chat(?:\/recovery)?$/.test(new URL(value.url()).pathname)) executionRequests.push(value.url())
        })
        await page.goto('/')
        const destination = product === 'writing' ? (english ? 'Writing' : '写作') : (english ? 'Game' : '游戏')
        if (width < 800) {
          await page.getByRole('button', { name: english ? 'Navigation' : '导航菜单', exact: true }).click()
          await page.getByRole('dialog').getByRole('button', { name: destination, exact: true }).click()
          if (product === 'writing') await page.getByRole('tab', { name: 'Agent', exact: true }).click()
        } else {
          await page.getByLabel(english ? 'Workbench sidebar' : '工作台侧边栏').getByRole('button', { name: destination, exact: true }).click()
        }
        const card = page.getByRole('region', { name: english ? 'Verify an interrupted operation' : '核实中断的操作' })
        await expect(card).toBeVisible()
        await expect(page.getByRole('button', { name: english ? 'Continue task' : '继续任务', exact: true })).toBeVisible()
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
        expect(activeReads).toBeGreaterThan(0)
        expect(executionRequests).toEqual([])
        await card.getByRole('radio', { name: english ? 'I cannot determine this yet' : '暂时无法确定' }).check()
        await card.getByRole('button', { name: english ? 'Submit' : '提交', exact: true }).click()
        await expect(card.getByRole('alert')).toBeVisible()
        await page.screenshot({ path: test.info().outputPath(`${product}-paused-${theme}-${width}.png`) })
        await card.getByRole('button', { name: english ? 'Cancel task' : '取消任务', exact: true }).click()
        await expect.poll(() => resolutions.at(-1)?.reason).toBe('task_aborted')
        expect(resolutions[0]?.answers).toEqual([{ question_id: 'effect', selected_option_ids: ['unknown'] }])
        expect(executionRequests).toEqual([])
      })
    }
  }
}
