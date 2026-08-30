import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { EmbeddedSidebar } from '@/components/navigation/embedded-sidebar'
import { cn } from '@/lib/utils'
import {
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'

export interface SectionedNavigationItem<TID extends string = string> {
  id: TID
  title: ReactNode
  description?: ReactNode
  icon?: LucideIcon
}

export interface SectionedNavigationGroup<TID extends string = string> {
  id: string
  title: ReactNode
  action?: ReactNode
  items: SectionedNavigationItem<TID>[]
}

interface SectionedNavigationProps<TID extends string> {
  groups: SectionedNavigationGroup<TID>[]
  activeId: TID
  onSelect: (id: TID) => void
  className?: string
}

/** Shared presentation for fixed or scroll-spy navigation; selection behavior stays with the caller. */
export function SectionedNavigation<TID extends string>({
  groups,
  activeId,
  onSelect,
  className,
}: SectionedNavigationProps<TID>) {
  return (
    <EmbeddedSidebar className={className}>
        <SidebarContent>
          <nav>
            {groups.map((group) => (
              <SidebarGroup key={group.id} className="py-1">
                <SidebarGroupLabel className={cn(group.action && 'pr-8')}>{group.title}</SidebarGroupLabel>
                {group.action}
                <SidebarGroupContent>
                  <SidebarMenu>
                    {group.items.map((item) => {
                      const Icon = item.icon
                      const active = activeId === item.id
                      return (
                        <SidebarMenuItem key={item.id}>
                          <SidebarMenuButton
                            type="button"
                            size={item.description ? 'lg' : 'default'}
                            isActive={active}
                            aria-current={active ? 'page' : undefined}
                            onClick={() => onSelect(item.id)}
                          >
                            {Icon ? <Icon aria-hidden="true" /> : null}
                            <span className="grid min-w-0 flex-1 gap-0.5">
                              <span className="truncate font-medium text-sidebar-foreground">{item.title}</span>
                              {item.description ? (
                                <span className="truncate text-[11px] text-sidebar-foreground/60">{item.description}</span>
                              ) : null}
                            </span>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      )
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            ))}
          </nav>
        </SidebarContent>
    </EmbeddedSidebar>
  )
}
