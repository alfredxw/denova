import { useEffect, useRef, useState, type ComponentProps, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { List } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsMobile } from '@/hooks/useIsMobile'
import { AdaptiveSurface } from './adaptive-surface'
import { MobileWorkspaceHeader } from './mobile-workspace-header'
import { MOBILE_PANES_CLOSE_EVENT } from './mobile-pane-events'
import { createStablePortalHost, StablePortalSlot } from './stable-portal-slot'

/** Phone view selection is independent of the desktop inspector's visibility. */
export function useResponsiveAgentOpen() {
  const isMobile = useIsMobile()
  const desktop = useState(false)
  const mobile = useState(false)
  return isMobile ? mobile : desktop
}

interface ResourceWorkspaceProps extends ComponentProps<typeof AdaptiveSurface> {
  title: string
  embedded?: boolean
  secondaryView?: {
    label: string
    /** Project tabs already select their destination group as part of navigation. */
    returnToContentOnSelection?: boolean
    available: boolean
    open: boolean
    onOpenChange: (open: boolean) => void
  }
  contentViews?: {
    value: string
    items: Array<{ value: string; label: string }>
    onValueChange: (value: string) => void
  }
}

/** Resource directories are transient; editors and Agent conversations are retained views.
 * Each secondary view has one portal host across phone views, drawers, and desktop splits. */
export function ResourceWorkspace({ title, embedded = false, secondaryView, contentViews, left, right, children, ...props }: ResourceWorkspaceProps) {
  const { t } = useTranslation()
  const isPhone = useIsMobile()
  const [secondaryHost] = useState(() => createStablePortalHost('flex h-full min-h-0 w-full min-w-0 flex-col'))
  const retainedSecondary = useRef<ReactNode>(null)
  if (right) retainedSecondary.current = right.content
  const secondaryContent = right?.content ?? retainedSecondary.current
  const directoryOpen = useRef(false)
  const contentValue = contentViews?.value ?? 'content'
  const view = isPhone && secondaryView?.available && secondaryView.open ? 'secondary' : contentValue
  const views = contentViews?.items ?? [{ value: 'content', label: t('workbench.mobile.content') }]

  useEffect(() => {
    const selected = () => {
      if (directoryOpen.current && secondaryView?.returnToContentOnSelection !== false) secondaryView?.onOpenChange(false)
    }
    window.addEventListener(MOBILE_PANES_CLOSE_EVENT, selected)
    return () => window.removeEventListener(MOBILE_PANES_CLOSE_EVENT, selected)
  }, [secondaryView?.onOpenChange, secondaryView?.returnToContentOnSelection])

  const secondarySlot = <StablePortalSlot host={secondaryHost} fallback={secondaryContent} className="h-full min-h-0" />
  const desktopSecondary = right ? { ...right, content: secondarySlot } : undefined
  return (
    <Tabs
      value={view}
      onValueChange={(value) => {
        secondaryView?.onOpenChange(value === 'secondary')
        if (value !== 'secondary') contentViews?.onValueChange(value)
      }}
      className="flex h-full min-h-0 min-w-0 flex-1 flex-col gap-0"
      data-mobile-resource-workspace={isPhone ? 'true' : undefined}
    >
      <AdaptiveSurface {...props} left={left && { ...left, side: isPhone ? 'right' : left.side }} right={isPhone ? undefined : desktopSecondary}>
        {(controls) => {
          directoryOpen.current = controls.openPaneId === left?.id
          const header = (
            <>
              {secondaryView?.available || views.length > 1 ? (
                <div className="flex min-w-0 flex-1 justify-center">
                  <TabsList className="nova-mobile-view-tabs" aria-label={t('workbench.mobile.pageViews', { page: title })}>
                    {views.map((item) => <TabsTrigger key={item.value} value={item.value} className="min-w-0 px-3">{item.label}</TabsTrigger>)}
                    {secondaryView?.available && <TabsTrigger value="secondary" className="min-w-0 px-3">{secondaryView?.label}</TabsTrigger>}
                  </TabsList>
                </div>
              ) : <h2 className="nova-mobile-page-title min-w-0 flex-1 truncate text-center">{title}</h2>}
              {left && (
                <Button variant="ghost" size="icon" aria-label={t('workbench.mobile.directory', { label: left.title })} aria-haspopup="dialog" aria-expanded={controls.openPaneId === left.id} onClick={controls.openLeft}>
                  <List />
                </Button>
              )}
            </>
          )
          return (
            <>
              {isPhone && (embedded ? <div className="flex h-12 shrink-0 items-center gap-1 border-b px-2">{header}</div> : <MobileWorkspaceHeader route="full">{header}</MobileWorkspaceHeader>)}
              {!isPhone && controls.isMobile && (
                <div className="flex shrink-0 items-center gap-2 border-b px-3 py-1">
                  {left && <Button variant="ghost" size="sm" onClick={controls.openLeft}><List />{left.title}</Button>}
                  {right && <Button variant="ghost" size="sm" className="ml-auto" onClick={controls.openRight}>{right.icon}{right.title}</Button>}
                </div>
              )}
              <TabsContent value={contentValue} forceMount inert={view === 'secondary'} className="mt-0 min-h-0 flex-1 data-[state=inactive]:hidden">
                {typeof children === 'function' ? children(isPhone && secondaryView ? {
                  ...controls,
                  openPaneId: controls.openPaneId ?? (view === 'secondary' ? right?.id ?? null : null),
                  openRight: () => secondaryView.onOpenChange(true),
                  closePane: () => { controls.closePane(); secondaryView.onOpenChange(false) },
                } : controls) : children}
              </TabsContent>
              {isPhone && <TabsContent value="secondary" forceMount inert={view !== 'secondary'} className="mt-0 min-h-0 flex-1 data-[state=inactive]:hidden">{secondarySlot}</TabsContent>}
            </>
          )
        }}
      </AdaptiveSurface>
      {secondaryHost && secondaryContent ? createPortal(secondaryContent, secondaryHost, 'resource-secondary') : null}
    </Tabs>
  )
}
