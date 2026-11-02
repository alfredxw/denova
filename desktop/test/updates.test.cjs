const assert = require('node:assert/strict')
const { EventEmitter } = require('node:events')
const { test } = require('node:test')
const { DesktopUpdates } = require('../src/updates.cjs')

test('manual checks wait for an in-flight automatic check', async t => {
  const updater = new EventEmitter()
  const result = Promise.withResolvers()
  updater.checkForUpdates = () => result.promise
  const updates = new DesktopUpdates({ updater, version: '1.0.0', enabled: true,
    backend: () => ({ request: async () => ({ policy: { Check: true, Download: false } }) }),
    reportError: () => 'failed' })
  t.after(() => updates.dispose())
  const automatic = updates.tick()
  await Promise.resolve()
  const manual = updates.check()
  result.resolve({ isUpdateAvailable: true, updateInfo: { version: '1.1.0' } })
  assert.equal((await manual)?.latest_version, '1.1.0')
  await automatic
})

test('updates respect download consent and restart only after the backend fence succeeds', async t => {
  const updater = new EventEmitter()
  let downloads = 0, restarts = 0, busy = true
  const activity = { policy: { Check: true, Download: false }, active_tasks: true }
  updater.checkForUpdates = async () => ({ isUpdateAvailable: true, updateInfo: { version: '1.1.0', releaseDate: '2026-10-30' } })
  updater.downloadUpdate = async () => { downloads++; updater.emit('update-downloaded', { version: '1.1.0' }) }
  const updates = new DesktopUpdates({ updater, version: '1.0.0', enabled: true, backend: () => ({ request: async () => activity }),
    restart: async idle => { assert.equal(idle, true); if (busy) return false; restarts++; return true }, reportError: () => 'failed' })
  t.after(() => updates.dispose())
  await updates.tick()
  assert.equal(downloads, 0)
  assert.equal((await updates.status()).check.update_available, true)
  activity.policy.Download = true
  activity.policy.Check = false
  await updates.tick()
  assert.equal(downloads, 0)
  activity.policy.Check = true
  await updates.tick()
  assert.equal(downloads, 1)
  assert.equal((await updates.status()).phase, 'staged')
  await updates.apply('when_idle')
  await updates.tick()
  assert.equal(restarts, 0)
  assert.equal(updates.state.restart_waiting, true)
  await updates.apply('cancel')
  busy = false
  await updates.tick()
  assert.equal(restarts, 0)
  await updates.apply('when_idle')
  await updates.tick()
  assert.equal(restarts, 1)
  assert.equal(updates.state.restart_waiting, false)
})
