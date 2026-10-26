import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { readFileSync } from 'node:fs'

const current = (JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8')).version as string).replace(/^v/, '')
const previous = '0.5.1'
const versions = [`v${current}`, 'v0.6.2', 'v0.6.1', 'v0.6.0']

test('upgrade notes cover every skipped release before downloading in Writing and Game', async ({ page, request }, testInfo) => {
  test.setTimeout(90_000)
  await createAndOpenBook(request, 'Upgrade Notes Browser Book')
  let staged = false
  let installs = 0
  await page.route('**/api/update/status', route => route.fulfill({ json: {
    phase: staged ? 'staged' : 'idle', current_version: previous, version: current,
    check: { current_version: previous, latest_version: current, update_available: true, can_install: true, platform: 'darwin-arm64' },
  } }))
  await page.route('**/api/update/notes?**', route => {
    const from = new URL(route.request().url()).searchParams.get('from')
    const english = route.request().headers()['x-denova-locale']?.startsWith('en')
    const entries = (from ? versions : [...versions, `v${previous}`]).map(version => ({
      version, published_at: '2026-10-20T00:00:00Z',
      summary: english ? `Changes in ${version}` : `${version} 的创作变化`,
      brief: (english ? `- Changes in ${version}` : `- ${version} 的创作变化`) + (version === versions[0]
        ? '\n- ' + (english ? 'Writing and Game share searchable Lore, reusable creative setups, and history that follows each saved branch. ' : '写作与游戏共用资料检索、可复用的创作方案，以及随已保存分支保留的历史记录。').repeat(5)
        : ''),
      body: english ? `### Added\n- Full details for ${version}` : `### 新增\n- ${version} 的完整详情`,
    }))
    return route.fulfill({ json: { from_version: from, to_version: current, complete: true, releases: entries } })
  })
  await page.route('**/api/update/install/stream', route => {
    installs++
    staged = true
    return route.fulfill({ contentType: 'text/event-stream', body: `event: update_result\ndata: ${JSON.stringify({ previous_version: previous, installed_version: current, apply_ready: true, staged: true })}\n\n` })
  })
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  for (const destination of ['写作', '游戏']) {
    await sidebar.getByRole('button', { name: destination, exact: true }).click()
    await sidebar.getByRole('button', { name: `发现新版本 ${current}`, exact: true }).click()
    const reader = page.getByRole('dialog', { name: `更新到 v${current}`, exact: true })
    for (const version of versions) await expect(reader.getByText(`${version} 的创作变化`, { exact: true })).toBeVisible()
    await expect(reader.getByText('4 个版本', { exact: true })).toBeVisible()
    await expect(reader.getByText(`v${previous}`, { exact: true })).toBeVisible()
    const newest = reader.locator('summary').first()
    await newest.click()
    await expect(reader.getByText(`v${current} 的完整详情`, { exact: true })).toBeVisible()
    await newest.press('Enter')
    await expect(reader.getByText(`v${current} 的创作变化`, { exact: true })).toBeVisible()
    await reader.getByRole('button', { name: '全部版本', exact: true }).click()
    await expect(reader.getByText(`v${previous} 的创作变化`, { exact: true })).toBeVisible()
    await reader.getByRole('button', { name: '返回本次升级', exact: true }).click()
    await expect(reader.getByText(`v${previous} 的创作变化`, { exact: true })).toBeHidden()
    await page.keyboard.press('Escape')
  }
  expect(installs).toBe(0)
  await sidebar.getByRole('button', { name: `发现新版本 ${current}`, exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '前往更新', exact: true }).click()
  await page.getByRole('button', { name: '查看本次更新', exact: true }).click()
  const reader = page.getByRole('dialog', { name: `更新到 v${current}`, exact: true })
  await reader.getByRole('button', { name: '安装更新', exact: true }).click()
  await expect(reader.getByRole('button', { name: '重启并安装', exact: true })).toBeEnabled()
  await expect(reader.getByRole('button', { name: '任务完成后重启', exact: true })).toBeEnabled()
  expect(installs).toBe(1)
  await page.keyboard.press('Escape')

  for (const scenario of [
    { width: 1440, height: 960, theme: 'dark', language: 'zh-CN' },
    { width: 1440, height: 960, theme: 'light', language: 'en-US' },
    { width: 390, height: 844, theme: 'dark', language: 'en-US' },
    { width: 320, height: 844, theme: 'light', language: 'zh-CN' },
  ]) {
    await page.setViewportSize(scenario)
    expect((await request.patch('/api/settings', { data: { layer: 'user', changes: { theme: scenario.theme, language: scenario.language } } })).ok()).toBe(true)
    await page.reload()
    await page.getByRole('button', { name: scenario.language === 'en-US' ? 'View this update' : '查看本次更新', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog.getByText(scenario.language === 'en-US' ? 'Changes in v0.6.0' : 'v0.6.0 的创作变化', { exact: true })).toBeVisible()
    await expect(dialog.getByRole('button', { name: scenario.language === 'en-US' ? 'Restart and install' : '重启并安装', exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`upgrade-notes-${scenario.width}-${scenario.theme}.png`), animations: 'disabled' })
    await page.keyboard.press('Escape')
  }
})

test('successful upgrade notice keeps its baseline after another download and across reloads', async ({ page, request }) => {
  await createAndOpenBook(request, 'Completed Upgrade Browser Book')
  let nextStaged = false
  let reads = 0
  const notice = { id: 'update:completed-upgrade', type: 'update', title: current, version: current, previous_version: previous, body: '', published_at: '2026-10-26T00:00:00Z' }
  await page.route('**/api/update/status', route => route.fulfill({ json: {
    phase: nextStaged ? 'staged' : 'succeeded', id: nextStaged ? 'next-upgrade' : 'completed-upgrade', current_version: current, version: nextStaged ? '9.0.0' : current,
    completed_update: { id: 'completed-upgrade', previous_version: previous, version: current, completed_at: notice.published_at },
  } }))
  await page.route('**/api/messages', route => route.fulfill({ json: { items: [{ ...notice, read_at: reads ? notice.published_at : undefined }], unread_count: reads ? 0 : 1 } }))
  await page.route('**/api/activity/summary', route => route.fulfill({ json: { message_unread_count: reads ? 0 : 1, automation_inbox_unread_count: 0, automation_running_count: 0 } }))
  await page.route('**/api/messages/*/read', route => { reads++; return route.fulfill({ json: { ...notice, read_at: notice.published_at } }) })
  await page.route('**/api/update/notes?**', route => {
    expect(new URL(route.request().url()).searchParams.get('from')).toBe(previous)
    expect(new URL(route.request().url()).searchParams.get('to')).toBe(current)
    return route.fulfill({ json: { from_version: previous, to_version: current, complete: true, releases: versions.map(version => ({ version, brief: `- ${version} 的升级变化` })) } })
  })
  await page.goto('/')
  await page.getByRole('button', { name: '查看变化', exact: true }).click()
  const reader = page.getByRole('dialog', { name: `已更新到 v${current}`, exact: true })
  await expect(reader.getByText('v0.6.0 的升级变化', { exact: true })).toBeVisible()
  await expect.poll(() => reads).toBe(1)
  await page.keyboard.press('Escape')
  nextStaged = true
  await page.reload()
  await expect(page.getByRole('button', { name: '版本 9.0.0 已准备好，重启更新', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '查看变化', exact: true })).toBeHidden()
  await page.getByRole('button', { name: '打开消息中心', exact: true }).click()
  const inbox = page.getByRole('dialog', { name: '消息中心', exact: true })
  await expect(inbox.getByRole('heading', { name: `已更新到 v${current}`, exact: true })).toBeVisible()
  await expect(inbox.getByText(`v${previous}`, { exact: true })).toBeVisible()
  await expect(inbox.getByText('v0.6.0 的升级变化', { exact: true })).toBeVisible()
  expect(reads).toBe(1)
})

test('missing, empty and failed release notes stay explicit without blocking installation', async ({ page, request, browserDiagnostics }) => {
  browserDiagnostics.allow(/console\.error: Failed to load resource:.*500/)
  browserDiagnostics.allow(/http\.5xx: GET .*\/api\/update\/notes\?.* returned 500/)
  await createAndOpenBook(request, 'Unavailable Upgrade Notes Book')
  let mode: 'failed' | 'partial' | 'empty' = 'failed'
  await page.route('**/api/update/status', route => route.fulfill({ json: { phase: 'idle', current_version: previous,
    check: { current_version: previous, latest_version: current, update_available: true, can_install: true, platform: 'darwin-arm64' },
  } }))
  await page.route('**/api/update/notes?**', route => mode === 'failed'
    ? route.fulfill({ status: 500, json: { error: '更新内容加载失败，请重试或查看发布记录。' } })
    : route.fulfill({ json: { from_version: previous, to_version: current, complete: false, releases: mode === 'empty' ? [] : [{ version: `v${current}`, brief: '- 已加载的变化' }] } }))
  await page.goto('/')
  await page.getByRole('button', { name: `发现新版本 ${current}`, exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '前往更新', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('更新内容加载失败')
  await expect(page.getByRole('button', { name: '安装更新', exact: true })).toBeEnabled()
  mode = 'partial'
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '部分版本记录缺失' })).toBeVisible()
  await page.getByRole('button', { name: '查看本次更新', exact: true }).click()
  await expect(page.getByRole('dialog').getByText('已加载的变化', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  mode = 'empty'
  await page.reload()
  await expect(page.getByText('此版本未提供更新说明', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '安装更新', exact: true })).toBeEnabled()
})
