import { useEffect, useRef, useState } from 'react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

const ACTIVE_TITLE_ATTRIBUTE = 'data-native-title-tooltip'
const ACTIVE_TITLE_SELECTOR = `[title],[${ACTIVE_TITLE_ATTRIBUTE}]`
// Match Radix's standard dwell time so legacy browser titles do not flash while
// the pointer merely crosses a control. Explicit Tooltip components keep their
// existing provider/instance timing and are intentionally unaffected.
const DEFAULT_POINTER_DELAY_MS = 700

type TooltipRect = {
  height: number
  left: number
  top: number
  width: number
}

type TooltipState = {
  rect: TooltipRect
  target: Element
  text: string
  visible: boolean
}

type NativeTitleTooltipProps = {
  /** Pointer hints wait briefly so moving across dense controls does not create visual noise. */
  pointerDelayMs?: number
}

/**
 * Renders legacy/native `title` attributes through the shared Radix tooltip.
 *
 * Denova contains native titles on ordinary DOM nodes and on DOM produced by
 * third-party editors. Event delegation keeps those dynamic nodes covered
 * without wrapping every element, while restoring the original attribute as
 * soon as the pointer/focus leaves so the DOM remains compatible with callers.
 */
export function NativeTitleTooltip({
  pointerDelayMs = DEFAULT_POINTER_DELAY_MS,
}: NativeTitleTooltipProps) {
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const pointerTargetRef = useRef<Element | null>(null)
  const focusTargetRef = useRef<Element | null>(null)
  const displayedTargetRef = useRef<Element | null>(null)
  const keyboardFocusNavigationRef = useRef(false)
  const storedTitlesRef = useRef(new WeakMap<Element, string>())
  const showTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    const storedTitles = storedTitlesRef.current
    let positionFrame: number | null = null

    const clearShowTimer = () => {
      if (showTimerRef.current === null) return
      clearTimeout(showTimerRef.current)
      showTimerRef.current = null
    }

    const readAndSuppressTitle = (target: Element) => {
      const currentTitle = target.getAttribute('title')
      if (currentTitle !== null) {
        storedTitles.set(target, currentTitle)
        target.removeAttribute('title')
        target.setAttribute(ACTIVE_TITLE_ATTRIBUTE, '')
      }
      return storedTitles.get(target)?.trim() || ''
    }

    const readTitle = (target: Element) => (
      target.getAttribute('title')?.trim()
      || storedTitles.get(target)?.trim()
      || ''
    )

    const restoreTitle = (target: Element | null) => {
      if (!target) return

      const storedTitle = storedTitles.get(target)
      // A React update may have supplied a newer title while the hint was open.
      // Preserve that value instead of overwriting it with the older snapshot.
      if (storedTitle !== undefined && !target.hasAttribute('title') && target.isConnected) {
        target.setAttribute('title', storedTitle)
      }
      target.removeAttribute(ACTIVE_TITLE_ATTRIBUTE)
      storedTitles.delete(target)
    }

    const measure = (target: Element): TooltipRect => {
      const rect = target.getBoundingClientRect()
      return {
        height: rect.height,
        left: rect.left,
        top: rect.top,
        width: rect.width,
      }
    }

    const showTarget = (target: Element | null, immediate: boolean) => {
      clearShowTimer()
      displayedTargetRef.current = target

      if (!target || !target.isConnected) {
        setTooltip(null)
        return
      }

      // Keyboard focus keeps the native attribute as an accessibility fallback;
      // only pointer hover must suppress the browser-drawn tooltip.
      const text = pointerTargetRef.current === target
        ? readAndSuppressTitle(target)
        : readTitle(target)
      if (!text) {
        setTooltip(null)
        return
      }

      const nextState: TooltipState = {
        rect: measure(target),
        target,
        text,
        visible: immediate || pointerDelayMs <= 0,
      }
      setTooltip(nextState)

      if (nextState.visible) return
      showTimerRef.current = setTimeout(() => {
        showTimerRef.current = null
        if (displayedTargetRef.current !== target) return
        setTooltip((current) => current?.target === target ? { ...current, visible: true } : current)
      }, pointerDelayMs)
    }

    const syncDisplayedTarget = (immediate = false) => {
      const nextTarget = pointerTargetRef.current || focusTargetRef.current
      if (displayedTargetRef.current === nextTarget) {
        if (immediate && nextTarget) {
          clearShowTimer()
          setTooltip((current) => current?.target === nextTarget
            ? { ...current, visible: true }
            : current)
        }
        return
      }
      showTarget(nextTarget, immediate || (!pointerTargetRef.current && Boolean(focusTargetRef.current)))
    }

    const findTitleTarget = (value: EventTarget | null) => {
      if (!(value instanceof Element)) return null
      return value.closest(ACTIVE_TITLE_SELECTOR)
    }

    const transitionTarget = (source: 'pointer' | 'focus', nextTarget: Element | null) => {
      const sourceRef = source === 'pointer' ? pointerTargetRef : focusTargetRef
      const otherTarget = source === 'pointer' ? focusTargetRef.current : pointerTargetRef.current
      const previousTarget = sourceRef.current
      if (previousTarget === nextTarget) return

      sourceRef.current = nextTarget
      if (source === 'pointer' && previousTarget && previousTarget !== nextTarget) {
        restoreTitle(previousTarget)
      }
      if (source === 'pointer' && nextTarget) readAndSuppressTitle(nextTarget)
      if (source === 'focus' && previousTarget && previousTarget !== otherTarget) {
        restoreTitle(previousTarget)
      }
      syncDisplayedTarget(source === 'focus')
    }

    const handlePointerOver = (event: PointerEvent) => {
      if (event.pointerType === 'touch') return
      const nextTarget = findTitleTarget(event.target)
      const previousTarget = findTitleTarget(event.relatedTarget)
      if (nextTarget === previousTarget) return
      transitionTarget('pointer', nextTarget)
    }

    const handlePointerOut = (event: PointerEvent) => {
      if (event.pointerType === 'touch') return
      const previousTarget = findTitleTarget(event.target)
      const nextTarget = findTitleTarget(event.relatedTarget)
      if (nextTarget === previousTarget) return
      transitionTarget('pointer', nextTarget)
    }

    const handlePointerDown = () => {
      keyboardFocusNavigationRef.current = false
      const previousFocusTarget = focusTargetRef.current
      focusTargetRef.current = null
      if (previousFocusTarget && previousFocusTarget !== pointerTargetRef.current) {
        restoreTitle(previousFocusTarget)
      }
      // Clicking an action dismisses any pending/visible hint. If a Radix menu
      // later restores focus to its trigger, that focus is still pointer-driven
      // and must not bypass the normal hover delay.
      clearShowTimer()
      setTooltip(null)
    }

    const handleFocusIn = (event: FocusEvent) => {
      if (!keyboardFocusNavigationRef.current) return
      keyboardFocusNavigationRef.current = false
      transitionTarget('focus', findTitleTarget(event.target))
    }

    const handleFocusOut = (event: FocusEvent) => {
      const previousTarget = findTitleTarget(event.target)
      const nextTarget = findTitleTarget(event.relatedTarget)
      if (nextTarget === previousTarget) return
      transitionTarget('focus', nextTarget)
    }

    const refreshPosition = () => {
      if (positionFrame !== null) return
      positionFrame = window.requestAnimationFrame(() => {
        positionFrame = null
        const target = displayedTargetRef.current
        if (!target?.isConnected) {
          showTarget(null, true)
          return
        }
        setTooltip((current) => current?.target === target
          ? { ...current, rect: measure(target) }
          : current)
      })
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Tab') keyboardFocusNavigationRef.current = true
      if (event.key !== 'Escape' || !displayedTargetRef.current) return
      clearShowTimer()
      setTooltip((current) => current ? { ...current, visible: false } : current)
    }

    const titleObserver = new MutationObserver((records) => {
      records.forEach((record) => {
        const target = record.target
        if (!(target instanceof Element)) return

        const updatedTitle = target.getAttribute('title')
        const pointerActive = target.hasAttribute(ACTIVE_TITLE_ATTRIBUTE)
        // This is the mutation generated by readAndSuppressTitle itself.
        if (pointerActive && updatedTitle === null) return
        if (pointerActive && updatedTitle !== null) {
          storedTitles.set(target, updatedTitle)
          target.removeAttribute('title')
        }
        if (displayedTargetRef.current === target) {
          const text = pointerActive
            ? storedTitles.get(target)?.trim() || ''
            : updatedTitle?.trim() || ''
          setTooltip((current) => text
            ? {
                rect: measure(target),
                target,
                text,
                visible: current?.target === target ? current.visible : true,
              }
            : null)
        }
      })
      if (displayedTargetRef.current && !displayedTargetRef.current.isConnected) {
        showTarget(null, true)
      }
    })

    document.addEventListener('pointerdown', handlePointerDown, { capture: true, passive: true })
    document.addEventListener('pointerover', handlePointerOver, { capture: true, passive: true })
    document.addEventListener('pointerout', handlePointerOut, { capture: true, passive: true })
    document.addEventListener('focusin', handleFocusIn, true)
    document.addEventListener('focusout', handleFocusOut, true)
    document.addEventListener('keydown', handleKeyDown, true)
    document.addEventListener('scroll', refreshPosition, { capture: true, passive: true })
    window.addEventListener('resize', refreshPosition)
    titleObserver.observe(document.documentElement, {
      attributeFilter: ['title'],
      attributes: true,
      childList: true,
      subtree: true,
    })

    return () => {
      clearShowTimer()
      if (positionFrame !== null) window.cancelAnimationFrame(positionFrame)
      titleObserver.disconnect()
      document.removeEventListener('pointerdown', handlePointerDown, true)
      document.removeEventListener('pointerover', handlePointerOver, true)
      document.removeEventListener('pointerout', handlePointerOut, true)
      document.removeEventListener('focusin', handleFocusIn, true)
      document.removeEventListener('focusout', handleFocusOut, true)
      document.removeEventListener('keydown', handleKeyDown, true)
      document.removeEventListener('scroll', refreshPosition, true)
      window.removeEventListener('resize', refreshPosition)
      restoreTitle(pointerTargetRef.current)
      if (focusTargetRef.current !== pointerTargetRef.current) restoreTitle(focusTargetRef.current)
    }
  }, [pointerDelayMs])

  if (!tooltip) return null

  return (
    <Tooltip open={tooltip.visible}>
      <TooltipTrigger asChild>
        <span
          aria-hidden="true"
          data-native-title-tooltip-anchor=""
          className="pointer-events-none fixed block"
          style={{
            height: tooltip.rect.height,
            left: tooltip.rect.left,
            top: tooltip.rect.top,
            width: tooltip.rect.width,
          }}
        />
      </TooltipTrigger>
      <TooltipContent data-native-title-tooltip-content="" side="top">
        {tooltip.text}
      </TooltipContent>
    </Tooltip>
  )
}
