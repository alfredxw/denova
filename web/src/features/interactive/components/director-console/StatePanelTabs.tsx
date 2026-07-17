import { Activity, Sparkles, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { StatePanelTab } from './types'

// 右栏状态面板的分区导航：变化 / 角色 / 世界，各自带数量徽标，
// 让「本回合有新变化」即使切到其他分区也保持可见。
export function StatePanelTabs({ activeTab, onChange, changesCount, actorsCount, worldCount }: {
  activeTab: StatePanelTab
  onChange: (tab: StatePanelTab) => void
  changesCount: number
  actorsCount: number
  worldCount: number
}) {
  const { t } = useTranslation()
  const items: Array<{ id: StatePanelTab; label: string; icon: React.ReactNode; count: number }> = [
    { id: 'changes', label: t('directorPanel.stateTab.changes'), icon: <Activity className="h-3.5 w-3.5" />, count: changesCount },
    { id: 'actors', label: t('directorPanel.stateTab.actors'), icon: <Users className="h-3.5 w-3.5" />, count: actorsCount },
    { id: 'world', label: t('directorPanel.stateTab.world'), icon: <Sparkles className="h-3.5 w-3.5" />, count: worldCount },
  ]
  return (
    <Tabs value={activeTab} onValueChange={(value) => onChange(value as StatePanelTab)} className="shrink-0 gap-0 border-b border-[var(--nova-border)] bg-[var(--director-canvas)]">
      <TabsList variant="line" aria-label={t('directorPanel.stateTab.label')} className="h-10 w-full gap-0 rounded-none px-3">
        {items.map((item) => (
          <TabsTrigger key={item.id} value={item.id} className="h-full flex-1 gap-1.5 rounded-none px-1 text-xs font-medium text-[var(--nova-text-faint)] after:bottom-0 after:bg-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] data-active:bg-transparent data-active:text-[var(--nova-text)]">
            {item.icon}
            <span className="min-w-0 truncate">{item.label}</span>
            {item.count > 0 ? <span aria-hidden="true" className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{item.count}</span> : null}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
