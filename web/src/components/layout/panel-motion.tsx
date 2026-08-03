import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from 'react'
import {
  Group,
  Panel,
  Separator,
  usePanelRef,
  type GroupProps,
  type PanelImperativeHandle,
  type PanelProps,
  type SeparatorProps,
} from 'react-resizable-panels'
import { cn } from '@/lib/utils'

type PanelSide = 'left' | 'right'

/**
 * A panel group whose resize state is explicit rather than derived with CSS `:has()`.
 *
 * Monaco replaces visible line nodes while scrolling. A relational selector on an ancestor panel
 * group makes each of those mutations invalidate styles across the whole group, so resize state is
 * captured once at the interaction boundary instead.
 */
export function PanelMotionGroup({ onPointerDownCapture, ...groupProps }: GroupProps) {
  const activeGroupRef = useRef<HTMLDivElement | null>(null)
  const pointerCleanupRef = useRef<(() => void) | null>(null)
  const clearResizeState = useCallback(() => {
    activeGroupRef.current?.removeAttribute('data-nova-panel-resizing')
    activeGroupRef.current = null
    pointerCleanupRef.current?.()
    pointerCleanupRef.current = null
  }, [])

  const handlePointerDownCapture = useCallback<NonNullable<GroupProps['onPointerDownCapture']>>((event) => {
    onPointerDownCapture?.(event)
    if (!(event.target instanceof Element)) return

    const separator = event.target.closest<HTMLElement>('[data-separator]')
    // Nested panel groups receive the same captured event. Only the separator's direct group owns
    // this resize interaction and should suspend its panel transitions.
    if (!separator || separator.parentElement !== event.currentTarget || separator.ariaDisabled === 'true') return

    clearResizeState()
    activeGroupRef.current = event.currentTarget
    // react-resizable-panels intentionally prevents the native pointer default from a document
    // capture listener once it has claimed a resize. That is a positive resize signal here, not a
    // cancellation signal, so the group still needs to disable its flex transition.
    // This interaction-only flag must take effect before the first pointer move. Updating it
    // imperatively avoids both a React commit delay and a re-render of the potentially heavy panel
    // subtree merely to disable its flex transition during a drag.
    event.currentTarget.setAttribute('data-nova-panel-resizing', 'true')

    const finishResize = () => {
      // react-resizable-panels finalizes its layout from a document capture listener. Defer cleanup
      // until the current event has fully dispatched so that final flex update also stays unanimated.
      queueMicrotask(clearResizeState)
    }
    const finishResizeOnBlur = () => clearResizeState()
    window.addEventListener('pointerup', finishResize, true)
    window.addEventListener('pointercancel', finishResize, true)
    window.addEventListener('blur', finishResizeOnBlur)
    pointerCleanupRef.current = () => {
      window.removeEventListener('pointerup', finishResize, true)
      window.removeEventListener('pointercancel', finishResize, true)
      window.removeEventListener('blur', finishResizeOnBlur)
    }
  }, [clearResizeState, onPointerDownCapture])

  useEffect(() => {
    return clearResizeState
  }, [clearResizeState])

  return (
    <Group
      {...groupProps}
      data-nova-panel-motion-group="true"
      onPointerDownCapture={handlePointerDownCapture}
    />
  )
}

interface CollapsibleResizablePanelProps extends Omit<PanelProps, 'children' | 'collapsedSize' | 'collapsible' | 'disabled' | 'panelRef'> {
  visible: boolean
  side: PanelSide
  children: ReactNode
  /** Explicit first-open or layout-context restore size for panes that may mount collapsed. */
  initialExpandSize?: number | string
  /** Resets the cached visible size when this panel starts representing another layout context. */
  restorationKey?: string
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
  initialExpandSize,
  restorationKey,
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
  const hasBeenVisibleRef = useRef(visible)
  const lastVisibleSizeRef = useRef<number | null>(null)
  const restorationKeyRef = useRef(restorationKey)
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
    const restorationContextChanged = restorationKeyRef.current !== restorationKey
    if (restorationContextChanged) {
      restorationKeyRef.current = restorationKey
      hasBeenVisibleRef.current = false
      lastVisibleSizeRef.current = null

      // A visible panel can switch between independently persisted layout contexts without
      // collapsing first. Apply the new context explicitly; the Group's in-memory cache is keyed
      // only by panel ids and would otherwise retain the previous context's width.
      if (visible && !programmaticCollapseEnabled) {
        if (initialExpandSize !== null && initialExpandSize !== undefined) panel.resize(initialExpandSize)
        hasBeenVisibleRef.current = true
        return
      }
    }
    if (visible) {
      if (!programmaticCollapseEnabled) {
        hasBeenVisibleRef.current = true
        return
      }
      // Keep the library's collapsible mode enabled for this layout pass so expand() can restore
      // the last user width. It is disabled immediately afterward, preventing drag-to-collapse.
      const firstOpen = !hasBeenVisibleRef.current
      panel.expand()
      // Changing the library's collapsible constraint re-registers the panel and can discard its
      // internal expanded-size cache. Keep our own pixel snapshot so mode switches restore the
      // exact visible width. A pane mounted hidden has no such snapshot, so its owner provides
      // either the persisted percentage or the configured default as an explicit target.
      const restoreSize = firstOpen ? initialExpandSize : lastVisibleSizeRef.current
      if (restoreSize !== null && restoreSize !== undefined) panel.resize(restoreSize)
      hasBeenVisibleRef.current = true
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
  }, [initialExpandSize, panelRef, programmaticCollapseEnabled, restorationKey, visible])

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
