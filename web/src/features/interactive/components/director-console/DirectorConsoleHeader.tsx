import { Clapperboard } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { gamePlanningTemplateName } from '../../game-planning'
import type { GamePlanningTemplate, StorySummary } from '../../types'

interface DirectorConsoleHeaderProps {
  branchId: string
  turnCount: number
  story?: StorySummary
  planningTemplates: GamePlanningTemplate[]
}

/** Identity and scope only; editable controls live in the tuning view. */
export function DirectorConsoleHeader({ branchId, turnCount, story, planningTemplates }: DirectorConsoleHeaderProps) {
  const { t } = useTranslation()
  const planningTemplate = planningTemplates.find((item) => item.id === story?.planning_template_id)

  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--director-canvas)_92%,transparent)] px-4 py-3.5 backdrop-blur-xl">
      <div className="flex min-w-0 items-center gap-3">
        <div data-testid="director-panel-icon" className="relative flex size-9 shrink-0 items-center justify-center rounded-[11px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]" aria-label={t('directorPanel.consoleTitle')}>
          <Clapperboard className="size-4" />
          <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full border-2 border-[var(--director-canvas)] bg-[var(--director-live)]" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <h2 className="director-console__display truncate text-sm font-semibold leading-5 text-[var(--nova-text)]">{t('directorPanel.consoleTitle')}</h2>
          </div>
          <p className="mt-0.5 truncate text-[10px] text-[var(--nova-text-muted)]">
            {planningTemplate
              ? gamePlanningTemplateName(planningTemplate, t)
              : t('directorPanel.noPlanningTemplate')}
          </p>
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[9px] text-[var(--nova-text-faint)]">
            <span className="truncate">{t('directorPanel.branch', { branch: branchId || 'main' })}</span>
            <span aria-hidden="true">/</span>
            <span className="shrink-0">{t('directorPanel.turnCount', { count: turnCount })}</span>
          </div>
        </div>
      </div>
    </header>
  )
}
