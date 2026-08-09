import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent, UIEvent, WheelEvent } from 'react'
import type { VirtuosoHandle } from 'react-virtuoso'
import { createDeferredBottomScrollScheduler, DEFAULT_BOTTOM_THRESHOLD, isElementNearBottom, UPWARD_SCROLL_KEYS } from '@/lib/bottom-scroll-controller'
import { useStreamingTailLayout } from './useStreamingTailLayout'

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
  /** Height covered by the composer and its breathing room inside the message viewport. */
  bottomInsetPx?: number
  awayFromBottomThreshold?: number
  resolveScroller?: () => HTMLElement | null
}

export function useVirtuosoBottomLock({ resetKey, itemCount, autoFollowEnabled, visible = true, bottomInsetPx, awayFromBottomThreshold = VIRTUOSO_AWAY_FROM_BOTTOM_THRESHOLD, resolveScroller }: VirtuosoBottomLockOptions) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const scrollerElementRef = useRef<HTMLElement | null>(null)
  const viewportScrollTopRef = useRef<number | null>(null)
  const lockedRef = useRef(true)
  const afterContentInteractionRef = useRef(false)
  const pointerGestureYRef = useRef<number | null>(null)
  const autoFollowEnabledRef = useRef(autoFollowEnabled)
  const previousAutoFollowEnabledRef = useRef(autoFollowEnabled)
  const previousVisibleRef = useRef(visible)
  const visibleRef = useRef(visible)
  const visibilitySettlingRef = useRef(!visible)
  autoFollowEnabledRef.current = autoFollowEnabled
  visibleRef.current = visible
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
      viewportScrollTopRef.current = element.scrollTop
    }
    return element
  }, [resolveScroller])

  const canAutoFollowStreamingTail = useCallback(() => (
    autoFollowEnabledRef.current &&
    visibleRef.current &&
    lockedRef.current &&
    !afterContentInteractionRef.current
  ), [])

  const recordScrollerPosition = useCallback((scroller: HTMLElement) => {
    viewportScrollTopRef.current = scroller.scrollTop
  }, [])

  const {
    rowRef: streamingRowRef,
    spacerPx: streamingSpacerPx,
    syncLayout: syncStreamingTailLayout,
    scrollLatestIntoView,
  } = useStreamingTailLayout({
    enabled: autoFollowEnabled,
    visible,
    resetKey,
    bottomInsetPx,
    resolveScroller: currentScrollerElement,
    canAutoFollow: canAutoFollowStreamingTail,
    onScrollTopChange: recordScrollerPosition,
  })

  const updateAwayFromBottom = useCallback((element = currentScrollerElement()) => {
    if (!isLayoutMeasurable()) return
    const layoutDistanceFromEnd = Boolean(
      element &&
      itemCount > 0 &&
      element.scrollHeight > element.clientHeight &&
      element.scrollHeight - element.scrollTop - element.clientHeight > awayFromBottomThreshold,
    )
    // A growing response deliberately leaves the absolute content end below
    // the viewport. Only explicit user intent may expose the return button.
    const away = autoFollowEnabledRef.current && lockedRef.current
      ? false
      : layoutDistanceFromEnd
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
    if (autoFollowEnabledRef.current && scrollLatestIntoView()) {
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
      viewportScrollTopRef.current = element.scrollTop
      updateAwayFromBottom(element)
    }
  }, [currentScrollerElement, itemCount, scrollLatestIntoView, updateAwayFromBottom])

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
      viewportScrollTopRef.current = scroller.scrollTop
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
    viewportScrollTopRef.current = nextScrollTop
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
    if (afterContentInteractionRef.current) {
      lockedRef.current = false
      updateAwayFromBottom(element)
      return
    }
    if (isNearBottom(element)) {
      lockedRef.current = true
    }
    updateAwayFromBottom(element)
  }, [isLayoutMeasurable, isNearBottom, updateAwayFromBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    scrollerElementRef.current = event.currentTarget
    viewportScrollTopRef.current = event.currentTarget.scrollTop
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
    if (isAfterContentEventTarget(event.target)) {
      pointerGestureYRef.current = null
      return
    }
    afterContentInteractionRef.current = false
    pointerGestureYRef.current = event.pointerType === 'touch' || event.pointerType === 'pen'
      ? event.clientY
      : null
    // Wheel/keyboard handlers carry directional intent. A pointer event whose
    // target is the scroller itself covers scrollbar dragging without treating
    // layout-driven scroll events as user input.
    if (event.target === event.currentTarget) unlockFromBottom()
  }, [unlockFromBottom])

  const onPointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    const previousY = pointerGestureYRef.current
    if (previousY === null) return
    pointerGestureYRef.current = event.clientY
    // Moving a touch pointer down scrolls the content upward. Use the gesture
    // direction instead of scrollTop deltas, which also change during layout.
    if (event.clientY > previousY + 1) unlockFromBottom()
  }, [unlockFromBottom])

  const onPointerEnd = useCallback(() => {
    pointerGestureYRef.current = null
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

  const scrollerRef = useCallback((ref: HTMLElement | Window | null) => {
    const element = ref instanceof HTMLElement ? ref : null
    scrollerElementRef.current = element
    if (element) {
      viewportScrollTopRef.current = element.scrollTop
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
      schedulerRef.current.schedule(() => {
        lockedRef.current = restoreBottomLock
        if (restoreBottomLock && itemCount > 0) {
          afterContentInteractionRef.current = false
          scrollToBottomNow()
        }
        visibilitySettlingRef.current = false
        updateAwayFromBottom()
      }, () => visible)
      return
    }
    if (!autoFollowEnabled) {
      // The state panel mounts in the same commit that ends streaming. Cancel
      // an automatic follow captured against the old footer before it can move
      // the newly completed viewport on the next animation frame.
      if (wasEnabled) {
        cancelScheduledScroll()
        const element = currentScrollerElement()
        const previousScrollTop = viewportScrollTopRef.current
        if (element && previousScrollTop !== null) {
          const preservedScrollTop = Math.max(
            0,
            Math.min(previousScrollTop, element.scrollHeight - element.clientHeight),
          )
          element.scrollTop = preservedScrollTop
          viewportScrollTopRef.current = preservedScrollTop
          updateAwayFromBottom(element)
        }
      }
      return
    }
    if (wasEnabled) return
    afterContentInteractionRef.current = false
    const element = currentScrollerElement()
    lockedRef.current = !element || isNearBottom(element)
  }, [autoFollowEnabled, cancelScheduledScroll, currentScrollerElement, isNearBottom, itemCount, scrollToBottomNow, updateAwayFromBottom, visible])

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

  useEffect(() => cancelScheduledScroll, [cancelScheduledScroll])

  return {
    virtuosoRef,
    scrollerRef,
    onScroll,
    onWheel,
    onKeyDown,
    onPointerDown,
    onPointerMove,
    onPointerUp: onPointerEnd,
    onPointerCancel: onPointerEnd,
    onAtBottomStateChange,
    streamingRowRef,
    syncStreamingTailLayout,
    streamingSpacerPx,
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
