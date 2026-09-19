import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { expect, test } from '../support/fixtures'
import { createAgentChatSession, createAndOpenBook, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent } from '../support/agent-chat'

test.afterEach(async ({ request }) => {
  const current = await (await request.get('/api/settings')).json()
  const response = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user,
    changes: { agent_runtimes: { ide: { selected: 'native' }, general: { selected: 'native' } } } } })
  expect(response.ok(), await response.text()).toBe(true)
})

for (const engine of ['codex', 'claude'] as const) {
  for (const kind of ['writing', 'general'] as const) {
    for (const theme of ['dark', 'light']) {
      test(`${kind} composer saves ${engine} model and effort in ${theme}`, async ({ page, request }) => {
        test.setTimeout(90_000)
        const current = await (await request.get('/api/settings')).json()
        const role = kind === 'writing' ? 'ide' : 'general'
        const model = { model: 'first-model', effort: 'medium' }
        const seeded = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user,
          changes: { theme, agent_runtimes: { [role]: { selected: engine, [engine]: model } } } } })
        expect(seeded.ok(), await seeded.text()).toBe(true)
        let projectId: string
        let sessionId: string
        if (kind === 'writing') {
          projectId = (await createAndOpenBook(request, `${engine} composer ${theme}`)).projectId
          const response = await request.post('/api/sessions', { data: { title: `${engine} selection` } })
          expect(response.ok(), await response.text()).toBe(true)
          sessionId = (await response.json()).id
          expect((await request.post('/api/sessions/switch', { data: { id: sessionId } })).ok()).toBe(true)
        } else {
          const directory = await mkdtemp(path.resolve('test-results', 'runtime', `${engine}-composer-`))
          projectId = (await registerAgentChatProject(request, directory)).id
          sessionId = (await createAgentChatSession(request, projectId, `${engine} selection`)).id
        }
        await page.route(`**/api/agent-runtimes/${engine}/models`, route => route.fulfill({ json: { items: [
          { id: 'first-model', display_name: 'First model', efforts: ['medium', 'high'] },
          { id: 'second-model', display_name: `Second model ${'long name '.repeat(15)}`, efforts: ['low'] },
        ] } }))
        const configPath = `/api/projects/${projectId}/conversation-config?${new URLSearchParams({ mode: kind === 'writing' ? 'writing' : 'agent_chat', session_id: sessionId })}`
        const readSelection = async () => (await (await request.get(configPath)).json()).runtime[engine]
        const openComposer = async () => {
          if (kind === 'writing') await openWritingAgent(page)
          else { await openAgentChatWorkbench(page); await openAgentChatSession(page, projectId, `${engine} selection`) }
        }
        await page.goto('/')
        await openComposer()
        const trigger = page.locator('[data-model-profile-trigger]').filter({ visible: true })
        await expect(trigger).toHaveAttribute('data-current-model', 'First model')
        const permissions = page.getByRole('button', { name: /Agent 安全模式: 工作区写入/ })
        if (engine === 'codex') await expect(permissions).toBeVisible()
        for (const width of [1440, 390]) {
          await page.setViewportSize({ width, height: 960 })
          if (kind === 'writing' && width === 390) await page.getByRole('tab', { name: 'Agent', exact: true }).click()
          await trigger.click()
          await expect(page.getByRole('menuitem', { name: 'First model', exact: true })).toBeVisible()
          expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
          await page.screenshot({ path: test.info().outputPath(`${engine}-composer-${width}.png`) })
          await page.getByRole('menuitem', { name: 'First model', exact: true }).click()
          await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0)
          if (engine === 'codex') {
            await permissions.click()
            await expect(page.getByRole('menuitem', { name: /^只读/ })).toBeVisible()
            await expect(page.getByRole('menuitem', { name: /^完全访问/ })).toBeVisible()
            expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
            await page.screenshot({ path: test.info().outputPath(`permissions-${width}.png`) })
            await page.getByRole('menuitem', { name: /^工作区写入/ }).click()
            await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0)
          }
        }
        await trigger.click()
        await page.getByRole('button', { name: '高', exact: true }).click()
        await expect.poll(readSelection).toEqual({ model: 'first-model', effort: 'high' })
        await expect(trigger).toHaveAttribute('data-current-thinking-level', 'high')
        await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0)
        await trigger.click()
        await page.getByRole('button', { name: '默认', exact: true }).click()
        await expect.poll(readSelection).toEqual({ model: 'first-model' })
        await expect(trigger).toHaveAttribute('data-current-thinking-level', 'default')
        await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0)
        await trigger.click()
        await page.getByRole('menuitem', { name: /^Second model/ }).click()
        await expect.poll(readSelection).toEqual({ model: 'second-model' })
        await expect(trigger).toHaveAttribute('data-current-model', /^Second model/)
        await page.setViewportSize({ width: 1440, height: 960 })
        await page.reload()
        await openComposer()
        await expect(trigger).toHaveAttribute('data-current-model', /^Second model/)
        await trigger.click()
        await expect(page.getByRole('button', { name: '低', exact: true })).toBeVisible()
        await expect(page.getByRole('button', { name: '高', exact: true })).toHaveCount(0)
        await page.getByRole('menuitem', { name: /^Second model/ }).click()
        if (engine === 'codex') {
          await permissions.click()
          await page.getByRole('menuitem', { name: /^只读/ }).click()
          await expect.poll(readSelection).toEqual({ model: 'second-model', sandbox: 'read-only' })
          await page.reload()
          await openComposer()
          await expect(page.getByRole('button', { name: /Agent 安全模式: 只读/ })).toBeVisible()
          await trigger.click()
          await page.getByRole('menuitem', { name: 'First model', exact: true }).click()
          await expect.poll(readSelection).toEqual({ model: 'first-model', sandbox: 'read-only' })
        }
        const saved = await (await request.get('/api/settings')).json()
        expect(saved.user.agent_runtimes[role][engine]).toEqual(model)
      })
    }
  }

}
