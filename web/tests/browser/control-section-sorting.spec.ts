import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

for (const { theme, width } of [{ theme: 'dark', width: 1440 }, { theme: 'light', width: 390 }]) {
  test(`expanded control cards retain their size when sorting in ${theme} at ${width}px`, async ({ page, request }) => {
    await page.setViewportSize({ width, height: 1000 })
    const settings = await (await request.get('/api/settings')).json()
    const configured = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: settings.revisions.user,
      changes: { theme, language: 'zh-CN' },
    } })
    expect(configured.ok(), await configured.text()).toBe(true)
    await createAndOpenBook(request, `Control sorting ${theme}`)
    await createStartedStory(request, '展开控制卡片排序')
    await page.goto('/')
    if (width < 768) {
      await page.getByRole('button', { name: '导航菜单', exact: true }).click()
      await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name: '游戏', exact: true }).click()
      await page.getByRole('button', { name: '显示控制台', exact: true }).click()
    } else {
      await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    }
    await page.getByRole('tab', { name: '控制', exact: true }).click()

    const cards = page.locator('[data-control-section]')
    const agent = page.locator('[data-control-section="narrative"]')
    const checks = page.locator('[data-control-section="checks"]')
    const agentHeader = agent.getByRole('button', { name: '叙事风格', exact: true })
    const checksHeader = checks.getByRole('button', { name: '回合判定', exact: true })
    await agentHeader.click()
    await expect(agentHeader).toHaveAttribute('aria-expanded', 'false')
    await expect(checksHeader).toHaveAttribute('aria-expanded', 'true')
    const before = (await checks.boundingBox())!
    const source = (await checksHeader.boundingBox())!
    const target = (await agentHeader.boundingBox())!
    expect(before.height).toBeGreaterThan((await agent.boundingBox())!.height * 2)

    await page.mouse.move(target.x + target.width / 2, source.y + source.height / 2)
    await page.mouse.down()
    await page.mouse.move(target.x + target.width / 2, source.y + source.height / 2 - 10, { steps: 2 })
    await expect(checksHeader).toHaveAttribute('aria-pressed', 'true')
    await page.mouse.move(target.x + target.width / 2, target.y + target.height / 2, { steps: 8 })
    await expect(page.getByRole('status').filter({ hasText: '移动至第 1 位' })).toHaveCount(1)
    await page.screenshot({ path: test.info().outputPath('expanded-card-drag.png') })
    const during = (await checks.boundingBox())!
    expect(during.height).toBeCloseTo(before.height, 0)
    expect(during.width).toBeCloseTo(before.width, 0)
    await page.mouse.up()
    await expect(cards.first()).toHaveAttribute('data-control-section', 'checks')
    await expect(checksHeader).toHaveAttribute('aria-expanded', 'true')
    await expect(agentHeader).toHaveAttribute('aria-expanded', 'false')

    // A collapsed card must not stretch over an expanded target during keyboard sorting.
    await expect(checks).toHaveCSS('transform', 'none')
    await expect(agent).toHaveCSS('transform', 'none')
    const collapsedHeight = (await agent.boundingBox())!.height
    await agentHeader.focus()
    await page.keyboard.press('Space')
    await expect(agentHeader).toHaveAttribute('aria-pressed', 'true')
    // KeyboardSensor attaches its document listener in the next browser task.
    await page.evaluate(() => new Promise<void>(resolve => setTimeout(resolve, 0)))
    await page.keyboard.press('ArrowUp')
    await expect(page.getByRole('status').filter({ hasText: '移动至第 1 位' })).toHaveCount(1)
    await expect.poll(async () => (await agent.boundingBox())!.height).toBeCloseTo(collapsedHeight, 0)
    await page.keyboard.press('Escape')
    await expect(cards.first()).toHaveAttribute('data-control-section', 'checks')
    await expect(checksHeader).toHaveAttribute('aria-expanded', 'true')

    await page.reload()
    if (width < 768) {
      await page.getByRole('button', { name: '显示控制台', exact: true }).click()
    }
    await page.getByRole('tab', { name: '控制', exact: true }).click()
    await expect(cards.first()).toHaveAttribute('data-control-section', 'checks')
    await expect(checksHeader).toHaveAttribute('aria-expanded', 'true')
    await expect(agentHeader).toHaveAttribute('aria-expanded', 'false')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  })
}
