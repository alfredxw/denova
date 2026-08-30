import type { ActiveInteractiveChat } from '../../api'

/** Waits for a user/network signal after a reconnect projection stops changing. */
export function waitForStoryStageReconnect(signal: AbortSignal, isDisposed: () => boolean) {
  if (signal.aborted || isDisposed()) return Promise.reject(signal.reason || new DOMException('Aborted', 'AbortError'))
  return new Promise<void>((resolve, reject) => {
    const cleanup = () => {
      window.removeEventListener('online', resume)
      window.removeEventListener('focus', resume)
      document.removeEventListener('visibilitychange', resumeWhenVisible)
      signal.removeEventListener('abort', abort)
    }
    const resume = () => {
      cleanup()
      if (signal.aborted || isDisposed()) reject(signal.reason || new DOMException('Aborted', 'AbortError'))
      else resolve()
    }
    const resumeWhenVisible = () => {
      if (document.visibilityState === 'visible') resume()
    }
    const abort = () => {
      cleanup()
      reject(signal.reason || new DOMException('Aborted', 'AbortError'))
    }
    window.addEventListener('online', resume, { once: true })
    window.addEventListener('focus', resume, { once: true })
    document.addEventListener('visibilitychange', resumeWhenVisible)
    signal.addEventListener('abort', abort, { once: true })
  })
}

export function isRetryableStoryStageObservationError(error: unknown) {
  if (!error || typeof error !== 'object') return true
  const status = Number((error as { status?: unknown }).status)
  if (!Number.isFinite(status)) return true
  if (status < 400 || status >= 500) return true
  return isStoryStageProjectionRefreshError(error)
}

export function isStoryStageProjectionRefreshError(error: unknown) {
  if (!error || typeof error !== 'object') return false
  const code = (error as { code?: unknown }).code
  return code === 'agent_runtime.rehydrate_required' ||
    code === 'agent_runtime.recovery_changed' ||
    code === 'agent_runtime.stream_attached'
}

export function storyStageProjectionFingerprint(active: ActiveInteractiveChat) {
  return JSON.stringify([
    active.task_id || '',
    active.active_operation_id || '',
    active.recovery_paused === true,
    active.pending_interruption_id || '',
    active.recovery_actions || [],
  ])
}
