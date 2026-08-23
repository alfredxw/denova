import type { CSSProperties, ReactNode } from 'react'
import { Sidebar, SidebarProvider } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

interface EmbeddedSidebarProps {
  children: ReactNode
  className?: string
}

/** Non-collapsible shadcn Sidebar frame for a resizable pane inside a page. */
export function EmbeddedSidebar({ children, className }: EmbeddedSidebarProps) {
  return (
    <SidebarProvider
      className={cn('nova-embedded-sidebar h-full min-h-0 w-full', className)}
      style={{ '--sidebar-width': '100%' } as CSSProperties}
    >
      <Sidebar collapsible="none" className="w-full">
        {children}
      </Sidebar>
    </SidebarProvider>
  )
}
