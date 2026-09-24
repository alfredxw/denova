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
 * Restores the interaction scroll position as the virtualized footer resizes.
 * Always use the current content height: retaining a historical maximum leaves
 * blank space after collapses and shorter tab selections, especially during animation.
 * The scroll controller clamps the anchor to the remaining scrollable content.
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

  useLayoutEffect(() => {
    const content = contentRef.current
    if (!content) return

    onInteractionReset()
    onLayoutStabilized()
    const resizeObserver = new ResizeObserver(onLayoutStabilized)
    resizeObserver.observe(content)
    return () => resizeObserver.disconnect()
  }, [onInteractionReset, onLayoutStabilized, resetKey])

  return (
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
  )
}
