import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { getModelStatus, releaseDelayedRequest } from '../support/model'

for (const product of ['writing', 'game'] as const) {
  for (const theme of ['dark', 'light'] as const) {
    test(`${product} pauses from the composer and reveals settled actions in ${theme}`, async ({ page, request }) => {
      page.setDefaultTimeout(10_000)
      await createAndOpenBook(request, `Composer pause ${product} ${theme}`)
      if (product === 'game') await createStartedStory(request, 'Composer pause story')
      const width = theme === 'dark' ? 1440 : 390
      const settings = await (await request.get('/api/settings')).json()
      await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, route => route.fulfill({ json: {
        ...settings, effective: { ...settings.effective, theme, language: 'zh-CN' },
      } }))
      await page.addInitScript(() => {
        localStorage.setItem('nova.locale.configured', 'zh-CN')
        localStorage.setItem('nova:onboarding:v1', JSON.stringify({ version: 1, skipped: true }))
      })
      await page.setViewportSize({ width, height: 960 })
      await page.goto('/')
      const destination = product === 'writing' ? '写作' : '游戏'
      if (width < 800) {
        await page.getByRole('button', { name: '导航菜单', exact: true }).click()
        await page.getByRole('dialog').getByRole('button', { name: destination, exact: true }).click()
        if (product === 'writing') await page.getByRole('tab', { name: 'Agent', exact: true }).click()
      } else {
        await page.getByLabel('工作台侧边栏').getByRole('button', { name: destination, exact: true }).click()
      }
      const composer = page.getByPlaceholder(product === 'writing' ? /输入消息/ : /你要做什么/).filter({ visible: true })
      await expect(composer).toBeVisible()
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      const previousRunActions = await page.getByRole('button', { name: '复制 Run ID', exact: true }).count()
      const marker = 'E2E_COMPOSER_PAUSE'
      await composer.fill(`检查门后的脚印。${marker}`)
      await page.locator('[data-action="send"]').filter({ visible: true }).click()
      try {
        await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[marker] ?? 0).toBe(1)
        await expect(page.getByText(/正在检查门后的脚印，接下来会继续核对/).last()).toBeVisible()
        await expect(page.getByRole('button', { name: '暂停任务', exact: true })).toHaveCount(0)
        await expect(page.getByRole('button', { name: '复制 Run ID', exact: true })).toHaveCount(previousRunActions)
        const output = page.locator('[data-nova-chat-item="run"]').last()
        await expect(output.getByRole('button', { name: '复制消息', exact: true })).toHaveCount(0)
        await page.screenshot({ path: test.info().outputPath('streaming.png') })
        const pauseRequest = page.waitForRequest(value => value.method() === 'POST' && value.postData()?.includes('"suspend"') === true)
        await page.locator('[data-action="stop"]').filter({ visible: true }).click()
        await pauseRequest
        await expect(page.getByRole('button', { name: '继续任务', exact: true })).toBeVisible()
        await output.hover()
        await expect(output.getByRole('button', { name: '复制消息', exact: true })).toBeVisible()
        if (width < 800) await output.getByRole('button', { name: '消息操作', exact: true }).click()
        await expect(page.getByRole('button', { name: '复制 Run ID', exact: true })).toHaveCount(previousRunActions + 1)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
        await page.screenshot({ path: test.info().outputPath('paused.png') })
      } finally {
        await releaseDelayedRequest(request, marker)
      }
    })
  }
}
