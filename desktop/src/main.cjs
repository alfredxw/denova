const { app, BrowserWindow, dialog, ipcMain, Menu, nativeImage, nativeTheme, net, protocol, session, shell, Tray } = require('electron')
const fs = require('node:fs')
const path = require('node:path')
const { Backend } = require('./backend.cjs')
const { DesktopUpdates } = require('./updates.cjs')
const { ORIGIN, isAppURL, forwardRequest } = require('./protocol.cjs')
const { initializeLog } = require('./log.cjs')
const locales = require('./locales.cjs')

app.setName('Denova')
if (process.env.DENOVA_DESKTOP_USER_DATA) app.setPath('userData', path.resolve(process.env.DENOVA_DESKTOP_USER_DATA))

let window, tray, backend, updates, preferences, locale, allowQuit = false, quitting = false, recovering = false
const t = key => locales[locale || 'en-US'][key]
const preferencePath = () => path.join(app.getPath('userData'), 'desktop.json')
function savePreferences() { fs.writeFileSync(preferencePath() + '.tmp', JSON.stringify(preferences)); fs.renameSync(preferencePath() + '.tmp', preferencePath()) }
function focus() { if (window && !window.isDestroyed()) { if (window.isMinimized()) window.restore(); window.show(); window.focus() } }

async function chooseDirectory(initialPath) {
  const options = { title: t('dataTitle'), defaultPath: typeof initialPath === 'string' ? initialPath : app.getPath('home'), properties: ['openDirectory', 'createDirectory'] }
  const result = window && !window.isDestroyed() ? await dialog.showOpenDialog(window, options) : await dialog.showOpenDialog(options)
  return { canceled: result.canceled, path: result.filePaths[0] || '' }
}

async function chooseData() {
  const selection = await chooseDirectory(preferences.dataDir)
  if (selection.canceled) return false
  // A selected existing folder is used in place; no copying or migration of
  // absolute host preferences into the portable data format is necessary.
  preferences.dataDir = selection.path
  savePreferences()
  return true
}

async function confirmStop(reason = 'quit') {
  if (!backend?.url || backend.closed) return true
  const activity = await backend.request('inspect')
  if (!activity.active_tasks && !activity.scheduled_tasks) return true
  console.info('[desktop] quit confirmation required', { activeTasks: activity.active_tasks, scheduledTasks: activity.scheduled_tasks })
  const [message, action] = {
    quit: ['quitMessage', 'quit'],
    update: ['updateMessage', 'update'],
    data: ['changeDataMessage', 'changeDataConfirm'],
  }[reason]
  const { response } = await dialog.showMessageBox({ type: 'question', message: t(message), detail: t('quitDetail'), buttons: [t('cancel'), t(action)], defaultId: 0, cancelId: 0 })
  return response === 1
}

async function quit() {
  if (quitting) return
  quitting = true
  try {
    if (!await confirmStop()) return
    updates?.dispose()
    await backend?.stop()
    console.info('[desktop] backend stopped; quitting application')
    allowQuit = true
    app.quit()
  } catch (error) { await showError(error) } finally { quitting = false }
}

async function restartForUpdate(onlyWhenIdle) {
  if (quitting) return false
  quitting = true
  try {
    if (!onlyWhenIdle && !await confirmStop('update')) return false
    if (!await backend.stop(onlyWhenIdle)) return false
    updates.dispose()
    updates.updater.once('error', error => {
      allowQuit = false
      queueMicrotask(() => void recover(error))
    })
    allowQuit = true
    // The child has exited and all journals are closed before NSIS/Squirrel
    // touches any installed files. Desktop updates replace the whole bundle.
    updates.updater.quitAndInstall(false, true)
    return true
  } finally { quitting = false }
}

async function changeData() {
  if (quitting) return
  quitting = true
  try {
    if (!await confirmStop('data')) return
    if (!await chooseData()) return
    updates?.dispose()
    await backend?.stop()
    // An explicit selection must survive relaunch even when this instance was
    // started with an environment override. Future external launches may still
    // deliberately supply their own directory.
    delete process.env.DENOVA_DIR
    console.info('[desktop] data directory changed; relaunching application')
    app.relaunch()
    allowQuit = true
    app.quit()
  } finally { quitting = false }
}

async function showError(error) {
  console.error('[desktop] operation failed', error)
  await dialog.showMessageBox({ type: 'error', message: t('operationFailed'), buttons: [t('ok')] })
}

function menus() {
  const perform = action => () => { void action().catch(showError) }
  const appMenu = [
    { role: 'about', label: t('about') },
    { label: t('open'), click: focus },
    { label: t('data'), click: perform(changeData) },
    { label: t('logs'), click: () => void shell.openPath(path.join(app.getPath('userData'), 'log')) },
    { type: 'separator' },
    ...(process.platform === 'darwin' ? [{ role: 'hide', label: t('hide') }] : []),
    { label: t('quit'), accelerator: 'CommandOrControl+Q', click: perform(quit) },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    { label: 'Denova', submenu: appMenu },
    { label: t('edit'), submenu: ['undo', 'redo', 'cut', 'copy', 'paste', 'selectAll'].map(role => ({ role, label: t(role) })) },
    { label: t('view'), submenu: [
      { role: 'reload', label: t('reload') }, { role: 'resetZoom', label: t('resetZoom') },
      { role: 'zoomIn', label: t('zoomIn') }, { role: 'zoomOut', label: t('zoomOut') },
      { role: 'togglefullscreen', label: t('fullscreen') }, { role: 'minimize', label: t('minimize') },
    ] },
  ]))
  tray?.setContextMenu(Menu.buildFromTemplate(appMenu.filter(item => !item.role)))
}

async function recover(error) {
  if (recovering || quitting) return
  recovering = true
  console.error('[desktop] backend unavailable', error)
  updates?.dispose()
  // Close the old pipe even when initialization timed out. Never run two
  // children against the same data folder or automatically replay tasks.
  await backend?.stop()
  while (!allowQuit) {
    const { response } = await dialog.showMessageBox({ type: 'error', message: t('failure'), detail: t('failureDetail'), buttons: [t('retry'), t('logs'), t('choose'), t('quit')], defaultId: 0, cancelId: 3 })
    if (response === 1) { await shell.openPath(path.join(app.getPath('userData'), 'log')); continue }
    if (response === 3) { allowQuit = true; app.quit(); break }
    if (response === 2 && !await chooseData()) continue
    try { await startBackend(); break } catch (nextError) { console.error('[desktop] retry failed', nextError); await backend?.stop() }
  }
  recovering = false
}

async function startBackend() {
  const directory = app.isPackaged ? path.join(process.resourcesPath, 'backend') : path.join(__dirname, '../build/backend')
  backend = new Backend(path.join(directory, process.platform === 'win32' ? 'denova.exe' : 'denova'), app.getPath('userData'), preferences.dataDir,
    { development: !app.isPackaged || process.argv.includes('--dev-mode') })
  backend.on('diagnostic', error => console.error('[desktop/backend]', error))
  // Readiness failures are handled by the caller; runtime failures by this listener.
  let ready = false
  backend.on('crash', error => { if (ready) void recover(error) })
  const info = await backend.ready
  console.info('[desktop] backend ready', { version: info.version })
  ready = true
  if (info.locale === 'zh-CN' || info.locale === 'en-US') locale = info.locale
  const version = app.getVersion()
  if (preferences.version && preferences.version !== version) {
    preferences.completedUpdate = { id: `desktop-${version}`, previous_version: preferences.version, version, completed_at: new Date().toISOString() }
  }
  preferences.version = version
  savePreferences()
  const { autoUpdater } = require('electron-updater')
  updates = new DesktopUpdates({ updater: autoUpdater, version, completedUpdate: preferences.completedUpdate, enabled: app.isPackaged,
    backend: () => backend, restart: restartForUpdate, reportError: key => t(key || 'operationFailed') })
  if (window && !window.isDestroyed()) window.destroy()
  window = new BrowserWindow({ width: 1280, height: 850, minWidth: 480, minHeight: 480, show: false,
    ...(process.platform === 'darwin' ? { titleBarStyle: 'hidden', titleBarOverlay: { height: 48 }, trafficLightPosition: { x: 12, y: 17 } } : {}),
    backgroundColor: nativeTheme.shouldUseDarkColors ? '#09090b' : '#ffffff',
    webPreferences: { preload: path.join(__dirname, 'preload.cjs'), contextIsolation: true, sandbox: true, nodeIntegration: false,
      additionalArguments: [`--denova-backend=${info.url}`] },
  })
  window.webContents.setWindowOpenHandler(({ url }) => { openExternal(url); return { action: 'deny' } })
  window.webContents.on('will-navigate', (event, url) => { if (!isAppURL(url)) { event.preventDefault(); openExternal(url) } })
  window.webContents.on('will-attach-webview', event => event.preventDefault())
  window.webContents.on('render-process-gone', (_event, details) => { if (!allowQuit) { console.error('[desktop] renderer exited', details); void recover(new Error('Renderer exited')) } })
  window.on('close', event => {
    if (allowQuit) return
    event.preventDefault()
    window.hide()
    if (process.platform === 'win32' && !preferences.backgroundExplained) {
      preferences.backgroundExplained = true
      savePreferences()
      tray.displayBalloon({ title: t('background'), content: t('backgroundDetail') })
    }
  })
  menus()
  await window.loadURL(ORIGIN + '/')
  focus()
}

function openExternal(value) {
  try { if (['https:', 'http:', 'mailto:'].includes(new URL(value).protocol)) void shell.openExternal(value) } catch { /* Reject malformed external links. */ }
}

function allowRendererPermission(contents, permission, details) {
  // Existing copy and game fullscreen actions belong to the trusted UI.
  // Embedded plugin/game documents do not inherit these permissions.
  return contents === window?.webContents && details.isMainFrame && isAppURL(details.requestingUrl) &&
    ['clipboard-sanitized-write', 'fullscreen'].includes(permission)
}

function registerIPC() {
  const handle = (name, action) => ipcMain.handle(name, async (event, ...args) => {
    if (event.sender !== window?.webContents || event.senderFrame !== window.webContents.mainFrame || !isAppURL(event.senderFrame.url)) throw new Error('Untrusted desktop caller')
    try { return await action(...args) } catch (error) { console.error(`[desktop/ipc] ${name} failed`, error); throw new Error(t('operationFailed')) }
  })
  handle('desktop:directory', chooseDirectory)
  handle('desktop:change-data-directory', changeData)
  handle('desktop:appearance', appearance => {
    if (!appearance || !['en-US', 'zh-CN'].includes(appearance.locale) || !['dark', 'light', 'system'].includes(appearance.theme)) throw new Error('Invalid appearance')
    locale = appearance.locale
    nativeTheme.themeSource = appearance.theme
    preferences.locale = locale
    preferences.theme = appearance.theme
    savePreferences()
    menus()
  })
  handle('desktop:update-status', () => updates.status())
  handle('desktop:update-check', () => updates.check())
  handle('desktop:update-download', () => updates.download())
  handle('desktop:update-apply', when => updates.apply(when))
}

if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', focus)
  app.on('activate', focus)
  app.on('before-quit', event => { if (!allowQuit) { event.preventDefault(); void quit() } })
  app.on('window-all-closed', () => {})
  app.whenReady().then(async () => {
    fs.mkdirSync(app.getPath('userData'), { recursive: true })
    initializeLog(app.getPath('userData'))
    try { preferences = JSON.parse(fs.readFileSync(preferencePath(), 'utf8')) } catch (error) { if (error.code !== 'ENOENT') throw error; preferences = {} }
    locale = preferences.locale || (app.getLocale().startsWith('zh') ? 'zh-CN' : 'en-US')
    nativeTheme.themeSource = preferences.theme || 'dark'
    if (process.env.DENOVA_DIR) preferences.dataDir = path.resolve(process.env.DENOVA_DIR)
    if (!preferences.dataDir) {
      const { response } = await dialog.showMessageBox({ title: t('dataTitle'), message: t('dataMessage'), detail: t('dataDetail'), buttons: [t('useDefault'), t('choose'), t('quit')], cancelId: 2 })
      if (response === 0) preferences.dataDir = path.join(app.getPath('home'), '.denova')
      if (response === 2 || response === 1 && !await chooseData()) { allowQuit = true; app.quit(); return }
      savePreferences()
    }
    if (process.platform === 'win32') {
      tray = new Tray(nativeImage.createFromPath(path.join(__dirname, '../assets/tray.png')))
      tray.setToolTip('Denova')
      tray.on('double-click', focus)
    }
    // Use a standard origin so extension iframe referrers and CSP keep their
    // Web semantics. This reserved host is handled in-process, without TLS or
    // DNS configuration; all other HTTPS requests retain Chromium networking.
    protocol.handle('https', request => isAppURL(request.url)
      ? forwardRequest(request, backend)
      : net.fetch(request, { bypassCustomProtocolHandlers: true }))
    session.defaultSession.setPermissionRequestHandler((contents, permission, callback, details) => callback(allowRendererPermission(contents, permission, details)))
    session.defaultSession.setPermissionCheckHandler((contents, permission, _origin, details) => allowRendererPermission(contents, permission, details))
    session.defaultSession.webRequest.onBeforeSendHeaders({ urls: ['ws://127.0.0.1/*'] }, (details, callback) => {
      // A sandboxed extension iframe shares webContents, so also require the
      // actual owning main frame before injecting the private credential.
      if (backend?.url && details.url.startsWith(backend.url.replace('http:', 'ws:') + '/api/terminal/sessions/') &&
          details.webContentsId === window?.webContents.id && details.frame === window.webContents.mainFrame && isAppURL(details.frame.url)) {
        details.requestHeaders['Origin'] = ORIGIN
        details.requestHeaders['X-Denova-Desktop'] = backend.token
      }
      callback({ requestHeaders: details.requestHeaders })
    })
    registerIPC()
    try { await startBackend() } catch (error) { await recover(error) }
  }).catch(async error => { console.error('[desktop] startup failed', error); await showError(error); allowQuit = true; app.quit() })
}
