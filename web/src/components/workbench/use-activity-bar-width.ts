import { useEffect, useRef, useState, type KeyboardEventHandler, type PointerEventHandler } from 'react'

// User-level width preference; it survives reloads and development hot updates.
const STORAGE_KEY = 'nova.layout.activityBarWidth'
const MIN_WIDTH = 112
const DEFAULT_WIDTH = 224
const MAX_WIDTH = 288
const KEYBOARD_STEP = 8

/** Previews resizing locally; only completed gestures persist width and expansion. */
export function useActivityBarWidth({ enabled, expanded, onToggle }: {
  enabled: boolean
  expanded: boolean
  onToggle: () => void
}) {
  const [width, setWidth] = useState(readStoredWidth)
  const [previewExpanded, setPreviewExpanded] = useState<boolean | null>(null)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const cancelResizeRef = useRef<(() => void) | null>(null)

  useEffect(() => () => cancelResizeRef.current?.(), [enabled, expanded])

  const commitWidth = (nextWidth: number) => {
    setWidth(nextWidth)
    window.localStorage.setItem(STORAGE_KEY, String(nextWidth))
  }

  const onResizePointerDown: PointerEventHandler<HTMLDivElement> = (event) => {
    const wrapper = wrapperRef.current
    const handle = event.currentTarget
    const sidebar = handle.closest<HTMLElement>('[data-slot="sidebar"]')
    if (!enabled || !wrapper || !sidebar || event.button !== 0 || !event.isPrimary || cancelResizeRef.current) return

    event.preventDefault()
    const pointerID = event.pointerId
    const startX = event.clientX
    const startWidth = handle.closest<HTMLElement>('[data-slot="sidebar-container"]')!.getBoundingClientRect().width
    let previewWidth = width
    let nextExpanded = expanded
    // Disable both the visible sidebar and its layout gap before the first move.
    sidebar.setAttribute('data-nova-activity-bar-resizing', 'true')
    handle.setPointerCapture(pointerID)

    const move = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== pointerID) return
      const requestedWidth = startWidth + moveEvent.clientX - startX
      // Separate thresholds prevent jitter at the boundary. Text never squeezes
      // below its readable minimum; the remaining distance snaps to the icon rail.
      const shouldExpand = requestedWidth >= MIN_WIDTH - (nextExpanded ? 8 : 0)
      if (shouldExpand !== nextExpanded) {
        nextExpanded = shouldExpand
        setPreviewExpanded(nextExpanded)
      }
      const nextWidth = nextExpanded ? clampWidth(requestedWidth) : width
      if (nextWidth === previewWidth) return
      previewWidth = nextWidth
      wrapper.style.setProperty('--sidebar-width', `${nextWidth}px`)
    }
    const finish = (result: 'commit' | 'cancel') => {
      cancelResizeRef.current = null
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', end)
      window.removeEventListener('pointercancel', cancel)
      window.removeEventListener('blur', cancel)
      window.removeEventListener('keydown', keyDown)
      handle.removeEventListener('lostpointercapture', cancel)
      // Apply the final pointer coordinate before restoring collapse/expand transitions.
      handle.getBoundingClientRect()
      sidebar.removeAttribute('data-nova-activity-bar-resizing')
      if (handle.hasPointerCapture(pointerID)) handle.releasePointerCapture(pointerID)
      const finalWidth = result === 'commit' && nextExpanded ? previewWidth : width
      wrapper.style.setProperty('--sidebar-width', `${finalWidth}px`)
      if (finalWidth !== width) commitWidth(finalWidth)
      setPreviewExpanded(null)
      if (result === 'commit' && nextExpanded !== expanded) onToggle()
    }
    const end = (endEvent: PointerEvent) => {
      if (endEvent.pointerId !== pointerID) return
      move(endEvent)
      finish('commit')
    }
    const cancel = () => finish('cancel')
    const keyDown = (keyEvent: KeyboardEvent) => {
      if (keyEvent.key !== 'Escape') return
      keyEvent.preventDefault()
      cancel()
    }
    cancelResizeRef.current = cancel
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', end)
    window.addEventListener('pointercancel', cancel)
    window.addEventListener('blur', cancel)
    window.addEventListener('keydown', keyDown)
    handle.addEventListener('lostpointercapture', cancel)
  }

  const onResizeKeyDown: KeyboardEventHandler<HTMLDivElement> = (event) => {
    if (!enabled || cancelResizeRef.current) return
    let nextWidth: number
    switch (event.key) {
      case 'Enter':
      case ' ':
        event.preventDefault()
        onToggle()
        return
      case 'Home':
      case 'ArrowLeft':
        event.preventDefault()
        if (!expanded) return
        if (event.key === 'Home' || width === MIN_WIDTH) { onToggle(); return }
        nextWidth = clampWidth(width - KEYBOARD_STEP)
        break
      case 'ArrowRight':
        event.preventDefault()
        if (!expanded) { onToggle(); return }
        nextWidth = clampWidth(width + KEYBOARD_STEP)
        break
      case 'End':
        if (!expanded) onToggle()
        nextWidth = MAX_WIDTH
        break
      default: return
    }
    event.preventDefault()
    if (nextWidth !== width) commitWidth(nextWidth)
  }

  return { width, expanded: previewExpanded ?? expanded, maxWidth: MAX_WIDTH, wrapperRef, onResizePointerDown, onResizeKeyDown }
}

function readStoredWidth() {
  if (typeof window === 'undefined') return DEFAULT_WIDTH
  const raw = window.localStorage.getItem(STORAGE_KEY)
  if (raw === null) return DEFAULT_WIDTH
  const value = Number(raw)
  return Number.isFinite(value) ? clampWidth(value) : DEFAULT_WIDTH
}

function clampWidth(width: number) {
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Math.round(width)))
}
