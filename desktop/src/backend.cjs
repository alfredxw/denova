const { spawn } = require('node:child_process')
const { EventEmitter } = require('node:events')
const { randomBytes } = require('node:crypto')
const { createInterface } = require('node:readline')

// Only this owner has the pipe and launch token. No lifecycle endpoint is
// exposed through HTTP, and no secret appears in process arguments or URLs.
class Backend extends EventEmitter {
  constructor(executable, cwd, dataDir, { development = false } = {}) {
    super()
    this.token = randomBytes(32).toString('hex')
    this.pending = new Map()
    this.nextID = 0
    this.stopping = false
    const env = { ...process.env, DENOVA_DIR: dataDir }
    delete env.NOVA_DIR
    delete env.DENOVA_WORKSPACE
    delete env.NOVA_WORKSPACE
    this.child = spawn(executable, development ? ['--dev-mode'] : [], { cwd, env, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] })
    this.child.stderr.on('data', chunk => process.stderr.write(chunk))
    this.child.stdin.on('error', error => this.emit('diagnostic', error))
    this.exited = new Promise(resolve => {
      this.child.once('close', (code, signal) => {
        this.closed = true
        for (const request of this.pending.values()) request.reject(new Error(`Backend exited: ${code ?? signal}`))
        this.pending.clear()
        resolve()
        if (!this.stopping) this.emit('crash', new Error(`Backend exited: ${code ?? signal}`))
      })
    })
    this.ready = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('Backend startup timed out')), 120_000)
      const fail = error => { clearTimeout(timeout); reject(error) }
      this.child.once('error', fail)
      this.child.once('close', () => fail(new Error('Backend exited before readiness')))
      createInterface({ input: this.child.stdout }).on('line', line => {
        let message
        try { message = JSON.parse(line) } catch { process.stderr.write(`Invalid backend message: ${line}\n`); return }
        if (message.type === 'ready') {
          clearTimeout(timeout)
          this.url = message.url
          this.version = message.version
          resolve(message)
        } else if (message.type === 'reply') {
          const request = this.pending.get(message.id)
          if (!request) return
          this.pending.delete(message.id)
          message.error ? request.reject(new Error(message.error)) : request.resolve(message.result)
        }
      })
    })
    this.child.stdin.write(JSON.stringify({ type: 'start', token: this.token }) + '\n')
  }

  request(type) {
    if (this.closed || this.stopping) return Promise.reject(new Error('Backend is stopping'))
    const id = ++this.nextID
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`Backend command timed out: ${type}`)) }, 15_000)
      this.pending.set(id, {
        resolve: value => { clearTimeout(timer); resolve(value) },
        reject: error => { clearTimeout(timer); reject(error) },
      })
      this.child.stdin.write(JSON.stringify({ id, type }) + '\n')
    })
  }

  async stop(onlyWhenIdle = false) {
    if (this.closed) return true
    if (this.stopping) { await this.exited; return true }
    // The busy check and admission fence happen atomically inside Go.
    if (onlyWhenIdle) {
      const reply = await this.request('shutdown-if-idle')
      if (!reply.stopping) return false
    }
    this.stopping = true
    this.child.stdin.end()
    // Closing the owner pipe is sufficient, including during startup. Do not
    // kill the child while it is flushing data or canceling running tasks.
    await this.exited
    return true
  }
}

module.exports = { Backend }
