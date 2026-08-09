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

/**
 * Keeps a virtualized footer's total height stable while its directly
 * interacted content switches between layouts of different heights.
 *
 * Inactive tab panels use `hidden`, so pre-mounting them preserves React state
 * but contributes no layout height. The reserve below the visible content
 * carries the difference from the tallest layout observed for the current
 * footer identity, preventing the browser from clamping the message scroller
 * before the scroll controller can restore the interaction anchor.
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

    let tallestObservedHeight = 0
    reserve.style.height = '0px'
    onInteractionReset()

    const stabilizeLayout = () => {
      const currentHeight = content.getBoundingClientRect().height
      tallestObservedHeight = Math.max(tallestObservedHeight, currentHeight)
      reserve.style.height = `${Math.max(0, tallestObservedHeight - currentHeight)}px`
      onLayoutStabilized()
    }

    stabilizeLayout()
    const observer = new ResizeObserver(stabilizeLayout)
    observer.observe(content)
    return () => observer.disconnect()
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
