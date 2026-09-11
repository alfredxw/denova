import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

for (const theme of ['dark', 'light']) {
  test(`configures a first language model from empty settings in ${theme} theme`, async ({ page, request }) => {
    await createAndOpenBook(request, `Model Setup ${theme}`)
    const current = await (await request.get('/api/settings')).json()
    try {
      const cleared = await request.patch('/api/settings', { data: {
        layer: 'user', base_revision: current.revisions.user,
        changes: { model_endpoints: [], model_profiles: [], agent_models: null, theme },
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
        await expect(page.getByText('尚未配置语言模型。', { exact: false })).toBeVisible()
        await expect(page.getByText('默认（尚未配置）', { exact: true })).toBeVisible()
        await expect(page.getByText('deepseek-v4-pro', { exact: true })).toHaveCount(0)
      }
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await page.getByText('尚未配置语言模型。', { exact: false }).scrollIntoViewIfNeeded()
      await page.screenshot({ path: test.info().outputPath(`empty-models-${theme}.png`) })
      await page.setViewportSize({ width: 720, height: 900 })
      await page.getByText('尚未配置语言模型。', { exact: false }).scrollIntoViewIfNeeded()
      await expect(page.getByText('尚未配置语言模型。', { exact: false })).toBeVisible()
      await page.screenshot({ path: test.info().outputPath(`empty-models-narrow-${theme}.png`) })
      await page.setViewportSize({ width: 1280, height: 900 })

      await page.getByRole('button', { name: '添加连接', exact: true }).first().click()
      await page.getByRole('combobox', { name: '服务商', exact: true }).click()
      await page.getByRole('option', { name: /openai-compatible/i }).click()
      await page.getByPlaceholder('Base URL', { exact: true }).fill('http://127.0.0.1:18081/v1')
      await page.getByRole('button', { name: '手动添加', exact: true }).click()
      await page.getByPlaceholder('输入模型名，或从候选列表选择').fill('my-first-model')
      await expect.poll(async () => {
        const saved = await (await request.get('/api/settings')).json()
        return { model: saved.user.model_profiles?.[0]?.model, selected: saved.user.agent_models?.default?.profile_id }
      }).toEqual({ model: 'my-first-model', selected: 'my-first-model' })
      await expect(page.getByText('所有更改均已保存', { exact: true })).toBeVisible()
      await page.reload()
      await expect(sidebar.getByRole('button', { name: '设置', exact: true })).toHaveAttribute('aria-current', 'page')
      await expect(page.getByText('my-first-model', { exact: true }).first()).toBeVisible()
      await expect(page.getByText('尚未配置语言模型。', { exact: false })).toHaveCount(0)
    } finally {
      const latest = await (await request.get('/api/settings')).json()
      const restored = await request.patch('/api/settings', { data: {
        layer: 'user', base_revision: latest.revisions.user,
        changes: {
          model_endpoints: current.user.model_endpoints,
          model_profiles: current.user.model_profiles,
          agent_models: current.user.agent_models ?? null,
          theme: current.user.theme ?? null,
        },
      } })
      expect(restored.ok(), await restored.text()).toBe(true)
    }
  })
}
