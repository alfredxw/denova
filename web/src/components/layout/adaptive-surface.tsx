import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Group, Panel } from 'react-resizable-panels'
import { useTranslation } from 'react-i18next'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost, type MobilePane, type MobilePaneControls } from './mobile-pane-host'
import { CollapsiblePanelSeparator, CollapsibleResizablePanel, InlineCollapsiblePane } from './panel-motion'
import { createStablePortalHost, StablePortalSlot } from './stable-portal-slot'
import { usePersistedPanelLayout } from './use-persisted-panel-layout'

export interface AdaptiveSurfacePane {
  id: string
  title: string
  side: 'left' | 'right'
  content: ReactNode
  icon?: ReactNode
  enabled?: boolean
  desktopClassName?: string
  mobileClassName?: string
  /** Keeps a desktop pane mounted but contracts it to zero or its compact size when false. */
  desktopVisible?: boolean
  /** Stable inline size used when a non-resizable desktop pane can be toggled. */
  desktopSize?: string
  /** Optional compact desktop state that replaces a hidden pane. */
  desktopCollapsedSize?: string
  desktopCollapsedContent?: ReactNode
  onOpen?: () => void
  onClose?: () => void
}

export interface AdaptiveSurfaceControls extends MobilePaneControls {
  isMobile: boolean
  openLeft: () => void
  openRight: () => void
}

interface AdaptiveSurfaceProps {
  left?: AdaptiveSurfacePane
  right?: AdaptiveSurfacePane
  children: ReactNode | ((controls: AdaptiveSurfaceControls) => ReactNode)
  className?: string
  mainClassName?: string
  desktopGridClassName?: string
  /** Turns the desktop left/main split into an accessible, persisted resize group. */
  leftResize?: AdaptiveSurfaceSideResize
  /** Turns the desktop main/right split into an accessible, persisted resize group. */
  rightResize?: AdaptiveSurfaceSideResize
  /** Collapse side panes into drawers when this surface is narrower than the given pixel width. */
  collapseAt?: number
  /** Whether collapsed panes cover the viewport or stay inside this surface. */
  mobilePaneScope?: 'viewport' | 'surface'
}

export interface AdaptiveSurfaceSideResize {
  layoutKey: string
  label: string
  defaultSize?: string
  minSize?: string
  maxSize?: string
  mainMinSize?: string
}

const closedControls: MobilePaneControls = {
  openPaneId: null,
  openPane: () => {},
  closePane: () => {},
  togglePane: () => {},
}

export function AdaptiveSurface({
  left,
  right,
  children,
  className = 'h-full min-h-0',
  mainClassName = 'min-h-0 min-w-0',
  desktopGridClassName,
  leftResize,
  rightResize,
  collapseAt,
  mobilePaneScope = 'viewport',
}: AdaptiveSurfaceProps) {
  const { t } = useTranslation()
  const viewportMobile = useIsMobile()
  const collapseWidth = normalizeCollapseWidth(collapseAt)
  const { containerRef, widthCollapsed } = useWidthCollapse(collapseWidth)
  const isMobile = viewportMobile || widthCollapsed
  const panes = [left, right].filter((pane): pane is AdaptiveSurfacePane => Boolean(pane && pane.enabled !== false))
  const [mainContentHost] = useState(() => createStablePortalHost('flex h-full min-h-0 w-full min-w-0 flex-col'))
  const [mobileOpenPaneId, setMobileOpenPaneId] = useState<string | null>(null)
  const mobileControlsRef = useRef<MobilePaneControls>(closedControls)
  const retainedLeftPaneRef = useRef<AdaptiveSurfacePane | null>(null)
  const retainedRightPaneRef = useRef<AdaptiveSurfacePane | null>(null)
  if (left && left.enabled !== false) retainedLeftPaneRef.current = left
  if (right && right.enabled !== false) retainedRightPaneRef.current = right
  const leftPanelLayout = usePersistedPanelLayout({
    storageKey: leftResize?.layoutKey,
    panelIds: ['left', 'main'],
  })
  const rightPanelLayout = usePersistedPanelLayout({
    storageKey: rightResize?.layoutKey,
    panelIds: ['main', 'right'],
  })

  useEffect(() => {
    if (!isMobile) setMobileOpenPaneId(null)
  }, [isMobile])

  const openPane = useCallback((id: string) => mobileControlsRef.current.openPane(id), [])
  const closePane = useCallback(() => mobileControlsRef.current.closePane(), [])
  const togglePane = useCallback((id: string) => mobileControlsRef.current.togglePane(id), [])
  const leftPaneId = left && left.enabled !== false ? left.id : null
  const rightPaneId = right && right.enabled !== false ? right.id : null
  const openLeft = useCallback(() => {
    if (isMobile && leftPaneId) openPane(leftPaneId)
  }, [isMobile, leftPaneId, openPane])
  const openRight = useCallback(() => {
    if (isMobile && rightPaneId) openPane(rightPaneId)
  }, [isMobile, openPane, rightPaneId])
  const adaptiveControls = useMemo<AdaptiveSurfaceControls>(() => ({
    openPaneId: mobileOpenPaneId,
    openPane,
    closePane,
    togglePane,
    isMobile,
    openLeft,
    openRight,
  }), [closePane, isMobile, mobileOpenPaneId, openLeft, openPane, openRight, togglePane])
  // A pane toggle must not rebuild a heavy editor/chat subtree. Call render-prop children only
  // when their own identity or adaptive controls change, then move that stable result by portal.
  const mainContent = useMemo(
    () => typeof children === 'function' ? children(adaptiveControls) : children,
    [adaptiveControls, children],
  )
  const mainContentPortal = mainContentHost ? createPortal(mainContent, mainContentHost, 'adaptive-main-content') : null
  const mainContentSlot = (
    <StablePortalSlot
      host={mainContentHost}
      fallback={mainContent}
      data-nova-adaptive-main="true"
      className={`flex h-full min-h-0 min-w-0 flex-col ${mainClassName}`}
    />
  )

  let surface: ReactNode
  if (isMobile) {
    const mobilePanes: MobilePane[] = panes.map((pane) => ({
      id: pane.id,
      title: pane.title,
      side: pane.side,
      icon: pane.icon,
      content: pane.content,
      onOpen: pane.onOpen,
      onClose: pane.onClose,
      className: pane.mobileClassName,
    }))
    surface = (
      <MobilePaneHost
        panes={mobilePanes}
        closeLabel={t('common.close')}
        className={`relative h-full min-h-0 ${className}`}
        openPaneId={mobileOpenPaneId}
        onOpenPaneChange={setMobileOpenPaneId}
        paneScope={mobilePaneScope}
      >
        {(controls) => (
          <AdaptiveMobileMainSlot controls={controls} controlsRef={mobileControlsRef}>
            {mainContentSlot}
          </AdaptiveMobileMainSlot>
        )}
      </MobilePaneHost>
    )
  } else {
    const desktopLeft = left && left.enabled !== false ? left : null
    const desktopRight = right && right.enabled !== false ? right : null
    const retainedDesktopLeft = retainedLeftPaneRef.current
    const retainedDesktopRight = retainedRightPaneRef.current
    const desktopLeftVisible = Boolean(desktopLeft && desktopLeft.desktopVisible !== false)
    const desktopRightVisible = Boolean(desktopRight && desktopRight.desktopVisible !== false)

    const rightResizeSurface = rightResize ? (
      <Group
        id={rightResize.layoutKey}
        data-nova-panel-motion-group="true"
        orientation="horizontal"
        resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
        defaultLayout={rightPanelLayout.defaultLayout}
        onLayoutChanged={(layout) => {
          if (desktopRightVisible) rightPanelLayout.persistUserLayout(layout)
        }}
        className="h-full min-h-0 min-w-0"
      >
        <Panel id="main" minSize={rightResize.mainMinSize ?? '240px'} className="min-w-0">
          {mainContentSlot}
        </Panel>
        <CollapsiblePanelSeparator
          visible={desktopRightVisible}
          aria-label={rightResize.label}
          className="nova-resize-handle nova-resize-divider nova-resize-divider-vertical relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none"
          {...rightPanelLayout.resizeHandleIntentProps}
        />
        <CollapsibleResizablePanel
          id="right"
          visible={desktopRightVisible}
          side="right"
          defaultSize={rightResize.defaultSize ?? '420px'}
          minSize={rightResize.minSize ?? '300px'}
          maxSize={rightResize.maxSize ?? '65%'}
          groupResizeBehavior="preserve-pixel-size"
          collapsedSize={retainedDesktopRight?.desktopCollapsedSize}
          collapsedChildren={retainedDesktopRight?.desktopCollapsedContent}
          className={`min-w-0 ${retainedDesktopRight?.desktopClassName ?? ''}`}
        >
          {retainedDesktopRight?.content ?? null}
        </CollapsibleResizablePanel>
      </Group>
    ) : null

    if (leftResize && retainedDesktopLeft) {
      const mainAndRight = rightResizeSurface ?? (
        <div className={`grid h-full min-h-0 min-w-0 ${defaultDesktopGridClassName(false, Boolean(desktopRight))}`}>
          {mainContentSlot}
          {desktopRight ? <AdaptiveDesktopPane pane={desktopRight} /> : null}
        </div>
      )
      surface = (
        <div className={`flex h-full min-h-0 min-w-0 ${className}`} data-nova-adaptive-resizable="left">
          <Group
            id={leftResize.layoutKey}
            data-nova-panel-motion-group="true"
            orientation="horizontal"
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            defaultLayout={leftPanelLayout.defaultLayout}
            onLayoutChanged={(layout) => {
              if (desktopLeftVisible) leftPanelLayout.persistUserLayout(layout)
            }}
            className="h-full min-h-0 min-w-0 flex-1"
          >
            <CollapsibleResizablePanel
              id="left"
              visible={desktopLeftVisible}
              side="left"
              defaultSize={leftResize.defaultSize ?? '288px'}
              minSize={leftResize.minSize ?? '200px'}
              maxSize={leftResize.maxSize ?? '40%'}
              groupResizeBehavior="preserve-pixel-size"
              collapsedSize={retainedDesktopLeft.desktopCollapsedSize}
              collapsedChildren={retainedDesktopLeft.desktopCollapsedContent}
              className={`min-w-0 ${retainedDesktopLeft.desktopClassName ?? ''}`}
            >
              {retainedDesktopLeft.content}
            </CollapsibleResizablePanel>
            <CollapsiblePanelSeparator
              visible={desktopLeftVisible}
              aria-label={leftResize.label}
              className="nova-resize-handle nova-resize-divider nova-resize-divider-vertical relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none"
              {...leftPanelLayout.resizeHandleIntentProps}
            />
            <Panel id="main" minSize={leftResize.mainMinSize ?? '320px'} className="min-w-0">
              {mainAndRight}
            </Panel>
          </Group>
        </div>
      )
    } else if (rightResizeSurface) {
      surface = (
        <div className={`flex h-full min-h-0 min-w-0 ${className}`} data-nova-adaptive-resizable="right">
          {desktopLeft ? <AdaptiveDesktopPane pane={desktopLeft} /> : null}
          {rightResizeSurface}
        </div>
      )
    } else {
      const gridClassName = desktopGridClassName || defaultDesktopGridClassName(Boolean(desktopLeft), Boolean(desktopRight))
      surface = (
        <div className={`grid h-full min-h-0 ${className} ${gridClassName}`}>
          {desktopLeft ? <AdaptiveDesktopPane pane={desktopLeft} /> : null}
          {mainContentSlot}
          {desktopRight ? <AdaptiveDesktopPane pane={desktopRight} /> : null}
        </div>
      )
    }
  }

  const renderedSurface = collapseWidth === null ? surface : (
    <div ref={containerRef} data-nova-adaptive-container="true" className="h-full min-h-0 min-w-0 w-full">
      {surface}
    </div>
  )

  return (
    <>
      {renderedSurface}
      {mainContentPortal}
    </>
  )
}

function AdaptiveDesktopPane({ pane }: { pane: AdaptiveSurfacePane }) {
  const visible = pane.desktopVisible !== false
  if (pane.desktopSize) {
    return (
      <InlineCollapsiblePane
        visible={visible}
        side={pane.side}
        size={pane.desktopSize}
        collapsedSize={pane.desktopCollapsedSize}
        collapsedChildren={pane.desktopCollapsedContent}
        className={pane.desktopClassName}
      >
        {pane.content}
      </InlineCollapsiblePane>
    )
  }
  return (
    <div className={pane.desktopClassName} hidden={!visible} aria-hidden={!visible} inert={!visible}>
      {pane.content}
    </div>
  )
}

function AdaptiveMobileMainSlot({
  controls,
  controlsRef,
  children,
}: {
  controls: MobilePaneControls
  controlsRef: { current: MobilePaneControls }
  children: ReactNode
}) {
  useLayoutEffect(() => {
    controlsRef.current = controls
  }, [controls, controlsRef])

  return children
}

function normalizeCollapseWidth(value: number | undefined) {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

function useWidthCollapse(collapseWidth: number | null) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [widthCollapsed, setWidthCollapsed] = useState(false)

  useEffect(() => {
    if (collapseWidth === null) {
      setWidthCollapsed(false)
      return
    }

    const container = containerRef.current
    if (!container) return

    const update = (width: number) => {
      // Hidden surfaces report zero width. Keep their last layout until they become measurable.
      if (!Number.isFinite(width) || width <= 0) return
      setWidthCollapsed((current) => {
        const next = width < collapseWidth
        return current === next ? current : next
      })
    }

    update(container.getBoundingClientRect().width)
    if (typeof ResizeObserver === 'undefined') return

    const observer = new ResizeObserver((entries) => {
      const entry = entries.find((item) => item.target === container)
      if (entry) update(entry.contentRect.width)
    })
    observer.observe(container)
    return () => observer.disconnect()
  }, [collapseWidth])

  return { containerRef, widthCollapsed }
}

function defaultDesktopGridClassName(hasLeft: boolean, hasRight: boolean) {
  if (hasLeft && hasRight) return 'grid-cols-[18rem_minmax(0,1fr)_minmax(320px,28rem)]'
  if (hasLeft) return 'grid-cols-[18rem_minmax(0,1fr)]'
  if (hasRight) return 'grid-cols-[minmax(0,1fr)_minmax(320px,28rem)]'
  return 'grid-cols-[minmax(0,1fr)]'
}
