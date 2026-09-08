import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronRight } from 'lucide-react'
import { MobilePaneHost, type MobilePane } from './mobile-pane-host'
import { closeMobilePanes, MOBILE_NAVIGATION_OPEN_EVENT, MOBILE_PROJECT_OPEN_EVENT } from './mobile-pane-events'
import { useMobileViewport } from '@/hooks/useMobileViewport'
import { Button } from '@/components/ui/button'

export { MOBILE_NAVIGATION_OPEN_EVENT, MOBILE_PROJECT_OPEN_EVENT } from './mobile-pane-events'

export interface MobileNavItem {
  id: string
  label: string
  icon: ReactNode
  active?: boolean
  disabled?: boolean
  onClick: () => void
}

interface WorkspaceMobileLayoutProps {
  topBar: ReactNode
  navigationTools: ReactNode
  main: ReactNode
  activityItems: MobileNavItem[]
  settingsItem: MobileNavItem
  projectDrawer?: MobilePane
  closeLabel: string
  navigationLabel: string
}

export function WorkspaceMobileLayout({ topBar, navigationTools, main, activityItems, settingsItem, projectDrawer, closeLabel, navigationLabel }: WorkspaceMobileLayoutProps) {
  const { t } = useTranslation()
  useMobileViewport()
  const [openPaneId, setOpenPaneId] = useState<string | null>(null)
  useEffect(() => {
    const openNavigation = () => { closeMobilePanes(); setOpenPaneId('navigation') }
    const openProject = () => { closeMobilePanes(); setOpenPaneId('project') }
    window.addEventListener(MOBILE_NAVIGATION_OPEN_EVENT, openNavigation)
    window.addEventListener(MOBILE_PROJECT_OPEN_EVENT, openProject)
    return () => {
      window.removeEventListener(MOBILE_NAVIGATION_OPEN_EVENT, openNavigation)
      window.removeEventListener(MOBILE_PROJECT_OPEN_EVENT, openProject)
    }
  }, [])
  // Group destinations for scanning without introducing another navigation state.
  const navigationGroups = [
    { id: 'creation', title: t('workbench.mobile.creation'), items: activityItems.filter((item) => item.id === 'writing' || item.id === 'story') },
    { id: 'tools', title: t('workbench.mobile.tools'), items: activityItems.filter((item) => item.id !== 'writing' && item.id !== 'story') },
    { id: 'settings', title: undefined, items: [settingsItem] },
  ]
  const navigation: MobilePane = {
    id: 'navigation', title: t('workbench.mobile.navigationMenu'), side: 'left', swipeToOpen: true,
    content: (
      <div className="nova-mobile-navigation flex h-full min-h-0 flex-col overflow-y-auto overscroll-contain">
        <div className="shrink-0 px-4 pt-2">{navigationTools}</div>
        <nav className="flex flex-col gap-5 p-4" aria-label={navigationLabel}>
          {navigationGroups.map((group) => (
            <section key={group.id} data-mobile-nav-group={group.id} aria-label={group.title}>
              {group.title && <h3 className="nova-mobile-section-label">{group.title}</h3>}
              <div className="nova-mobile-nav-group">
                {group.items.map((item) => (
                  <Button key={item.id} type="button" variant="ghost" className="nova-mobile-nav-row" disabled={item.disabled} aria-label={item.label} aria-current={item.active ? 'page' : undefined} onClick={() => { setOpenPaneId(null); item.onClick() }}>
                    <span className="nova-mobile-nav-icon" aria-hidden="true">{item.icon}</span>
                    <span className="min-w-0 flex-1">{item.label}</span>
                    {item.active ? <Check data-icon="inline-end" aria-hidden="true" /> : <ChevronRight data-icon="inline-end" aria-hidden="true" />}
                  </Button>
                ))}
              </div>
            </section>
          ))}
        </nav>
      </div>
    ),
  }
  return (
    <MobilePaneHost panes={[navigation, ...(projectDrawer ? [projectDrawer] : [])]} closeLabel={closeLabel} openPaneId={openPaneId} onOpenPaneChange={setOpenPaneId} className="h-full min-h-0">
      <div data-nova-app-shell="true" data-nova-mobile-shell="true" className="fixed inset-x-0 flex w-full min-w-0 flex-col overflow-hidden bg-background text-foreground">
        {topBar}
        <div className="min-h-0 flex-1 overflow-hidden">{main}</div>
      </div>
    </MobilePaneHost>
  )
}
