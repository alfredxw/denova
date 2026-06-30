import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEvent, UIEvent, WheelEvent } from 'react'
import type { VirtuosoHandle } from 'react-virtuoso'

export const VIRTUOSO_BOTTOM_THRESHOLD = 12
export const VIRTUOSO_AWAY_FROM_BOTTOM_THRESHOLD = 160

const UPWARD_SCROLL_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

export interface ScrollElementBottomIntoViewOptions {
  bottomInsetPx?: number
  visibleBottomPx?: number
  lockAfterScroll?: boolean
}

export function useVirtuosoBottomLock({ resetKey, contentKey, itemCount, awayFromBottomThreshold = VIRTUOSO_AWAY_FROM_BOTTOM_THRESHOLD, resolveScroller }: { resetKey?: string; contentKey: string; itemCount: number; awayFromBottomThreshold?: number; resolveScroller?: () => HTMLElement | null }) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const scrollerElementRef = useRef<HTMLElement | null>(null)
  const lockedRef = useRef(true)
  const lastScrollTopRef = useRef(0)
  const lastLockedBottomScrollTopRef = useRef(0)
  const scrollRafRef = useRef<number[]>([])
  const scrollTimerRef = useRef<number | null>(null)
  const scheduleScrollRef = useRef<() => void>(() => {})
  const detachScrollerListenersRef = useRef<(() => void) | null>(null)
  const [isAwayFromBottom, setIsAwayFromBottom] = useState(false)

  const cancelScheduledScroll = useCallback(() => {
    for (const id of scrollRafRef.current) {
      cancelAnimationFrame(id)
    }
    scrollRafRef.current = []
    if (scrollTimerRef.current !== null) {
      window.clearTimeout(scrollTimerRef.current)
      scrollTimerRef.current = null
    }
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
    element.scrollHeight - element.scrollTop - element.clientHeight <= VIRTUOSO_BOTTOM_THRESHOLD
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
    if (!lockedRef.current || itemCount <= 0) return
    cancelScheduledScroll()
    scrollToBottomNow()
    scrollRafRef.current.push(requestAnimationFrame(() => {
      if (!lockedRef.current) return
      scrollToBottomNow()
      scrollRafRef.current.push(requestAnimationFrame(() => {
        if (!lockedRef.current) return
        scrollToBottomNow()
      }))
    }))
    scrollTimerRef.current = window.setTimeout(() => {
      scrollTimerRef.current = null
      if (!lockedRef.current) return
      scrollToBottomNow()
    }, 80)
  }, [cancelScheduledScroll, detectManualScrollAway, itemCount, scrollToBottomNow])

  const unlockFromBottom = useCallback(() => {
    lockedRef.current = false
    cancelScheduledScroll()
  }, [cancelScheduledScroll])

  const scrollToBottom = useCallback(() => {
    lockedRef.current = true
    cancelScheduledScroll()
    scrollToBottomNow()
    scrollRafRef.current.push(requestAnimationFrame(() => {
      if (!lockedRef.current) return
      scrollToBottomNow()
      scrollRafRef.current.push(requestAnimationFrame(() => {
        if (!lockedRef.current) return
        scrollToBottomNow()
      }))
    }))
    scrollTimerRef.current = window.setTimeout(() => {
      scrollTimerRef.current = null
      if (!lockedRef.current) return
      scrollToBottomNow()
    }, 80)
  }, [cancelScheduledScroll, scrollToBottomNow])

  const scrollElementIntoView = useCallback((element: HTMLElement) => {
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

  const handleScrollElement = useCallback((element: HTMLElement) => {
    const currentTop = element.scrollTop
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
    if (event.deltaY < 0) unlockFromBottom()
  }, [unlockFromBottom])

  const onKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (UPWARD_SCROLL_KEYS.has(event.key)) unlockFromBottom()
  }, [unlockFromBottom])

  const onAtBottomStateChange = useCallback((atBottom: boolean) => {
    if (atBottom) {
      const element = scrollerElementRef.current
      if (element && !isNearBottom(element)) {
        updateAwayFromBottom(element)
        return
      }
      lockedRef.current = true
      setIsAwayFromBottom(false)
    } else {
      updateAwayFromBottom()
    }
  }, [isNearBottom, updateAwayFromBottom])

  const followOutput = useCallback((atBottom: boolean) => {
    detectManualScrollAway()
    if (atBottom) lockedRef.current = true
    return lockedRef.current ? 'auto' : false
  }, [detectManualScrollAway])

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
        if (event.deltaY < 0) unlockFromBottom()
      }
      const handleNativeKeyDown = (event: globalThis.KeyboardEvent) => {
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

  useEffect(() => {
    scheduleScrollRef.current = scheduleScrollToBottom
  }, [scheduleScrollToBottom])

  useLayoutEffect(() => {
    lockedRef.current = true
    scheduleScrollRef.current()
    return cancelScheduledScroll
  }, [cancelScheduledScroll, resetKey])

  useLayoutEffect(() => {
    scheduleScrollToBottom()
  }, [contentKey, scheduleScrollToBottom])

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
    onAtBottomStateChange,
    followOutput,
    isAwayFromBottom,
    scrollToBottom,
    scrollElementIntoView,
    scrollElementBottomIntoView,
  }
}
