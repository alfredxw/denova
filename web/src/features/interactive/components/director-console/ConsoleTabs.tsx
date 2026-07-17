import { Drama, Gauge, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ConsoleTab, DirectorStatusLike } from './types'

export function ConsoleTabs({ activeTab, onChange, stateCount, directorStatus, directorActive }: {
  activeTab: ConsoleTab
  onChange: (tab: ConsoleTab) => void
  stateCount: number
  directorStatus?: DirectorStatusLike
  directorActive?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Tabs value={activeTab} onValueChange={(value) => onChange(value as ConsoleTab)} className="shrink-0 gap-0 border-b border-[var(--nova-border)] bg-[var(--director-canvas)]">
      <TabsList variant="line" aria-label={t('directorPanel.consoleTabs')} className="h-11 w-full gap-0 rounded-none px-3">
        <TabsTrigger value="state" className={triggerClass}>
          <Gauge className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('directorPanel.consoleTab.state')}</span>
          {stateCount > 0 ? <span aria-hidden="true" className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{stateCount}</span> : null}
        </TabsTrigger>
        <TabsTrigger value="director" className={triggerClass}>
          <Drama className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('directorPanel.consoleTab.director')}</span>
          <DirectorStatusIndicator status={directorStatus} active={directorActive} />
        </TabsTrigger>
      </TabsList>
    </Tabs>
  )
}

const triggerClass = 'h-full flex-1 gap-1.5 rounded-none px-1 text-xs font-medium text-[var(--nova-text-faint)] after:bottom-0 after:bg-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] data-active:bg-transparent data-active:text-[var(--nova-text)]'

// 导演 tab 上的实时状态指示：运行中转圈、失败红点，让用户在状态 tab 也能感知后台动态。
function DirectorStatusIndicator({ status, active }: { status?: DirectorStatusLike; active?: boolean }) {
  if (active || status?.status === 'running') return <Loader2 aria-hidden="true" className="h-3 w-3 shrink-0 animate-spin text-[var(--director-brass)]" />
  if (status?.status === 'failed') return <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--nova-danger)]" />
  return null
}
