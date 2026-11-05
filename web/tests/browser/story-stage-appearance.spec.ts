import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'

for (const theme of ['light', 'dark']) {
  test(`overrides the ${theme} app theme on the stage and keeps outlined dialogue highlighted`, async ({ page, request }) => {
    const settings = await request.patch('/api/settings', { data: {
      layer: 'user', changes: { theme, language: 'zh-CN', interactive_stage_theme: null, interactive_stage_text_outline: null, interactive_stage_scrim_opacity: 0.2 },
    } })
    expect(settings.ok(), await settings.text()).toBe(true)
    await createAndOpenBook(request, `Text outline ${theme}`)
    const story = await createStartedStory(request, '舞台文字描边')
    const snapshotURL = `/api/interactive/stories/${story.id}/snapshot`
    const { current_turn: turn } = await (await request.get(snapshotURL)).json()
    const prose = '薄雾笼罩着铁轨，远处的信号灯在暮色里忽明忽灭。'
    const dialogue = '“跟紧我，别走散。”'
    const edited = await request.patch(`/api/interactive/stories/${story.id}/turns/${turn.id}/narrative`, { data: {
      branch_id: 'main', expected_narrative: turn.narrative, narrative: `${prose}\n\n${dialogue}`,
    } })
    expect(edited.ok(), await edited.text()).toBe(true)

    await page.route('**/api/interactive/stories/*/snapshot*', async route => {
      const response = await route.fetch()
      const snapshot = await response.json()
      for (const turn of [snapshot.current_turn, ...(snapshot.turns || [])]) {
        if (turn?.turn_result) turn.turn_result.presentation = { background: { path: 'outline-backdrop.svg', name: 'Mixed contrast' } }
      }
      await route.fulfill({ response, json: snapshot })
    })
    await page.route('**/*outline-backdrop.svg*', route => route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540"><rect width="960" height="540" fill="#e8e4d4"/><path d="M0 0H280L680 540H0Z" fill="#172335"/><path d="M60 540L380 300L820 540M180 540L380 300L670 540" fill="none" stroke="#708c93" stroke-width="12"/></svg>',
    }))
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
    await page.getByRole('tab', { name: '控制', exact: true }).click()

    const outline = page.getByRole('switch', { name: '文字描边', exact: true })
    const themeSelect = page.getByRole('combobox', { name: '舞台主题', exact: true })
    const stage = page.getByTestId('story-stage-card')
    const stageRoot = stage.locator('..')
    const paragraph = stage.getByText(prose, { exact: true })
    const speech = stage.locator('.nova-dialogue-highlight').filter({ hasText: dialogue })
    const scrim = page.getByTestId('story-stage-scrim')
    const stageTheme = theme === 'light' ? 'dark' : 'light'
    const stageSurface = stageTheme === 'dark' ? 'rgb(18, 18, 18)' : 'rgb(245, 245, 245)'
    const dialogueColor = stageTheme === 'dark' ? 'rgb(255, 209, 102)' : 'rgb(160, 74, 26)'
    await expect(page.locator('[data-stage-layer="background"]')).toBeVisible()
    await expect(themeSelect).toHaveText('跟随应用')
    await expect(stageRoot).toHaveCSS('color-scheme', theme)
    await expect(outline).not.toBeChecked()
    await expect(paragraph).toHaveCSS('-webkit-text-stroke-width', '0px')

    await themeSelect.click()
    await page.getByRole('option', { name: stageTheme === 'dark' ? '深色' : '浅色', exact: true }).click()
    await expect(stageRoot).toHaveCSS('background-color', stageSurface)
    await expect(scrim).toHaveCSS('background-color', stageSurface)
    await expect(stageRoot).toHaveCSS('color-scheme', stageTheme)
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await expect(themeSelect).toHaveCSS('color-scheme', theme)
    await expect(speech).toHaveCSS('color', dialogueColor)

    await stage.getByRole('button', { name: '输入动作', exact: true }).click()
    const actionsMenu = page.getByRole('menu')
    await expect(actionsMenu).toHaveCSS('color-scheme', stageTheme)
    await expect(actionsMenu).toHaveCSS('background-color', stageSurface)
    await page.keyboard.press('Escape')
    await expect(actionsMenu).toBeHidden()

    await outline.click()
    await expect(outline).toBeChecked()
    await expect(paragraph).toHaveCSS('color', 'rgb(255, 255, 255)')
    await expect(paragraph).not.toHaveCSS('-webkit-text-stroke-width', '0px')
    await expect(paragraph).toHaveCSS('-webkit-text-stroke-color', 'rgb(0, 0, 0)')
    await expect(paragraph).toHaveCSS('paint-order', /^stroke/)
    await expect(speech).toHaveCSS('color', 'rgb(255, 209, 102)')
    await expect(speech).not.toHaveCSS('-webkit-text-stroke-width', '0px')
    await expect(scrim).toHaveCSS('opacity', '0.2')
    await expect(stage.locator('.nova-chat-canvas')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(stage.getByPlaceholder(/你要做什么/)).toHaveCSS('-webkit-text-stroke-width', '0px')
    await page.screenshot({ path: test.info().outputPath(`stage-${stageTheme}-desktop.png`) })

    await page.reload()
    await expect(outline).toBeChecked()
    await expect(stageRoot).toHaveCSS('color-scheme', stageTheme)
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(paragraph).toHaveCSS('color', 'rgb(255, 255, 255)')
    await page.screenshot({ path: test.info().outputPath(`stage-${stageTheme}-mobile.png`) })
    await page.getByRole('button', { name: '显示控制台', exact: true }).click()
    await page.getByRole('dialog', { name: '控制台', exact: true }).getByRole('tab', { name: '控制', exact: true }).click()
    await outline.click()
    await expect(outline).not.toBeChecked()
    await expect(paragraph).toHaveCSS('-webkit-text-stroke-width', '0px')
    await expect(scrim).toHaveCSS('opacity', '0.2')
    await themeSelect.click()
    await page.getByRole('option', { name: '跟随应用', exact: true }).click()
    await expect(stageRoot).toHaveCSS('color-scheme', theme)
  })
}
