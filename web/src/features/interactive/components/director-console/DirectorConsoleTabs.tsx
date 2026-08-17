import { Activity, GitBranch, Sparkles, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { DirectorConsoleTab } from './types'

interface DirectorConsoleTabsProps {
  activeTab: DirectorConsoleTab
  onChange: (tab: DirectorConsoleTab) => void
  changesCount: number
  actorsCount: number
  worldCount: number
  branchesCount: number
}

/** Compact navigation for the state views and the story-bound branch preview. */
export function DirectorConsoleTabs({ activeTab, onChange, changesCount, actorsCount, worldCount, branchesCount }: DirectorConsoleTabsProps) {
  const { t } = useTranslation()
  const items: Array<{ id: DirectorConsoleTab; label: string; icon: React.ReactNode; count: number }> = [
    { id: 'changes', label: t('directorPanel.stateTab.changes'), icon: <Activity className="h-3.5 w-3.5" />, count: changesCount },
    { id: 'actors', label: t('directorPanel.stateTab.actors'), icon: <Users className="h-3.5 w-3.5" />, count: actorsCount },
    { id: 'world', label: t('directorPanel.stateTab.world'), icon: <Sparkles className="h-3.5 w-3.5" />, count: worldCount },
    { id: 'branches', label: t('directorPanel.stateTab.branches'), icon: <GitBranch className="h-3.5 w-3.5" />, count: branchesCount },
  ]

  return (
    <Tabs value={activeTab} onValueChange={(value) => onChange(value as DirectorConsoleTab)} className="shrink-0 gap-0 border-b border-[var(--nova-border)] bg-[var(--director-canvas)]">
      <TabsList variant="line" aria-label={t('directorPanel.consoleTabs')} className="h-10 w-full gap-0 rounded-none px-2">
        {items.map((item) => (
          <TabsTrigger
            key={item.id}
            value={item.id}
            aria-label={item.label}
            className="h-full min-w-0 flex-1 gap-1 rounded-none px-1 text-xs font-medium text-[var(--nova-text-faint)] after:bottom-0 after:bg-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] data-[state=active]:bg-transparent data-[state=active]:text-[var(--nova-text)]"
          >
            {item.icon}
            <span className="director-console-tab__label min-w-0 truncate">{item.label}</span>
            {item.count > 0 ? <span aria-hidden="true" className="director-console-tab__count shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{item.count}</span> : null}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
