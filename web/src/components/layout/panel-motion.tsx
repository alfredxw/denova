import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from 'react'
import {
  Panel,
  Separator,
  usePanelRef,
  type PanelImperativeHandle,
  type PanelProps,
  type SeparatorProps,
} from 'react-resizable-panels'
import { cn } from '@/lib/utils'

type PanelSide = 'left' | 'right'

interface CollapsibleResizablePanelProps extends Omit<PanelProps, 'children' | 'collapsedSize' | 'collapsible' | 'disabled' | 'panelRef'> {
  visible: boolean
  side: PanelSide
  children: ReactNode
  /** Size used only for an explicit button-driven collapsed state. */
  collapsedSize?: string
  collapsedChildren?: ReactNode
  panelRef?: RefObject<PanelImperativeHandle | null>
  contentClassName?: string
}

/**
 * A resizable panel that stays mounted while its flex share animates to or from its compact size.
 * Keeping the panel in the Group preserves both its last user size and child state.
 */
export function CollapsibleResizablePanel({
  visible,
  side,
  children,
  collapsedSize = '0px',
  collapsedChildren,
  panelRef: externalPanelRef,
  className,
  contentClassName,
  onResize,
  ...panelProps
}: CollapsibleResizablePanelProps) {
  const internalPanelRef = usePanelRef()
  const panelRef = externalPanelRef ?? internalPanelRef
  const [programmaticCollapseEnabled, setProgrammaticCollapseEnabled] = useState(!visible)
  const lastVisibleSizeRef = useRef<number | null>(null)
  const hasCollapsedContent = collapsedChildren !== null && collapsedChildren !== undefined
  const stableContentMinWidth = typeof panelProps.minSize === 'number' ? `${panelProps.minSize}px` : panelProps.minSize
  const handleResize = useCallback<NonNullable<PanelProps['onResize']>>((size, id, previousSize) => {
    onResize?.(size, id, previousSize)
    if (visible && Number.isFinite(size.inPixels) && size.inPixels > 0) {
      lastVisibleSizeRef.current = size.inPixels
    }
  }, [onResize, visible])

  useEffect(() => {
    const panel = panelRef.current
    if (!panel) return
    if (visible) {
      if (!programmaticCollapseEnabled) return
      // Keep the library's collapsible mode enabled for this layout pass so expand() can restore
      // the last user width. It is disabled immediately afterward, preventing drag-to-collapse.
      panel.expand()
      // Changing the library's collapsible constraint re-registers the panel and can discard its
      // internal expanded-size cache. Keep our own pixel snapshot so mode switches restore the
      // exact visible width instead of falling back to the minimum.
      if (lastVisibleSizeRef.current !== null) panel.resize(lastVisibleSizeRef.current)
      setProgrammaticCollapseEnabled(false)
    } else {
      if (!programmaticCollapseEnabled) {
        // Re-render the Panel with collapsible enabled before using the imperative API; the
        // library intentionally ignores collapse() while that constraint is disabled.
        setProgrammaticCollapseEnabled(true)
        return
      }
      panel.collapse()
    }
  }, [panelRef, programmaticCollapseEnabled, visible])

  return (
    <Panel
      {...panelProps}
      panelRef={panelRef}
      onResize={handleResize}
      collapsedSize={collapsedSize}
      collapsible={!visible || programmaticCollapseEnabled}
      disabled={!visible}
      aria-hidden={!visible && !hasCollapsedContent}
      inert={!visible && !hasCollapsedContent}
      data-nova-panel-motion="resizable"
      data-nova-panel-side={side}
      data-state={visible ? 'open' : 'closed'}
      data-nova-drag-collapse="disabled"
      data-nova-panel-has-collapsed-content={hasCollapsedContent ? 'true' : undefined}
      className={cn('relative min-h-0', hasCollapsedContent ? 'overflow-visible' : 'overflow-hidden', className)}
    >
      <div data-nova-panel-motion-clip="true" className="h-full min-h-0 overflow-hidden">
        <div
          data-nova-panel-motion-content="true"
          data-nova-panel-side={side}
          data-state={visible ? 'open' : 'closed'}
          aria-hidden={!visible}
          className={cn('h-full min-h-0', contentClassName)}
          style={{ minWidth: stableContentMinWidth }}
        >
          {children}
        </div>
      </div>
      {hasCollapsedContent ? (
        <div
          data-nova-panel-motion-collapsed-content="true"
          data-nova-panel-side={side}
          data-state={visible ? 'open' : 'closed'}
          aria-hidden={visible}
          className={cn('absolute inset-y-0 min-h-0', side === 'left' ? 'left-0' : 'right-0')}
          style={{ width: collapsedSize, minWidth: collapsedSize }}
        >
          {collapsedChildren}
        </div>
      ) : null}
    </Panel>
  )
}

interface CollapsiblePanelSeparatorProps extends SeparatorProps {
  visible: boolean
}

/** A resize target that contracts with its adjacent animated panel. */
export function CollapsiblePanelSeparator({ visible, className, ...separatorProps }: CollapsiblePanelSeparatorProps) {
  return (
    <Separator
      {...separatorProps}
      disabled={!visible}
      aria-hidden={!visible}
      inert={!visible}
      data-nova-panel-motion-separator="true"
      data-state={visible ? 'open' : 'closed'}
      className={className}
    />
  )
}

interface InlineCollapsiblePaneProps {
  visible: boolean
  side: PanelSide
  size: string
  children: ReactNode
  /** Optional compact state that remains usable after the full pane contracts. */
  collapsedSize?: string
  collapsedChildren?: ReactNode
  className?: string
}

/**
 * A fixed-size inline pane for flex/grid layouts that need the same motion as resizable panels.
 * The inner width remains stable so text and trees do not reflow during the short transition.
 */
export function InlineCollapsiblePane({
  visible,
  side,
  size,
  children,
  collapsedSize = '0px',
  collapsedChildren,
  className,
}: InlineCollapsiblePaneProps) {
  const hasCollapsedContent = collapsedChildren !== null && collapsedChildren !== undefined
  return (
    <div
      data-nova-panel-motion="inline"
      data-nova-panel-side={side}
      data-state={visible ? 'open' : 'closed'}
      data-nova-panel-has-collapsed-content={hasCollapsedContent ? 'true' : undefined}
      aria-hidden={!visible && !hasCollapsedContent}
      inert={!visible && !hasCollapsedContent}
      className={cn('relative min-h-0 min-w-0 shrink-0', hasCollapsedContent ? 'overflow-visible' : 'overflow-hidden', className)}
      style={{
        '--nova-inline-panel-size': size,
        '--nova-inline-panel-collapsed-size': collapsedSize,
      } as CSSProperties}
    >
      <div data-nova-panel-motion-clip="true" className="h-full min-h-0 overflow-hidden">
        <div
          data-nova-panel-motion-content="true"
          data-nova-panel-side={side}
          data-state={visible ? 'open' : 'closed'}
          aria-hidden={!visible}
          className="h-full min-h-0"
        >
          {children}
        </div>
      </div>
      {hasCollapsedContent ? (
        <div
          data-nova-panel-motion-collapsed-content="true"
          data-nova-panel-side={side}
          data-state={visible ? 'open' : 'closed'}
          aria-hidden={visible}
          className={cn('absolute inset-y-0 min-h-0', side === 'left' ? 'left-0' : 'right-0')}
        >
          {collapsedChildren}
        </div>
      ) : null}
    </div>
  )
}
