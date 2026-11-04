import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { readFileSync, rmSync } from 'node:fs'
import path from 'node:path'
import { binary, workspace, until } from './driver.mjs'

// Run the exact production bundle with its inspector and automation disabled.
const directory = workspace('denova-package-')
const child = spawn(binary, [], { env: { ...process.env, DENOVA_DIR: path.join(directory, 'data'), DENOVA_DESKTOP_USER_DATA: path.join(directory, 'shell') }, stdio: 'ignore' })
try {
  await until(() => readFileSync(path.join(directory, 'shell/log/desktop.log'), 'utf8').includes('Desktop page loaded'), 120000)
  const exited = once(child, 'exit')
  child.kill('SIGTERM')
  const [code] = await exited
  if (process.platform !== 'win32') assert.equal(code, 0)
  // Windows child.kill terminates the test process, rather than sending a Unix
  // signal. The automation suite separately exercises the native Quit menu.
  console.log('Production application launched and rendered with automation disabled')
} finally { if (child.exitCode === null) child.kill(); rmSync(directory, { recursive: true, force: true }) }
