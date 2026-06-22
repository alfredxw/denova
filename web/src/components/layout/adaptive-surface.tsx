import type { ReactNode } from 'react'
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
}: AdaptiveSurfaceProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const panes = [left, right].filter((pane): pane is AdaptiveSurfacePane => Boolean(pane && pane.enabled !== false))

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
    return (
      <MobilePaneHost panes={mobilePanes} closeLabel={t('common.close')} className={`relative h-full min-h-0 ${className}`}>
        {(controls) => <div data-nova-adaptive-main="true" className={`flex h-full min-h-0 min-w-0 flex-col ${mainClassName}`}>{renderChildren(controls)}</div>}
      </MobilePaneHost>
    )
  }

  const desktopControls: AdaptiveSurfaceControls = {
    ...closedControls,
    isMobile: false,
    openLeft: () => {},
    openRight: () => {},
  }
  const gridClassName = desktopGridClassName || defaultDesktopGridClassName(Boolean(left && left.enabled !== false), Boolean(right && right.enabled !== false))

  return (
    <div className={`grid h-full min-h-0 ${className} ${gridClassName}`}>
      {left && left.enabled !== false ? <div className={left.desktopClassName}>{left.content}</div> : null}
      <div data-nova-adaptive-main="true" className={`flex h-full min-h-0 min-w-0 flex-col ${mainClassName}`}>{renderChildren(desktopControls)}</div>
      {right && right.enabled !== false ? <div className={right.desktopClassName}>{right.content}</div> : null}
    </div>
  )
}

function defaultDesktopGridClassName(hasLeft: boolean, hasRight: boolean) {
  if (hasLeft && hasRight) return 'grid-cols-[18rem_minmax(0,1fr)_minmax(320px,28rem)]'
  if (hasLeft) return 'grid-cols-[18rem_minmax(0,1fr)]'
  if (hasRight) return 'grid-cols-[minmax(0,1fr)_minmax(320px,28rem)]'
  return 'grid-cols-[minmax(0,1fr)]'
}
