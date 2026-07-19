export interface SaveLaneRequest<T> {
  scopeKey: string
  value: T
}

export interface SaveLaneOptions<T, TResult = void> {
  delayMs: number
  save: (request: SaveLaneRequest<T>) => Promise<TResult>
  onSaved?: (request: SaveLaneRequest<T>, result: TResult) => void
  onError?: (request: SaveLaneRequest<T>, error: unknown) => void
}

export type SaveLaneStatus = 'saved' | 'pending' | 'saving' | 'blocked' | 'error'

export class SaveLaneBlockedError extends Error {
  readonly reason: unknown

  constructor(reason: unknown = null) {
    super('Autosave is blocked until the draft becomes valid')
    this.name = 'SaveLaneBlockedError'
    this.reason = reason
  }
}

export interface SaveLaneSnapshot {
  scopeKey: string | null
  status: SaveLaneStatus
  error: unknown | null
}

export interface SaveLane<T, TResult = void> {
  reset: (scopeKey: string) => void
  setDelayMs: (delayMs: number) => void
  edit: (value: T) => void
  /** Replaces dirty work after an external reload without moving the user's after-delay deadline. */
  reload: (value: T) => boolean
  cancel: () => void
  flush: () => Promise<TResult | null>
  block: (reason?: unknown) => void
  unblock: () => void
  dispose: (mode?: 'flush' | 'discard') => Promise<TResult | null>
  hasWork: () => boolean
  getSnapshot: () => SaveLaneSnapshot
  subscribe: (listener: () => void) => () => void
}

interface QueuedSave<T> {
  generation: number
  request: SaveLaneRequest<T>
}

/**
 * Creates one keyed autosave lane. The lane debounces edits, serializes writes,
 * and coalesces all waiting snapshots to the latest value.
 */
export function createSaveLane<T, TResult = void>(
  options: SaveLaneOptions<T, TResult>,
): SaveLane<T, TResult> {
  let delayMs = normalizeDelayMs(options.delayMs)
  let scopeKey: string | null = null
  let generation = 0
  let timer: ReturnType<typeof setTimeout> | null = null
  let pending: QueuedSave<T> | null = null
  let pendingReady = false
  let inFlight: QueuedSave<T> | null = null
  let blocked = false
  let disposed = false
  let snapshot: SaveLaneSnapshot = { scopeKey: null, status: 'saved', error: null }
  const listeners = new Set<() => void>()
  const idleWaiters = new Set<(result: TResult | null) => void>()
  let flushResult: TResult | null = null

  const publish = (status: SaveLaneStatus, error: unknown | null = null) => {
    if (snapshot.scopeKey === scopeKey && snapshot.status === status && Object.is(snapshot.error, error)) return
    snapshot = { scopeKey, status, error }
    listeners.forEach(listener => listener())
  }

  const clearDelay = () => {
    if (timer !== null) clearTimeout(timer)
    timer = null
  }

  const settleIfQuiescent = () => {
    if (inFlight !== null || (pending !== null && pendingReady)) return
    const result = flushResult
    flushResult = null
    idleWaiters.forEach(resolve => resolve(result))
    idleWaiters.clear()
  }

  const abandonWaiters = () => {
    flushResult = null
    idleWaiters.forEach(resolve => resolve(null))
    idleWaiters.clear()
  }

  const isCurrent = (queued: QueuedSave<T>) => queued.generation === generation
  const isCurrentSaving = () => inFlight?.generation === generation

  const drain = () => {
    if (blocked || inFlight !== null || !pendingReady || pending === null) return
    const queued = pending
    pending = null
    pendingReady = false
    inFlight = queued
    if (isCurrent(queued)) publish('saving')

    let savePromise: Promise<TResult>
    try {
      savePromise = options.save(queued.request)
    } catch (error) {
      savePromise = Promise.reject(error)
    }

    void savePromise.then(
      result => {
        inFlight = null
        let observerError: unknown | null = null
        if (isCurrent(queued)) {
          try {
            options.onSaved?.(queued.request, result)
          } catch (error) {
            observerError = error
            console.error('[save-lane] onSaved observer failed after persistence completed', error)
          }
          if (isCurrent(queued)) {
            if (idleWaiters.size > 0) flushResult = result
            if (observerError !== null) publish('error', observerError)
            else if (blocked) publish('blocked', snapshot.error)
            else if (pending !== null) publish('pending')
            else publish('saved')
          }
        }
        drain()
        settleIfQuiescent()
      },
      error => {
        inFlight = null
        if (isCurrent(queued)) {
          try {
            options.onError?.(queued.request, error)
          } catch (observerError) {
            console.error('[save-lane] onError observer failed while retaining the save request', observerError)
          }
          if (pending === null) {
            pending = queued
            pendingReady = false
            publish(blocked ? 'blocked' : 'error', error)
          } else {
            publish(blocked ? 'blocked' : 'pending', blocked ? snapshot.error : null)
          }
        }
        drain()
        settleIfQuiescent()
      },
    )
  }

  const armDelay = () => {
    clearDelay()
    pendingReady = false
    if (blocked || pending === null) return
    timer = setTimeout(() => {
      timer = null
      pendingReady = true
      drain()
    }, delayMs)
  }

  const discard = () => {
    disposed = true
    generation += 1
    clearDelay()
    pending = null
    pendingReady = false
    blocked = false
    abandonWaiters()
    listeners.clear()
  }

  const lane: SaveLane<T, TResult> = {
    reset(nextScopeKey) {
      if (disposed) throw new Error('Cannot reset a disposed save lane')
      generation += 1
      abandonWaiters()
      clearDelay()
      pending = null
      pendingReady = false
      blocked = false
      scopeKey = nextScopeKey
      publish('saved')
      settleIfQuiescent()
    },
    setDelayMs(nextDelayMs) {
      if (disposed) throw new Error('Cannot configure a disposed save lane')
      delayMs = normalizeDelayMs(nextDelayMs)
      if (timer !== null && pending !== null) armDelay()
    },
    edit(value) {
      if (disposed) throw new Error('Cannot edit a disposed save lane')
      if (scopeKey === null) throw new Error('Save lane must be reset to a scope before editing')
      pending = { generation, request: { scopeKey, value } }
      if (blocked) {
        clearDelay()
        pendingReady = false
        publish('blocked', snapshot.error)
        return
      }
      if (!isCurrentSaving()) publish('pending')
      if (idleWaiters.size > 0) {
        clearDelay()
        pendingReady = true
        drain()
      } else {
        armDelay()
      }
    },
    reload(value) {
      if (disposed) throw new Error('Cannot reload a disposed save lane')
      if (scopeKey === null) throw new Error('Save lane must be reset to a scope before reloading')
      if (pending === null && inFlight === null) return false
      pending = { generation, request: { scopeKey, value } }
      if (blocked) {
        pendingReady = false
        publish('blocked', snapshot.error)
        return true
      }
      if (timer !== null) {
        publish(isCurrentSaving() ? 'saving' : 'pending')
        return true
      }
      pendingReady = true
      publish(isCurrentSaving() ? 'saving' : 'pending')
      drain()
      return true
    },
    cancel() {
      if (disposed) return
      clearDelay()
      pending = null
      pendingReady = false
      if (!isCurrentSaving()) publish('saved')
      settleIfQuiescent()
    },
    flush() {
      if (disposed) return Promise.resolve(null)
      if (blocked) return Promise.reject(new SaveLaneBlockedError(snapshot.error))
      clearDelay()
      if (pending !== null) pendingReady = true
      if (inFlight === null && pending === null) return Promise.resolve(null)
      const promise = new Promise<TResult | null>(resolve => idleWaiters.add(resolve))
      drain()
      settleIfQuiescent()
      return promise
    },
    block(reason = null) {
      blocked = true
      clearDelay()
      pendingReady = false
      publish('blocked', reason)
      settleIfQuiescent()
    },
    unblock() {
      if (!blocked) return
      blocked = false
      if (pending === null) {
        publish(isCurrentSaving() ? 'saving' : 'saved')
        return
      }
      publish(isCurrentSaving() ? 'saving' : 'pending')
      armDelay()
    },
    async dispose(mode = 'discard') {
      if (disposed) return null
      const result = mode === 'flush' ? await lane.flush() : null
      discard()
      return result
    },
    hasWork() {
      return inFlight !== null || pending !== null
    },
    getSnapshot() {
      return snapshot
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
  return lane
}

function normalizeDelayMs(value: number): number {
  if (!Number.isFinite(value) || value < 0) throw new Error('Save lane delay must be a non-negative finite number')
  return Math.floor(value)
}
