import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { readFile } from 'node:fs/promises'

for (const { theme, width } of [{ theme: 'dark', width: 1440 }, { theme: 'light', width: 390 }]) {
  test(`remote writing, game and resources avoid local-only polling in ${theme}`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, 'Remote access')
    await createStartedStory(request, 'Remote story')
    const settings = await request.patch('/api/settings', { data: { layer: 'user', changes: { theme } } })
    expect(settings.ok()).toBe(true)
    await page.route('**/api/auth/status', route => route.fulfill({ json: { local: false, authenticated: true } }))
    const platformRequests: string[] = []
    const coverRequests: string[] = []
    page.on('request', request => {
      if (new URL(request.url()).pathname.startsWith('/api/platform/manage/')) platformRequests.push(request.url())
      if (new URL(request.url()).pathname === '/api/books/cover' && new URL(request.url()).searchParams.get('path') === book.workspace) coverRequests.push(request.url())
    })
    await page.setViewportSize({ width, height: 900 })
    await page.clock.install()
    await page.goto('/')
    const select = async (name: string) => {
      if (width < 768) {
        await page.getByRole('button', { name: '导航菜单', exact: true }).click()
        await page.getByRole('dialog', { name: '导航菜单', exact: true }).getByRole('button', { name, exact: true }).click()
      } else await page.getByRole('navigation').first().getByRole('button', { name, exact: true }).click()
    }
    await select('书籍管理')
    await expect(page.getByRole('heading', { name: '书籍管理', exact: true })).toBeVisible()
    await select('写作')
    if (width < 768) await page.getByRole('tab', { name: 'Agent', exact: true }).click()
    await expect(page.getByPlaceholder('输入消息，/ 选择命令或 Skills', { exact: true })).toBeVisible()
    await select('游戏')
    await expect(page.getByPlaceholder(/你要做什么/)).toBeVisible()
    await select('资源中心')
    const resources = page.getByTestId('resource-market')
    await expect(resources.getByText('此功能需要在运行 Denova 的设备上操作。', { exact: true }).filter({ visible: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '添加资源', exact: true })).toBeDisabled()
    await expect(page.getByRole('button', { name: '导出资源包', exact: true })).toBeDisabled()
    if (width < 768) await page.getByRole('button', { name: '资源中心导航目录', exact: true }).click()
    await page.getByRole('button', { name: '插件与游戏', exact: true }).filter({ visible: true }).click()
    // Advance beyond the five-second plugin page poll without wall-clock sleeps.
    await page.clock.runFor(6000)
    expect(platformRequests).toEqual([])
    expect(coverRequests).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: test.info().outputPath(`remote-${theme}-${width}.png`) })
    await page.clock.resume()
    const uploaded = await request.post('/api/books/cover/upload', { multipart: {
      path: book.workspace,
      file: { name: 'cover.png', mimeType: 'image/png', buffer: await readFile(new URL('../fixtures/comic-page.png', import.meta.url)) },
    } })
    expect(uploaded.ok(), await uploaded.text()).toBe(true)
    await select('书籍管理')
    await page.reload()
    const cover = page.getByRole('img', { name: book.title, exact: true }).first()
    await expect(cover).toBeVisible()
    await expect.poll(() => cover.evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0)).toBe(true)
    expect(coverRequests.length).toBeGreaterThan(0)
  })
}
