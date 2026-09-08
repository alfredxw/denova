import { useEffect, useRef, type RefObject } from 'react'

type Side = 'left' | 'right'
interface DrawerSwipeOptions {
  surface: RefObject<HTMLDivElement | null>
  drawer: RefObject<HTMLDivElement | null>
  openSide?: Side
  sides: Side[]
  onOpen: (side: Side) => void
  onClose: () => void
}

/** Only claim horizontal edge gestures; vertical reading and nested controls keep their touches. */
export function useMobileDrawerSwipe(options: DrawerSwipeOptions) {
  const latest = useRef(options)
  latest.current = options
  useEffect(() => {
    let start: { x: number; y: number; side: Side; closing: boolean } | undefined
    const begin = (event: TouchEvent) => {
      start = undefined
      const { surface, drawer, openSide, sides } = latest.current
      const target = event.target
      if (event.touches.length !== 1 || !(target instanceof Element)) return
      if (target.closest('input, textarea, [contenteditable="true"], [role="slider"], [data-nova-swipe-ignore="true"]')) return
      const { clientX: x, clientY: y } = event.touches[0]
      if (openSide && drawer.current?.contains(target)) {
        // Dismiss from the drawer header, preserving horizontal editors inside it.
        if (target.closest('[data-mobile-drawer-header]')) start = { x, y, side: openSide, closing: true }
      } else if (!openSide && surface.current?.contains(target) && !target.closest('[role="dialog"], [role="menu"], [data-slot="popover-content"]')) {
        const side = x <= 24 ? 'left' : x >= window.innerWidth - 24 ? 'right' : undefined
        if (side && sides.includes(side)) start = { x, y, side, closing: false }
      }
    }
    const move = (event: TouchEvent) => {
      if (!start) return
      if (event.touches.length !== 1) { start = undefined; return }
      const dx = event.touches[0].clientX - start.x
      const dy = event.touches[0].clientY - start.y
      if (Math.abs(dy) > 8 && Math.abs(dy) >= Math.abs(dx)) { start = undefined; return }
      if (Math.abs(dx) > 8 && event.cancelable) event.preventDefault()
    }
    const end = (event: TouchEvent) => {
      const gesture = start
      start = undefined
      if (!gesture || !event.changedTouches.length) return
      const dx = event.changedTouches[0].clientX - gesture.x
      const dy = event.changedTouches[0].clientY - gesture.y
      const direction = (gesture.side === 'left' ? 1 : -1) * (gesture.closing ? -1 : 1)
      if (dx * direction < 48 || Math.abs(dx) < Math.abs(dy) * 1.35) return
      if (event.cancelable) event.preventDefault()
      if (gesture.closing) latest.current.onClose()
      else latest.current.onOpen(gesture.side)
    }
    const cancel = () => { start = undefined }
    document.addEventListener('touchstart', begin, { passive: true })
    document.addEventListener('touchmove', move, { passive: false })
    document.addEventListener('touchend', end, { passive: false })
    document.addEventListener('touchcancel', cancel)
    return () => {
      document.removeEventListener('touchstart', begin)
      document.removeEventListener('touchmove', move)
      document.removeEventListener('touchend', end)
      document.removeEventListener('touchcancel', cancel)
    }
  }, [])
}
