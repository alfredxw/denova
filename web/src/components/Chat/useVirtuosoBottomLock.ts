import { useCallback, useEffect, useRef } from 'react'
import type { KeyboardEvent, UIEvent, WheelEvent } from 'react'
import type { VirtuosoHandle } from 'react-virtuoso'

export const VIRTUOSO_BOTTOM_THRESHOLD = 12

const UPWARD_SCROLL_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

export function useVirtuosoBottomLock({ resetKey, contentKey, itemCount }: { resetKey?: string; contentKey: string; itemCount: number }) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const scrollerElementRef = useRef<HTMLElement | null>(null)
  const lockedRef = useRef(true)
  const lastScrollTopRef = useRef(0)
  const lastLockedBottomScrollTopRef = useRef(0)
  const scrollRafRef = useRef<number[]>([])
  const scrollTimerRef = useRef<number | null>(null)
  const scheduleScrollRef = useRef<() => void>(() => {})
  const detachScrollerListenersRef = useRef<(() => void) | null>(null)

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

  const scrollToBottomNow = useCallback(() => {
    if (itemCount <= 0) return
    virtuosoRef.current?.scrollToIndex({ index: 'LAST', align: 'end', behavior: 'auto' })
    const element = scrollerElementRef.current
    if (element) {
      element.scrollTop = Math.max(0, element.scrollHeight - element.clientHeight)
      lastScrollTopRef.current = element.scrollTop
      lastLockedBottomScrollTopRef.current = element.scrollTop
    }
  }, [itemCount])

  const isNearBottom = useCallback((element: HTMLElement) => (
    element.scrollHeight - element.scrollTop - element.clientHeight <= VIRTUOSO_BOTTOM_THRESHOLD
  ), [])

  const detectManualScrollAway = useCallback(() => {
    const element = scrollerElementRef.current
    if (!element) return
    if (!isNearBottom(element) && element.scrollTop < lastLockedBottomScrollTopRef.current - 1) {
      lockedRef.current = false
      cancelScheduledScroll()
    }
  }, [cancelScheduledScroll, isNearBottom])

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
  }, [isNearBottom, unlockFromBottom])

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    handleScrollElement(event.currentTarget)
  }, [handleScrollElement])

  const onWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (event.deltaY < 0) unlockFromBottom()
  }, [unlockFromBottom])

  const onKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (UPWARD_SCROLL_KEYS.has(event.key)) unlockFromBottom()
  }, [unlockFromBottom])

  const onAtBottomStateChange = useCallback((atBottom: boolean) => {
    if (atBottom) lockedRef.current = true
  }, [])

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
  }, [handleScrollElement, unlockFromBottom])

  useEffect(() => {
    scheduleScrollRef.current = scheduleScrollToBottom
  }, [scheduleScrollToBottom])

  useEffect(() => {
    lockedRef.current = true
    scheduleScrollRef.current()
    return cancelScheduledScroll
  }, [cancelScheduledScroll, resetKey])

  useEffect(() => {
    scheduleScrollToBottom()
  }, [contentKey, scheduleScrollToBottom])

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
  }
}
