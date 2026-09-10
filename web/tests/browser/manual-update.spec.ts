import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('manual update stays tucked away and stages an upload before explicit restart', async ({ page, request, browserDiagnostics }, testInfo) => {
  test.setTimeout(90_000)
  browserDiagnostics.allow(/console\.error: Failed to load resource:.*400/)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await createAndOpenBook(request, 'Manual Update Browser Book')
  let uploads = 0
  let applies = 0
  let finishUpload!: () => void
  const uploadReady = new Promise<void>(resolve => { finishUpload = resolve })
  await page.route('**/api/update/upload', async route => {
    uploads++
    expect(route.request().headers()['content-type']).toContain('multipart/form-data')
    expect(route.request().postDataBuffer()?.toString()).toContain('denova-v0.5.0-windows-x64.zip')
    if (uploads === 1) {
      await route.fulfill({ status: 400, json: { error: '安装包与当前电脑的操作系统或处理器架构不匹配。' } })
    } else {
      await uploadReady
      await route.fulfill({ json: { previous_version: '0.4.5', installed_version: '0.5.0', status: 'staged', staged: true, apply_ready: true, restart_required: true } })
    }
  })
  await page.route('**/api/update/apply', async route => {
    applies++
    await route.fulfill({ json: { status: 'applying', version: '0.5.0' } })
  })
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  for (const destination of ['写作', '游戏']) {
    await sidebar.getByRole('button', { name: destination, exact: true }).click()
    await expect(sidebar.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
    await sidebar.getByRole('button', { name: '设置', exact: true }).click()
    await page.getByRole('button', { name: '应用更新', exact: true }).click()
    await expect(page.getByRole('button', { name: '手动更新', exact: true })).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByRole('button', { name: '选择安装包', exact: true })).toBeHidden()
  }

  const releaseFile = { name: 'denova-v0.5.0-windows-x64.zip', mimeType: 'application/zip', buffer: Buffer.from('browser upload fixture') }
  await page.getByRole('button', { name: '手动更新', exact: true }).click()
  await expect(page.getByRole('link', { name: '打开 Release', exact: true })).toHaveAttribute('href', 'https://github.com/alfredxw/denova/releases/latest')
  await page.getByLabel('选择安装包', { exact: true }).setInputFiles(releaseFile)
  await expect(page.getByText('安装包与当前电脑的操作系统或处理器架构不匹配。', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '重启并安装', exact: true })).toBeHidden()
  await page.getByLabel('选择安装包', { exact: true }).setInputFiles(releaseFile)
  await expect(page.getByRole('button', { name: '正在上传并校验', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled()
  finishUpload()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  expect(applies).toBe(0)
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled()
  const heading = page.locator('section > button').filter({ hasText: '应用更新' })
  await heading.click()
  await heading.click()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  await page.getByRole('button', { name: '重启并安装', exact: true }).click()
  await expect(page.getByText('Denova 正在重启并应用更新。新版本可用后页面会自动刷新。')).toBeVisible()
  expect(applies).toBe(1)

  for (const scenario of [
    { width: 1440, height: 960, theme: 'dark', language: 'zh-CN' },
    { width: 1440, height: 960, theme: 'light', language: 'en-US' },
    { width: 390, height: 844, theme: 'dark', language: 'en-US' },
    { width: 390, height: 844, theme: 'light', language: 'zh-CN' },
  ]) {
    const english = scenario.language === 'en-US'
    const labels = english
      ? { settings: 'Settings', categories: 'Categories', updates: 'App Updates', manual: 'Manual update', select: 'Select release archive' }
      : { settings: '设置', categories: '设置分类', updates: '应用更新', manual: '手动更新', select: '选择安装包' }
    await page.setViewportSize(scenario)
    expect((await request.patch('/api/settings', { data: { layer: 'user', changes: { theme: scenario.theme, language: scenario.language } } })).ok()).toBe(true)
    await page.reload()
    await expect(page.getByRole('heading', { name: labels.settings, exact: true })).toBeVisible()
    if (scenario.width < 768) {
      await page.locator('.nova-mobile-topbar').getByRole('button', { name: labels.categories, exact: true }).click()
      await page.getByRole('dialog', { name: labels.categories, exact: true }).getByRole('button', { name: labels.updates, exact: true }).click()
    } else {
      await page.getByRole('button', { name: labels.updates, exact: true }).click()
    }
    await page.getByRole('button', { name: labels.manual, exact: true }).click()
    await expect(page.getByRole('button', { name: labels.select, exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`manual-update-${scenario.width}-${scenario.theme}.png`), animations: 'disabled' })
  }
})

test('automatic download still stages its streamed result after a failed attempt', async ({ page, request }) => {
  await createAndOpenBook(request, 'Downloaded Update Browser Book')
  await page.route('**/api/update/check', route => route.fulfill({ json: {
    current_version: '0.4.5', latest_version: '0.5.0', update_available: true, can_install: true, platform: 'windows-x64',
  } }))
  let installs = 0
  await page.route('**/api/update/install/stream', route => {
    installs++
    const result = installs === 1
      ? 'event: error\ndata: {"message":"下载失败，请重试。"}\n\n'
      : 'event: update_result\ndata: {"installed_version":"0.5.0","apply_ready":true,"staged":true}\n\n'
    return route.fulfill({ contentType: 'text/event-stream', body: 'event: update_progress\ndata: {"phase":"downloading","percent":50}\n\n' + result })
  })
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true }).click()
  await page.getByRole('button', { name: '应用更新', exact: true }).click()
  await page.getByRole('button', { name: '检查更新', exact: true }).click()
  await page.getByRole('button', { name: '安装更新', exact: true }).click()
  await expect(page.getByText('下载失败，请重试。', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '安装更新', exact: true }).click()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  await expect(page.getByRole('button', { name: '重启并安装', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled()
})

test('release upload exceeds the former 72 MiB API limit and reaches localized validation', async ({ request }) => {
  const response = await request.post('/api/update/upload', {
    headers: { 'X-Denova-Locale': 'en-US' },
    multipart: { file: { name: 'denova-v0.5.0-windows-x64.zip', mimeType: 'application/zip', buffer: Buffer.alloc(73 * 1024 * 1024) } },
  })
  expect(response.status()).toBe(400)
  expect(await response.json()).toMatchObject({ error: 'Select a stable release newer than the running version. Development builds cannot update manually.' })
})
