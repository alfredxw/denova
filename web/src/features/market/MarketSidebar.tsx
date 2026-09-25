import {
  ArrowUpRight,
  BookOpen,
  ChevronDown,
  Compass,
  Gamepad2,
  Library,
  Package,
  Palette,
  Plug,
  RefreshCw,
  SlidersHorizontal,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmbeddedSidebar } from '@/components/navigation/embedded-sidebar'
import { closeMobilePanes } from '@/components/layout/mobile-pane-events'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
} from '@/components/ui/sidebar'
import { submissionURL, type MarketEntry } from './api'

export type MarketSection = 'discover' | 'acquired' | 'updates'
export type MarketCategory =
  | 'all'
  | 'presets'
  | 'styles'
  | 'skills'
  | 'content'
  | 'plugins'
  | 'games'
const categories = [
  { id: 'presets', icon: SlidersHorizontal },
  { id: 'styles', icon: Palette },
  { id: 'skills', icon: Sparkles },
  { id: 'content', icon: BookOpen },
  { id: 'plugins', icon: Plug },
  { id: 'games', icon: Gamepad2 },
] as const

export function matchesCategory(
  entry: MarketEntry,
  category: MarketCategory,
): boolean {
  switch (category) {
    case 'all':
      return true
    case 'presets':
      return entry.kinds.some((kind) => kind.startsWith('preset.'))
    case 'styles':
      return entry.kinds.includes('style.reference')
    case 'skills':
      return entry.kinds.includes('skill')
    case 'content':
      return entry.kinds.some((kind) =>
        ['lore.item', 'game.opening', 'project.cover'].includes(kind),
      )
    case 'plugins':
      return entry.kinds.includes('extension.plugin')
    case 'games':
      return entry.kinds.includes('extension.game')
  }
}

export function MarketSidebar({
  section,
  category,
  entries,
  acquiredCount,
  updateCount,
  onSelect,
}: {
  section: MarketSection
  category: MarketCategory
  entries: MarketEntry[]
  acquiredCount: number
  updateCount: number
  onSelect: (section: MarketSection, category: MarketCategory) => void
}) {
  const { t } = useTranslation()
  const select = (next: MarketSection, selected: MarketCategory = 'all') => {
    onSelect(next, selected)
    closeMobilePanes()
  }
  return (
    <EmbeddedSidebar className="nova-sidebar">
      <nav
        aria-label={t('market.navigation')}
        className="flex h-full min-h-0 flex-col"
      >
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>{t('market.title')}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {(
                  [
                    { id: 'discover', icon: Compass, count: entries.length },
                    { id: 'acquired', icon: Library, count: acquiredCount },
                    { id: 'updates', icon: RefreshCw, count: updateCount },
                  ] as const
                ).map(({ id, icon: Icon, count }) => (
                  <SidebarMenuItem key={id}>
                    <SidebarMenuButton
                      isActive={section === id && category === 'all'}
                      aria-current={
                        section === id && category === 'all'
                          ? 'page'
                          : undefined
                      }
                      onClick={() => select(id)}
                    >
                      <Icon />
                      <span>{t(`market.${id}`)}</span>
                    </SidebarMenuButton>
                    <SidebarMenuBadge>{count}</SidebarMenuBadge>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          <SidebarSeparator />
          <Collapsible defaultOpen className="group/types">
            <SidebarGroup>
              <SidebarGroupLabel asChild>
                <CollapsibleTrigger className="w-full gap-2">
                  <ChevronDown className="transition-transform group-data-[state=closed]/types:-rotate-90" />
                  <span>{t('market.type')}</span>
                </CollapsibleTrigger>
              </SidebarGroupLabel>
              <CollapsibleContent>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {categories.map(({ id, icon: Icon }) => (
                      <SidebarMenuItem key={id}>
                        <SidebarMenuButton
                          isActive={section === 'discover' && category === id}
                          aria-current={
                            section === 'discover' && category === id
                              ? 'page'
                              : undefined
                          }
                          onClick={() => select('discover', id)}
                        >
                          <Icon />
                          <span>{t(`market.category.${id}`)}</span>
                        </SidebarMenuButton>
                        <SidebarMenuBadge>
                          {
                            entries.filter((entry) =>
                              matchesCategory(entry, id),
                            ).length
                          }
                        </SidebarMenuBadge>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>
        </SidebarContent>
        <SidebarSeparator />
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild>
                <a href={submissionURL} target="_blank" rel="noreferrer">
                  <Package />
                  <span>{t('market.submit')}</span>
                  <ArrowUpRight className="ml-auto" />
                </a>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </nav>
    </EmbeddedSidebar>
  )
}
