import { useEffect, useState } from 'react'

/**
 * Returns the height (in px) of the on-screen keyboard / bottom chrome that
 * overlaps the layout viewport, so floating composers can lift above it.
 *
 * Why this exists: the mobile shell uses `100dvh` and the agent/story panes
 * are `position: fixed`/`absolute` pinned to the layout-viewport bottom. On
 * iOS Safari the soft keyboard overlays the layout viewport without resizing
 * it, so a composer pinned to `bottom: 0` ends up hidden behind the keyboard.
 * `window.visualViewport` reports the actually-visible area, and the gap
 * `innerHeight - visualViewport.height - visualViewport.offsetTop` is the
 * portion covered by the keyboard.
 *
 * The inset is reported as 0 unless an editable element (input / textarea /
 * contenteditable) is focused. That gates the lift to "the keyboard is up
 * because the user is typing" and avoids false positives from visual-viewport
 * changes caused by scrolling or browser-chrome show/hide. On desktop and on
 * Android (where `dvh` already shrinks for the keyboard) this stays 0, so
 * non-mobile and already-correct layouts are unaffected.
 *
 * Returns 0 when VisualViewport is unavailable.
 */
export function useKeyboardInset(): number {
  const [inset, setInset] = useState(0)

  useEffect(() => {
    if (typeof window === 'undefined' || !window.visualViewport) return
    const viewport = window.visualViewport

    const recompute = () => {
      if (!isEditableFocused()) {
        setInset(0)
        return
      }
      const covered = window.innerHeight - viewport.height - viewport.offsetTop
      setInset(covered > 0 ? Math.round(covered) : 0)
    }

    recompute()
    viewport.addEventListener('resize', recompute)
    viewport.addEventListener('scroll', recompute)
    // focus/blur (capture) so the inset resets the moment an input gains or
    // loses focus, independent of when VisualViewport fires.
    window.addEventListener('focus', recompute, true)
    window.addEventListener('blur', recompute, true)
    return () => {
      viewport.removeEventListener('resize', recompute)
      viewport.removeEventListener('scroll', recompute)
      window.removeEventListener('focus', recompute, true)
      window.removeEventListener('blur', recompute, true)
    }
  }, [])

  return inset
}

function isEditableFocused(): boolean {
  if (typeof document === 'undefined') return false
  const element = document.activeElement
  if (!(element instanceof HTMLElement)) return false
  if (element.isContentEditable) return true
  const tag = element.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT'
}
