import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'

const STREAM_CONTEXT_MIN_PX = 88
const STREAM_CONTEXT_RATIO = 1 / 3
const STREAM_TAIL_GAP_PX = 64
const LAYOUT_EPSILON_PX = 0.5

type StreamingTailPhase = 'idle' | 'growing' | 'following'

interface StreamingTailLayoutOptions {
  enabled: boolean
  visible: boolean
  resetKey?: string
  bottomInsetPx?: number
  resolveScroller: () => HTMLElement | null
  canAutoFollow: () => boolean
  onScrollTopChange: (scroller: HTMLElement) => void
}

interface StreamingTailLayout {
  rowRef: (row: HTMLElement | null) => void
  spacerPx?: number
  syncLayout: () => void
  scrollLatestIntoView: () => boolean
}

/**
 * Gives a new streaming turn room to grow before it reaches the composer, then
 * preserves its visible bottom with one absolute viewport invariant. This is
 * intentionally independent from Virtuoso's bottom-follow state so only one
 * controller writes the scroll position during streaming.
 */
export function useStreamingTailLayout({
  enabled,
  visible,
  resetKey,
  bottomInsetPx,
  resolveScroller,
  canAutoFollow,
  onScrollTopChange,
}: StreamingTailLayoutOptions): StreamingTailLayout {
  const enabledRef = useRef(enabled)
  const visibleRef = useRef(visible)
  const bottomInsetRef = useRef<number | null>(normaliseInset(bottomInsetPx))
  const rowElementRef = useRef<HTMLElement | null>(null)
  const rowObserverRef = useRef<ResizeObserver | null>(null)
  const scrollerObserverRef = useRef<ResizeObserver | null>(null)
  const phaseRef = useRef<StreamingTailPhase>(enabled ? 'growing' : 'idle')
  const previousEnabledRef = useRef(false)
  const previousResetKeyRef = useRef(resetKey)
  const previousBottomInsetRef = useRef<number | null>(normaliseInset(bottomInsetPx))
  const pendingStartAnchorRef = useRef(enabled)
  const targetStartSpacerRef = useRef<number | null>(null)
  const [spacerPx, setSpacerPx] = useState<number | undefined>()

  enabledRef.current = enabled
  visibleRef.current = visible
  const explicitInset = normaliseInset(bottomInsetPx)
  if (explicitInset !== null) bottomInsetRef.current = explicitInset

  const resolveBottomInset = useCallback((scroller: HTMLElement) => {
    if (bottomInsetRef.current !== null) return bottomInsetRef.current
    const spacer = scroller.querySelector<HTMLElement>('[data-nova-chat-bottom-spacer]')
    const measuredInset = Math.max(0, spacer?.getBoundingClientRect().height || 0)
    bottomInsetRef.current = measuredInset
    return measuredInset
  }, [])

  const setSpacer = useCallback((next: number | undefined) => {
    setSpacerPx(current => sameOptionalPixelValue(current, next) ? current : next)
  }, [])

  const updateGrowingSpacer = useCallback((scroller: HTMLElement, inset: number) => {
    const nextSpacer = calculateStreamingTailSpacer(scroller.clientHeight, inset)
    if (pendingStartAnchorRef.current) targetStartSpacerRef.current = nextSpacer
    setSpacer(nextSpacer)
  }, [setSpacer])

  const clearRowObserver = useCallback(() => {
    rowObserverRef.current?.disconnect()
    rowObserverRef.current = null
  }, [])

  const syncLayout = useCallback(() => {
    if (!enabledRef.current || !visibleRef.current || !canAutoFollow()) return
    const row = rowElementRef.current
    const scroller = resolveScroller()
    if (!row || !scroller) return

    const bottomInset = resolveBottomInset(scroller)
    const rowBottom = row.getBoundingClientRect().bottom
    const safeBottom = scroller.getBoundingClientRect().bottom - bottomInset
    const viewportDelta = rowBottom - safeBottom

    if (phaseRef.current === 'growing') {
      if (viewportDelta <= LAYOUT_EPSILON_PX) return
      phaseRef.current = 'following'
      // The response has consumed its runway. Removing it here makes the
      // ordinary completed footer the exact remaining scroll range.
      setSpacer(undefined)
    }
    if (phaseRef.current !== 'following' || Math.abs(viewportDelta) < LAYOUT_EPSILON_PX) return

    scroller.scrollTop += viewportDelta
    onScrollTopChange(scroller)
  }, [canAutoFollow, onScrollTopChange, resolveBottomInset, resolveScroller, setSpacer])

  const attachRowObserver = useCallback(() => {
    clearRowObserver()
    const row = rowElementRef.current
    if (!enabledRef.current || !visibleRef.current || !row || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver((entries) => {
      if (entries.some(entry => entry.target === row)) syncLayout()
    })
    rowObserverRef.current = observer
    observer.observe(row)
  }, [clearRowObserver, syncLayout])

  const rowRef = useCallback((row: HTMLElement | null) => {
    if (rowElementRef.current === row) return
    clearRowObserver()
    rowElementRef.current = row
    attachRowObserver()
  }, [attachRowObserver, clearRowObserver])

  const scrollLatestIntoView = useCallback(() => {
    const row = rowElementRef.current
    const scroller = resolveScroller()
    if (!row || !scroller) return false
    phaseRef.current = 'following'
    setSpacer(undefined)
    const bottomInset = resolveBottomInset(scroller)
    const viewportDelta = row.getBoundingClientRect().bottom
      - (scroller.getBoundingClientRect().bottom - bottomInset)
    if (Math.abs(viewportDelta) >= LAYOUT_EPSILON_PX) scroller.scrollTop += viewportDelta
    onScrollTopChange(scroller)
    return true
  }, [onScrollTopChange, resolveBottomInset, resolveScroller, setSpacer])

  useLayoutEffect(() => {
    const wasEnabled = previousEnabledRef.current
    const resetChanged = previousResetKeyRef.current !== resetKey
    const insetChanged = explicitInset !== null && previousBottomInsetRef.current !== explicitInset
    previousEnabledRef.current = enabled
    previousResetKeyRef.current = resetKey
    if (explicitInset !== null) previousBottomInsetRef.current = explicitInset
    if (!visible) return

    const scroller = resolveScroller()
    if (!scroller) return
    const bottomInset = resolveBottomInset(scroller)

    if (enabled && (!wasEnabled || resetChanged)) {
      phaseRef.current = 'growing'
      pendingStartAnchorRef.current = true
      updateGrowingSpacer(scroller, bottomInset)
      return
    }

    if (enabled && phaseRef.current === 'growing' && insetChanged) {
      updateGrowingSpacer(scroller, bottomInset)
      return
    }

    if (!enabled && wasEnabled) {
      pendingStartAnchorRef.current = false
      targetStartSpacerRef.current = null
      // The runway exists only while output can still grow. Keeping its unused
      // remainder after completion turns the temporary response space into a
      // permanent blank footer and leaves the viewport short of the real end.
      setSpacer(undefined)
      phaseRef.current = 'idle'
    }
  }, [enabled, explicitInset, resetKey, resolveBottomInset, resolveScroller, setSpacer, updateGrowingSpacer, visible])

  useLayoutEffect(() => {
    if (!enabled || !visible || !pendingStartAnchorRef.current || !canAutoFollow()) return
    const targetSpacer = targetStartSpacerRef.current
    if (targetSpacer === null || !sameOptionalPixelValue(spacerPx, targetSpacer)) return
    const scroller = resolveScroller()
    if (!scroller) return
    scroller.scrollTop = Math.max(0, scroller.scrollHeight - scroller.clientHeight)
    onScrollTopChange(scroller)
    pendingStartAnchorRef.current = false
    syncLayout()
  }, [canAutoFollow, enabled, onScrollTopChange, resolveScroller, spacerPx, syncLayout, visible])

  useEffect(() => {
    attachRowObserver()
    return clearRowObserver
  }, [attachRowObserver, clearRowObserver, enabled, visible])

  useEffect(() => {
    scrollerObserverRef.current?.disconnect()
    scrollerObserverRef.current = null
    if (!enabled || !visible || typeof ResizeObserver === 'undefined') return
    const scroller = resolveScroller()
    if (!scroller) return
    const observer = new ResizeObserver(() => {
      if (phaseRef.current !== 'growing') return
      const bottomInset = resolveBottomInset(scroller)
      updateGrowingSpacer(scroller, bottomInset)
      syncLayout()
    })
    scrollerObserverRef.current = observer
    observer.observe(scroller)
    return () => {
      observer.disconnect()
      if (scrollerObserverRef.current === observer) scrollerObserverRef.current = null
    }
  }, [enabled, resolveBottomInset, resolveScroller, syncLayout, updateGrowingSpacer, visible])

  useEffect(() => () => {
    clearRowObserver()
    scrollerObserverRef.current?.disconnect()
  }, [clearRowObserver])

  return { rowRef, spacerPx, syncLayout, scrollLatestIntoView }
}

/** Matches ChatGPT's one-third context / two-thirds response viewport split. */
export function calculateStreamingTailSpacer(viewportHeight: number, bottomInset: number): number {
  const safeHeight = Math.max(0, viewportHeight - bottomInset)
  const contextHeight = Math.min(safeHeight, Math.max(STREAM_CONTEXT_MIN_PX, safeHeight * STREAM_CONTEXT_RATIO))
  const responseHeight = Math.max(0, safeHeight - contextHeight)
  return bottomInset + Math.max(0, responseHeight - STREAM_TAIL_GAP_PX)
}

function normaliseInset(value: number | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, value) : null
}

function sameOptionalPixelValue(left: number | undefined, right: number | undefined): boolean {
  if (left === undefined || right === undefined) return left === right
  return Math.abs(left - right) < LAYOUT_EPSILON_PX
}
