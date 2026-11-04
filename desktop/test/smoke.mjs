import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { launch, workspace, until, root } from './driver.mjs'

const directory = workspace('denova-mygo-smoke-')
const artifacts = path.join(root, 'output/desktop')
mkdirSync(artifacts, { recursive: true })
const model = spawn(process.execPath, [path.join(root, 'web/scripts/e2e-model-server.mjs')], { env: { ...process.env, DENOVA_E2E_MODEL_PORT: '0' }, stdio: ['ignore', 'pipe', 'inherit'] })
let app
const click = async text => until(() => app.evaluate(text => {
  const button = [...document.querySelectorAll('button,[role=option]')].find(e => e.getClientRects().length && (e.getAttribute('aria-label') === text || e.textContent.trim() === text))
  if (!button || button.disabled) return false
  button.click(); return true
}, text))
const ready = async () => {
  await until(() => app.evaluate(() => !!document.querySelector('[aria-label="工作台侧边栏"], [aria-label="Workbench sidebar"]')), 60000)
  await app.evaluate(() => { window.smokeErrors = []; window.addEventListener('error', e => window.smokeErrors.push(e.message)); window.addEventListener('unhandledrejection', e => window.smokeErrors.push(String(e.reason))) })
}
const reload = async () => { await app.evaluate(() => { window.smokeReloadPending = true; location.reload(); return true }); await until(() => app.evaluate(() => !window.smokeReloadPending && document.readyState === 'complete')); await ready() }
try {
  const modelURL = await new Promise((resolve, reject) => { let text = ''; model.stdout.on('data', chunk => { text += chunk; const match = text.match(/http:\/\/127\.0\.0\.1:\d+/); if (match) resolve(match[0]) }); model.once('error', reject) })
  writeFileSync(path.join(directory, 'data/config.toml'), `language = "zh-CN"\nupdate_check_enabled = false\nupdate_download_enabled = false\nmodel_max_retries = 1\n\n[[model_endpoints]]\nid = "e2e"\nname = "Desktop fixture"\nprovider = "openai-compatible"\nprotocol = "openai-chat-completions"\napi_key = "test"\nbase_url = "${modelURL}/v1"\n\n[[model_profiles]]\nid = "e2e"\nname = "Desktop fixture"\nendpoint_id = "e2e"\nmodel = "denova-e2e"\ncontext_window_tokens = 100000\n\n[agent_models.default]\nprofile_id = "e2e"\nthinking_level = "off"\n\n[agent_models.interactive_story]\nprofile_id = "e2e"\nthinking_level = "off"\n`)
  app = await launch(directory); await ready()
  const backend = await app.evaluate(() => window.denovaDesktop.backendOrigin)
  const origin = await app.evaluate(() => location.origin)
  assert.equal(await app.evaluate(() => typeof window.require), 'undefined')
  for (const Origin of [origin, 'https://foreign.invalid']) assert.equal((await fetch(backend + '/api/settings', { headers: { Origin } })).status, 403)
  assert.equal((await app.api('/api/update/status')).distribution, 'desktop')
  assert.equal((await app.evaluate(() => window.denovaDesktop.updates.preferences())).automaticChecks, false)
  await app.screenshot(path.join(artifacts, 'empty.png'))
  console.log('Startup, origin isolation and official updater preferences passed')

  const book = await app.api('/api/books/create', 'POST', { title: 'Desktop smoke — A long book title for the navigation header', author: 'Denova tests' })
  await app.api('/api/workspace/switch', 'POST', { path: book.workspace })
  await app.api(`/api/projects/${book.project_id}/files/operations`, 'POST', { operations: [{ id: 'seed', kind: 'create', path: 'chapters/desktop-chapter.md', type: 'file', content: '# 起点\n\n初稿。' }] })
  assert.equal(await app.evaluate(async ({ workspace, png }) => {
    const body = new FormData(); body.append('path', workspace)
    body.append('file', new Blob([Uint8Array.from(atob(png), c => c.charCodeAt(0))], { type: 'image/png' }), 'cover.png')
    const response = await fetch('/api/books/cover/upload', { method: 'POST', body })
    if (!response.ok) throw new Error(await response.text())
    return true
  }, { workspace: book.workspace, png: readFileSync(path.join(root, 'desktop/assets/tray.png')).toString('base64') }), true)
  await reload(); await click('写作')
  await until(() => app.evaluate(() => { const e = [...document.querySelectorAll('button')].find(e => e.textContent.trim().startsWith('desktop chapter')); if (!e) return false; e.click(); return true }))
  await until(() => app.evaluate(() => { const e = document.querySelector('.editor-content .ProseMirror'); return e?.isContentEditable && e.getClientRects().length && e.textContent.includes('初稿') }))
  await app.evaluate(() => { const e = document.querySelector('.editor-content .ProseMirror'); e.focus(); const range = document.createRange(); range.selectNodeContents(e); const selection = getSelection(); selection.removeAllRanges(); selection.addRange(range); if (!document.execCommand('insertText', false, '桌面写作\n\nMyGo 真实保存。')) throw new Error('WebView did not insert text'); document.dispatchEvent(new KeyboardEvent('keydown', { key: 's', code: 'KeyS', metaKey: navigator.platform.includes('Mac'), ctrlKey: !navigator.platform.includes('Mac'), bubbles: true })); return true })
  await until(async () => (await app.api(`/api/projects/${book.project_id}/files/file?path=chapters/desktop-chapter.md`)).content.includes('MyGo 真实保存。'))
  await app.screenshot(path.join(artifacts, 'writing-dark.png'))
  console.log('WebView editing and writing persistence passed')

  assert.equal(await app.evaluate(async project => {
    const c = new AbortController(); const r = await fetch(`/api/projects/${project}/events`, { signal: AbortSignal.any([c.signal, AbortSignal.timeout(5000)]) }); const first = await r.body.getReader().read(); c.abort(); return !first.done && first.value.length > 0
  }, book.project_id), true)
  const terminal = await app.api(`/api/projects/${book.project_id}/terminal/sessions`, 'POST', { owner_tab_id: 'desktop-smoke', profile_id: 'shell', cols: 80, rows: 24 })
  assert.equal(await app.evaluate(({ backend, terminal }) => new Promise((resolve, reject) => { const socket = new WebSocket(`${backend.replace('http:', 'ws:')}/api/terminal/sessions/${terminal.id}/attach?token=${terminal.token}`); socket.onopen = () => { socket.close(); resolve(true) }; socket.onerror = () => reject(new Error('Terminal WebSocket failed')) }), { backend, terminal }), true)
  assert.equal(await app.evaluate(({ backend, terminal }) => new Promise(resolve => { const socket = new WebSocket(`${backend.replace('http:', 'ws:')}/api/terminal/sessions/${terminal.id}/attach?token=invalid`); socket.onopen = () => { socket.close(); resolve(false) }; socket.onerror = () => resolve(true) }), { backend, terminal }), true)
  await app.api(`/api/terminal/sessions/${terminal.id}`, 'DELETE')
  console.log('Streaming cancellation and terminal authentication passed')

  const story = await app.api('/api/interactive/stories', 'POST', { title: 'Desktop game', origin: '一扇石门挡在旧车站入口。', protagonist: { mode: 'custom', name: '旅人', profile: '调查旧车站。' }, choice_count: 2, planning_mode: 'disabled', state_schema_policy: { mode: 'fixed_template' } })
  await app.api('/api/interactive/chat', 'POST', { command_id: `desktop-opening-${story.id}`, mode: 'story', story_id: story.id, branch: 'main', start_opening: true })
  await until(async () => (await app.api(`/api/interactive/stories/${story.id}/snapshot?branch=main`)).turns?.length === 1)
  await click('游戏')
  await until(() => app.evaluate(() => document.body.textContent.includes('暮色落在旧车站外')))
  for (const theme of ['dark', 'light']) {
    await app.api('/api/settings', 'PATCH', { layer: 'user', changes: { theme } }); await reload()
    await until(() => app.evaluate(theme => document.documentElement.dataset.theme === theme, theme))
    assert.equal(await app.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
    await app.screenshot(path.join(artifacts, `game-${theme}.png`))
  }
  console.log('Game generation, persistence and both themes passed')

  const project = await app.api('/api/agent-chat/projects/directory', 'POST', { name: 'Desktop extension fixture' })
  const projectDir = (await app.api('/api/agent-chat/projects')).projects.find(p => p.id === project.id).path
  for (const fixture of ['runtime', 'static']) cpSync(path.join(root, 'internal/platform/testdata', fixture), projectDir, { recursive: true })
  const development = (await app.api('/api/platform/manage/development')).find(s => s.projectId === project.id)
  const candidate = await app.api(`/api/platform/manage/development/${development.developmentId}/check`)
  await app.api('/api/platform/manage/packages/install', 'POST', { candidateId: candidate.candidateId, grants: candidate.manifest.permissions.required })
  await reload(); await click('新建')
  await until(() => app.evaluate(() => { const e = document.querySelector('#story-game-type'); if (!e) return false; e.click(); return true }))
  await click('测试包')
  await app.evaluate(() => { window.addEventListener('message', e => { const frame = document.querySelector('iframe[title="游戏画面"]'); if (frame && e.source === frame.contentWindow && e.origin === new URL(frame.src).origin && e.data.type === 'test:storage-ready') window.storageProbe = e.data }); return true })
  await click('创建故事线')
  const probe = await until(() => app.evaluate(() => window.storageProbe))
  assert.equal(probe.value, 0)
  assert.equal(probe.nativeBridge, 'undefined')
  await app.screenshot(path.join(artifacts, 'extension.png'))
  console.log('Extension origin handshake, storage read and native bridge isolation passed')

  await click('设置'); await click('软件更新'); await click('检查更新')
  // LoadHTML has no URL in WebView2; identify the new native window by ID.
  const updateWindow = await until(async () => (await app.command('windows')).windows.find(w => w.id !== app.windowID))
  await until(async () => /更新|update/i.test((await app.command('eval', { window: updateWindow.id, script: 'document.body.innerText' })).value))
  await app.command('eval', { window: updateWindow.id, script: 'setTimeout(() => window.mygo.window.close(), 100); true' })
  await until(async () => !(await app.command('windows')).windows.some(w => w.id === updateWindow.id))
  console.log('Official update window passed')
  await app.api('/api/settings', 'PATCH', { layer: 'user', changes: { language: 'en-US' } }); await reload()
  await until(() => app.evaluate(() => !!document.querySelector('[aria-label="Workbench sidebar"]')))
  await until(() => app.command('menu', { path: ['Denova', 'Check for Updates…'] }))
  const englishUpdate = await until(async () => (await app.command('windows')).windows.find(w => w.id !== app.windowID))
  await until(async () => /update/i.test((await app.command('eval', { window: englishUpdate.id, script: 'document.body.innerText' })).value))
  await app.command('eval', { window: englishUpdate.id, script: 'setTimeout(() => window.mygo.window.close(), 100); true' })
  await until(async () => !(await app.command('windows')).windows.some(w => w.id === englishUpdate.id))
  await app.api('/api/settings', 'PATCH', { layer: 'user', changes: { language: 'zh-CN' } }); await reload()
  await app.evaluate(() => localStorage.setItem('desktop-smoke-persistence', 'retained'))
  await app.evaluate(() => { window.mygo.window.close(); return true })
  await until(async () => !(await app.command('windows')).windows[0].visible)
  assert.equal((await fetch(backend + '/api/auth/status')).ok, true)
  await app.close(); app = null
  await assert.rejects(fetch(backend + '/api/auth/status'))
  app = await launch(directory); await ready()
  assert.equal(await app.evaluate(() => localStorage.getItem('desktop-smoke-persistence')), 'retained')
  assert.match((await app.api(`/api/projects/${book.project_id}/files/file?path=chapters/desktop-chapter.md`)).content, /MyGo 真实保存/)
  assert.deepEqual(await app.evaluate(() => window.smokeErrors), [])
  await app.close(); app = null
  console.log(`Desktop smoke passed (${process.platform}/${process.arch})`)
} finally {
  if (app) { try { console.error('Renderer diagnostics', await app.evaluate(() => ({ errors: window.smokeErrors, text: document.body.innerText.slice(0, 2000), resources: performance.getEntriesByType('resource').filter(r => r.duration > 5000).map(r => r.name) }))); await app.screenshot(path.join(artifacts, 'last-screen.png')); await app.close() } catch (error) { console.error(error); process.kill(app.pid, 'SIGTERM') } }
  cpSync(path.join(directory, 'shell/log'), path.join(artifacts, 'log'), { recursive: true })
  model.kill(); await once(model, 'exit')
  if (process.env.DENOVA_KEEP_TEST_DATA !== 'true') rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); else console.log(directory)
}
