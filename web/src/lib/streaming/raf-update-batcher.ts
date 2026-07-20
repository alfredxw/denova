export type RafStateUpdate<T> = (current: T) => T

export interface RafUpdateBatcher<T> {
  enqueue: (update: RafStateUpdate<T>) => void
  flush: () => void
  discard: () => void
}

/** Coalesces ordered state updates into one commit per animation frame. */
export function createRafUpdateBatcher<T>(commit: (update: RafStateUpdate<T>) => void): RafUpdateBatcher<T> {
  let frameID: number | null = null
  let pending: RafStateUpdate<T>[] = []

  const commitPending = () => {
    if (pending.length === 0) return
    const updates = pending
    pending = []
    commit(current => updates.reduce((next, apply) => apply(next), current))
  }

  const flush = () => {
    if (frameID !== null) {
      cancelAnimationFrame(frameID)
      frameID = null
    }
    commitPending()
  }

  const discard = () => {
    if (frameID !== null) {
      cancelAnimationFrame(frameID)
      frameID = null
    }
    pending = []
  }

  const enqueue = (update: RafStateUpdate<T>) => {
    pending.push(update)
    if (frameID !== null) return
    frameID = requestAnimationFrame(() => {
      frameID = null
      commitPending()
    })
  }

  return { enqueue, flush, discard }
}
