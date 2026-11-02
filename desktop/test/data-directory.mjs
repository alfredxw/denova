import assert from 'node:assert/strict'
import { once } from 'node:events'
import { mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const requireWeb = createRequire(path.join(root, 'web/package.json'))
const { _electron: electron, expect } = requireWeb('@playwright/test')
const requireDesktop = createRequire(path.join(root, 'desktop/package.json'))
const temporary = mkdtempSync(path.join(os.tmpdir(), 'denova-data-directory-'))
const dataDir = path.join(temporary, 'original')
const nextDir = path.join(temporary, 'selected')
const userData = path.join(temporary, 'shell')
const artifacts = path.join(root, 'output/playwright/desktop')
const packaged = process.env.DENOVA_DESKTOP_PACKAGED === 'true'
const executablePath = packaged
  ? path.join(root, 'desktop/dist', process.platform === 'darwin' ? 'mac-arm64/Denova.app/Contents/MacOS/Denova' : 'win-unpacked/Denova.exe')
  : requireDesktop('electron')
const env = { ...process.env, DENOVA_DIR: dataDir, DENOVA_DESKTOP_USER_DATA: userData }
delete env.NOVA_DIR
delete env.ELECTRON_RUN_AS_NODE
let application, page

async function launch() {
  application = await electron.launch({ executablePath, args: [...(packaged ? [] : [path.join(root, 'desktop')]), '--dev-mode'], env })
  page = await application.firstWindow()
  page.setDefaultTimeout(30_000)
  await page.waitForURL('https://denova.localhost/**')
  // Empty roots resolve to the bookshelf, but the shell can restore Settings
  // after a relaunch. Wait for either surface before using its toggle button.
  await expect(page.getByRole('heading', { name: '书籍管理', exact: true })
    .or(page.locator('[data-nova-settings-content]')).first()).toBeVisible({ timeout: 30_000 })
  const skip = page.getByRole('button', { name: '跳过引导', exact: true }).last()
  if (await skip.isVisible()) await skip.click()
  const settings = page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true })
  if (await settings.getAttribute('aria-current') !== 'page') await settings.click()
  await page.getByRole('button', { name: '数据目录', exact: true }).click()
  await expect(page.getByRole('button', { name: '切换目录…', exact: true })).toBeVisible()
}

try {
  mkdirSync(artifacts, { recursive: true })
  for (const directory of [dataDir, nextDir]) {
    mkdirSync(directory)
    writeFileSync(path.join(directory, 'config.toml'), 'language = "zh-CN"\nupdate_check_enabled = false\nupdate_download_enabled = false\n')
  }
  writeFileSync(path.join(dataDir, 'keep.txt'), 'Original data stays in place')
  await launch()
  const change = page.getByRole('button', { name: '切换目录…', exact: true })
  const initialPath = await page.locator('[data-settings-section="paths"] code').first().textContent()
  assert.equal(realpathSync(initialPath), realpathSync(dataDir))

  for (const theme of ['dark', 'light']) {
    await page.evaluate(async theme => {
      const response = await fetch('/api/settings', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ layer: 'user', changes: { theme } }) })
      if (!response.ok) throw new Error('Could not change the test theme')
    }, theme)
    await page.reload()
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: theme === 'dark' ? '写作' : '游戏', exact: true }).click()
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '设置', exact: true }).click()
    for (const width of [1280, 500]) {
      await application.evaluate(({ BrowserWindow }, width) => BrowserWindow.getAllWindows()[0].setBounds({ width, height: 850 }), width)
      await page.locator('[data-settings-section="paths"]').scrollIntoViewIfNeeded()
      await expect(change).toBeVisible()
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      await page.screenshot({ path: path.join(artifacts, `data-directory-${theme}-${width}.png`) })
    }
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setBounds({ width: 1280, height: 850 }))
  }

  // Native selection is deterministic here; the real renderer, preload, IPC,
  // backend, disk preferences, and process shutdown remain in the test.
  await application.evaluate(({ dialog }) => {
    globalThis.directoryPickerCalls = 0
    dialog.showOpenDialog = async () => {
      globalThis.directoryPickerCalls++
      return { canceled: true, filePaths: [] }
    }
  })
  await change.click()
  await expect(change).toBeEnabled()
  assert.equal(await application.evaluate(() => globalThis.directoryPickerCalls), 1)
  assert.equal(realpathSync(JSON.parse(readFileSync(path.join(userData, 'desktop.json'))).dataDir), realpathSync(dataDir))
  console.log('Directory picker cancellation passed')

  // A failed settings save must not proceed to the native directory chooser.
  const config = path.join(dataDir, 'config.toml')
  renameSync(config, config + '.backup')
  mkdirSync(config)
  try {
    await page.getByRole('textbox', { name: 'Skills 目录', exact: true }).fill(path.join(dataDir, 'custom-skills'))
    await change.click()
    await expect(page.getByText('切换数据目录失败，请重试。', { exact: true })).toBeVisible()
    assert.equal(await application.evaluate(() => globalThis.directoryPickerCalls), 1)
    console.log('Failed settings save blocks directory switching')
  } finally {
    rmSync(config, { recursive: true, force: true })
    renameSync(config + '.backup', config)
  }

  // Canceling an active-task confirmation must keep the old backend alive.
  await application.evaluate(({ app, dialog }) => {
    const { Backend } = process.mainModule.require(app.getAppPath() + '/src/backend.cjs')
    globalThis.directoryInspect = Backend.prototype.request
    Backend.prototype.request = function (type) {
      return type === 'inspect' ? Promise.resolve({ active_tasks: true, scheduled_tasks: false }) : globalThis.directoryInspect.call(this, type)
    }
    dialog.showMessageBox = async options => {
      if (options.message !== '切换数据目录并重启 Denova？') throw new Error(`Unexpected confirmation: ${options.message}`)
      return { response: 0 }
    }
  })
  await change.click()
  await expect(change).toBeEnabled()
  assert.equal(await application.evaluate(() => globalThis.directoryPickerCalls), 1)
  const settings = await page.evaluate(async () => (await fetch('/api/settings')).json())
  assert.equal(settings.user.skills_dir, path.join(dataDir, 'custom-skills'))
  console.log('Active-task cancellation and pending settings flush passed')

  await application.evaluate(({ app, dialog }, { nextDir, receipt }) => {
    const { Backend } = process.mainModule.require(app.getAppPath() + '/src/backend.cjs')
    Backend.prototype.request = globalThis.directoryInspect
    dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [nextDir] })
    // Let the harness own the next launch so its debugger and cleanup follow
    // the new process. Record exactly what a native relaunch would inherit.
    app.relaunch = () => process.mainModule.require('node:fs').writeFileSync(receipt, JSON.stringify({ override: process.env.DENOVA_DIR ?? null }))
  }, { nextDir, receipt: path.join(temporary, 'relaunch.json') })
  const previousOrigin = await page.evaluate(() => window.denovaDesktop.backendOrigin)
  const exited = once(application.process(), 'exit', { signal: AbortSignal.timeout(30_000) })
  await change.click()
  const [code] = await exited
  assert.equal(code, 0)
  application = undefined
  assert.equal(JSON.parse(readFileSync(path.join(temporary, 'relaunch.json'))).override, null)
  await assert.rejects(fetch(previousOrigin + '/api/auth/status'))
  console.log('Directory selection stops the old backend and clears the environment override')
  delete env.DENOVA_DIR
  await launch()
  const selectedPath = await page.locator('[data-settings-section="paths"] code').first().textContent()
  assert.equal(realpathSync(selectedPath), realpathSync(nextDir))
  assert.equal(readFileSync(path.join(dataDir, 'keep.txt'), 'utf8'), 'Original data stays in place')
  assert.equal((await page.evaluate(async () => (await fetch('/api/settings')).json())).user.skills_dir || '', '')
  console.log(`Data directory switch passed (${process.platform}/${process.arch}): cancellation, save failure, task confirmation, settings flush, override removal, shutdown, restart, and original data preservation.`)
} catch (error) {
  if (page && !page.isClosed()) {
    await page.screenshot({ path: path.join(artifacts, 'data-directory-failure.png') }).catch(() => {})
    console.error((await page.locator('body').ariaSnapshot()).slice(0, 5000))
  }
  throw error
} finally {
  if (application) {
    await application.evaluate(({ app, dialog }) => {
      dialog.showMessageBox = async () => ({ response: 1 })
      app.quit()
    }).catch(() => {})
    await application.close()
  }
  rmSync(temporary, { recursive: true, force: true })
}
