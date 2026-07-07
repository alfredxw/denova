import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function DirectorConsoleHeader() {
  const { t } = useTranslation()
  return (
    <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)]/95 px-3 py-2 backdrop-blur-xl">
      <div className="flex min-h-9 min-w-0 items-center gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <div data-testid="memory-panel-icon" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-accent-blue)] shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]" aria-label={t('memoryPanel.consoleTitle')} title={t('memoryPanel.consoleTitle')}>
            <Sparkles className="h-4 w-4" />
          </div>
          <h2 className="min-w-0 truncate text-sm font-semibold leading-5 text-[var(--nova-text)]">{t('memoryPanel.consoleTitle')}</h2>
        </div>
      </div>
    </div>
  )
}
