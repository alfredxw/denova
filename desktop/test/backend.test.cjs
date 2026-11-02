const assert = require('node:assert/strict')
const { spawn } = require('node:child_process')
const { once } = require('node:events')
const { mkdtempSync, readFileSync, rmSync } = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { test } = require('node:test')

test('backend survives disconnected launcher pipes and desktop diagnostics remain logged', async t => {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'denova-backend-log-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const fixture = `
    const lines = require('node:readline').createInterface({ input: process.stdin })
    lines.on('line', line => {
      const command = JSON.parse(line)
      if (command.type === 'start') {
        process.stdout.write(JSON.stringify({ type: 'ready', url: 'http://127.0.0.1:1' }) + '\\n')
      } else {
        process.stderr.write('Backend diagnostic after disconnect\\n')
        process.stdout.write('Invalid control message after disconnect\\n')
        process.stdout.write(JSON.stringify({ type: 'reply', id: command.id, result: { active_tasks: false } }) + '\\n')
      }
    })
    lines.on('close', () => process.exit(0))
  `
  // Keep real child processes and OS pipes; only replace the Go executable
  // with a deterministic private-control protocol fixture.
  const host = spawn(process.execPath, ['-e', `
    const assert = require('node:assert/strict')
    const { mock } = require('node:test')
    const childProcess = require('node:child_process')
    const spawn = childProcess.spawn
    mock.method(childProcess, 'spawn', (_executable, _args, options) =>
      spawn(process.execPath, ['-e', ${JSON.stringify(fixture)}], options))
    const { Backend } = require(${JSON.stringify(require.resolve('../src/backend.cjs'))})
    const { initializeLog } = require(${JSON.stringify(require.resolve('../src/log.cjs'))})
    process.on('uncaughtExceptionMonitor', error => process.send(error.stack))
    process.once('message', async directory => {
      initializeLog(directory)
      const backend = new Backend(process.execPath, directory, directory)
      try {
        await backend.ready
        assert.deepEqual(await backend.request('inspect'), { active_tasks: false })
        console.info('Desktop stdout after disconnect')
        console.warn('Desktop stderr after disconnect')
      } finally {
        await backend.stop()
        process.disconnect()
      }
    })
  `], { stdio: ['ignore', 'pipe', 'pipe', 'ipc'] })
  const failures = []
  host.on('message', message => failures.push(message))
  t.after(() => { if (host.exitCode === null) host.kill() })
  const exited = once(host, 'exit', { signal: AbortSignal.timeout(5000) })
  const disconnected = [once(host.stdout, 'close'), once(host.stderr, 'close')]
  host.stdout.destroy()
  host.stderr.destroy()
  await Promise.all(disconnected)
  host.send(directory)
  const [code, signal] = await exited
  assert.equal(code, 0, `Backend host did not exit normally: ${signal}\n${failures.join('\n')}`)
  const log = readFileSync(path.join(directory, 'log/desktop.log'), 'utf8')
  assert.match(log, /Desktop stdout after disconnect/)
  assert.match(log, /Desktop stderr after disconnect/)
})
