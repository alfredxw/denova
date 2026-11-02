import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const requireWeb = createRequire(path.join(root, 'web/package.json'))
const { _electron: electron, expect } = requireWeb('@playwright/test')
const requireDesktop = createRequire(path.join(root, 'desktop/package.json'))
const { Backend } = requireDesktop('./src/backend.cjs')
const temporary = mkdtempSync(path.join(os.tmpdir(), 'denova-desktop-'))
const artifacts = path.join(root, 'output/playwright/desktop')
const dataDir = path.join(temporary, 'data')
const userData = path.join(temporary, 'shell')
mkdirSync(dataDir, { recursive: true })
mkdirSync(artifacts, { recursive: true })
const model = spawn(process.execPath, [path.join(root, 'web/scripts/e2e-model-server.mjs')], {
  env: { ...process.env, DENOVA_E2E_MODEL_PORT: '0' }, stdio: ['ignore', 'pipe', 'pipe'],
})
model.stderr.pipe(process.stderr)
let application, page, backendOrigin
const errors = []
const packaged = process.env.DENOVA_DESKTOP_PACKAGED === 'true'
const binary = packaged
  ? path.join(root, 'desktop/dist', process.platform === 'darwin' ? 'mac-arm64/Denova.app/Contents/MacOS/Denova' : 'win-unpacked/Denova.exe')
  : requireDesktop('electron')
const env = { ...process.env, DENOVA_DIR: dataDir, DENOVA_DESKTOP_USER_DATA: userData }
delete env.NOVA_DIR
delete env.ELECTRON_RUN_AS_NODE

async function launch() {
  application = await electron.launch({ executablePath: binary, args: [...(packaged ? [] : [path.join(root, 'desktop')]), '--dev-mode'], env, timeout: 120_000 })
  application.process().stderr.on('data', chunk => { /* Keep backend logs for failed smoke runs. */ logs += chunk.toString() })
  page = await application.firstWindow({ timeout: 120_000 })
  page.setDefaultTimeout(30_000)
  page.on('pageerror', error => errors.push(error.message))
  await page.waitForURL('https://denova.localhost/**', { timeout: 30_000 })
  // CI displays can clamp the initial window below the desktop breakpoint.
  await application.evaluate(({ app, BrowserWindow }) => {
    const window = BrowserWindow.getAllWindows()[0]
    window.setContentSize(1280, 850)
    app.focus({ steal: true })
    window.focus()
  })
  await expect(page.getByLabel('工作台侧边栏')).toBeVisible({ timeout: 60_000 })
  backendOrigin = await page.evaluate(() => window.denovaDesktop.backendOrigin)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  console.log('Desktop window ready')
}

async function api(route, method = 'GET', body) {
  return page.evaluate(async ({ route, method, body }) => {
    const response = await fetch(route, { method, signal: AbortSignal.timeout(30_000), ...(body !== undefined ? { body: JSON.stringify(body), headers: { 'Content-Type': 'application/json' } } : {}) })
    const text = await response.text()
    if (!response.ok) throw new Error(`${route}: ${response.status} ${text}`)
    return text && response.headers.get('content-type')?.includes('application/json') ? JSON.parse(text) : text
  }, { route, method, body })
}

async function close() {
  const current = application
  if (!current) return
  await current.evaluate(({ dialog }) => {
    dialog.showMessageBox = async options => {
      if (options.message !== '退出 Denova？') throw new Error(`Unexpected quit dialog: ${options.message}`)
      return { response: 1, checkboxChecked: false }
    }
  })
  // Playwright must detach its debugger explicitly once stderr is disconnected;
  // otherwise Node waits for a debugger that cannot see its shutdown message.
  const [[code, signal]] = await Promise.all([
    once(current.process(), 'exit', { signal: AbortSignal.timeout(30_000) }),
    current.close(),
  ])
  assert.equal(code, 0, `Desktop did not quit normally: ${signal}`)
  application = undefined
}

let logs = ''
try {
  const modelURL = await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('Model fixture did not start')), 15_000)
    model.stdout.on('data', chunk => { const match = chunk.toString().match(/http:\/\/127\.0\.0\.1:\d+/); if (match) { clearTimeout(timeout); resolve(match[0]) } })
    model.once('error', reject)
  })
  writeFileSync(path.join(dataDir, 'config.toml'), `language = "zh-CN"\nupdate_check_enabled = false\nupdate_download_enabled = false\nmodel_max_retries = 1\n\n[[model_endpoints]]\nid = "e2e"\nname = "Desktop fixture"\nprovider = "openai-compatible"\nprotocol = "openai-chat-completions"\napi_key = "test"\nbase_url = "${modelURL}/v1"\n\n[[model_profiles]]\nid = "e2e"\nname = "Desktop fixture"\nendpoint_id = "e2e"\nmodel = "denova-e2e"\ncontext_window_tokens = 100000\n\n[agent_models.default]\nprofile_id = "e2e"\nthinking_level = "off"\n\n[agent_models.interactive_story]\nprofile_id = "e2e"\nthinking_level = "off"\n`)
  await launch()
  assert.equal(await page.evaluate(() => typeof window.require), 'undefined')
  if (process.platform === 'darwin') {
    const windowHeights = await application.evaluate(({ BrowserWindow }) => {
      const window = BrowserWindow.getAllWindows()[0]
      return [window.getBounds().height, window.getContentBounds().height]
    })
    assert.equal(windowHeights[0], windowHeights[1], 'macOS content must extend through the native title bar')
    for (const zoom of [0.8, 1]) {
      await application.evaluate(({ BrowserWindow }, zoom) => BrowserWindow.getAllWindows()[0].webContents.setZoomFactor(zoom), zoom)
      // CSS layout rounds fractional pixels differently from the native overlay.
      await expect.poll(() => page.locator('.nova-activity-brand-row > span').last().evaluate(element =>
        element.getBoundingClientRect().left - navigator.windowControlsOverlay.getTitlebarAreaRect().x)).toBeGreaterThanOrEqual(-1)
    }
  }
  await page.screenshot({ path: path.join(artifacts, 'desktop-empty.png'), animations: 'disabled' })
  await application.evaluate(async ({ clipboard, ClipboardItem }) => {
    // A fresh desktop can expose an empty item without any clipboard formats.
    globalThis.denovaSmokeClipboard = await Promise.all((await clipboard.read()).filter(item => item.types.length > 0).map(async item =>
      new ClipboardItem(Object.fromEntries(await Promise.all(item.types.map(async type => [type, await item.getType(type)]))))))
  })
  try {
    await page.evaluate(() => navigator.clipboard.writeText('Denova desktop clipboard smoke'))
    assert.equal(await application.evaluate(({ clipboard }) => clipboard.readText()), 'Denova desktop clipboard smoke')
  } finally {
    await application.evaluate(async ({ clipboard }) => {
      if (globalThis.denovaSmokeClipboard.length) await clipboard.write(globalThis.denovaSmokeClipboard)
      else clipboard.clear()
      delete globalThis.denovaSmokeClipboard
    })
  }
  console.log('Clipboard passed')
  for (const origin of ['https://foreign.invalid', 'https://denova.localhost']) {
    assert.equal((await fetch(backendOrigin + '/api/settings', { headers: { Origin: origin } })).status, 403)
  }
  const status = await api('/api/update/status')
  assert.equal(status.distribution, 'desktop')
  assert.match(await page.evaluate(async () => (await fetch('/api/update/install/stream', { method: 'POST' })).text()), /desktop updates are managed/i)

  // The process lock rejects a second backend and is released after normal quit.
  const probeDir = path.join(temporary, 'probe')
  mkdirSync(probeDir)
  const executable = path.join(root, 'desktop/build/backend', process.platform === 'win32' ? 'denova.exe' : 'denova')
  const duplicate = new Backend(executable, probeDir, dataDir, { development: true })
  await assert.rejects(duplicate.ready, /exited/)
  await duplicate.stop()
  console.log('Startup, sandbox and data lock passed')

  const book = await api('/api/books/create', 'POST', { title: 'Desktop smoke — A long book title for the navigation header', author: 'Denova tests' })
  await api('/api/workspace/switch', 'POST', { path: book.workspace })
  await api(`/api/projects/${book.project_id}/files/operations`, 'POST', { operations: [{ id: 'seed', kind: 'create', path: 'chapters/desktop-chapter.md', type: 'file', content: '# 起点\n\n初稿。' }] })
  await page.reload()
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '写作', exact: true }).click()
  await page.getByRole('button', { name: /^desktop chapter\b/ }).click()
  const editor = page.locator('.editor-content .ProseMirror')
  await expect(editor).toBeVisible()
  await editor.click()
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
  await page.keyboard.insertText('桌面写作\n\nElectron 真实保存。')
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+S' : 'Control+S')
  await expect.poll(async () => (await api(`/api/projects/${book.project_id}/files/file?path=chapters/desktop-chapter.md`)).content).toContain('Electron 真实保存。')
  console.log('Writing persistence passed')
  await page.screenshot({ path: path.join(artifacts, 'writing-desktop.png'), animations: 'disabled' })
  if (process.platform === 'darwin') {
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '收起', exact: true }).click()
    await expect.poll(() => page.getByLabel('工作台侧边栏').evaluate(element => element.getBoundingClientRect().width)).toBe(48)
    const navigationToggle = page.locator('[data-activity-bar-toggle]')
    // DOM clicks bypass native window hit testing. Also check the actual drag
    // rectangles so an unrelated header cannot swallow this floating button.
    await expect.poll(() => navigationToggle.evaluate(button => {
      const target = button.getBoundingClientRect()
      const overlaps = []
      for (const element of document.querySelectorAll('*')) {
        if (element.contains(button)) continue
        for (const pseudo of [null, '::before', '::after']) {
          const style = getComputedStyle(element, pseudo)
          if (style.getPropertyValue('-webkit-app-region') !== 'drag' || (pseudo && style.content === 'none')) continue
          let rect = element.getBoundingClientRect()
          if (pseudo) {
            const left = rect.left + element.clientLeft + parseFloat(style.left)
            const top = rect.top + element.clientTop + parseFloat(style.top)
            rect = { left, top, right: left + parseFloat(style.width), bottom: top + parseFloat(style.height) }
          }
          if (rect.left < target.right && rect.right > target.left && rect.top < target.bottom && rect.bottom > target.top) {
            overlaps.push(`${element.getAttribute('data-slot') || element.className}${pseudo || ''}`)
          }
        }
      }
      return overlaps
    })).toEqual([])
    await expect.poll(() => navigationToggle.evaluate(element =>
      element.getBoundingClientRect().left - navigator.windowControlsOverlay.getTitlebarAreaRect().x)).toBeGreaterThanOrEqual(-1)
    await expect.poll(() => navigationToggle.evaluate(element =>
      element.getBoundingClientRect().bottom <= navigator.windowControlsOverlay.getTitlebarAreaRect().height)).toBe(true)
    for (const name of ['作品目录', '项目文件', '全局搜索']) {
      const tab = page.getByRole('button', { name, exact: true })
      await expect.poll(() => tab.evaluate(element => {
        return element.getBoundingClientRect().top >= navigator.windowControlsOverlay.getTitlebarAreaRect().height
          && element.scrollWidth <= element.clientWidth
      })).toBe(true)
      await tab.click()
    }
    await page.getByRole('button', { name: '作品目录', exact: true }).click()
    await page.screenshot({ path: path.join(artifacts, 'writing-collapsed.png'), animations: 'disabled' })
    await page.getByRole('button', { name: '隐藏目录', exact: true }).click()
    await expect.poll(() => page.getByRole('tab', { name: 'desktop chapter', exact: true }).evaluate(element =>
      element.getBoundingClientRect().left - document.querySelector('[data-activity-bar-toggle]').getBoundingClientRect().right)).toBeGreaterThanOrEqual(-1)
    await page.getByRole('button', { name: '显示目录', exact: true }).click()
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '工作台', exact: true }).click()
    const projectToggle = page.locator('[data-agent-chat-sidebar-toggle] > button')
    await expect(projectToggle).toBeVisible()
    for (let i = 0; i < 2; i++) {
      await projectToggle.click()
      await expect.poll(() => projectToggle.evaluate(element =>
        element.getBoundingClientRect().left - document.querySelector('[data-activity-bar-toggle]').getBoundingClientRect().right)).toBeGreaterThanOrEqual(-1)
    }
    await page.screenshot({ path: path.join(artifacts, 'agent-workbench-collapsed.png'), animations: 'disabled' })
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '写作', exact: true }).click()
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '展开', exact: true }).click()
  }

  const streamed = await page.evaluate(async project => {
    const controller = new AbortController()
    const response = await fetch(`/api/projects/${project}/events`, { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]) })
    const first = await response.body.getReader().read()
    controller.abort()
    return !first.done && first.value.length > 0
  }, book.project_id)
  assert.equal(streamed, true)
  console.log('SSE delivery and cancellation passed')

  const terminal = await api(`/api/projects/${book.project_id}/terminal/sessions`, 'POST', { owner_tab_id: 'desktop-smoke', profile_id: 'shell', cols: 80, rows: 24 })
  const socketOpened = await page.evaluate(({ origin, terminal }) => new Promise((resolve, reject) => {
    const socket = new WebSocket(`${origin.replace('http:', 'ws:')}/api/terminal/sessions/${terminal.id}/attach?token=${terminal.token}`)
    socket.onopen = () => { socket.close(); resolve(true) }
    socket.onerror = () => reject(new Error('Desktop terminal WebSocket failed'))
  }), { origin: backendOrigin, terminal })
  assert.equal(socketOpened, true)
  await api(`/api/terminal/sessions/${terminal.id}`, 'DELETE')
  console.log('Terminal WebSocket passed')

  const story = await api('/api/interactive/stories', 'POST', { title: 'Desktop game', origin: '一扇石门挡在旧车站入口。', protagonist: { mode: 'custom', name: '旅人', profile: '调查旧车站。' }, choice_count: 2, planning_mode: 'disabled', state_schema_policy: { mode: 'fixed_template' } })
  await api('/api/interactive/chat', 'POST', { command_id: `desktop-opening-${story.id}`, mode: 'story', story_id: story.id, branch: 'main', start_opening: true })
  await expect.poll(async () => (await api(`/api/interactive/stories/${story.id}/snapshot?branch=main`)).turns?.length, { timeout: 30_000 }).toBe(1)
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  await expect(page.getByText('暮色落在旧车站外，石门后的轨道传来遥远的回声。', { exact: true })).toBeVisible({ timeout: 30_000 })
  console.log('Game turn persisted and rendered')
  for (const theme of ['dark', 'light']) {
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(1280, 850))
    await api('/api/settings', 'PATCH', { layer: 'user', changes: { theme } })
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await expect(page.getByText('暮色落在旧车站外，石门后的轨道传来遥远的回声。', { exact: true })).toBeVisible()
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '收起', exact: true }).click()
    if (process.platform === 'darwin') {
      await expect.poll(() => page.getByLabel('工作台侧边栏').evaluate(element => element.getBoundingClientRect().width)).toBe(48)
      await expect.poll(() => page.locator('.nova-story-stage-controls').evaluate(element =>
        element.getBoundingClientRect().left - document.querySelector('[data-activity-bar-toggle]').getBoundingClientRect().right)).toBeGreaterThanOrEqual(-1)
    }
    await page.screenshot({ path: path.join(artifacts, `game-${theme}-collapsed.png`), animations: 'disabled' })
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '展开', exact: true }).click()
    for (const width of [1280, 500]) {
      await application.evaluate(({ BrowserWindow }, width) => BrowserWindow.getAllWindows()[0].setSize(width, 850), width)
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: path.join(artifacts, `game-${theme}-${width}.png`), animations: 'disabled' })
    }
  }
  await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(1280, 850))
  const extensionProject = await api('/api/agent-chat/projects/directory', 'POST', { name: 'Desktop extension fixture' })
  const extensionDirectory = (await api('/api/agent-chat/projects')).projects.find(project => project.id === extensionProject.id).path
  for (const fixture of ['runtime', 'static']) cpSync(path.join(root, 'internal/platform/testdata', fixture), extensionDirectory, { recursive: true })
  const development = (await api('/api/platform/manage/development')).find(source => source.projectId === extensionProject.id)
  const candidate = await api(`/api/platform/manage/development/${development.developmentId}/check`)
  await api('/api/platform/manage/packages/install', 'POST', { candidateId: candidate.candidateId, grants: candidate.manifest.permissions.required })
  await page.reload()
  await page.getByRole('button', { name: '新建', exact: true }).filter({ visible: true }).click()
  await page.getByLabel('游戏类型').click()
  await page.getByRole('option', { name: '测试包', exact: true }).click()
  await page.getByLabel('故事线名称（可选）').fill('Desktop extension')
  await page.getByRole('button', { name: '创建故事线', exact: true }).click()
  const extension = page.frameLocator('iframe[title="游戏画面"]')
  await expect(extension.getByRole('button', { name: '保存测试数据', exact: true })).toBeEnabled()
  await extension.getByRole('button', { name: '保存测试数据', exact: true }).click()
  await expect(extension.locator('[role="status"]')).toHaveText('测试数据已保存')
  assert.equal(await extension.locator('body').evaluate(() => typeof window.denovaDesktop), 'undefined')
  if (process.platform === 'darwin') {
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '收起', exact: true }).click()
    await expect.poll(() => page.locator('[data-slot="game-player-menu"]').evaluate(element =>
      element.getBoundingClientRect().left - document.querySelector('[data-activity-bar-toggle]').getBoundingClientRect().right)).toBeGreaterThanOrEqual(-1)
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '展开', exact: true }).click()
  }
  console.log('Extension iframe, handshake and storage passed')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true }).click()
  await page.getByRole('button', { name: '软件更新', exact: true }).click()
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '手动更新', exact: true })).toHaveCount(0)
  await page.screenshot({ path: path.join(artifacts, 'desktop-updates.png'), animations: 'disabled' })
  await api('/api/settings', 'PATCH', { layer: 'user', changes: { language: 'en-US' } })
  await page.reload()
  await expect(page.getByLabel('Workbench sidebar')).toBeVisible()
  await expect.poll(() => application.evaluate(({ Menu }) => Menu.getApplicationMenu().items[1].label)).toBe('Edit')
  await api('/api/settings', 'PATCH', { layer: 'user', changes: { language: 'zh-CN' } })
  await page.reload()
  await expect(page.getByLabel('工作台侧边栏')).toBeVisible()
  console.log('Desktop update UI and native language sync passed')
  await page.evaluate(() => localStorage.setItem('desktop-smoke-persistence', 'retained'))
  await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close())
  assert.equal(await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].isVisible()), false)
  assert.equal((await fetch(backendOrigin + '/api/auth/status')).ok, true)
  await close()
  console.log('Window close keeps backend alive; quit closes backend')
  await assert.rejects(fetch(backendOrigin + '/api/auth/status'))
  await launch()
  assert.equal(await page.evaluate(() => localStorage.getItem('desktop-smoke-persistence')), 'retained')
  assert.match((await api(`/api/projects/${book.project_id}/files/file?path=chapters/desktop-chapter.md`)).content, /Electron 真实保存/)
  console.log('Restart persistence passed')
  await application.evaluate(() => {
    globalThis.denovaSmokeMainErrors = []
    process.on('uncaughtExceptionMonitor', error => globalThis.denovaSmokeMainErrors.push(error.stack))
  })
  const output = [application.process().stdout, application.process().stderr]
  const disconnected = output.map(stream => once(stream, 'close'))
  for (const stream of output) stream.destroy()
  await Promise.all(disconnected)
  await application.evaluate(() => {
    console.info('[desktop/smoke] stdout after launcher disconnect')
    console.warn('[desktop/smoke] stderr after launcher disconnect')
  })
  assert.deepEqual(await application.evaluate(() => globalThis.denovaSmokeMainErrors), [])
  await api('/api/auth/status')
  console.log('Disconnected launcher pipes preserve logging and backend requests')
  // Native fullscreen needs an active window, which a test launcher cannot
  // assume on macOS. CDP key events also do not invoke the native Escape action.
  await application.evaluate(({ app, BrowserWindow }) => {
    const window = BrowserWindow.getAllWindows()[0]
    app.focus({ steal: true })
    window.focus()
    globalThis.denovaSmokeFullscreen = 'windowed'
    window.once('enter-full-screen', () => { globalThis.denovaSmokeFullscreen = 'entered' })
    window.once('leave-full-screen', () => { globalThis.denovaSmokeFullscreen = 'left' })
  })
  await expect.poll(() => application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].isFocused())).toBe(true)
  await page.evaluate(() => { void document.documentElement.requestFullscreen() })
  await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(true)
  // macOS reports isFullScreen before its animation has finished. Wait for the
  // native transition event before requesting the opposite transition.
  await expect.poll(() => application.evaluate(() => globalThis.denovaSmokeFullscreen), { timeout: 15_000 }).toBe('entered')
  assert.equal(await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].isFullScreen()), true)
  await page.evaluate(() => { void document.exitFullscreen() })
  await expect.poll(() => application.evaluate(() => globalThis.denovaSmokeFullscreen), { timeout: 15_000 }).toBe('left')
  await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(false)
  assert.equal(await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].isFullScreen()), false)
  console.log('Fullscreen passed')
  await close()
  const shellLog = readFileSync(path.join(userData, 'log/desktop.log'), 'utf8')
  assert.match(shellLog, /stdout after launcher disconnect/)
  assert.match(shellLog, /stderr after launcher disconnect/)
  assert.match(shellLog, /backend stopped; quitting application/)
  console.log('Disconnected launcher pipes preserve logging and normal shutdown')
  assert.deepEqual(errors, [])
  console.log(`Desktop smoke passed (${process.platform}/${process.arch}, ${packaged ? 'packaged' : 'development'}): writing, Game, extensions, clipboard, fullscreen, SSE, terminal, isolation, lock, close/background and restart persistence.`)
} finally {
  if (errors.length) console.error('Renderer failures:', errors)
  if (page && !page.isClosed()) { try { await page.screenshot({ path: path.join(artifacts, 'last-screen.png') }) } catch {} }
  if (application) {
    // Failed journeys may leave a model task active. Exercise the normal quit
    // path with an affirmative dialog response instead of killing its data.
    try { await application.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1 }) }); await close() }
    catch { await application?.close() }
  }
  writeFileSync(path.join(artifacts, 'desktop.log'), logs)
  try { cpSync(path.join(userData, 'log'), path.join(artifacts, 'backend-log'), { recursive: true }) } catch {}
  model.kill()
  await once(model, 'exit')
  rmSync(temporary, { recursive: true, force: true })
}
