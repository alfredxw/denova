import { useLayoutEffect } from 'react'

/** The compact shell owns keyboard resizing, including its portalled sheets. */
export function useMobileViewport() {
  useLayoutEffect(() => {
    const root = document.documentElement
    const viewport = window.visualViewport
    let frame = 0
    const update = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        // Pinch zoom is a user reading preference, not a keyboard resize.
        if (viewport && viewport.scale !== 1) return
        const height = viewport?.height ?? window.innerHeight
        const editing = document.activeElement?.matches('input, textarea, [contenteditable="true"]')
        const keyboardOpen = Boolean(editing && window.innerHeight - height > 120)
        // Standalone browsers can retain stale visual viewport measurements after
        // backgrounding or keyboard dismissal. CSS owns the unobstructed viewport.
        if (keyboardOpen) root.style.setProperty('--nova-mobile-height', `${height}px`)
        else root.style.removeProperty('--nova-mobile-height')
        root.style.setProperty('--nova-mobile-top', keyboardOpen ? `${viewport?.offsetTop ?? 0}px` : '0px')
        root.dataset.novaKeyboardOpen = String(keyboardOpen)
      })
    }
    update()
    viewport?.addEventListener('resize', update)
    viewport?.addEventListener('scroll', update)
    window.addEventListener('resize', update)
    window.addEventListener('pageshow', update)
    document.addEventListener('visibilitychange', update)
    document.addEventListener('focusin', update)
    document.addEventListener('focusout', update)
    return () => {
      cancelAnimationFrame(frame)
      viewport?.removeEventListener('resize', update)
      viewport?.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
      window.removeEventListener('pageshow', update)
      document.removeEventListener('visibilitychange', update)
      document.removeEventListener('focusin', update)
      document.removeEventListener('focusout', update)
      root.style.removeProperty('--nova-mobile-height')
      root.style.removeProperty('--nova-mobile-top')
      delete root.dataset.novaKeyboardOpen
    }
  }, [])
}
