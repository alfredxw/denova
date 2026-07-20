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

export function useVirtuosoBottomLock({ resetKey, contentKey, itemCount, autoFollowEnabled, awayFromBottomThreshold = VIRTUOSO_AWAY_FROM_BOTTOM_THRESHOLD, resolveScroller }: { resetKey?: string; contentKey: string; itemCount: number; autoFollowEnabled: boolean; awayFromBottomThreshold?: number; resolveScroller?: () => HTMLElement | null }) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const scrollerElementRef = useRef<HTMLElement | null>(null)
  const lockedRef = useRef(true)
  const afterContentInteractionRef = useRef(false)
  const previousAutoFollowEnabledRef = useRef(autoFollowEnabled)
  const lastScrollTopRef = useRef(0)
  const lastLockedBottomScrollTopRef = useRef(0)
  const schedulerRef = useRef(createDeferredBottomScrollScheduler())
  const scheduleScrollRef = useRef<() => void>(() => {})
  const detachScrollerListenersRef = useRef<(() => void) | null>(null)
  const [isAwayFromBottom, setIsAwayFromBottom] = useState(false)

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

  const updateAwayFromBottom = useCallback((element = currentScrollerElement()) => {
    const away = Boolean(element && itemCount > 0 && element.scrollHeight > element.clientHeight && element.scrollHeight - element.scrollTop - element.clientHeight > awayFromBottomThreshold)
    setIsAwayFromBottom(prev => prev === away ? prev : away)
  }, [awayFromBottomThreshold, currentScrollerElement, itemCount])

  const isNearBottom = useCallback((element: HTMLElement) => (
    isElementNearBottom(element, VIRTUOSO_BOTTOM_THRESHOLD)
  ), [])

  const scrollToBottomNow = useCallback(() => {
    if (itemCount <= 0) {
      setIsAwayFromBottom(false)
      return
    }
    virtuosoRef.current?.scrollToIndex({ index: 'LAST', align: 'end', behavior: 'auto' })
    const element = currentScrollerElement()
    if (element) {
      element.scrollTop = Math.max(0, element.scrollHeight - element.clientHeight)
      lastScrollTopRef.current = element.scrollTop
      lastLockedBottomScrollTopRef.current = element.scrollTop
      updateAwayFromBottom(element)
    }
  }, [currentScrollerElement, itemCount, updateAwayFromBottom])

  const detectManualScrollAway = useCallback(() => {
    const element = currentScrollerElement()
    if (!element) return
    if (!isNearBottom(element) && element.scrollTop < lastLockedBottomScrollTopRef.current - 1) {
      lockedRef.current = false
      cancelScheduledScroll()
    }
    updateAwayFromBottom(element)
  }, [cancelScheduledScroll, currentScrollerElement, isNearBottom, updateAwayFromBottom])

  const scheduleScrollToBottom = useCallback(() => {
    detectManualScrollAway()
    if (!autoFollowEnabled) {
      cancelScheduledScroll()
      return
    }
    if (!lockedRef.current || itemCount <= 0) return
    schedulerRef.current.schedule(scrollToBottomNow, () => autoFollowEnabled && lockedRef.current && itemCount > 0)
  }, [autoFollowEnabled, cancelScheduledScroll, detectManualScrollAway, itemCount, scrollToBottomNow])

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
    virtuosoRef.current?.scrollToIndex({
      index: Math.max(0, Math.min(itemCount - 1, index)),
      align: options?.align || 'start',
      behavior: options?.behavior || 'smooth',
    })
    updateAwayFromBottom()
  }, [cancelScheduledScroll, itemCount, updateAwayFromBottom])

  const handleScrollElement = useCallback((element: HTMLElement) => {
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
  }, [isNearBottom, unlockFromBottom, updateAwayFromBottom])

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
  }, [isNearBottom, updateAwayFromBottom])

  const followOutput = useCallback((_atBottom: boolean) => {
    detectManualScrollAway()
    // `atBottom` describes the layout before newly revealed footer content is
    // measured. An explicit interaction unlock must win over that stale value.
    return autoFollowEnabled && lockedRef.current && !afterContentInteractionRef.current ? 'auto' : false
  }, [autoFollowEnabled, detectManualScrollAway])

  const scrollerRef = useCallback((ref: HTMLElement | Window | null) => {
    detachScrollerListenersRef.current?.()
    detachScrollerListenersRef.current = null
    const element = ref instanceof HTMLElement ? ref : null
    scrollerElementRef.current = element
    if (element) {
      lastScrollTopRef.current = element.scrollTop
      updateAwayFromBottom(element)
      const handleNativeScroll = () => handleScrollElement(element)
      const handleNativeWheel = (event: globalThis.WheelEvent) => {
        if (event.deltaY !== 0) afterContentInteractionRef.current = false
        if (event.deltaY < 0) unlockFromBottom()
      }
      const handleNativeKeyDown = (event: globalThis.KeyboardEvent) => {
        if (isAfterContentEventTarget(event.target)) return
        if (UPWARD_SCROLL_KEYS.has(event.key) || DOWNWARD_SCROLL_KEYS.has(event.key)) {
          afterContentInteractionRef.current = false
        }
        if (UPWARD_SCROLL_KEYS.has(event.key)) unlockFromBottom()
      }
      element.addEventListener('scroll', handleNativeScroll, { passive: true })
      element.addEventListener('wheel', handleNativeWheel, { passive: true })
      element.addEventListener('keydown', handleNativeKeyDown)
      detachScrollerListenersRef.current = () => {
        element.removeEventListener('scroll', handleNativeScroll)
        element.removeEventListener('wheel', handleNativeWheel)
        element.removeEventListener('keydown', handleNativeKeyDown)
      }
    }
  }, [handleScrollElement, unlockFromBottom, updateAwayFromBottom])

  useLayoutEffect(() => {
    // Resetting a list is an explicit one-shot navigation and remains separate
    // from automatic following, which is active only while output is streaming.
    scheduleScrollRef.current = scrollToBottom
  }, [scrollToBottom])

  useLayoutEffect(() => {
    const wasEnabled = previousAutoFollowEnabledRef.current
    previousAutoFollowEnabledRef.current = autoFollowEnabled
    if (!autoFollowEnabled || wasEnabled) return
    afterContentInteractionRef.current = false
    const element = currentScrollerElement()
    lockedRef.current = !element || isNearBottom(element)
    if (element) {
      lastScrollTopRef.current = element.scrollTop
      if (lockedRef.current) lastLockedBottomScrollTopRef.current = element.scrollTop
    }
  }, [autoFollowEnabled, currentScrollerElement, isNearBottom])

  useLayoutEffect(() => {
    scheduleScrollToBottom()
  }, [contentKey, scheduleScrollToBottom])

  useLayoutEffect(() => {
    afterContentInteractionRef.current = false
    lockedRef.current = true
    scheduleScrollRef.current()
    return cancelScheduledScroll
  }, [cancelScheduledScroll, resetKey])

  useEffect(() => {
    updateAwayFromBottom()
  }, [itemCount, updateAwayFromBottom])

  useEffect(() => () => {
    detachScrollerListenersRef.current?.()
    cancelScheduledScroll()
  }, [cancelScheduledScroll])

  return {
    virtuosoRef,
    scrollerRef,
    onScroll,
    onWheel,
    onKeyDown,
    onPointerDown,
    onAtBottomStateChange,
    followOutput,
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
