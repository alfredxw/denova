import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent, UIEvent, WheelEvent } from 'react'
import type { VirtuosoHandle } from 'react-virtuoso'
import { createDeferredBottomScrollScheduler, DEFAULT_BOTTOM_THRESHOLD, isElementNearBottom, UPWARD_SCROLL_KEYS } from '@/lib/bottom-scroll-controller'

export const VIRTUOSO_BOTTOM_THRESHOLD = DEFAULT_BOTTOM_THRESHOLD
const VIRTUOSO_AWAY_FROM_BOTTOM_THRESHOLD = 160
const DOWNWARD_SCROLL_KEYS = new Set(['ArrowDown', 'PageDown', 'End', ' '])

export interface ScrollElementBottomIntoViewOptions {
  bottomInsetPx?: number
  visibleBottomPx?: number
  lockAfterScroll?: boolean
}

interface VirtuosoBottomLockOptions {
  resetKey?: string
  itemCount: number
  autoFollowEnabled: boolean
  /** Persistent AgentChat tabs stay mounted under `display: none`; hidden geometry is not measurable. */
  visible?: boolean
  awayFromBottomThreshold?: number
  resolveScroller?: () => HTMLElement | null
}

export function useVirtuosoBottomLock({ resetKey, itemCount, autoFollowEnabled, visible = true, awayFromBottomThreshold = VIRTUOSO_AWAY_FROM_BOTTOM_THRESHOLD, resolveScroller }: VirtuosoBottomLockOptions) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const scrollerElementRef = useRef<HTMLElement | null>(null)
  const lockedRef = useRef(true)
  const afterContentInteractionRef = useRef(false)
  const previousAutoFollowEnabledRef = useRef(autoFollowEnabled)
  const previousVisibleRef = useRef(visible)
  const visibleRef = useRef(visible)
  const visibilitySettlingRef = useRef(!visible)
  visibleRef.current = visible
  const lastScrollTopRef = useRef(0)
  const lastLockedBottomScrollTopRef = useRef(0)
  const streamingRowElementRef = useRef<HTMLElement | null>(null)
  const streamingTailExtentRef = useRef<number | null>(null)
  const streamingRowScrollerRef = useRef<HTMLElement | null>(null)
  const streamingTailResetKeyRef = useRef(resetKey)
  const streamingRowObserverRef = useRef<ResizeObserver | null>(null)
  const schedulerRef = useRef(createDeferredBottomScrollScheduler())
  const scheduleScrollRef = useRef<() => void>(() => {})
  const [isAwayFromBottom, setIsAwayFromBottom] = useState(false)
  const isLayoutMeasurable = useCallback(() => (
    visible && previousVisibleRef.current === visible && !visibilitySettlingRef.current
  ), [visible])

  const cancelScheduledScroll = useCallback(() => {
    schedulerRef.current.cancel()
  }, [])

  const currentScrollerElement = useCallback(() => {
    const element = scrollerElementRef.current || resolveScroller?.() || null
    if (element && element !== scrollerElementRef.current) {
      scrollerElementRef.current = element
    }
    return element
  }, [resolveScroller])

  const disconnectStreamingRowObserver = useCallback(() => {
    streamingRowObserverRef.current?.disconnect()
    streamingRowObserverRef.current = null
  }, [])

  /** Compensates a committed tail-layout delta before the browser can paint it. */
  const syncStreamingTailLayout = useCallback(() => {
    if (!isLayoutMeasurable()) return
    const row = streamingRowElementRef.current
    if (!row) return
    const scroller = currentScrollerElement()
    if (!scroller) return
    const nextExtent = measureStreamingTailExtent(row, scroller)
    const previousExtent = streamingTailExtentRef.current
    streamingTailExtentRef.current = nextExtent
    streamingRowScrollerRef.current = scroller
    if (previousExtent === null || !autoFollowEnabled || !lockedRef.current || afterContentInteractionRef.current) return
    const layoutDelta = nextExtent - previousExtent
    if (Math.abs(layoutDelta) < 0.5) return
    scroller.scrollTop += layoutDelta
    lastScrollTopRef.current = scroller.scrollTop
    lastLockedBottomScrollTopRef.current = scroller.scrollTop
  }, [autoFollowEnabled, currentScrollerElement, isLayoutMeasurable])

  const compensateStreamingRowResize = useCallback((entries: ResizeObserverEntry[]) => {
    const row = streamingRowElementRef.current
    if (!row || !entries.some(entry => entry.target === row)) return
    syncStreamingTailLayout()
  }, [syncStreamingTailLayout])

  /** Keeps the active row's visible bottom stable until Virtuoso commits its deferred size measurement. */
  const streamingRowRef = useCallback((row: HTMLElement | null) => {
    if (streamingRowElementRef.current === row) return
    disconnectStreamingRowObserver()
    streamingRowElementRef.current = row
    if (!row) return
    if (streamingTailResetKeyRef.current !== resetKey) {
      streamingTailResetKeyRef.current = resetKey
      streamingTailExtentRef.current = null
      streamingRowScrollerRef.current = null
    }
    // A child ref is attached before Virtuoso publishes its parent scroller ref.
    // Resolve the already-mounted ancestor so the first visible tail still
    // establishes a baseline for a same-frame row replacement.
    const scroller = currentScrollerElement() || row.closest<HTMLElement>('[data-virtuoso-scroller]')
    if (scroller && scroller !== scrollerElementRef.current) scrollerElementRef.current = scroller
    const layoutContextChanged = Boolean(scroller && streamingRowScrollerRef.current && streamingRowScrollerRef.current !== scroller)
    // Preserve the previous tail extent across DOM identity changes. Waiting,
    // thinking, tool, and prose rows can replace one another in a single commit;
    // their content-coordinate delta is the scroll compensation for that frame.
    if (scroller && (streamingTailExtentRef.current === null || layoutContextChanged || (visible && !previousVisibleRef.current))) {
      streamingTailExtentRef.current = measureStreamingTailExtent(row, scroller)
      streamingRowScrollerRef.current = scroller
    }
    if (!autoFollowEnabled || !visible || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(compensateStreamingRowResize)
    streamingRowObserverRef.current = observer
    observer.observe(row)
  }, [autoFollowEnabled, compensateStreamingRowResize, currentScrollerElement, disconnectStreamingRowObserver, resetKey, visible])

  const updateAwayFromBottom = useCallback((element = currentScrollerElement()) => {
    if (!isLayoutMeasurable()) return
    const away = Boolean(element && itemCount > 0 && element.scrollHeight > element.clientHeight && element.scrollHeight - element.scrollTop - element.clientHeight > awayFromBottomThreshold)
    setIsAwayFromBottom(prev => prev === away ? prev : away)
  }, [awayFromBottomThreshold, currentScrollerElement, isLayoutMeasurable, itemCount])

  const isNearBottom = useCallback((element: HTMLElement) => (
    isElementNearBottom(element, VIRTUOSO_BOTTOM_THRESHOLD)
  ), [])

  const scrollToBottomNow = useCallback(() => {
    if (itemCount <= 0) {
      setIsAwayFromBottom(false)
      return
    }
    if (virtuosoRef.current) {
      virtuosoRef.current.scrollToIndex({ index: 'LAST', align: 'end', behavior: 'auto' })
      return
    }
    const element = currentScrollerElement()
    if (element) {
      element.scrollTop = Math.max(0, element.scrollHeight - element.clientHeight)
      lastScrollTopRef.current = element.scrollTop
      lastLockedBottomScrollTopRef.current = element.scrollTop
      updateAwayFromBottom(element)
    }
  }, [currentScrollerElement, itemCount, updateAwayFromBottom])

  const detectManualScrollAway = useCallback(() => {
    if (!isLayoutMeasurable()) return
    const element = currentScrollerElement()
    if (!element) return
    if (!isNearBottom(element) && element.scrollTop < lastLockedBottomScrollTopRef.current - 1) {
      lockedRef.current = false
      cancelScheduledScroll()
    }
    updateAwayFromBottom(element)
  }, [cancelScheduledScroll, currentScrollerElement, isLayoutMeasurable, isNearBottom, updateAwayFromBottom])

  const scheduleScrollToBottom = useCallback(() => {
    detectManualScrollAway()
    if (!autoFollowEnabled || !isLayoutMeasurable()) {
      cancelScheduledScroll()
      return
    }
    if (!lockedRef.current || itemCount <= 0) return
    schedulerRef.current.schedule(scrollToBottomNow, () => autoFollowEnabled && isLayoutMeasurable() && lockedRef.current && itemCount > 0)
  }, [autoFollowEnabled, cancelScheduledScroll, detectManualScrollAway, isLayoutMeasurable, itemCount, scrollToBottomNow])

  const unlockFromBottom = useCallback(() => {
    lockedRef.current = false
    cancelScheduledScroll()
  }, [cancelScheduledScroll])

  const releaseBottomLock = useCallback(() => {
    afterContentInteractionRef.current = true
    unlockFromBottom()
  }, [unlockFromBottom])

  const scrollToBottom = useCallback(() => {
    afterContentInteractionRef.current = false
    lockedRef.current = true
    schedulerRef.current.schedule(scrollToBottomNow, () => lockedRef.current)
  }, [scrollToBottomNow])

  const scrollElementIntoView = useCallback((element: HTMLElement) => {
    afterContentInteractionRef.current = false
    lockedRef.current = false
    cancelScheduledScroll()
    element.scrollIntoView?.({ block: 'start', inline: 'nearest', behavior: 'auto' })
    const scroller = currentScrollerElement()
    if (scroller) {
      lastScrollTopRef.current = scroller.scrollTop
      updateAwayFromBottom(scroller)
    }
  }, [cancelScheduledScroll, currentScrollerElement, updateAwayFromBottom])

  const scrollElementBottomIntoView = useCallback((element: HTMLElement, options: number | ScrollElementBottomIntoViewOptions = 0) => {
    const lockAfterScroll = typeof options !== 'number' && options.lockAfterScroll === true
    afterContentInteractionRef.current = false
    lockedRef.current = lockAfterScroll
    cancelScheduledScroll()
    const scroller = currentScrollerElement()
    if (!scroller) {
      element.scrollIntoView?.({ block: 'end', inline: 'nearest', behavior: 'auto' })
      return
    }
    const bottomInsetPx = typeof options === 'number' ? options : Math.max(0, options.bottomInsetPx || 0)
    const visibleBottomPx = typeof options === 'number' ? undefined : options.visibleBottomPx
    const scrollerRect = scroller.getBoundingClientRect()
    const elementRect = element.getBoundingClientRect()
    const measuredBottom = typeof visibleBottomPx === 'number' && Number.isFinite(visibleBottomPx)
      ? Math.max(scrollerRect.top, Math.min(scrollerRect.bottom, visibleBottomPx))
      : null
    const targetBottom = measuredBottom ?? scrollerRect.bottom - bottomInsetPx
    const nextScrollTop = Math.max(
      0,
      Math.min(scroller.scrollHeight - scroller.clientHeight, scroller.scrollTop + elementRect.bottom - targetBottom),
    )
    scroller.scrollTop = nextScrollTop
    lastScrollTopRef.current = nextScrollTop
    if (lockAfterScroll) lastLockedBottomScrollTopRef.current = nextScrollTop
    updateAwayFromBottom(scroller)
  }, [cancelScheduledScroll, currentScrollerElement, updateAwayFromBottom])

  const scrollToIndex = useCallback((index: number, options?: { align?: 'start' | 'center' | 'end'; behavior?: 'auto' | 'smooth' }) => {
    if (itemCount <= 0) return
    afterContentInteractionRef.current = false
    lockedRef.current = false
    cancelScheduledScroll()
    // Virtuoso reports visible ranges in the absolute firstItemIndex coordinate space,
    // but its imperative scrollToIndex API accepts an index relative to the current data.
    virtuosoRef.current?.scrollToIndex({
      index: Math.max(0, Math.min(itemCount - 1, index)),
      align: options?.align || 'start',
      behavior: options?.behavior || 'smooth',
    })
    updateAwayFromBottom()
  }, [cancelScheduledScroll, itemCount, updateAwayFromBottom])

  const handleScrollElement = useCallback((element: HTMLElement) => {
    if (!isLayoutMeasurable()) return
    const currentTop = element.scrollTop
    if (afterContentInteractionRef.current) {
      lockedRef.current = false
      lastScrollTopRef.current = currentTop
      updateAwayFromBottom(element)
      return
    }
    const previousTop = lastScrollTopRef.current
    if (isNearBottom(element)) {
      lockedRef.current = true
      lastLockedBottomScrollTopRef.current = currentTop
    } else if (currentTop < previousTop - 1) {
      unlockFromBottom()
    }
    lastScrollTopRef.current = currentTop
    updateAwayFromBottom(element)
  }, [isLayoutMeasurable, isNearBottom, unlockFromBottom, updateAwayFromBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    scrollerElementRef.current = event.currentTarget
    handleScrollElement(event.currentTarget)
  }, [handleScrollElement])

  const onWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (event.deltaY !== 0) afterContentInteractionRef.current = false
    if (event.deltaY < 0) unlockFromBottom()
  }, [unlockFromBottom])

  const onKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (isAfterContentEventTarget(event.target)) return
    if (UPWARD_SCROLL_KEYS.has(event.key) || DOWNWARD_SCROLL_KEYS.has(event.key)) {
      afterContentInteractionRef.current = false
    }
    if (UPWARD_SCROLL_KEYS.has(event.key)) unlockFromBottom()
  }, [unlockFromBottom])

  const onPointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (!isAfterContentEventTarget(event.target)) afterContentInteractionRef.current = false
  }, [])

  const onAtBottomStateChange = useCallback((atBottom: boolean) => {
    if (!isLayoutMeasurable()) return
    if (atBottom) {
      if (afterContentInteractionRef.current) {
        updateAwayFromBottom()
        return
      }
      const element = scrollerElementRef.current
      if (element && !isNearBottom(element)) {
        updateAwayFromBottom(element)
        return
      }
      lockedRef.current = true
      setIsAwayFromBottom(false)
    } else {
      // A footer interaction can emit `false`, then another stale `true`, while
      // Virtuoso measures the newly revealed content. Keep the suppression
      // active until the user explicitly returns to the bottom or the list is
      // reset; otherwise that second callback immediately restores the lock.
      updateAwayFromBottom()
    }
  }, [isLayoutMeasurable, isNearBottom, updateAwayFromBottom])

  const followOutput = useCallback((_atBottom: boolean) => {
    if (!isLayoutMeasurable()) return false
    detectManualScrollAway()
    // `atBottom` describes the layout before newly revealed footer content is
    // measured. An explicit interaction unlock must win over that stale value.
    return autoFollowEnabled && lockedRef.current && !afterContentInteractionRef.current ? 'auto' : false
  }, [autoFollowEnabled, detectManualScrollAway, isLayoutMeasurable])

  const scrollerRef = useCallback((ref: HTMLElement | Window | null) => {
    const element = ref instanceof HTMLElement ? ref : null
    scrollerElementRef.current = element
    if (element) {
      lastScrollTopRef.current = element.scrollTop
      updateAwayFromBottom(element)
    }
  }, [updateAwayFromBottom])

  useLayoutEffect(() => {
    // Resetting a list is an explicit one-shot navigation and remains separate
    // from automatic following, which is active only while output is streaming.
    scheduleScrollRef.current = scrollToBottom
  }, [scrollToBottom])

  useLayoutEffect(() => {
    const wasEnabled = previousAutoFollowEnabledRef.current
    const wasVisible = previousVisibleRef.current
    previousAutoFollowEnabledRef.current = autoFollowEnabled
    previousVisibleRef.current = visible
    if (!visible) {
      visibilitySettlingRef.current = true
      cancelScheduledScroll()
      return
    }
    if (!wasVisible) {
      // A hidden persistent tab may have accumulated an entire run while Virtuoso had no
      // measurable viewport. Restore only a lock that existed before hiding; a user's manual
      // scroll-away remains authoritative when they return to the tab.
      const restoreBottomLock = lockedRef.current
      streamingTailExtentRef.current = null
      streamingRowScrollerRef.current = null
      schedulerRef.current.schedule(() => {
        lockedRef.current = restoreBottomLock
        if (restoreBottomLock && itemCount > 0) {
          afterContentInteractionRef.current = false
          scrollToBottomNow()
        }
        const row = streamingRowElementRef.current
        const scroller = currentScrollerElement()
        if (row && scroller) {
          streamingTailExtentRef.current = measureStreamingTailExtent(row, scroller)
          streamingRowScrollerRef.current = scroller
        }
        visibilitySettlingRef.current = false
        updateAwayFromBottom()
      }, () => visible)
      return
    }
    if (!autoFollowEnabled || wasEnabled) return
    afterContentInteractionRef.current = false
    const element = currentScrollerElement()
    lockedRef.current = !element || isNearBottom(element)
    if (element) {
      lastScrollTopRef.current = element.scrollTop
      if (lockedRef.current) lastLockedBottomScrollTopRef.current = element.scrollTop
    }
    scheduleScrollToBottom()
  }, [autoFollowEnabled, cancelScheduledScroll, currentScrollerElement, isNearBottom, itemCount, scheduleScrollToBottom, scrollToBottomNow, updateAwayFromBottom, visible])

  useLayoutEffect(() => {
    afterContentInteractionRef.current = false
    lockedRef.current = true
    if (visibleRef.current) {
      visibilitySettlingRef.current = false
      scheduleScrollRef.current()
    } else {
      cancelScheduledScroll()
    }
    return cancelScheduledScroll
  }, [cancelScheduledScroll, resetKey])

  useEffect(() => {
    updateAwayFromBottom()
  }, [itemCount, updateAwayFromBottom])

  useLayoutEffect(() => {
    if (autoFollowEnabled) return
    streamingTailExtentRef.current = null
    streamingRowScrollerRef.current = null
  }, [autoFollowEnabled])

  useEffect(() => cancelScheduledScroll, [cancelScheduledScroll])
  useEffect(() => disconnectStreamingRowObserver, [disconnectStreamingRowObserver])

  return {
    virtuosoRef,
    scrollerRef,
    onScroll,
    onWheel,
    onKeyDown,
    onPointerDown,
    onAtBottomStateChange,
    followOutput,
    streamingRowRef,
    syncStreamingTailLayout,
    isAwayFromBottom,
    scrollToBottom,
    releaseBottomLock,
    scrollElementIntoView,
    scrollElementBottomIntoView,
    scrollToIndex,
  }
}

function isAfterContentEventTarget(target: EventTarget | null): boolean {
  return target instanceof Element && Boolean(target.closest('[data-nova-chat-after-content]'))
}

/** Measures the tail in scroll-content coordinates, independent of scrollTop. */
function measureStreamingTailExtent(row: HTMLElement, scroller: HTMLElement): number {
  return row.getBoundingClientRect().bottom - scroller.getBoundingClientRect().top + scroller.scrollTop
}
