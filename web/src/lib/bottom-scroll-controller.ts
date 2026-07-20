export const DEFAULT_BOTTOM_THRESHOLD = 12
export const UPWARD_SCROLL_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

export interface DeferredBottomScrollScheduler {
  cancel: () => void
  schedule: (scrollNow: () => void, shouldContinue: () => boolean) => void
}

export function isElementNearBottom(element: HTMLElement, threshold = DEFAULT_BOTTOM_THRESHOLD): boolean {
  return element.scrollHeight - element.scrollTop - element.clientHeight <= threshold
}

/** Coalesces bottom writes so one scroll container is written at most once per frame. */
export function createDeferredBottomScrollScheduler(): DeferredBottomScrollScheduler {
  let frameID: number | null = null
  let pending: { scrollNow: () => void; shouldContinue: () => boolean } | null = null

  const cancel = () => {
    pending = null
    if (frameID !== null) {
      cancelAnimationFrame(frameID)
      frameID = null
    }
  }

  const schedule = (scrollNow: () => void, shouldContinue: () => boolean) => {
    pending = { scrollNow, shouldContinue }
    if (frameID !== null) return
    frameID = requestAnimationFrame(() => {
      frameID = null
      const request = pending
      pending = null
      if (request?.shouldContinue()) request.scrollNow()
    })
  }

  return { cancel, schedule }
}
