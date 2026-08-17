import { useLayoutEffect, useRef, type ReactNode } from 'react'

interface StableAfterContentBoundaryProps {
  children: ReactNode
  className: string
  resetKey?: string
  onInteractionStart: () => void
  onInteraction: () => void
  onInteractionReset: () => void
  onLayoutStabilized: () => void
}

const HEIGHT_SCOPE_ATTRIBUTE = 'data-nova-chat-after-content-height-scope'

/**
 * Keeps a virtualized footer's total height stable while its directly
 * interacted content switches between layouts of different heights.
 *
 * Inactive tab panels use `hidden`, so pre-mounting them preserves React state
 * but contributes no layout height. The reserve below the visible content
 * carries the difference from the tallest layout observed for the current
 * footer identity and optional height scope, preventing the browser from
 * clamping the message scroller before the scroll controller can restore the
 * interaction anchor. A scoped footer can therefore keep peer tabs stable
 * without letting an expanded layout inflate its compact preview.
 */
export function StableAfterContentBoundary({
  children,
  className,
  resetKey,
  onInteractionStart,
  onInteraction,
  onInteractionReset,
  onLayoutStabilized,
}: StableAfterContentBoundaryProps) {
  const contentRef = useRef<HTMLDivElement | null>(null)
  const reserveRef = useRef<HTMLDivElement | null>(null)

  useLayoutEffect(() => {
    const content = contentRef.current
    const reserve = reserveRef.current
    if (!content || !reserve) return

    const tallestObservedHeightByScope = new Map<string, number>()
    reserve.style.height = '0px'
    onInteractionReset()

    const stabilizeLayout = () => {
      const currentHeight = content.getBoundingClientRect().height
      const heightScope = content.querySelector<HTMLElement>(`[${HEIGHT_SCOPE_ATTRIBUTE}]`)
        ?.getAttribute(HEIGHT_SCOPE_ATTRIBUTE) || ''
      const tallestObservedHeight = Math.max(tallestObservedHeightByScope.get(heightScope) || 0, currentHeight)
      tallestObservedHeightByScope.set(heightScope, tallestObservedHeight)
      reserve.style.height = `${Math.max(0, tallestObservedHeight - currentHeight)}px`
      onLayoutStabilized()
    }

    stabilizeLayout()
    const resizeObserver = new ResizeObserver(stabilizeLayout)
    resizeObserver.observe(content)
    const scopeObserver = typeof MutationObserver === 'undefined'
      ? null
      : new MutationObserver(stabilizeLayout)
    scopeObserver?.observe(content, {
      attributes: true,
      attributeFilter: [HEIGHT_SCOPE_ATTRIBUTE],
      subtree: true,
    })
    return () => {
      resizeObserver.disconnect()
      scopeObserver?.disconnect()
    }
  }, [onInteractionReset, onLayoutStabilized, resetKey])

  return (
    <>
      <div
        ref={contentRef}
        data-nova-chat-after-content
        className={className}
        onPointerDownCapture={onInteractionStart}
        onKeyDownCapture={onInteractionStart}
        onClickCapture={onInteraction}
      >
        {children}
      </div>
      <div ref={reserveRef} aria-hidden="true" data-nova-chat-after-content-reserve className="shrink-0" />
    </>
  )
}
