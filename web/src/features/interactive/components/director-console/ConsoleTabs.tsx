import type { ReactNode } from 'react'
import { Brain, FileText, Gauge, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ConsoleTab } from './types'

export function ConsoleTabs({ activeTab, onChange }: { activeTab: ConsoleTab; onChange: (tab: ConsoleTab) => void }) {
  const { t } = useTranslation()
  const items: Array<{ id: ConsoleTab; label: string; icon: ReactNode }> = [
    { id: 'run', label: t('memoryPanel.consoleTab.run'), icon: <Sparkles className="h-3.5 w-3.5" /> },
    { id: 'state', label: t('memoryPanel.consoleTab.state'), icon: <Gauge className="h-3.5 w-3.5" /> },
    { id: 'memory', label: t('memoryPanel.consoleTab.memory'), icon: <Brain className="h-3.5 w-3.5" /> },
    { id: 'plan', label: t('memoryPanel.consoleTab.plan'), icon: <FileText className="h-3.5 w-3.5" /> },
  ]
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] px-3 py-2">
      <div className="grid grid-cols-4 gap-1 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1" aria-label={t('memoryPanel.consoleTabs')}>
        {items.map((item) => (
          <button
            key={item.id}
            type="button"
            className={`flex h-8 min-w-0 items-center justify-center gap-1 rounded-[6px] px-1.5 text-[11px] transition-colors ${activeTab === item.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]' : 'text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]'}`}
            aria-pressed={activeTab === item.id}
            onClick={() => onChange(item.id)}
          >
            {item.icon}
            <span className="min-w-0 truncate">{item.label}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
