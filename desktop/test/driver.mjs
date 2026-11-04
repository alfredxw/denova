import { spawn, execFileSync } from 'node:child_process'
import { once } from 'node:events'
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
export const binary = process.env.DENOVA_DESKTOP_BINARY || path.join(root, 'desktop/dist', process.platform === 'darwin' ? 'darwin-arm64/Denova.app/Contents/MacOS/Denova' : 'windows-amd64/Denova.exe')
const control = process.platform === 'win32' ? path.join(root, 'desktop/build/automation-control.exe') : null
if (control) execFileSync('go', ['build', '-o', control, './desktop/test/control'], { cwd: root, stdio: 'inherit' })
export const delay = ms => new Promise(resolve => setTimeout(resolve, ms))
export async function until(test, timeout = 30000) {
  const end = Date.now() + timeout
  let error
  while (Date.now() < end) { try { const value = await test(); if (value) return value } catch (e) { error = e }; await delay(100) }
  throw error || new Error(`Condition did not become true in ${timeout} ms`)
}
export async function launch(directory, extraEnv = {}) {
  const socket = path.join(directory, 'automation.sock')
  const env = { DENOVA_DIR: path.join(directory, 'data'), DENOVA_DESKTOP_USER_DATA: path.join(directory, 'shell'), MYGO_AUTOMATION: socket, ...extraEnv }
  // Launch Services gives the app its own resource coalition. Executing its
  // binary from a terminal would attribute WebKit XPCs to that terminal too.
  const mac = process.platform === 'darwin'
  const pids = () => execFileSync('ps', ['-axo', 'pid=,comm='], { encoding: 'utf8' }).split('\n').flatMap(line => { const m = line.trim().match(/^(\d+)\s+(.*)$/); return m?.[2] === binary ? [+m[1]] : [] })
  const previous = mac ? pids() : []
  const child = mac
    ? spawn('open', ['-W', '-n', '-a', path.resolve(binary, '../../..'), ...Object.entries(env).flatMap(([k, v]) => ['--env', `${k}=${v}`]), '--args', '--dev-mode'], { stdio: ['ignore', 'pipe', 'pipe'] })
    : spawn(binary, ['--dev-mode'], { env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] })
  let logs = ''
  let windowID
  child.stdout.on('data', data => { logs += data })
  child.stderr.on('data', data => { logs += data })
  const command = (method, params = {}) => new Promise((resolve, reject) => {
    // Windows resolves an omitted window to the focused window. An official
    // update dialog can take focus, so keep application operations on the host.
    const request = JSON.stringify({ id: 1, method, params: method === 'windows' ? params : { window: windowID, ...params } }) + '\n'
    const decode = data => { try { const r = JSON.parse(data); if (r.error) reject(new Error(r.error.message)); else resolve(r.result) } catch (error) { reject(error) } }
    if (control) {
      const client = spawn(control, [socket], { stdio: ['pipe', 'pipe', 'pipe'] })
      let output = '', error = ''
      client.stdout.on('data', data => { output += data })
      client.stderr.on('data', data => { error += data })
      client.on('error', reject)
      client.on('close', code => code ? reject(new Error(error)) : decode(output))
      client.stdin.end(request)
      return
    }
    const connection = net.createConnection(socket)
    const timer = setTimeout(() => { connection.destroy(); reject(new Error(`Automation timeout: ${method}\n${logs}`)) }, 45000)
    let data = ''
    connection.on('error', error => { clearTimeout(timer); reject(error) })
    connection.on('connect', () => connection.write(request))
    connection.on('data', chunk => { data += chunk; if (!data.includes('\n')) return; clearTimeout(timer); connection.end(); decode(data.split('\n')[0]) })
    connection.on('end', () => { clearTimeout(timer); if (!data.includes('\n')) reject(new Error(`Automation connection ended: ${method}`)) })
  })
  const evaluate = (fn, arg) => command('eval', { script: `(${fn.toString()})(${JSON.stringify(arg) ?? 'undefined'})` }).then(result => result.value)
  try { await until(async () => { if (child.exitCode !== null) throw new Error(`Desktop exited: ${child.exitCode}\n${logs}`); windowID = (await command('windows')).windows[0]?.id; return windowID !== undefined }, 120000) }
  catch (error) {
    if (mac) for (const pid of pids().filter(pid => !previous.includes(pid))) { try { process.kill(pid, 'SIGTERM') } catch {} }
    child.kill(); throw error
  }
  const pid = mac ? await until(() => pids().find(p => !previous.includes(p))) : child.pid
  return { child, pid, windowID, command, evaluate,
    api: (route, method = 'GET', body) => evaluate(async ({ route, method, body }) => { const r = await fetch(route, { method, ...(body !== undefined ? { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) } : {}) }); const text = await r.text(); if (!r.ok) throw new Error(`${route}: ${r.status} ${text}`); return text && !r.headers.get('content-type')?.includes('text/event-stream') ? JSON.parse(text) : null }, { route, method, body }),
    screenshot: async file => { const result = await command('capture'); writeFileSync(file, Buffer.from(result.png, 'base64')) },
    close: async () => { const ended = once(child, 'exit'); await command('menu', { path: ['Denova', '退出 Denova'] }).catch(() => command('menu', { path: ['Denova', 'Quit Denova'] })); await Promise.race([ended, delay(15000).then(() => { if (child.exitCode === null) throw new Error('Desktop failed to quit') })]) },
  }
}
export function workspace(prefix = 'denova-mygo-') { const directory = mkdtempSync(path.join(os.tmpdir(), prefix)); mkdirSync(path.join(directory, 'data')); writeFileSync(path.join(directory, 'data/config.toml'), 'language = "zh-CN"\nupdate_check_enabled = false\nupdate_download_enabled = false\n'); return directory }
