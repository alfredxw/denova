import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { root, until, workspace } from './driver.mjs'

if (process.platform !== 'win32') throw new Error('This test requires native Windows')
const directory = workspace('denova-restart-')
const binary = path.join(root, 'desktop/build/Denova.exe')
const script = path.join(root, 'scripts/restart-desktop.ps1')
const env = { ...process.env, DENOVA_DIR: path.join(directory, 'data'), DENOVA_DESKTOP_USER_DATA: path.join(directory, 'shell') }
const logPath = path.join(directory, 'shell/log/desktop.log')
const powershell = command => execFileSync('powershell.exe', ['-NoProfile', '-Command', command], { encoding: 'utf8' })
const quote = value => `'${value.replaceAll("'", "''")}'`
const hosts = () => JSON.parse(powershell(`ConvertTo-Json -InputObject @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq ${quote(binary)} } | Select-Object ProcessId,ParentProcessId)`))
assert.equal(hosts().length, 0, 'Quit the development desktop before running the isolated test')
const launchers = []
const start = (...args) => {
  // Use a different cwd and Windows PowerShell 5.1, including paths with spaces.
  const child = spawn('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', script, ...args], { cwd: directory, env, stdio: 'pipe' })
  let output = ''
  child.stdout.on('data', data => { output += data; process.stdout.write(data) })
  child.stderr.on('data', data => { output += data; process.stderr.write(data) })
  launchers.push({ child, output: () => output })
  return child
}
const logs = () => readFileSync(logPath, 'utf8')
const loaded = async count => until(() => {
  const text = logs()
  if ((text.match(/Desktop page loaded/g) || []).length < count) return
  return [...text.matchAll(/Desktop ready.*backend_pid=(\d+)/g)].at(-1)?.[1]
}, 300000)
const exited = child => until(() => child.exitCode !== null, 30000).then(() => assert.equal(child.exitCode, 0))
const gone = ids => assert.equal(powershell(`@(Get-Process -Id ${ids.join(',')} -ErrorAction SilentlyContinue).Count`).trim(), '0')
try {
  const first = start(...(process.argv.includes('--rebuild') ? ['-Rebuild'] : []))
  const backend1 = await loaded(1)
  const host1 = hosts()[0]
  assert.ok(host1, 'Desktop started')
  execFileSync(binary, [`--quit-pid=${host1.ProcessId + 1}`], { env })
  assert.equal(hosts()[0]?.ProcessId, host1.ProcessId, 'A different target PID must not quit this instance')

  const second = start()
  const backend2 = await loaded(2)
  await exited(first)
  const host2 = hosts()[0]
  assert.equal(hosts().length, 1, 'Restart must leave exactly one desktop')
  assert.notEqual(host2.ProcessId, host1.ProcessId)
  assert.notEqual(backend2, backend1)
  gone([host1.ProcessId, host1.ParentProcessId, backend1])
  assert.match(logs(), /Desktop shutdown completed/, 'Restart drained the previous backend normally')

  execFileSync(binary, [`--quit-pid=${host2.ProcessId}`], { env })
  await exited(second)
  gone([host2.ProcessId, host2.ParentProcessId, backend2])
  execFileSync(binary, [`--quit-pid=${host2.ProcessId}`], { env })
  assert.equal(hosts().length, 0, 'A quit request without a running instance must not open a window')
  console.log('Windows restart passed: fresh start, PID guard, normal shutdown, HMR cleanup, second start, and no-instance quit')
} finally {
  writeFileSync(path.join(directory, 'restart.log'), launchers.map(item => item.output()).join('\n'))
  for (const host of hosts()) {
    try { execFileSync(binary, [`--quit-pid=${host.ProcessId}`], { env }) } catch {}
  }
  console.log(`Restart diagnostics: ${directory}`)
}
