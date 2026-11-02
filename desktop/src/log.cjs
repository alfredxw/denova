const fs = require('node:fs')
const path = require('node:path')
const { format } = require('node:util')

// Shell diagnostics sit next to the existing rotating Go logs. Retain one
// previous shell log; no renderer content or private control messages are logged.
function initializeLog(userData) {
  const directory = path.join(userData, 'log')
  fs.mkdirSync(directory, { recursive: true })
  const file = path.join(directory, 'desktop.log')
  if (fs.existsSync(file) && fs.statSync(file).size > 4 * 1024 * 1024) fs.renameSync(file, file + '.previous')
  // A GUI application can outlive the launcher reading its output pipes.
  for (const output of [process.stdout, process.stderr]) {
    output.on('error', error => { if (error.code !== 'EPIPE') throw error })
  }
  for (const level of ['info', 'warn', 'error']) {
    const original = console[level].bind(console)
    console[level] = (...args) => {
      original(...args)
      try { fs.appendFileSync(file, `${new Date().toISOString()} ${level.toUpperCase()} ${format(...args)}\n`) }
      catch (error) { original('[desktop/log] write failed', error) }
    }
  }
}

module.exports = { initializeLog }
