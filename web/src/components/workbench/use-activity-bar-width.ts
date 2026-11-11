import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEventHandler, type PointerEventHandler } from 'react'

// User-level width preference; it survives reloads and development hot updates.
const STORAGE_KEY = 'nova.layout.activityBarWidth'
const MIN_WIDTH = 112
const DEFAULT_WIDTH = 160
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
  const previewSidebarRef = useRef<HTMLElement>(null)
  const cancelResizeRef = useRef<(() => void) | null>(null)

  useEffect(() => () => cancelResizeRef.current?.(), [enabled, expanded])

  useLayoutEffect(() => {
    const sidebar = previewSidebarRef.current
    if (previewExpanded !== null || !sidebar) return
    // Release the preview after the saved width and expansion reach the DOM.
    sidebar.style.removeProperty('--nova-activity-bar-drag-width')
    delete sidebar.dataset.novaActivityBarResizing
    previewSidebarRef.current = null
  }, [previewExpanded])

  const commitWidth = (nextWidth: number) => {
    setWidth(nextWidth)
    window.localStorage.setItem(STORAGE_KEY, String(nextWidth))
  }

  const onResizePointerDown: PointerEventHandler<HTMLDivElement> = (event) => {
    const handle = event.currentTarget
    const sidebar = handle.closest<HTMLElement>('[data-slot="sidebar-container"]')
    if (!enabled || !sidebar || event.button !== 0 || !event.isPrimary || cancelResizeRef.current) return

    event.preventDefault()
    const pointerID = event.pointerId
    const startX = event.clientX
    const startWidth = sidebar.getBoundingClientRect().width
    let lastX = startX
    let requestedWidth = width
    let nextExpanded = expanded
    let frame: number | null = null
    previewSidebarRef.current = sidebar
    sidebar.style.setProperty('--nova-activity-bar-drag-width', `${startWidth}px`)
    sidebar.dataset.novaActivityBarResizing = 'true'
    setPreviewExpanded(expanded)
    handle.setPointerCapture(pointerID)

    const updateRequest = (clientX: number) => {
      lastX = clientX
      requestedWidth = startWidth + clientX - startX
      // Hysteresis keeps the menu contents stable when the pointer hovers at the boundary.
      nextExpanded = requestedWidth >= MIN_WIDTH - (nextExpanded ? 8 : 0)
    }
    const move = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== pointerID || moveEvent.clientX === lastX) return
      updateRequest(moveEvent.clientX)
      if (frame !== null) return
      frame = window.requestAnimationFrame(() => {
        frame = null
        const dragWidth = Math.max(0, Math.min(MAX_WIDTH, requestedWidth))
        sidebar.style.setProperty('--nova-activity-bar-drag-width', `${dragWidth}px`)
        setPreviewExpanded(nextExpanded)
      })
    }
    const finish = (result: 'commit' | 'cancel') => {
      cancelResizeRef.current = null
      if (frame !== null) window.cancelAnimationFrame(frame)
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', end)
      window.removeEventListener('pointercancel', cancelPointer)
      window.removeEventListener('blur', cancel)
      window.removeEventListener('keydown', keyDown)
      handle.removeEventListener('lostpointercapture', cancelPointer)
      if (handle.hasPointerCapture(pointerID)) handle.releasePointerCapture(pointerID)
      setPreviewExpanded(null)
      if (result === 'commit') {
        const finalWidth = clampWidth(requestedWidth)
        if (nextExpanded && finalWidth !== width) commitWidth(finalWidth)
        if (nextExpanded !== expanded) onToggle()
      }
    }
    const end = (endEvent: PointerEvent) => {
      if (endEvent.pointerId !== pointerID) return
      if (endEvent.clientX !== lastX) updateRequest(endEvent.clientX)
      finish('commit')
    }
    const cancel = () => finish('cancel')
    const cancelPointer = (cancelEvent: PointerEvent) => {
      if (cancelEvent.pointerId === pointerID) cancel()
    }
    const keyDown = (keyEvent: KeyboardEvent) => {
      if (keyEvent.key !== 'Escape') return
      keyEvent.preventDefault()
      cancel()
    }
    cancelResizeRef.current = cancel
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', end)
    window.addEventListener('pointercancel', cancelPointer)
    window.addEventListener('blur', cancel)
    window.addEventListener('keydown', keyDown)
    handle.addEventListener('lostpointercapture', cancelPointer)
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

  return { width, expanded: previewExpanded ?? expanded, maxWidth: MAX_WIDTH, onResizePointerDown, onResizeKeyDown }
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
