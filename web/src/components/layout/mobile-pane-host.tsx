import { useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { X } from 'lucide-react'

export interface MobilePane {
  id: string
  title: string
  side: 'left' | 'right'
  content: ReactNode
  icon?: ReactNode
  onOpen?: () => void
  onClose?: () => void
  className?: string
}

export interface MobilePaneControls {
  openPaneId: string | null
  openPane: (id: string) => void
  closePane: () => void
  togglePane: (id: string) => void
}

interface MobilePaneHostProps {
  panes: MobilePane[]
  closeLabel: string
  children: ReactNode | ((controls: MobilePaneControls) => ReactNode)
  className?: string
  openPaneId?: string | null
  onOpenPaneChange?: (id: string | null) => void
}

const EDGE_SWIPE_WIDTH = 22
const EDGE_SWIPE_THRESHOLD = 48
const HORIZONTAL_INTENT_RATIO = 1.35
const HORIZONTAL_COMMIT_RATIO = 1.15
const DRAG_START_DISTANCE = 8
const DRAWER_SETTLE_MS = 220
const DRAWER_OPEN_RATIO = 0.18

export function MobilePaneHost({
  panes,
  closeLabel,
  children,
  className = 'relative h-full min-h-0',
  openPaneId: controlledOpenPaneId,
  onOpenPaneChange,
}: MobilePaneHostProps) {
  const [internalOpenPaneId, setInternalOpenPaneId] = useState<string | null>(null)
  const [dragState, setDragState] = useState<{
    paneId: string
    side: 'left' | 'right'
    progress: number
    dragging: boolean
  } | null>(null)
  const openPaneId = controlledOpenPaneId === undefined ? internalOpenPaneId : controlledOpenPaneId
  const paneIds = useMemo(() => new Set(panes.map((pane) => pane.id)), [panes])
  const openPane = panes.find((pane) => pane.id === openPaneId) || null
  const dragPane = dragState ? panes.find((pane) => pane.id === dragState.paneId) || null : null
  const visiblePane = openPane || dragPane
  const latestOpenPaneRef = useRef<MobilePane | null>(openPane)
  const settleTimerRef = useRef<number | null>(null)
  latestOpenPaneRef.current = openPane

  useEffect(() => {
    return () => {
      if (settleTimerRef.current) window.clearTimeout(settleTimerRef.current)
    }
  }, [])

  const setOpenPaneId = (nextId: string | null) => {
    const current = latestOpenPaneRef.current
    if (current?.id === nextId) return
    current?.onClose?.()
    if (controlledOpenPaneId === undefined) setInternalOpenPaneId(nextId)
    onOpenPaneChange?.(nextId)
    if (nextId) panes.find((pane) => pane.id === nextId)?.onOpen?.()
  }

  const controls: MobilePaneControls = {
    openPaneId,
    openPane: (id) => {
      if (paneIds.has(id)) setOpenPaneId(id)
    },
    closePane: () => setOpenPaneId(null),
    togglePane: (id) => setOpenPaneId(openPaneId === id ? null : id),
  }

  useEffect(() => {
    if (openPaneId && !paneIds.has(openPaneId)) setOpenPaneId(null)
  }, [openPaneId, paneIds])

  // Allow external code (e.g. file selection in the project drawer) to close
  // all open panes by dispatching this event. Used for file-tree auto-close.
  useEffect(() => {
    const close = () => setOpenPaneId(null)
    window.addEventListener('nova:mobile-close-panes', close)
    return () => window.removeEventListener('nova:mobile-close-panes', close)
  }, [])

  const hostRef = useEdgeSwipe({
    leftEnabled: panes.some((pane) => pane.side === 'left'),
    rightEnabled: panes.some((pane) => pane.side === 'right'),
    onDragStart: (side) => {
      const pane = panes.find((item) => item.side === side)
      if (!pane) return
      if (settleTimerRef.current) {
        window.clearTimeout(settleTimerRef.current)
        settleTimerRef.current = null
      }
      setDragState({ paneId: pane.id, side, progress: 0.02, dragging: true })
    },
    onDragProgress: (side, progress) => {
      const pane = panes.find((item) => item.side === side)
      if (!pane) return
      setDragState({ paneId: pane.id, side, progress, dragging: true })
    },
    onDragEnd: (side, shouldOpen, progress) => {
      const pane = panes.find((item) => item.side === side)
      if (!pane) return
      if (shouldOpen) {
        setDragState({ paneId: pane.id, side, progress, dragging: false })
        setOpenPaneId(pane.id)
        window.requestAnimationFrame(() => setDragState(null))
        return
      }
      setDragState({ paneId: pane.id, side, progress: 0, dragging: false })
      settleTimerRef.current = window.setTimeout(() => {
        setDragState(null)
        settleTimerRef.current = null
      }, DRAWER_SETTLE_MS)
    },
  })

  const paneProgress = openPane ? 1 : dragState?.progress ?? 0
  const paneSide = visiblePane?.side ?? dragState?.side

  return (
    <div ref={hostRef} className={className} data-nova-mobile-pane-host="true">
      {typeof children === 'function' ? children(controls) : children}
      {visiblePane && paneSide ? (
        <MobileDrawer
          pane={visiblePane}
          closeLabel={closeLabel}
          progress={paneProgress}
          dragging={dragState?.dragging ?? false}
          side={paneSide}
          onClose={() => setOpenPaneId(null)}
        />
      ) : null}
    </div>
  )
}

function MobileDrawer({
  pane,
  closeLabel,
  progress,
  dragging,
  side,
  onClose,
}: {
  pane: MobilePane
  closeLabel: string
  progress: number
  dragging: boolean
  side: 'left' | 'right'
  onClose: () => void
}) {
  const titleId = `nova-mobile-pane-title-${pane.id}`
  const clampedProgress = clamp(progress, 0, 1)
  const offset = side === 'left' ? (clampedProgress - 1) * 100 : (1 - clampedProgress) * 100
  const drawerStyle: CSSProperties = {
    transform: `translate3d(${offset}%, 0, 0)`,
    transition: dragging ? 'none' : `transform ${DRAWER_SETTLE_MS}ms var(--nova-ease)`,
  }
  const overlayStyle: CSSProperties = {
    opacity: clampedProgress * 0.5,
    transition: dragging ? 'none' : `opacity ${DRAWER_SETTLE_MS}ms var(--nova-ease)`,
  }
  const sideClassName = side === 'left'
    ? 'left-0 border-r'
    : 'right-0 border-l'

  return (
    <>
      <div
        aria-hidden="true"
        data-nova-mobile-pane-overlay="true"
        className="fixed inset-0 z-50 bg-black"
        style={overlayStyle}
        onClick={onClose}
      />
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        data-state="open"
        data-nova-mobile-pane-content="true"
        data-side={side}
        className={`fixed inset-y-0 z-50 flex w-[min(92vw,420px)] max-w-none flex-col gap-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] sm:max-w-none ${sideClassName} ${pane.className || ''}`}
        style={drawerStyle}
      >
        <div className="nova-topbar flex h-11 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3">
          <h2 id={titleId} className="flex min-w-0 items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
            {pane.icon ? <span className="flex h-4 w-4 shrink-0 items-center justify-center text-[var(--nova-text-muted)]">{pane.icon}</span> : null}
            <span className="min-w-0 truncate">{pane.title}</span>
          </h2>
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={closeLabel} onClick={onClose}>
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-hidden">{pane.content}</div>
      </section>
    </>
  )
}

function useEdgeSwipe({
  leftEnabled,
  rightEnabled,
  onDragStart,
  onDragProgress,
  onDragEnd,
}: {
  leftEnabled: boolean
  rightEnabled: boolean
  onDragStart: (side: 'left' | 'right') => void
  onDragProgress: (side: 'left' | 'right', progress: number) => void
  onDragEnd: (side: 'left' | 'right', shouldOpen: boolean, progress: number) => void
}) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const gestureRef = useRef<{
    startX: number
    startY: number
    side: 'left' | 'right'
    source: 'pointer' | 'mouse' | 'touch'
    committed: boolean
    pointerId?: number
  } | null>(null)

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const beginGesture = (clientX: number, clientY: number, target: EventTarget | null, source: 'pointer' | 'mouse' | 'touch', pointerId?: number) => {
      if (shouldIgnoreNestedPaneHost(target, host) || shouldIgnoreSwipeTarget(target)) return
      const width = window.innerWidth
      const side = clientX <= EDGE_SWIPE_WIDTH ? 'left' : width - clientX <= EDGE_SWIPE_WIDTH ? 'right' : null
      if (!side || (side === 'left' && !leftEnabled) || (side === 'right' && !rightEnabled)) return
      gestureRef.current = { startX: clientX, startY: clientY, side, source, committed: false, pointerId }
    }

    const updateGesture = (clientX: number, clientY: number, source: 'pointer' | 'mouse' | 'touch', pointerId?: number, event?: Event) => {
      const gesture = gestureRef.current
      if (!gesture || gesture.source !== source || (pointerId !== undefined && gesture.pointerId !== pointerId)) return
      const deltaX = clientX - gesture.startX
      const deltaY = clientY - gesture.startY
      const distance = gesture.side === 'left' ? deltaX : -deltaX
      if (distance <= 0) return
      if (!gesture.committed) {
        if (Math.abs(deltaY) > DRAG_START_DISTANCE && distance < Math.abs(deltaY) * HORIZONTAL_COMMIT_RATIO) {
          gestureRef.current = null
          return
        }
        if (distance < DRAG_START_DISTANCE || distance < Math.abs(deltaY) * HORIZONTAL_COMMIT_RATIO) return
        gesture.committed = true
        onDragStart(gesture.side)
      }
      event?.preventDefault()
      onDragProgress(gesture.side, dragProgressForDistance(distance))
    }

    const finishGesture = (clientX: number, clientY: number, source: 'pointer' | 'mouse' | 'touch', pointerId?: number) => {
      const gesture = gestureRef.current
      if (!gesture || gesture.source !== source || (pointerId !== undefined && gesture.pointerId !== pointerId)) return
      gestureRef.current = null
      const deltaX = clientX - gesture.startX
      const deltaY = clientY - gesture.startY
      const distance = gesture.side === 'left' ? deltaX : -deltaX
      const progress = dragProgressForDistance(Math.max(0, distance))
      const openDistance = Math.max(EDGE_SWIPE_THRESHOLD, drawerGestureWidth() * DRAWER_OPEN_RATIO)
      const shouldOpen = distance >= openDistance && distance >= Math.abs(deltaY) * HORIZONTAL_INTENT_RATIO
      if (!gesture.committed && shouldOpen) {
        onDragStart(gesture.side)
        onDragProgress(gesture.side, progress)
      }
      if (gesture.committed || shouldOpen) onDragEnd(gesture.side, shouldOpen, progress)
    }

    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType === 'mouse' || event.button !== 0) return
      beginGesture(event.clientX, event.clientY, event.target, 'pointer', event.pointerId)
    }

    const onPointerUp = (event: PointerEvent) => {
      finishGesture(event.clientX, event.clientY, 'pointer', event.pointerId)
    }

    const onPointerMove = (event: PointerEvent) => {
      updateGesture(event.clientX, event.clientY, 'pointer', event.pointerId, event)
    }

    const onPointerCancel = (event: PointerEvent) => {
      if (gestureRef.current?.source === 'pointer' && gestureRef.current.pointerId === event.pointerId) gestureRef.current = null
    }

    const onMouseDown = (event: MouseEvent) => {
      if (event.button !== 0) return
      beginGesture(event.clientX, event.clientY, event.target, 'mouse')
    }

    const onMouseUp = (event: MouseEvent) => {
      finishGesture(event.clientX, event.clientY, 'mouse')
    }

    const onMouseMove = (event: MouseEvent) => {
      updateGesture(event.clientX, event.clientY, 'mouse', undefined, event)
    }

    const onTouchStart = (event: TouchEvent) => {
      if (window.PointerEvent || event.touches.length !== 1) return
      const touch = event.touches[0]
      beginGesture(touch.clientX, touch.clientY, event.target, 'touch')
    }

    const onTouchEnd = (event: TouchEvent) => {
      if (window.PointerEvent || event.changedTouches.length === 0) return
      const touch = event.changedTouches[0]
      finishGesture(touch.clientX, touch.clientY, 'touch')
    }

    const onTouchMove = (event: TouchEvent) => {
      if (window.PointerEvent || event.touches.length !== 1) return
      const touch = event.touches[0]
      updateGesture(touch.clientX, touch.clientY, 'touch', undefined, event)
    }

    const onMouseOrTouchCancel = () => {
      const source = gestureRef.current?.source
      if (source === 'mouse' || source === 'touch') gestureRef.current = null
    }

    host.addEventListener('pointerdown', onPointerDown, { passive: true })
    host.addEventListener('mousedown', onMouseDown, { passive: true })
    host.addEventListener('touchstart', onTouchStart, { passive: true })
    window.addEventListener('pointerup', onPointerUp, { passive: true })
    window.addEventListener('pointermove', onPointerMove, { passive: false })
    window.addEventListener('pointercancel', onPointerCancel, { passive: true })
    window.addEventListener('mouseup', onMouseUp, { passive: true })
    window.addEventListener('mousemove', onMouseMove, { passive: false })
    window.addEventListener('mouseleave', onMouseOrTouchCancel, { passive: true })
    window.addEventListener('touchend', onTouchEnd, { passive: true })
    window.addEventListener('touchmove', onTouchMove, { passive: false })
    window.addEventListener('touchcancel', onMouseOrTouchCancel, { passive: true })
    return () => {
      host.removeEventListener('pointerdown', onPointerDown)
      host.removeEventListener('mousedown', onMouseDown)
      host.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointercancel', onPointerCancel)
      window.removeEventListener('mouseup', onMouseUp)
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseleave', onMouseOrTouchCancel)
      window.removeEventListener('touchend', onTouchEnd)
      window.removeEventListener('touchmove', onTouchMove)
      window.removeEventListener('touchcancel', onMouseOrTouchCancel)
    }
  }, [leftEnabled, onDragEnd, onDragProgress, onDragStart, rightEnabled])

  return hostRef
}

function dragProgressForDistance(distance: number) {
  return clamp(distance / drawerGestureWidth(), 0, 1)
}

function drawerGestureWidth() {
  return Math.min(window.innerWidth * 0.92, 420)
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function shouldIgnoreSwipeTarget(target: EventTarget | null) {
  if (!(target instanceof Element)) return false
  return Boolean(target.closest('[data-nova-swipe-ignore="true"]'))
}

function shouldIgnoreNestedPaneHost(target: EventTarget | null, host: HTMLElement) {
  if (!(target instanceof Element)) return false
  const closestHost = target.closest('[data-nova-mobile-pane-host="true"]')
  return Boolean(closestHost && closestHost !== host)
}
