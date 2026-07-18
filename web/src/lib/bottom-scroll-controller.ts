export const DEFAULT_BOTTOM_THRESHOLD = 12
export const UPWARD_SCROLL_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

const DEFERRED_SCROLL_DELAY_MS = 80

export interface DeferredBottomScrollScheduler {
  cancel: () => void
  schedule: (scrollNow: () => void, shouldContinue: () => boolean) => void
}

export function isElementNearBottom(element: HTMLElement, threshold = DEFAULT_BOTTOM_THRESHOLD): boolean {
  return element.scrollHeight - element.scrollTop - element.clientHeight <= threshold
}

/** Shared multi-frame scheduler used by DOM and Virtuoso bottom-lock adapters. */
export function createDeferredBottomScrollScheduler(): DeferredBottomScrollScheduler {
  let frameIDs: number[] = []
  let timerID: number | null = null

  const cancel = () => {
    for (const id of frameIDs) cancelAnimationFrame(id)
    frameIDs = []
    if (timerID !== null) {
      window.clearTimeout(timerID)
      timerID = null
    }
  }

  const schedule = (scrollNow: () => void, shouldContinue: () => boolean) => {
    cancel()
    if (!shouldContinue()) return
    scrollNow()
    frameIDs.push(requestAnimationFrame(() => {
      if (!shouldContinue()) return
      scrollNow()
      frameIDs.push(requestAnimationFrame(() => {
        if (shouldContinue()) scrollNow()
      }))
    }))
    timerID = window.setTimeout(() => {
      timerID = null
      if (shouldContinue()) scrollNow()
    }, DEFERRED_SCROLL_DELAY_MS)
  }

  return { cancel, schedule }
}
