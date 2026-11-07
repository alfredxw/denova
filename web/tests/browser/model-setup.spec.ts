import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

for (const theme of ['dark', 'light']) {
  test(theme === 'light' ? 'renders empty language-model settings at wide and narrow widths in light' : 'configures and restores a first language model in dark', async ({ page, request }) => {
    test.setTimeout(60_000)
    await createAndOpenBook(request, `Model Setup ${theme}`)
    const current = await (await request.get('/api/settings')).json()
    const cleared = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: current.revisions.user,
      changes: { model_endpoints: [], model_profiles: [], agent_models: null, theme, language: 'zh-CN' },
    } })
    expect(cleared.ok(), await cleared.text()).toBe(true)
    const empty = await cleared.json()
    expect(empty.effective.model_endpoints ?? []).toEqual([])
    expect(empty.effective.model_profiles ?? []).toEqual([])

    await page.addInitScript(() => localStorage.setItem('nova:onboarding:v1', JSON.stringify({ version: 1, skipped: true })))
    await page.goto('/')
    const sidebar = page.getByLabel('工作台侧边栏')
    for (const destination of ['写作', '游戏']) {
      await sidebar.getByRole('button', { name: destination, exact: true }).click()
      await sidebar.getByRole('button', { name: '设置', exact: true }).click()
      await page.locator('.nova-settings-view nav').getByRole('button', { name: '语言模型', exact: true }).click()
      await expect(page.getByText('添加连接，填写服务商、API 地址和密钥，再添加模型。', { exact: true })).toBeVisible()
      await expect(page.getByText('默认（尚未配置）', { exact: true })).toBeVisible()
      await expect(page.getByText('deepseek-v4-pro', { exact: true })).toHaveCount(0)
    }
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await page.getByText('添加连接，填写服务商、API 地址和密钥，再添加模型。', { exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`empty-models-${theme}.png`) })
    await page.setViewportSize({ width: 720, height: 900 })
    await page.getByText('添加连接，填写服务商、API 地址和密钥，再添加模型。', { exact: true }).scrollIntoViewIfNeeded()
    await expect(page.getByText('添加连接，填写服务商、API 地址和密钥，再添加模型。', { exact: true })).toBeVisible()
    await page.screenshot({ path: test.info().outputPath(`empty-models-narrow-${theme}.png`) })
    await page.setViewportSize({ width: 1280, height: 900 })
    // Empty-state rendering covers both themes; provider editing, persistence
    // and English labels are independent of theme and run once below.
    if (theme === 'light') return

    await page.getByRole('button', { name: '添加连接', exact: true }).first().click()
    const protocolPicker = page.getByRole('combobox', { name: 'API 协议', exact: true })
    await protocolPicker.click()
    await page.getByRole('option', { name: 'Anthropic Messages', exact: true }).click()
    await page.getByRole('combobox', { name: '服务商', exact: true }).click()
    await expect(page.getByRole('option').first()).toHaveText('自定义')
    await expect(page.getByRole('option', { name: /openai-compatible/i })).toHaveCount(0)
    await expect(page.getByRole('option', { name: 'Alibaba Cloud Model Studio', exact: true })).toBeVisible()
    await page.screenshot({ path: test.info().outputPath(`custom-provider-zh-${theme}.png`) })
    await page.getByRole('option', { name: '自定义', exact: true }).click()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.getByRole('combobox', { name: '服务商', exact: true }).click()
    await expect(page.getByRole('option').first()).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`custom-provider-zh-narrow-${theme}.png`) })
    await page.getByRole('option', { name: '自定义', exact: true }).click()
    await page.setViewportSize({ width: 1280, height: 900 })
    await expect(page.getByRole('combobox', { name: '服务商', exact: true })).toHaveText('自定义')
    await expect(protocolPicker).toHaveText('Anthropic Messages')
    await page.getByPlaceholder('Base URL', { exact: true }).fill(`http://127.0.0.1:${process.env.DENOVA_E2E_MODEL_PORT || '18081'}/v1`)
    await page.getByRole('button', { name: '手动添加', exact: true }).click()
    await page.getByPlaceholder('输入模型名，或从候选列表选择').fill('my-first-model')
    await expect.poll(async () => {
      const saved = await (await request.get('/api/settings')).json()
      return { model: saved.user.model_profiles?.[0]?.model, selected: saved.user.agent_models?.default?.profile_id }
    }).toEqual({ model: 'my-first-model', selected: 'my-first-model' })
    for (const [label, protocol] of [
      ['Chat Completions', 'openai-chat-completions'],
      ['Responses API', 'openai-responses'],
      ['Anthropic Messages', 'anthropic-messages'],
      ['Gemini 原生协议', 'google-generative-ai'],
    ]) {
      await protocolPicker.click()
      await page.getByRole('option', { name: label, exact: true }).click()
      await expect.poll(async () => {
        const saved = await (await request.get('/api/settings')).json()
        return { provider: saved.user.model_endpoints?.[0]?.provider, protocol: saved.user.model_endpoints?.[0]?.protocol }
      }).toEqual({ provider: 'openai-compatible', protocol })
    }
    await expect(page.getByText('所有更改均已保存', { exact: true })).toBeVisible()
    await page.reload()
    await expect(sidebar.getByRole('button', { name: '设置', exact: true })).toHaveAttribute('aria-current', 'page')
    await page.locator('.nova-settings-view nav').getByRole('button', { name: '语言模型', exact: true }).click()
    await expect(page.getByText('my-first-model', { exact: true }).first()).toBeVisible()
    await expect(page.getByText('添加连接，填写服务商、API 地址和密钥，再添加模型。', { exact: true })).toHaveCount(0)
    const connection = page.locator('[data-settings-level="connection"]').first()
    await connection.getByRole('button').first().click()
    await expect(protocolPicker).toHaveText('Gemini 原生协议')
    await expect(page.getByRole('combobox', { name: '服务商', exact: true })).toHaveText('自定义')

    const saved = await (await request.get('/api/settings')).json()
    const localized = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: saved.revisions.user, changes: { language: 'en-US' },
    } })
    expect(localized.ok(), await localized.text()).toBe(true)
    await page.reload()
    for (const destination of ['Writing', 'Game']) {
      const englishSidebar = page.getByLabel('Workbench sidebar')
      await englishSidebar.getByRole('button', { name: destination, exact: true }).click()
      await englishSidebar.getByRole('button', { name: 'Settings', exact: true }).click()
      await page.locator('.nova-settings-view nav').getByRole('button', { name: 'Language Models', exact: true }).click()
      const connectionTrigger = connection.getByRole('button').first()
      if (await connectionTrigger.getAttribute('aria-expanded') === 'false') await connectionTrigger.click()
      const providerPicker = page.getByRole('combobox', { name: 'Provider', exact: true })
      await expect(providerPicker).toHaveText('Custom Endpoint')
      await expect(page.getByRole('combobox', { name: 'API protocol', exact: true })).toHaveText('Gemini native API')
      await providerPicker.click()
      await expect(page.getByRole('option').first()).toHaveText('Custom Endpoint')
      await expect(page.getByRole('option', { name: /openai-compatible|自定义/i })).toHaveCount(0)
      await page.screenshot({ path: test.info().outputPath(`custom-provider-en-${destination}-${theme}.png`) })
      await page.getByRole('option', { name: 'Custom Endpoint', exact: true }).click()
    }
  })
}
