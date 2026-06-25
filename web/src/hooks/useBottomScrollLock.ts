import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import type { KeyboardEvent, RefObject, UIEvent, WheelEvent } from 'react'

interface BottomScrollLockOptions {
  enabled?: boolean
  resetKey?: string | number
  contentKey?: string | number
  bottomThreshold?: number
}

interface BottomScrollLockHandlers<T extends HTMLElement> {
  ref: RefObject<T | null>
  onScroll: (event: UIEvent<T>) => void
  onWheel: (event: WheelEvent<T>) => void
  onKeyDown: (event: KeyboardEvent<T>) => void
  lockToBottom: () => void
  unlockFromBottom: () => void
  isLockedToBottom: () => boolean
}

const DEFAULT_BOTTOM_THRESHOLD = 12
const UPWARD_SCROLL_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

/**
 * Keeps streaming chat-like panes pinned to the bottom until the user scrolls up.
 * Content growth alone never unlocks the pane; only an upward scroll intent does.
 */
export function useBottomScrollLock<T extends HTMLElement>({
  enabled = true,
  resetKey,
  contentKey,
  bottomThreshold = DEFAULT_BOTTOM_THRESHOLD,
}: BottomScrollLockOptions = {}): BottomScrollLockHandlers<T> {
  const containerRef = useRef<T | null>(null)
  const lockedRef = useRef(true)
  const lastScrollTopRef = useRef(0)
  const scrollRafRef = useRef<number[]>([])
  const scrollTimerRef = useRef<number | null>(null)

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

  const isNearBottom = useCallback((el: HTMLElement) => (
    el.scrollHeight - el.scrollTop - el.clientHeight <= bottomThreshold
  ), [bottomThreshold])

  const scrollToBottomNow = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    el.scrollTop = Math.max(0, el.scrollHeight - el.clientHeight)
    lastScrollTopRef.current = el.scrollTop
  }, [])

  const scheduleScrollToBottom = useCallback(() => {
    if (!enabled || !lockedRef.current) return
    cancelScheduledScroll()
    scrollToBottomNow()
    scrollRafRef.current.push(requestAnimationFrame(() => {
      if (!enabled || !lockedRef.current) return
      scrollToBottomNow()
      scrollRafRef.current.push(requestAnimationFrame(() => {
        if (!enabled || !lockedRef.current) return
        scrollToBottomNow()
      }))
    }))
    scrollTimerRef.current = window.setTimeout(() => {
      scrollTimerRef.current = null
      if (!enabled || !lockedRef.current) return
      scrollToBottomNow()
    }, 80)
  }, [cancelScheduledScroll, enabled, scrollToBottomNow])

  const lockToBottom = useCallback(() => {
    lockedRef.current = true
    scheduleScrollToBottom()
  }, [scheduleScrollToBottom])

  const unlockFromBottom = useCallback(() => {
    lockedRef.current = false
    cancelScheduledScroll()
  }, [cancelScheduledScroll])

  const handleScroll = useCallback((event: UIEvent<T>) => {
    if (!enabled) return
    const el = event.currentTarget
    const currentTop = el.scrollTop
    const previousTop = lastScrollTopRef.current
    if (isNearBottom(el)) {
      lockedRef.current = true
    } else if (currentTop < previousTop - 1) {
      unlockFromBottom()
    }
    lastScrollTopRef.current = currentTop
  }, [enabled, isNearBottom, unlockFromBottom])

  const handleWheel = useCallback((event: WheelEvent<T>) => {
    if (!enabled) return
    if (event.deltaY < 0) {
      unlockFromBottom()
    }
  }, [enabled, unlockFromBottom])

  const handleKeyDown = useCallback((event: KeyboardEvent<T>) => {
    if (!enabled) return
    if (UPWARD_SCROLL_KEYS.has(event.key)) {
      unlockFromBottom()
    }
  }, [enabled, unlockFromBottom])

  useLayoutEffect(() => {
    lockedRef.current = true
    scheduleScrollToBottom()
    return cancelScheduledScroll
  }, [cancelScheduledScroll, resetKey, scheduleScrollToBottom])

  useLayoutEffect(() => {
    scheduleScrollToBottom()
  }, [contentKey, scheduleScrollToBottom])

  useEffect(() => {
    if (!enabled) return
    const el = containerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(scheduleScrollToBottom)
    observer.observe(el)
    return () => observer.disconnect()
  }, [enabled, scheduleScrollToBottom])

  useEffect(() => {
    if (!enabled) return
    const el = containerRef.current
    if (!el || typeof MutationObserver === 'undefined') return
    const observer = new MutationObserver(scheduleScrollToBottom)
    observer.observe(el, { childList: true, characterData: true, subtree: true })
    return () => observer.disconnect()
  }, [enabled, scheduleScrollToBottom])

  useEffect(() => cancelScheduledScroll, [cancelScheduledScroll])

  return {
    ref: containerRef,
    onScroll: handleScroll,
    onWheel: handleWheel,
    onKeyDown: handleKeyDown,
    lockToBottom,
    unlockFromBottom,
    isLockedToBottom: () => lockedRef.current,
  }
}
