import { Clapperboard } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function DirectorConsoleHeader({ branchId, turnCount }: { branchId: string; turnCount: number }) {
  const { t } = useTranslation()
  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--director-canvas)_92%,transparent)] px-4 pb-3 pt-4 backdrop-blur-xl">
      <div className="flex min-w-0 items-center gap-3">
        <div data-testid="memory-panel-icon" className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]" aria-label={t('memoryPanel.consoleTitle')} title={t('memoryPanel.consoleTitle')}>
          <Clapperboard className="h-4.5 w-4.5" />
          <span className="absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full border-2 border-[var(--director-canvas)] bg-[var(--director-live)]" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-[9px] font-semibold uppercase tracking-[0.2em] text-[var(--nova-text-faint)]">{t('memoryPanel.consoleEyebrow')}</p>
          <h2 className="director-console__display min-w-0 truncate text-base font-semibold leading-6 text-[var(--nova-text)]">{t('memoryPanel.consoleTitle')}</h2>
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[9px] text-[var(--nova-text-faint)]">
            <span className="truncate">{t('memoryPanel.branch', { branch: branchId || 'main' })}</span>
            <span aria-hidden="true">/</span>
            <span className="shrink-0">{t('memoryPanel.turnCount', { count: turnCount })}</span>
          </div>
        </div>
      </div>
    </header>
  )
}
