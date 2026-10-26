import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test.beforeEach(async ({ page }) => {
  // This journey owns installation state, not GitHub release availability.
  await page.route('**/api/update/notes?**', route => route.fulfill({ json: {
    from_version: new URL(route.request().url()).searchParams.get('from'),
    to_version: new URL(route.request().url()).searchParams.get('to'), complete: true, releases: [],
  } }))
})

test('a completed background restart refreshes Game even if settings were never opened', async ({ page, request }) => {
  await createAndOpenBook(request, 'Background Restart Browser Book')
  let completed = false
  await page.route('**/api/update/status', route => route.fulfill({ json: {
    id: 'background-restart', phase: completed ? 'succeeded' : 'staged',
    version: '0.5.2', current_version: completed ? '0.5.2' : '0.5.1',
  } }))
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  await expect(page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page).not.toHaveURL(/denova_reload=/)
  completed = true
  await expect(page).toHaveURL(/denova_reload=0\.5\.2-/, { timeout: 20_000 })
})

test('manual update stays tucked away and stages an upload before explicit restart', async ({ page, request, browserDiagnostics }, testInfo) => {
  test.setTimeout(90_000)
  browserDiagnostics.allow(/console\.error: Failed to load resource:.*(?:400|500)/)
  browserDiagnostics.allow(/http\.5xx: POST .*\/api\/update\/apply returned 500/)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await createAndOpenBook(request, 'Manual Update Browser Book')
  let uploads = 0
  let applies = 0
  let pendingPhase = 'idle'
  await page.route('**/api/update/status', route => route.fulfill({ json: { phase: pendingPhase, id: 'browser-update', current_version: '0.4.5', version: '0.5.0', log_path: '.denova-updates/pending-0.5.0-1234567890/apply.log' } }))
  let finishUpload!: () => void
  const uploadReady = new Promise<void>(resolve => { finishUpload = resolve })
  await page.route('**/api/update/upload', async route => {
    uploads++
    expect(route.request().headers()['content-type']).toContain('multipart/form-data')
    expect(route.request().postDataBuffer()?.toString()).toContain('denova-v0.5.0-windows-x64.zip')
    if (uploads === 1) {
      await route.fulfill({ status: 400, json: { error: '安装包与当前电脑的操作系统或处理器架构不匹配。', code: 'update.platform_mismatch', request_id: 'update-upload-request', details: { detail: 'archive platform windows/amd64 does not match darwin/arm64', operation: 'update.upload', backend_version: '0.5.0', platform: 'darwin/arm64' } } })
    } else {
      await uploadReady
      pendingPhase = 'staged'
      await route.fulfill({ json: { previous_version: '0.4.5', installed_version: '0.5.0', status: 'staged', staged: true, apply_ready: true, restart_required: true } })
    }
  })
  await page.route('**/api/update/apply', async route => {
    applies++
    if (applies === 1) {
      await route.fulfill({ status: 500, json: { error: '无法启动更新程序，请重试。' } })
      return
    }
    pendingPhase = 'waiting'
    await route.fulfill({ json: { status: 'restarting', id: 'browser-update', version: '0.5.0' } })
  })
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  for (const destination of ['写作', '游戏']) {
    await sidebar.getByRole('button', { name: destination, exact: true }).click()
    await expect(sidebar.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
    await sidebar.getByRole('button', { name: '设置', exact: true }).click()
    await page.getByRole('button', { name: '软件更新', exact: true }).click()
    await expect(page.getByRole('button', { name: '手动更新', exact: true })).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByRole('button', { name: '选择安装包', exact: true })).toBeHidden()
  }

  const releaseFile = { name: 'denova-v0.5.0-windows-x64.zip', mimeType: 'application/zip', buffer: Buffer.from('browser upload fixture') }
  await page.getByRole('button', { name: '手动更新', exact: true }).click()
  await expect(page.getByRole('link', { name: '打开 Release', exact: true })).toHaveAttribute('href', 'https://github.com/alfredxw/denova/releases/latest')
  await page.getByLabel('选择安装包', { exact: true }).setInputFiles(releaseFile)
  const diagnostic = page.getByRole('alert')
  await expect(diagnostic).toContainText('安装包与当前电脑的操作系统或处理器架构不匹配。')
  await expect(diagnostic).toContainText('archive platform windows/amd64 does not match darwin/arm64')
  await expect(diagnostic).toContainText('update-upload-request')
  await expect(diagnostic).toContainText('update.platform_mismatch')
  await expect(diagnostic.getByRole('button', { name: '复制诊断信息' })).toBeVisible()
  await expect(page.getByRole('button', { name: '重启并安装', exact: true })).toBeHidden()
  await page.getByLabel('选择安装包', { exact: true }).setInputFiles(releaseFile)
  await expect(page.getByRole('button', { name: '正在上传并校验', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled()
  finishUpload()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  expect(applies).toBe(0)
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '通用', exact: true }).click()
  await page.getByRole('button', { name: '软件更新', exact: true }).click()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: '软件更新', exact: true }).click()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  await page.getByRole('button', { name: '重启并安装', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('无法启动更新程序，请重试。')
  await expect(page.getByRole('button', { name: '重启并安装', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: '重启并安装', exact: true }).click()
  await expect(page.getByText('Denova 正在重启并应用更新。新版本可用后页面会自动刷新。')).toBeVisible()
  expect(applies).toBe(2)
  // The old backend can still answer while shutdown is draining.
  await page.waitForTimeout(1200)
  await expect(page).not.toHaveURL(/denova_reload=/)
  pendingPhase = 'failed'
  await expect(page.getByRole('alert')).toContainText('更新未完成')

  for (const scenario of [
    { width: 1440, height: 960, theme: 'dark', language: 'zh-CN' },
    { width: 1440, height: 960, theme: 'light', language: 'en-US' },
    { width: 390, height: 844, theme: 'dark', language: 'en-US' },
    { width: 390, height: 844, theme: 'light', language: 'zh-CN' },
  ]) {
    const english = scenario.language === 'en-US'
    const labels = english
      ? { settings: 'Settings', categories: 'Categories', updates: 'Software updates', manual: 'Manual update', select: 'Select release archive' }
      : { settings: '设置', categories: '设置分类', updates: '软件更新', manual: '手动更新', select: '选择安装包' }
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
    await page.getByRole('button', { name: labels.select, exact: true }).scrollIntoViewIfNeeded()
    await expect(page.getByRole('button', { name: labels.select, exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`manual-update-${scenario.width}-${scenario.theme}.png`), animations: 'disabled' })
  }
})

test('automatic download still stages its streamed result after a failed attempt', async ({ page, request }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const book = await createAndOpenBook(request, 'Downloaded Update Browser Book')
  let downloaded = false
  await page.route('**/api/update/status', route => route.fulfill({ json: { phase: downloaded ? 'staged' : 'idle', current_version: '0.4.5', version: '0.5.0' } }))
  await page.route('**/api/update/check', route => route.fulfill({ json: {
    current_version: '0.4.5', latest_version: '0.5.0', update_available: true, can_install: true, platform: 'windows-x64',
  } }))
  let installs = 0
  await page.route('**/api/update/install/stream', route => {
    installs++
    downloaded = installs > 1
    const result = installs === 1
      ? 'event: error\ndata: {"message":"下载失败，请重试。"}\n\n'
      : 'event: update_result\ndata: {"installed_version":"0.5.0","apply_ready":true,"staged":true}\n\n'
    return route.fulfill({ contentType: 'text/event-stream', body: 'event: update_progress\ndata: {"phase":"downloading","percent":50}\n\n' + result })
  })
  await page.goto('/')
  await expect(page.getByRole('button', { name: `切换书籍，当前：${book.title}`, exact: true })).toBeVisible()
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true }).click()
  await page.getByRole('button', { name: '软件更新', exact: true }).click()
  await page.getByRole('button', { name: '检查更新', exact: true }).click()
  await expect(page.getByText('最新版本：0.5.0', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '安装更新', exact: true }).click()
  await expect(page.getByText('下载失败，请重试。', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '安装更新', exact: true }).click()
  await expect(page.getByText(/版本 0.5.0 已就绪/)).toBeVisible()
  await expect(page.getByRole('button', { name: '重启并安装', exact: true })).toBeEnabled()
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled()
})

test('release upload exceeds the former 72 MiB API limit and reaches localized validation', async ({ request }) => {
  // Reach the same version validation in development and packaged backends on
  // every supported host, without depending on the current release version.
  const platform = process.platform === 'win32' ? 'windows' : process.platform
  const extension = process.platform === 'win32' ? 'zip' : 'tar.gz'
  const response = await request.post('/api/update/upload', {
    headers: { 'X-Denova-Locale': 'en-US' },
    multipart: { file: { name: `denova-v0.0.0-${platform}-${process.arch}.${extension}`, mimeType: 'application/octet-stream', buffer: Buffer.alloc(73 * 1024 * 1024) } },
  })
  expect(response.status()).toBe(400)
  expect(await response.json()).toMatchObject({ error: 'Select a stable release newer than the running version. Development builds cannot update manually.' })
})

test('background preparation survives navigation and idle restart can be canceled', async ({ page, request }, testInfo) => {
  test.setTimeout(90_000)
  await createAndOpenBook(request, 'Background Update Browser Book')
  expect((await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'zh-CN', update_check_enabled: true, update_download_enabled: false } } })).ok()).toBe(true)
  let phase = 'failed'
  let operation = ''
  let waiting = false
  let checkRequests = 0
  let statusRequests = 0
  const commands: string[] = []
  await page.route('**/api/update/check', route => { checkRequests++; return route.abort() })
  await page.route('**/api/update/status', route => {
    statusRequests++
    return route.fulfill({ json: {
      phase, operation, restart_waiting: waiting, busy: true,
      current_version: '0.5.1', version: '0.5.2', id: 'prepared-in-background',
      progress: { phase: phase === 'staged' ? 'staged' : 'downloading', percent: phase === 'staged' ? 100 : 50, downloaded_bytes: phase === 'staged' ? 1000 : 500, total_bytes: 1000 },
      check: { current_version: '0.5.1', latest_version: '0.5.2', update_available: true, can_install: true, platform: 'darwin-arm64', release_url: '', published_at: '' },
    } })
  })
  await page.route('**/api/update/apply', async route => {
    const when = route.request().postDataJSON().when
    commands.push(when)
    waiting = when === 'when_idle'
    await route.fulfill({ json: { id: 'prepared-in-background', version: '0.5.2', status: waiting ? 'waiting_for_idle' : 'canceled' } })
  })
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true }).click()
  await page.getByRole('button', { name: '软件更新', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('更新未完成')
  phase = 'idle'
  operation = 'downloading'
  await expect(page.getByRole('button', { name: '安装中', exact: true })).toBeDisabled()
  await expect(page.getByRole('alert')).toBeHidden()
  await expect(page.getByText('50%', { exact: true })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '自动下载更新', exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: '自动下载更新', exact: true }).click()
  await page.getByRole('option', { name: '开启', exact: true }).click()
  await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.update_download_enabled).toBe(true)
  // Download completion is discovered without an install request from this page.
  phase = 'staged'
  operation = ''
  await expect(page.getByText(/版本 0.5.2 已就绪/)).toBeVisible()
  await page.getByRole('button', { name: '任务完成后重启', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '正在等待活动任务' })).toBeVisible()
  expect(commands).toEqual(['when_idle'])
  // Reloading must restore the wait without starting the three-minute restart
  // timeout: work may take arbitrarily long before the updater takes over.
  await page.reload()
  await page.getByRole('button', { name: '软件更新', exact: true }).click()
  await expect(page.getByRole('button', { name: '取消等待', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: '取消等待', exact: true }).click()
  await expect(page.getByRole('button', { name: '任务完成后重启', exact: true })).toBeEnabled()
  expect(commands).toEqual(['when_idle', 'cancel'])
  expect(checkRequests).toBe(0)
  expect(statusRequests).toBeGreaterThan(2)
  for (const size of [{ width: 1440, height: 960 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(size)
    await page.getByRole('button', { name: '任务完成后重启', exact: true }).scrollIntoViewIfNeeded()
    await expect(page.getByRole('button', { name: '任务完成后重启', exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`background-update-${size.width}.png`), animations: 'disabled' })
  }
})
