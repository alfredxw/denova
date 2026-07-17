import type { ReactNode } from 'react'
import { Activity, FileText, Gauge } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ConsoleTab } from './types'

export function ConsoleTabs({ activeTab, onChange, stateCount }: { activeTab: ConsoleTab; onChange: (tab: ConsoleTab) => void; stateCount: number }) {
  const { t } = useTranslation()
  const items: Array<{ id: ConsoleTab; label: string; icon: ReactNode; count?: number }> = [
    { id: 'state', label: t('directorPanel.consoleTab.state'), icon: <Gauge className="h-3.5 w-3.5" />, count: stateCount },
    { id: 'plan', label: t('directorPanel.consoleTab.plan'), icon: <FileText className="h-3.5 w-3.5" /> },
    { id: 'run', label: t('directorPanel.consoleTab.run'), icon: <Activity className="h-3.5 w-3.5" /> },
  ]
  return (
    <nav className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--director-canvas)] px-3" aria-label={t('directorPanel.consoleTabs')}>
      <div className="grid grid-cols-3 gap-0">
        {items.map((item) => (
          <button
            key={item.id}
            type="button"
            className={`relative flex h-11 min-w-0 items-center justify-center gap-1.5 px-1 text-[10px] font-medium transition-colors after:absolute after:inset-x-2 after:bottom-0 after:h-[2px] after:rounded-full after:transition-opacity focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--director-brass)] ${activeTab === item.id ? 'text-[var(--nova-text)] after:bg-[var(--director-brass)] after:opacity-100' : 'text-[var(--nova-text-faint)] after:opacity-0 hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]'}`}
            aria-pressed={activeTab === item.id}
            aria-current={activeTab === item.id ? 'page' : undefined}
            aria-label={item.label}
            onClick={() => onChange(item.id)}
          >
            {item.icon}
            <span className="min-w-0 truncate">{item.label}</span>
            {typeof item.count === 'number' && item.count > 0 ? <span aria-hidden="true" className="shrink-0 font-mono text-[8px] text-[var(--nova-text-faint)]">{item.count}</span> : null}
          </button>
        ))}
      </div>
    </nav>
  )
}
