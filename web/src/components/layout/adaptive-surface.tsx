import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost, type MobilePane, type MobilePaneControls } from './mobile-pane-host'

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
  /** Collapse side panes into drawers when this surface is narrower than the given pixel width. */
  collapseAt?: number
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
  collapseAt,
}: AdaptiveSurfaceProps) {
  const { t } = useTranslation()
  const viewportMobile = useIsMobile()
  const collapseWidth = normalizeCollapseWidth(collapseAt)
  const { containerRef, widthCollapsed } = useWidthCollapse(collapseWidth)
  const isMobile = viewportMobile || widthCollapsed
  const panes = [left, right].filter((pane): pane is AdaptiveSurfacePane => Boolean(pane && pane.enabled !== false))
  const [mainContentHost] = useState(createAdaptiveMainHost)
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
  const mainContentSlot = <AdaptiveMainSlot host={mainContentHost} fallback={mainContent} className={mainClassName} />

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
    const gridClassName = desktopGridClassName || defaultDesktopGridClassName(Boolean(left && left.enabled !== false), Boolean(right && right.enabled !== false))

    surface = (
      <div className={`grid h-full min-h-0 ${className} ${gridClassName}`}>
        {left && left.enabled !== false ? <div className={left.desktopClassName}>{left.content}</div> : null}
        {mainContentSlot}
        {right && right.enabled !== false ? <div className={right.desktopClassName}>{right.content}</div> : null}
      </div>
    )
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

function createAdaptiveMainHost() {
  if (typeof document === 'undefined') return null
  const host = document.createElement('div')
  host.className = 'flex h-full min-h-0 w-full min-w-0 flex-col'
  return host
}

function AdaptiveMainSlot({ host, fallback, className }: { host: HTMLDivElement | null; fallback: ReactNode; className: string }) {
  const slotRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const slot = slotRef.current
    if (!host || !slot) return
    slot.appendChild(host)
    return () => {
      if (host.parentNode === slot) host.remove()
    }
  }, [host])

  const slotClassName = `flex h-full min-h-0 min-w-0 flex-col ${className}`
  if (!host) return <div data-nova-adaptive-main="true" className={slotClassName}>{fallback}</div>
  return <div ref={slotRef} data-nova-adaptive-main="true" className={slotClassName} />
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
