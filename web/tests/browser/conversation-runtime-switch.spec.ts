import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import { createAgentChatSession, createAndOpenBook, createStartedStory, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent } from '../support/agent-chat'

// Engine processes are covered at the application/tool boundary in Go. This
// browser test isolates the composer contract without requiring CLI login.
for (const kind of ['writing', 'general', 'game'] as const) {
  for (const theme of ['dark', 'light'] as const) {
    test(`${kind} switches runtimes without losing draft in ${theme}`, async ({ page, request }) => {
      test.setTimeout(90_000)
      const current = await (await request.get('/api/settings')).json()
      const seeded = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user,
        changes: { theme, agent_runtimes: { ide: { selected: 'native' }, general: { selected: 'native' }, interactive_story: { selected: 'native' } } } } })
      expect(seeded.ok(), await seeded.text()).toBe(true)
      let projectId = (await createAndOpenBook(request, `Runtime ${kind} ${theme}`)).projectId
      let sessionId = ''
      let storyId = ''
      if (kind === 'game') storyId = (await createStartedStory(request, `Runtime ${theme}`)).id
      else if (kind === 'general') {
        projectId = (await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'switch-')))).id
        sessionId = (await createAgentChatSession(request, projectId, 'Runtime switching')).id
      } else {
        const created = await request.post('/api/sessions', { data: { title: 'Runtime switching' } })
        expect(created.ok(), await created.text()).toBe(true)
        sessionId = (await created.json()).id
        expect((await request.post('/api/sessions/switch', { data: { id: sessionId } })).ok()).toBe(true)
      }
      const mode = kind === 'general' ? 'agent_chat' : kind === 'game' ? 'interactive' : 'writing'
      const params = new URLSearchParams({ mode, session_id: sessionId, story_id: storyId, branch_id: 'main' })
      const configPath = `/api/projects/${projectId}/conversation-config`
      let snapshot = await (await request.get(`${configPath}?${params}`)).json()
      expect(snapshot.runtime_capabilities.goal).toBe(kind !== 'game')
      const gameGoalRequests: string[] = []
      page.on('request', request => {
        const url = new URL(request.url())
        if (url.pathname.endsWith('/conversation-goal') && url.searchParams.get('mode') === 'interactive') gameGoalRequests.push(url.href)
      })
      const switches: string[] = []
      await page.route('**/conversation-config**', async route => {
        if (!route.request().url().includes(configPath)) { await route.continue(); return }
        if (route.request().method() === 'PATCH') {
          const body = route.request().postDataJSON()
          expect(body.binding.mode).toBe(kind === 'game' ? 'interactive' : 'agent_chat')
          expect(body.binding.story_id || '').toBe(storyId)
          expect(body.binding.session_id || '').toBe(sessionId)
          expect(Object.keys(body.changes)).toEqual(['runtime'])
          switches.push(body.changes.runtime.kind)
          snapshot = { ...snapshot, revision: snapshot.revision + 1, runtime: body.changes.runtime,
            runtime_capabilities: { cancel: true, goal: kind !== 'game', queue: true, pause: true } }
        }
        await route.fulfill({ json: snapshot })
      })
      await page.route('**/api/agent-runtimes/*/models', route => route.fulfill({ json: { default_id: 'test-external', items: [{ id: 'test-external', display_name: `External model ${'long label '.repeat(12)}`, efforts: ['medium'] }] } }))
      const statuses: Record<string, string> = { native: 'ready', codex: 'ready', claude: 'ready' }
      await page.route('**/api/agent-runtimes/codex/check', route => { statuses.codex = 'ready'; return route.fulfill({ json: { id: 'codex', status: 'ready' } }) })
      await page.route('**/api/agent-runtimes', route => route.fulfill({ json: { items: ['native', 'codex', 'claude'].map(id => ({ id, name_key: `agentRuntime.${id}`, status: statuses[id] })) } }))
      await page.goto('/')
      if (kind === 'writing') await openWritingAgent(page)
      else if (kind === 'general') { await openAgentChatWorkbench(page); await openAgentChatSession(page, projectId, 'Runtime switching') }
      else await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
      // Navigation can leave the previous Writing composer visible until Game
      // hydrates. Wait for the destination editor before entering the draft.
      const editor = kind === 'game' ? page.getByPlaceholder(/你要做什么/)
        : page.locator('[contenteditable="true"]').filter({ visible: true }).last()
      await expect(editor).toBeVisible()
      const trigger = page.locator('[data-model-profile-trigger]').filter({ visible: true })
      await expect(trigger).toBeEnabled()
      await editor.fill('Keep this draft / 保留这段草稿')
      await expect(page.locator('[data-action="send"]').filter({ visible: true })).toBeEnabled()
      if (kind === 'writing' && theme === 'dark') {
        statuses.codex = 'unchecked'
        statuses.claude = 'not_installed'
        await page.setViewportSize({ width: 390, height: 960 })
        await page.getByRole('tab', { name: 'Agent', exact: true }).click()
        await trigger.click()
        await page.getByRole('menuitem', { name: '运行时：Native', exact: true }).click()
        await expect(page.getByRole('menuitem', { name: 'Claude Code', exact: true })).toBeDisabled()
        await expect(page.getByText('未安装', { exact: true })).toBeVisible()
        await page.screenshot({ path: test.info().outputPath('unavailable-runtime.png'), animations: 'disabled' })
        await page.getByRole('menuitem', { name: 'Codex', exact: true }).click()
        await expect(page.getByText('尚未检查', { exact: true })).toHaveCount(0)
        await expect(page.getByRole('menuitem', { name: 'Codex', exact: true })).toBeEnabled()
        expect(switches).toEqual([])
        await page.keyboard.press('Escape')
        // The closing animation retains Radix's focus and pointer layer until
        // unmount. Finish dismissal before resizing or opening another menu.
        await expect(page.getByRole('menu')).toHaveCount(0)
        statuses.claude = 'ready'
      }
      for (const [engine, previous, width] of [['Codex', 'Native', 1440], ['Claude Code', 'Codex', 390], ['Native', 'Claude Code', 390]] as const) {
        await page.setViewportSize({ width, height: 960 })
        if (kind === 'writing' && width === 390) await page.getByRole('tab', { name: 'Agent', exact: true }).click()
        if (kind === 'writing' && width === 1440) await expect(page.getByRole('tab', { name: 'Agent', exact: true })).toHaveCount(0)
        await trigger.click()
        const runtimeLink = page.getByRole('menuitem', { name: `运行时：${previous}`, exact: true })
        await expect(runtimeLink).toBeVisible()
        await expect(page.getByText('切换运行时', { exact: true })).toHaveCount(0)
        expect((await runtimeLink.locator('span').boundingBox())?.height).toBeLessThan(30)
        await page.screenshot({ path: test.info().outputPath(`${kind}-${theme}-${width}-model-menu.png`), animations: 'disabled' })
        await runtimeLink.click()
        const option = page.getByRole('menuitem', { name: engine, exact: true })
        await expect(option).toBeVisible()
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
        await expect.poll(async () => {
          const box = await page.locator('[data-slot="dropdown-menu-sub-content"]').boundingBox()
          return Boolean(box && box.x >= 0 && box.x + box.width <= width)
        }).toBe(true)
        await page.screenshot({ path: test.info().outputPath(`${kind}-${theme}-${width}-${engine}.png`), animations: 'disabled' })
        await option.click()
        const currentRuntime = page.getByRole('menuitem', { name: `运行时：${engine}`, exact: true })
        await expect(currentRuntime).toBeVisible()
        // A changed label can render before the submenu finishes closing.
        await expect(page.locator('[data-slot="dropdown-menu-sub-content"]')).toHaveCount(0)
        await currentRuntime.press('Escape')
        await expect(trigger).toHaveAttribute('aria-expanded', 'false')
        await expect(page.getByRole('menu')).toHaveCount(0)
        await expect(editor).toHaveText('Keep this draft / 保留这段草稿')
        await page.getByRole('button', { name: '输入动作', exact: true }).filter({ visible: true }).click()
        await expect(page.getByText('切换运行时', { exact: true })).toHaveCount(0)
        await expect(page.getByRole('menuitem', { name: '上下文分析', exact: true })).toHaveCount(engine === 'Native' ? 1 : 0)
        await expect(page.getByRole('menuitemcheckbox', { name: '目标', exact: true })).toHaveCount(kind === 'game' ? 0 : 1)
        await page.keyboard.press('Escape')
        await expect(page.getByRole('menu')).toHaveCount(0)
      }
      expect(switches).toEqual(['codex', 'claude', 'native'])
      expect(gameGoalRequests).toEqual([])
      await trigger.click()
      await page.getByRole('menuitem', { name: '配置', exact: true }).click()
      await expect(page.getByRole('heading', { name: kind === 'writing' ? '写作 Agent' : kind === 'game' ? '游戏 Agent' : 'General Agent', exact: true })).toBeVisible()
      const runtimeSection = page.locator('[data-agent-configuration-section="runtime"]')
      await expect(runtimeSection.getByRole('combobox', { name: '执行引擎' })).toBeVisible()
      // A rendered destination is not enough: a stale modal pointer lock makes
      // the entire page look frozen even though routing has completed.
      await expect(page.locator('body')).not.toHaveCSS('pointer-events', 'none')
      const enginePicker = runtimeSection.getByRole('combobox', { name: '执行引擎' })
      await enginePicker.click()
      await expect(page.getByRole('option', { name: 'Native', exact: true })).toBeVisible()
      await page.keyboard.press('Escape')
      await runtimeSection.getByRole('button', { name: '运行时', exact: true }).click()
      await expect(enginePicker).toBeHidden()
    })
  }
}
