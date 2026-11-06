import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'

for (const theme of ['dark', 'light'] as const) {
  test(`retains settings across Writing and Game and reconciles background changes (${theme})`, async ({ page, request }) => {
    const compact = theme === 'light'
    if (compact) await page.setViewportSize({ width: 840, height: 800 })
    await createAndOpenBook(request, 'Settings Cache')
    const seeded = await request.patch('/api/settings', { data: {
      layer: 'user', changes: { theme, max_open_tabs: 7, auto_save_interval_ms: 500 },
    } })
    expect(seeded.ok()).toBe(true)
    await page.goto('/')
    const navigate = async (name: string) => {
      if (compact) await page.getByRole('button', { name: '导航菜单', exact: true }).click()
      await page.getByLabel(compact ? '移动端工作台导航' : '工作台侧边栏')
        .getByRole('button', { name, exact: true }).click()
    }
    const content = page.locator('[data-nova-settings-content]')
    await navigate('设置')
    const tabs = content.getByRole('spinbutton', { name: '最大同时打开 Tab 数', exact: true })
    await expect(tabs).toHaveValue('7')
    await expect(content.getByText(/章节组最少章节|章节组最多章节|默认叙事|默认写作 Skill/)).toHaveCount(0)
    const retained = await content.elementHandle()

    for (const [index, destination] of ['写作', '游戏'].entries()) {
      await navigate(destination)
      await expect(content).toBeHidden()
      const external = await request.patch('/api/settings', { data: {
        layer: 'user', changes: { auto_save_interval_ms: 600 + index },
      } })
      expect(external.ok()).toBe(true)

      let release!: () => void
      const gate = new Promise<void>(resolve => { release = resolve })
      let reads = 0
      await page.route('**/api/settings', async route => {
        if (route.request().method() === 'GET') {
          reads++
          await gate
        }
        await route.continue()
      })
      try {
        await navigate('设置')
        await expect(content).toBeVisible()
        expect(await retained!.evaluate(element => element === document.querySelector('[data-nova-settings-content]'))).toBe(true)
        await expect(tabs).toHaveValue(String(index === 0 ? 7 : 8))
        await expect(page.locator('[data-slot=loading-state]:visible')).toHaveCount(0)
        await expect.poll(() => reads).toBe(1)
        // An edit accepted while revalidation is pending must survive its response.
        await tabs.fill(String(8 + index))
      } finally {
        release()
      }
      await expect(content.getByRole('spinbutton', { name: '修改后自动保存延迟 (ms)', exact: true })).toHaveValue(String(600 + index))
      await expect(tabs).toHaveValue(String(8 + index))
      await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.max_open_tabs).toBe(8 + index)
      await page.unrouteAll({ behavior: 'wait' })
    }
  })
}

test('restores inherited writing preferences from the composer menu', async ({ page, request }) => {
  await createAndOpenBook(request, 'Writing Preferences')
  const seeded = await request.patch('/api/settings', { data: {
    layer: 'user', changes: { ide_story_teller_id: 'classic', writing_skill_default: 'novel-standard' },
  } })
  expect(seeded.ok()).toBe(true)
  await page.goto('/')
  await openWritingAgent(page)
  const actions = page.getByRole('button', { name: '输入动作', exact: true })
  await actions.click()

  for (const [label, previous, restored, key] of [
    ['叙事', '稳健叙事', '节奏叙事', 'ide_story_teller_id'],
    ['写作 Skill', 'Standard', 'Lite', 'writing_skill_default'],
  ]) {
    const submenu = page.getByRole('menuitem', { name: label, exact: true })
    await expect(submenu).toContainText(previous)
    await submenu.press('ArrowRight')
    await page.getByRole('menu', { name: new RegExp(`^${label}`) })
      .getByRole('menuitem', { name: '恢复默认', exact: true }).click()
    await expect(submenu).toContainText(restored)
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user[key]).toBeUndefined()
  }

  await page.reload()
  await openWritingAgent(page)
  await actions.click()
  await expect(page.getByRole('menuitem', { name: '叙事', exact: true })).toContainText('节奏叙事')
  await expect(page.getByRole('menuitem', { name: '写作 Skill', exact: true })).toContainText('Lite')
})
