import { createContext, useContext, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Menu } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { createStablePortalHost, StablePortalSlot } from './stable-portal-slot'
import { MOBILE_NAVIGATION_OPEN_EVENT } from './mobile-pane-events'

type HeaderRoute = 'writing' | 'interactive' | 'full'
const HeaderHost = createContext<{ host: HTMLDivElement | null; route: HeaderRoute } | null>(null)
const HeaderVisible = createContext(true)

/** Hidden retained routes must not contribute controls through the shared header portal. */
export function MobileWorkspaceHeaderScope({ visible, children }: { visible: boolean; children: ReactNode }) {
  const parentVisible = useContext(HeaderVisible)
  return <HeaderVisible.Provider value={parentVisible && visible}>{children}</HeaderVisible.Provider>
}

/** Routes own their controls; the shell owns the single mobile title bar.
 * Keep this provider mounted across breakpoints to preserve portalled editors. */
export function MobileWorkspaceHeaderProvider({ route, children }: { route: HeaderRoute; children: ReactNode }) {
  const [host] = useState(() => createStablePortalHost('flex min-w-0 flex-1 items-center'))
  return <HeaderHost.Provider value={{ host, route }}>{children}</HeaderHost.Provider>
}

export function MobileWorkspaceHeader({ route, children }: { route: HeaderRoute; children: ReactNode }) {
  const context = useContext(HeaderHost)
  const visible = useContext(HeaderVisible)
  // Cached destinations stay mounted, but only the current route contributes chrome.
  if (!visible || (context && context.route !== route)) return null
  const content = <div data-mobile-header-content className="flex min-w-0 flex-1 items-center gap-1">{children}</div>
  return context?.host ? createPortal(content, context.host) : content
}

export function MobileWorkspaceHeaderOutlet({ title }: { title: string }) {
  const { t } = useTranslation()
  const host = useContext(HeaderHost)?.host ?? null
  return (
    <header className="nova-mobile-topbar nova-topbar flex h-12 shrink-0 items-center gap-1 border-b px-3">
      <Button variant="ghost" size="icon" aria-label={t('workbench.mobile.navigationMenu')} onClick={() => window.dispatchEvent(new Event(MOBILE_NAVIGATION_OPEN_EVENT))}><Menu /></Button>
      <span data-mobile-header-fallback className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">{title}</span>
      <StablePortalSlot host={host} fallback={null} className="nova-mobile-header-slot flex min-w-0 flex-1" />
    </header>
  )
}
