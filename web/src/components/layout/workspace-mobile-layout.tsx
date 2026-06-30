import { useEffect, useState, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { MobilePaneHost, type MobilePane } from './mobile-pane-host'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'

export const MOBILE_NAVIGATION_OPEN_EVENT = 'nova:mobile-navigation-open'

export interface MobileNavItem {
  id: string
  label: string
  icon: ReactNode
  active?: boolean
  expanded?: boolean
  disabled?: boolean
  onClick: () => void
}

interface MobileDrawer {
  id: 'project' | 'agent'
  title: string
  icon: ReactNode
  side: 'left' | 'right'
  content: ReactNode
  onOpen?: () => void
  onClose?: () => void
}

interface WorkspaceMobileLayoutProps {
  topBar: ReactNode
  main: ReactNode
  activityItems: MobileNavItem[]
  settingsItem: MobileNavItem
  projectDrawer?: MobileDrawer
  agentDrawer?: MobileDrawer
  closeLabel: string
  navigationLabel: string
  compactNavigation?: boolean
  compactNavigationLabel?: string
}

export function WorkspaceMobileLayout({
  topBar,
  main,
  activityItems,
  settingsItem,
  projectDrawer,
  agentDrawer,
  closeLabel,
  navigationLabel,
  compactNavigation = false,
  compactNavigationLabel,
}: WorkspaceMobileLayoutProps) {
  const [navigationOpen, setNavigationOpen] = useState(false)
  const drawers = [projectDrawer, agentDrawer].filter((drawer): drawer is MobileDrawer => Boolean(drawer)).map((drawer) => ({
    ...drawer,
    className: drawer.side === 'left' ? 'w-[min(90vw,390px)]' : 'w-[min(90vw,390px)]',
  })) as MobilePane[]

  useEffect(() => {
    if (!compactNavigation) return
    const openNavigation = () => setNavigationOpen(true)
    window.addEventListener(MOBILE_NAVIGATION_OPEN_EVENT, openNavigation)
    return () => window.removeEventListener(MOBILE_NAVIGATION_OPEN_EVENT, openNavigation)
  }, [compactNavigation])

  useEffect(() => {
    if (!compactNavigation) setNavigationOpen(false)
  }, [compactNavigation])

  return (
    <MobilePaneHost panes={drawers} closeLabel={closeLabel} className="h-dvh w-screen overflow-hidden">
      {({ openPaneId, closePane, togglePane }) => {
        const runNavAction = (action: () => void) => {
          closePane()
          setNavigationOpen(false)
          action()
        }
        const navigationItems: MobileNavItem[] = [
          ...(projectDrawer ? [{
            id: projectDrawer.id,
            label: projectDrawer.title,
            icon: projectDrawer.icon,
            expanded: openPaneId === projectDrawer.id,
            onClick: () => {
              setNavigationOpen(false)
              togglePane(projectDrawer.id)
            },
          }] : []),
          ...activityItems.map((item) => ({ ...item, onClick: () => runNavAction(item.onClick) })),
          ...(agentDrawer ? [{
            id: agentDrawer.id,
            label: agentDrawer.title,
            icon: agentDrawer.icon,
            expanded: openPaneId === agentDrawer.id,
            onClick: () => {
              setNavigationOpen(false)
              togglePane(agentDrawer.id)
            },
          }] : []),
          { ...settingsItem, onClick: () => runNavAction(settingsItem.onClick) },
        ]
        const navigationSheet = (
          <Sheet open={navigationOpen} onOpenChange={setNavigationOpen}>
            <SheetContent side="bottom" showCloseButton={false} aria-describedby={undefined} className="max-h-[70dvh] gap-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
              <div className="nova-topbar flex h-11 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3">
                <SheetTitle className="text-xs font-semibold text-[var(--nova-text)]">{compactNavigationLabel || navigationLabel}</SheetTitle>
                <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={closeLabel} onClick={() => setNavigationOpen(false)}>
                  <X className="h-4 w-4" />
                </button>
              </div>
              <div className="grid max-h-[calc(70dvh-2.75rem)] grid-cols-3 gap-2 overflow-y-auto p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
                {navigationItems.map((item) => (
                  <MobileNavButton key={item.id} item={item} />
                ))}
              </div>
            </SheetContent>
          </Sheet>
        )
        return (
          <div data-nova-app-shell="true" data-nova-mobile-shell="true" className="relative flex h-dvh w-screen flex-col overflow-hidden bg-[var(--nova-bg)] text-[var(--nova-text)]">
            {topBar}
            <div className="min-h-0 flex-1 overflow-hidden">
              {main}
            </div>
            {compactNavigation ? (
              navigationSheet
            ) : (
              <nav className="nova-mobile-nav flex shrink-0 items-stretch gap-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-0.5 py-1.5" aria-label={navigationLabel}>
                {projectDrawer ? <MobileNavButton item={navigationItems[0]} /> : null}
                {activityItems.map((item) => (
                  <MobileNavButton key={item.id} item={{ ...item, onClick: () => runNavAction(item.onClick) }} />
                ))}
                {agentDrawer ? <MobileNavButton item={navigationItems[projectDrawer ? activityItems.length + 1 : activityItems.length]} /> : null}
                <MobileNavButton item={{ ...settingsItem, onClick: () => runNavAction(settingsItem.onClick) }} />
              </nav>
            )}
          </div>
        )
      }}
    </MobilePaneHost>
  )
}

function MobileNavButton({ item }: { item: MobileNavItem }) {
  return (
    <button
      type="button"
      className={`nova-mobile-nav-item flex min-h-[48px] flex-1 flex-col items-center justify-center gap-0.5 rounded-[var(--nova-radius)] px-1 text-[10px] text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:opacity-45 ${item.active ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : ''} ${item.expanded && !item.active ? 'is-expanded border border-[var(--nova-border)] text-[var(--nova-text-muted)]' : ''}`}
      disabled={item.disabled}
      aria-label={item.label}
      title={item.label}
      aria-current={item.active ? 'page' : undefined}
      aria-expanded={item.expanded || undefined}
      onClick={item.onClick}
    >
      <span className="flex h-5 w-5 items-center justify-center">{item.icon}</span>
      <span className="max-w-full truncate max-md:hidden">{item.label}</span>
    </button>
  )
}
