import { GitBranch, LayoutDashboard, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { DirectorConsoleTab } from './types'

interface DirectorConsoleTabsProps {
  activeTab: DirectorConsoleTab
  onChange: (tab: DirectorConsoleTab) => void
  branchesCount: number
}

/** Stable information architecture: observe, tune, or manage routes. */
export function DirectorConsoleTabs({ activeTab, onChange, branchesCount }: DirectorConsoleTabsProps) {
  const { t } = useTranslation()
  const items: Array<{ id: DirectorConsoleTab; label: string; icon: React.ReactNode; count?: number }> = [
    { id: 'overview', label: t('directorPanel.consoleTab.overview'), icon: <LayoutDashboard className="size-3.5" /> },
    { id: 'tuning', label: t('directorPanel.consoleTab.tuning'), icon: <SlidersHorizontal className="size-3.5" /> },
    { id: 'routes', label: t('directorPanel.consoleTab.routes'), icon: <GitBranch className="size-3.5" />, count: branchesCount },
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
            {(item.count || 0) > 0 ? <span aria-hidden="true" className="director-console-tab__count shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{item.count}</span> : null}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
