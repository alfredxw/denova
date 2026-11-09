import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { readFileSync, rmSync } from 'node:fs'
import path from 'node:path'
import { binary, workspace, until } from './driver.mjs'

// Run the exact production bundle with its inspector and automation disabled.
const directory = workspace('denova-package-')
// Exercise a new installation: the product defaults to language = "auto".
// The smoke fixture's explicit language would hide native startup failures.
rmSync(path.join(directory, 'data/config.toml'))
const env = { ...process.env, DENOVA_DIR: path.join(directory, 'data'), DENOVA_DESKTOP_USER_DATA: path.join(directory, 'shell') }
const child = spawn(binary, [], { env, stdio: ['ignore', 'ignore', 'pipe'] })
let diagnostics = ''
child.stderr.on('data', data => { diagnostics += data })
async function stop() {
  if (child.exitCode === null && child.signalCode === null) {
    // Windows child.kill skips the shell's backend drain and leaves it writing.
    if (process.platform === 'win32') execFileSync(binary, [`--quit-pid=${child.pid}`], { env, stdio: 'ignore', timeout: 30000 })
    else child.kill('SIGTERM')
    await until(() => child.exitCode !== null || child.signalCode !== null, 30000)
  }
}
try {
  await until(() => child.exitCode !== null || child.signalCode !== null || readFileSync(path.join(directory, 'shell/log/desktop.log'), 'utf8').includes('Desktop page loaded'), 120000)
  assert.equal(child.exitCode, null, `Desktop exited before rendering a new installation\n${diagnostics}`)
  assert.equal(child.signalCode, null, `Desktop was terminated before rendering a new installation\n${diagnostics}`)
  await stop()
  assert.equal(child.exitCode, 0, diagnostics)
  assert.match(readFileSync(path.join(directory, 'shell/log/desktop.log'), 'utf8'), /Desktop shutdown completed/)
  console.log('Production application launched and rendered with automation disabled')
} finally {
  await stop()
  rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 })
}
