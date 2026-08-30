import type { ContextMenuAnchorRect } from '@pierre/trees'
import type { ComponentProps, KeyboardEvent, ReactNode } from 'react'
import { useEffect, useLayoutEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import './FileTreeMenu.css'

interface FileTreeMenuProps {
  children: ReactNode
  anchorRect: ContextMenuAnchorRect
}

const VIEWPORT_INSET = 8
const ANCHOR_GAP = 4

/** Accessible menu portaled out of Pierre's slot so it never changes tree sizing. */
export function FileTreeMenu({ children, anchorRect }: FileTreeMenuProps) {
  const ref = useRef<HTMLDivElement | null>(null)

  useLayoutEffect(() => {
    const menu = ref.current
    if (!menu) return
    const position = () => {
      const rect = menu.getBoundingClientRect()
      const { left, top } = placeMenu(anchorRect, rect.width, rect.height, window.innerWidth, window.innerHeight)
      menu.style.left = `${left}px`
      menu.style.top = `${top}px`
      menu.style.visibility = 'visible'
    }
    position()
    window.addEventListener('resize', position)
    return () => window.removeEventListener('resize', position)
  }, [anchorRect])

  useEffect(() => {
    ref.current?.focus()
  }, [])

  return createPortal(
    <div
      ref={ref}
      role="menu"
      tabIndex={-1}
      data-file-tree-context-menu-root="true"
      className="nova-file-tree-menu nova-file-tree-menu-surface"
      onKeyDown={moveMenuFocus}
    >
      {children}
    </div>,
    document.body,
  )
}

export function FileTreeMenuItem({ className, variant = 'default', ...props }: ComponentProps<'button'> & {
  variant?: 'default' | 'destructive'
}) {
  return (
    <button
      type="button"
      role="menuitem"
      data-variant={variant}
      className={cn('nova-file-tree-menu-item', className)}
      {...props}
    />
  )
}

export function FileTreeMenuSeparator() {
  return <div role="separator" className="nova-file-tree-menu-separator" />
}

/** Visually describes an implemented shortcut without changing the menu item's accessible name. */
export function FileTreeMenuShortcut({ children }: { children: ReactNode }) {
  return <span aria-hidden="true" className="nova-file-tree-menu-shortcut">{children}</span>
}

function placeMenu(
  anchor: ContextMenuAnchorRect,
  width: number,
  height: number,
  viewportWidth: number,
  viewportHeight: number,
) {
  return {
    left: placeAxis(anchor.right + ANCHOR_GAP, anchor.left - width - ANCHOR_GAP, width, viewportWidth),
    top: placeAxis(anchor.bottom + ANCHOR_GAP, anchor.top - height - ANCHOR_GAP, height, viewportHeight),
  }
}

function placeAxis(preferred: number, fallback: number, size: number, viewportSize: number) {
  const maximum = Math.max(VIEWPORT_INSET, viewportSize - size - VIEWPORT_INSET)
  const candidate = preferred + size <= viewportSize - VIEWPORT_INSET || fallback < VIEWPORT_INSET
    ? preferred
    : fallback
  return Math.round(Math.min(Math.max(candidate, VIEWPORT_INSET), maximum))
}

function moveMenuFocus(event: KeyboardEvent<HTMLDivElement>) {
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp' && event.key !== 'Home' && event.key !== 'End') return
  const items = [...event.currentTarget.querySelectorAll<HTMLElement>('[role="menuitem"]:not(:disabled)')]
  if (items.length === 0) return
  event.preventDefault()
  const current = items.indexOf(document.activeElement as HTMLElement)
  const next = event.key === 'Home'
    ? 0
    : event.key === 'End'
      ? items.length - 1
      : event.key === 'ArrowDown'
        ? (current + 1) % items.length
        : (current <= 0 ? items.length : current) - 1
  items[next]?.focus()
}
