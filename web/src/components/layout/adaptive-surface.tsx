import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Group, Panel, Separator, type Layout } from 'react-resizable-panels'
import { useTranslation } from 'react-i18next'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost, type MobilePane, type MobilePaneControls } from './mobile-pane-host'
import { createStablePortalHost, StablePortalSlot } from './stable-portal-slot'

export interface AdaptiveSurfacePane {
  id: string
  title: string
  side: 'left' | 'right'
  content: ReactNode
  icon?: ReactNode
  enabled?: boolean
  desktopClassName?: string
  mobileClassName?: string
  onOpen?: () => void
  onClose?: () => void
}

interface AdaptiveSurfaceControls extends MobilePaneControls {
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
  /** Turns the desktop main/right split into an accessible, persisted resize group. */
  rightResize?: AdaptiveSurfaceRightResize
  /** Collapse side panes into drawers when this surface is narrower than the given pixel width. */
  collapseAt?: number
}

export interface AdaptiveSurfaceRightResize {
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
  rightResize,
  collapseAt,
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

  useEffect(() => {
    if (!isMobile) setMobileOpenPaneId(null)
  }, [isMobile])

  const renderChildren = (controls: MobilePaneControls): ReactNode => {
    const nextControls: AdaptiveSurfaceControls = {
      ...controls,
      isMobile,
      openLeft: () => {
        const pane = panes.find((item) => item.side === 'left')
        if (pane) controls.openPane(pane.id)
      },
      openRight: () => {
        const pane = panes.find((item) => item.side === 'right')
        if (pane) controls.openPane(pane.id)
      },
    }
    return typeof children === 'function' ? children(nextControls) : children
  }

  const mobileControls: MobilePaneControls = {
    openPaneId: mobileOpenPaneId,
    openPane: (id) => mobileControlsRef.current.openPane(id),
    closePane: () => mobileControlsRef.current.closePane(),
    togglePane: (id) => mobileControlsRef.current.togglePane(id),
  }
  const mainContent = renderChildren(isMobile ? mobileControls : closedControls)
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
    if (desktopRight && rightResize) {
      surface = (
        <div className={`flex h-full min-h-0 min-w-0 ${className}`} data-nova-adaptive-resizable="true">
          {desktopLeft ? <div className={desktopLeft.desktopClassName}>{desktopLeft.content}</div> : null}
          <Group
            id={rightResize.layoutKey}
            orientation="horizontal"
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            defaultLayout={readStoredLayout(rightResize.layoutKey)}
            onLayoutChanged={(layout) => storeLayout(rightResize.layoutKey, layout)}
            className="min-h-0 min-w-0 flex-1"
          >
            <Panel id="main" minSize={rightResize.mainMinSize ?? '240px'} className="min-w-0">
              {mainContentSlot}
            </Panel>
            <Separator
              aria-label={rightResize.label}
              className="nova-resize-handle relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none bg-transparent transition-colors focus-visible:bg-[var(--nova-active)] focus-visible:outline-none"
            />
            <Panel
              id="right"
              defaultSize={rightResize.defaultSize ?? '420px'}
              minSize={rightResize.minSize ?? '300px'}
              maxSize={rightResize.maxSize ?? '65%'}
              groupResizeBehavior="preserve-pixel-size"
              className="min-w-0"
            >
              <div className={`h-full min-h-0 ${desktopRight.desktopClassName ?? ''}`}>{desktopRight.content}</div>
            </Panel>
          </Group>
        </div>
      )
    } else {
      const gridClassName = desktopGridClassName || defaultDesktopGridClassName(Boolean(desktopLeft), Boolean(desktopRight))
      surface = (
        <div className={`grid h-full min-h-0 ${className} ${gridClassName}`}>
          {desktopLeft ? <div className={desktopLeft.desktopClassName}>{desktopLeft.content}</div> : null}
          {mainContentSlot}
          {desktopRight ? <div className={desktopRight.desktopClassName}>{desktopRight.content}</div> : null}
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

function readStoredLayout(key: string): Layout | undefined {
  if (typeof window === 'undefined') return undefined
  try {
    const value = window.localStorage.getItem(key)
    return value ? JSON.parse(value) as Layout : undefined
  } catch {
    return undefined
  }
}

function storeLayout(key: string, layout: Layout) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify(layout))
  } catch {
    // Private browsing and storage policies may disable persistence; resizing still works.
  }
}
