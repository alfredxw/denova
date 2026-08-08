export type RafStateUpdate<T> = (current: T) => T

export interface RafUpdateBatcher<T> {
  enqueue: (update: RafStateUpdate<T>) => void
  flush: () => void
  discard: () => void
}

export interface RafUpdateBatcherOptions {
  /** Minimum time between React commits. Updates remain ordered and are never dropped. */
  minIntervalMs?: number
}

// Text streaming stays perceptually fluid at this cadence while leaving most
// of each frame available for Markdown, virtualization, and input handling.
export const STREAMING_RENDER_INTERVAL_MS = 80

/** Coalesces ordered state updates and commits them on a render-budgeted animation frame. */
export function createRafUpdateBatcher<T>(
  commit: (update: RafStateUpdate<T>) => void,
  options: RafUpdateBatcherOptions = {},
): RafUpdateBatcher<T> {
  let frameID: number | null = null
  let timerID: ReturnType<typeof setTimeout> | null = null
  let lastCommitAt: number | null = null
  let pending: RafStateUpdate<T>[] = []
  const minIntervalMs = Math.max(0, options.minIntervalMs || 0)

  const commitPending = () => {
    if (pending.length === 0) return
    const updates = pending
    pending = []
    lastCommitAt = Date.now()
    commit(current => updates.reduce((next, apply) => apply(next), current))
  }

  const cancelScheduledCommit = () => {
    if (frameID !== null) {
      cancelAnimationFrame(frameID)
      frameID = null
    }
    if (timerID !== null) {
      clearTimeout(timerID)
      timerID = null
    }
  }

  const scheduleFrame = () => {
    if (frameID !== null) return
    frameID = requestAnimationFrame(() => {
      frameID = null
      commitPending()
    })
  }

  const scheduleCommit = () => {
    if (frameID !== null || timerID !== null) return
    const elapsed = lastCommitAt === null ? minIntervalMs : Date.now() - lastCommitAt
    const delay = Math.max(0, minIntervalMs - elapsed)
    if (delay === 0) {
      scheduleFrame()
      return
    }
    timerID = setTimeout(() => {
      timerID = null
      scheduleFrame()
    }, delay)
  }

  const flush = () => {
    cancelScheduledCommit()
    commitPending()
  }

  const discard = () => {
    cancelScheduledCommit()
    pending = []
  }

  const enqueue = (update: RafStateUpdate<T>) => {
    pending.push(update)
    scheduleCommit()
  }

  return { enqueue, flush, discard }
}
