import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'
import zhRuntime from '../../src/i18n/locales/zh-CN/agentRuntime'

for (const engine of ['codex', 'claude'] as const) {
  for (const theme of ['dark', 'light'] as const) {
    test(`${engine} API profiles and new-conversation defaults work without CLI login in ${theme}`, async ({ page, request, browserDiagnostics }) => {
      test.setTimeout(90_000)
      browserDiagnostics.allow(/console\.error: Failed to load resource:.*409.*\/api\/agent-runtimes\/(codex|claude)\/models/)
      const before = await (await request.get('/api/settings')).json()
      const profileID = `${engine}-api`
      const label = `API ${engine} ${'long model name '.repeat(8)}`.trim()
      const save = async (changes: Record<string, unknown>) => {
        const current = await (await request.get('/api/settings')).json()
        const result = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user, changes } })
        expect(result.ok(), await result.text()).toBe(true)
      }
      try {
        await save({ theme, language: 'zh-CN', agent_runtimes: { ide: { selected: engine, [engine]: { model: 'cli-model' } } },
          model_endpoints: [...before.user.model_endpoints, { id: 'runtime-api', provider: 'openai-compatible', protocol: engine === 'codex' ? 'openai-responses' : 'anthropic-messages', base_url: 'http://127.0.0.1:1/v1', api_key: 'fixture-only' }],
          model_profiles: [...before.user.model_profiles, { id: profileID, endpoint_id: 'runtime-api', model: 'gateway-model', name: label }],
        })
        const book = await createAndOpenBook(request, `${engine} API ${theme}`)
        const catalogResponse = await request.get('/api/agent-runtimes')
        expect(catalogResponse.ok()).toBe(true)
        const catalog = await catalogResponse.json()
        await page.route('**/api/agent-runtimes', async route => {
          await route.fulfill({ json: { items: catalog.items.map((item: { id: string }) => item.id === engine ? { ...item, status: 'auth_required' } : item) } })
        })
        await page.route(`**/api/agent-runtimes/${engine}/models`, route => route.fulfill({ status: 409, json: { error: 'Sign-in required' } }))
        await page.goto('/')
        await page.getByRole('button', { name: 'Agents', exact: true }).click()
        const runtime = page.locator('[data-agent-configuration-section="runtime"]')
        await runtime.getByRole('radio', { name: 'Denova 模型', exact: true }).click()
        const picker = runtime.getByRole('combobox', { name: '引擎模型', exact: true })
        await expect(picker).toBeEnabled()
        await picker.click()
        await page.getByRole('option', { name: label, exact: true }).click()
        await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide[engine]).toEqual({ profile_id: profileID })
        await expect(runtime.getByText(/使用 Denova 模型档案的 API 连接/)).toBeVisible()
        for (const width of [1440, 390]) {
          await page.setViewportSize({ width, height: 960 })
          await picker.click()
          await expect(page.getByRole('option', { name: label, exact: true })).toBeVisible()
          expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
          await page.screenshot({ path: test.info().outputPath(`api-picker-${width}.png`) })
          await page.keyboard.press('Escape')
        }
        await page.setViewportSize({ width: 1440, height: 960 })
        const created = await request.post('/api/sessions', { data: { title: 'API conversation' } })
        expect(created.ok(), await created.text()).toBe(true)
        const sessionId = (await created.json()).id
        const configURL = `/api/projects/${book.projectId}/conversation-config?mode=writing&session_id=${sessionId}`
        const original = await (await request.get(configURL)).json()
        expect(original.runtime).toEqual({ kind: engine, [engine]: { profile_id: profileID } })
        await page.reload()
        await openWritingAgent(page)
        await page.getByRole('button', { name: '会话历史', exact: true }).click()
        await page.getByRole('option', { name: '切换到会话 API conversation', exact: true }).click()
        const trigger = page.locator('[data-model-profile-trigger]').filter({ visible: true })
        await trigger.click()
        await page.getByRole('menuitem', { name: '配置', exact: true }).click()
        await expect(runtime.getByRole('group', { name: '模型来源' })).toBeVisible()
        const enginePicker = runtime.getByRole('combobox', { name: '执行引擎', exact: true })
        await enginePicker.click()
        await page.getByRole('option', { name: 'Native', exact: true }).click()
        await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_runtimes.ide.selected).toBe('native')
        await expect(runtime.getByText(zhRuntime['agentRuntime.defaultsOnly'], { exact: true })).toBeVisible()
        expect(await (await request.get(configURL)).json()).toMatchObject({ revision: original.revision, runtime: original.runtime })
        for (const width of [1440, 390]) {
          await page.setViewportSize({ width, height: 960 })
          await enginePicker.scrollIntoViewIfNeeded()
          expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
          await page.screenshot({ path: test.info().outputPath(`runtime-defaults-${width}.png`) })
        }
        await page.setViewportSize({ width: 1440, height: 960 })
        await openWritingAgent(page)
        await trigger.click()
        await expect(page.getByRole('menuitem', { name: `运行时：${engine === 'codex' ? 'Codex' : 'Claude Code'}`, exact: true })).toBeVisible()
        const apiOption = page.getByRole('menuitem', { name: `API · ${label}`, exact: true })
        await expect(apiOption).toBeEnabled()
        await apiOption.click()
        await expect(trigger).toHaveAttribute('data-current-model', label)
        await trigger.click()
        await expect(page.getByRole('button', { name: '高', exact: true })).toHaveCount(0)
        await page.keyboard.press('Escape')
        const saved = await (await request.get(configURL)).json()
        expect(saved.runtime[engine]).toEqual({ profile_id: profileID })
        expect(JSON.stringify(saved)).not.toContain('fixture-only')
        const newSession = await request.post('/api/sessions', { data: { title: 'New Native conversation' } })
        expect(newSession.ok(), await newSession.text()).toBe(true)
        const newSessionId = (await newSession.json()).id
        const newConfig = await (await request.get(`/api/projects/${book.projectId}/conversation-config?mode=writing&session_id=${newSessionId}`)).json()
        expect(newConfig.runtime?.kind ?? 'native').toBe('native')
        await page.reload()
        await openWritingAgent(page)
        await page.getByRole('button', { name: '会话历史', exact: true }).click()
        await page.getByRole('option', { name: '切换到会话 New Native conversation', exact: true }).click()
        await trigger.click()
        await expect(page.getByRole('menuitem', { name: '运行时：Native', exact: true })).toBeVisible()
      } finally {
        await save({ theme: before.user.theme, language: 'zh-CN', agent_runtimes: { ide: { selected: 'native' } }, model_endpoints: before.user.model_endpoints, model_profiles: before.user.model_profiles })
      }
    })
  }
}
