// Adapt electron-updater to the existing settings UI. Installation and signing
// stay with electron-updater; the Go owner supplies activity and admission.
class DesktopUpdates {
  constructor({ updater, version, completedUpdate, enabled, backend, restart, reportError }) {
    this.updater = updater
    this.backend = backend
    this.restart = restart
    this.reportError = reportError
    this.enabled = enabled
    this.state = { distribution: 'desktop', phase: 'idle', current_version: version, completed_update: completedUpdate }
    updater.autoDownload = false
    updater.autoInstallOnAppQuit = false
    updater.allowDowngrade = false
    this.onError = error => {
      console.error('[desktop/update] failed', error)
      this.state.operation_error = this.reportError()
      this.state.operation = undefined
    }
    this.onProgress = progress => {
      this.state.progress = { phase: 'downloading', percent: progress.percent, downloaded_bytes: progress.transferred, total_bytes: progress.total }
    }
    this.onDownloaded = info => {
      this.state.phase = 'staged'
      this.state.version = info.version
      this.state.id = `desktop-${info.version}`
      this.state.operation = undefined
      this.state.progress = { phase: 'staged', percent: 100 }
    }
    updater.on('error', this.onError)
    updater.on('download-progress', this.onProgress)
    updater.on('update-downloaded', this.onDownloaded)
    this.timer = setInterval(() => void this.tick(), 2000)
    this.lastCheck = 0
    this.lastDownload = 0
  }

  dispose() {
    clearInterval(this.timer)
    this.updater.off('error', this.onError)
    this.updater.off('download-progress', this.onProgress)
    this.updater.off('update-downloaded', this.onDownloaded)
  }

  async status() {
    const activity = await this.backend().request('inspect')
    return { ...this.state, busy: activity.active_tasks }
  }

  async check() {
    if (!this.enabled) throw new Error(this.reportError('updateDisabled'))
    if (this.state.operation === 'downloading' || this.state.phase !== 'idle') return this.state.check
    // electron-updater shares the pending check promise. Manual callers must
    // await it too, rather than receive an empty or stale cached result.
    this.state.operation = 'checking'
    this.state.operation_error = undefined
    this.lastCheck = Date.now()
    try {
      const result = await this.updater.checkForUpdates()
      if (!result) throw new Error('No update result')
      const info = result.updateInfo
      this.state.check = {
        current_version: this.state.current_version,
        latest_version: info.version,
        update_available: result.isUpdateAvailable,
        can_install: result.isUpdateAvailable,
        platform: `${process.platform}-${process.arch}`,
        release_url: `https://github.com/alfredxw/denova/releases/tag/v${info.version}`,
        published_at: info.releaseDate,
      }
      this.state.checked_at = new Date().toISOString()
      return this.state.check
    } finally { this.state.operation = undefined }
  }

  async download() {
    if (!this.enabled || this.state.operation || !this.state.check?.update_available) throw new Error('Update is not available')
    if (this.state.phase !== 'staged') {
      this.state.operation = 'downloading'
      this.state.operation_error = undefined
      this.lastDownload = Date.now()
      try { await this.updater.downloadUpdate() } finally { this.state.operation = undefined }
    }
    return { previous_version: this.state.current_version, installed_version: this.state.version, installed: false, staged: true, apply_ready: true, restart_required: true }
  }

  async apply(when) {
    if (!['now', 'when_idle', 'cancel'].includes(when)) throw new Error('Invalid restart mode')
    if (this.state.phase !== 'staged') throw new Error('No staged desktop update')
    const result = { id: this.state.id, version: this.state.version }
    if (when === 'cancel') {
      this.state.restart_waiting = false
      return { ...result, status: 'canceled' }
    }
    if (when === 'when_idle') {
      this.state.restart_waiting = true
      return { ...result, status: 'waiting_for_idle' }
    }
    return { ...result, status: await this.restart(false) ? 'restarting' : 'canceled' }
  }

  async tick() {
    if (this.ticking || !this.enabled || !this.backend() || this.backend().stopping || this.backend().closed) return
    this.ticking = true
    try {
      if (this.state.restart_waiting) {
        if (await this.restart(true)) this.state.restart_waiting = false
        return
      }
      const { policy } = await this.backend().request('inspect')
      if (!policy.Check || this.state.operation || this.state.phase !== 'idle') return
      if (Date.now() - this.lastCheck > 6 * 60 * 60 * 1000) await this.check()
      if (policy.Download && this.state.check?.update_available && Date.now() - this.lastDownload > 60 * 60 * 1000) await this.download()
    } catch (error) {
      console.error('[desktop/update] background operation failed', error)
      this.state.operation_error = this.reportError()
      // A failed restart requires an explicit retry, not a dialog every tick.
      this.state.restart_waiting = false
    } finally { this.ticking = false }
  }
}

module.exports = { DesktopUpdates }
