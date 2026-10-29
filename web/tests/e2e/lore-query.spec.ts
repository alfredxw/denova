import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { submitAgentChatMessage } from '../support/agent-chat'

for (const mode of ['writing', 'game'] as const) {
  test(`queries lore precisely and displays partial results in ${mode}`, async ({ page, request }, testInfo) => {
    const book = await createAndOpenBook(request, `Lore query ${mode}`)
    if (mode === 'game') await createStartedStory(request, 'Lore query game')
    const base = `/api/projects/${book.projectId}/book/lore`
    for (const data of [
      { id: 'hero', name: 'Hero', type: 'character', brief_description: 'E2E_QUERY_HERO_BRIEF', content: 'E2E_QUERY_HERO_BODY', load_mode: 'manual' },
      { id: 'harbor', name: 'Harbor', type: 'location', brief_description: 'E2E_QUERY_HARBOR_BRIEF', content: 'Hero visits the harbor. E2E_QUERY_HARBOR_BODY', load_mode: 'manual' },
      { id: 'hidden', name: 'Hidden', enabled: false, content: 'E2E_QUERY_HIDDEN_BODY' },
    ]) expect((await request.post(`${base}/items`, { data })).ok()).toBe(true)
    const settings = await (await request.get('/api/settings')).json()
    expect((await request.patch('/api/settings', { data: { layer: 'user', base_revision: settings.revisions.user, changes: {
      theme: mode === 'writing' ? 'dark' : 'light', language: 'zh-CN',
    } } })).ok()).toBe(true)
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.goto('/')
    const sidebar = page.getByLabel('工作台侧边栏')
    await sidebar.getByRole('button', { name: mode === 'writing' ? '写作' : '游戏', exact: true }).click()
    await sidebar.getByRole('button', { name: '资料库', exact: true }).click()
    await expect(page.getByTestId('lore-library')).toBeVisible()
    const composer = page.getByPlaceholder(/输入消息/).filter({ visible: true })
    if (await composer.count() === 0) await page.getByRole('button', { name: '配置管理', exact: true }).click()
    await expect(composer).toBeVisible()
    await submitAgentChatMessage(page, composer, 'E2E_LORE_QUERY 请核对角色资料及分页结果。')
    await expect(page.getByText('E2E lore query verified.', { exact: true })).toBeVisible()
    const execution = page.locator('[data-agent-execution-process] [data-slot="collapsible-trigger"]').filter({ visible: true }).first()
    await expect(execution).toHaveAttribute('aria-busy', 'false')
    // Completion preserves the process disclosure; loaded history may start collapsed.
    if (await execution.getAttribute('data-state') === 'closed') await execution.click()
    const query = page.locator('[data-nova-tool-header]').filter({ hasText: '查询资料库', visible: true })
    await expect(query).toHaveCount(4)
    await query.nth(1).click()
    await expect(page.getByText('未找到的资料', { exact: true })).toBeVisible()
    await expect(page.locator('[data-nova-tool-detail-output]').filter({ visible: true })).toContainText('E2E_QUERY_HARBOR_BODY')
    await page.screenshot({ path: testInfo.outputPath(`lore-query-${mode}-wide.png`) })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.getByRole('tab', { name: 'Agent', exact: true }).click()
    await expect(query.nth(1)).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`lore-query-${mode}-narrow.png`) })
    await page.reload()
    await page.getByRole('tab', { name: 'Agent', exact: true }).click()
    await expect(page.getByText('E2E lore query verified.', { exact: true })).toBeVisible()
  })
}
