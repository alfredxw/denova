import { useEffect, type CSSProperties, type ReactNode, type RefObject } from 'react'
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
  panelRef?: RefObject<PanelImperativeHandle | null>
  contentClassName?: string
}

/**
 * A resizable panel that stays mounted while its flex share animates to or from zero.
 * Keeping the panel in the Group preserves both its last user size and child state.
 */
export function CollapsibleResizablePanel({
  visible,
  side,
  children,
  panelRef: externalPanelRef,
  className,
  contentClassName,
  ...panelProps
}: CollapsibleResizablePanelProps) {
  const internalPanelRef = usePanelRef()
  const panelRef = externalPanelRef ?? internalPanelRef
  const stableContentMinWidth = typeof panelProps.minSize === 'number' ? `${panelProps.minSize}px` : panelProps.minSize

  useEffect(() => {
    const panel = panelRef.current
    if (!panel) return
    if (visible) panel.expand()
    else panel.collapse()
  }, [panelRef, visible])

  return (
    <Panel
      {...panelProps}
      panelRef={panelRef}
      collapsedSize="0px"
      collapsible
      disabled={!visible}
      aria-hidden={!visible}
      inert={!visible}
      data-nova-panel-motion="resizable"
      data-nova-panel-side={side}
      data-state={visible ? 'open' : 'closed'}
      className={cn('min-h-0 overflow-hidden', className)}
    >
      <div
        data-nova-panel-motion-content="true"
        data-nova-panel-side={side}
        data-state={visible ? 'open' : 'closed'}
        className={cn('h-full min-h-0', contentClassName)}
        style={{ minWidth: stableContentMinWidth }}
      >
        {children}
      </div>
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
  className?: string
}

/**
 * A fixed-size inline pane for flex/grid layouts that need the same motion as resizable panels.
 * The inner width remains stable so text and trees do not reflow during the short transition.
 */
export function InlineCollapsiblePane({ visible, side, size, children, className }: InlineCollapsiblePaneProps) {
  return (
    <div
      data-nova-panel-motion="inline"
      data-nova-panel-side={side}
      data-state={visible ? 'open' : 'closed'}
      aria-hidden={!visible}
      inert={!visible}
      className={cn('min-h-0 min-w-0 shrink-0 overflow-hidden', className)}
      style={{ '--nova-inline-panel-size': size } as CSSProperties}
    >
      <div
        data-nova-panel-motion-content="true"
        data-nova-panel-side={side}
        data-state={visible ? 'open' : 'closed'}
        className="h-full min-h-0"
      >
        {children}
      </div>
    </div>
  )
}
