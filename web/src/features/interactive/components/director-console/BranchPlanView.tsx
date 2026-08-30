import { Map } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import type { BranchPlan } from '../../types'

interface BranchPlanViewProps {
  plan?: BranchPlan
  planningEnabled: boolean
}

/** Read-only projection of the Game Agent's complete plan for the active branch. */
export function BranchPlanView({ plan, planningEnabled }: BranchPlanViewProps) {
  const { t, i18n } = useTranslation()
  const markdown = plan?.markdown.trim() || ''
  const updatedAt = formatUpdatedAt(plan?.updated_at, i18n.language)

  return (
    <section className="director-console__scroll h-full min-h-0 overflow-y-auto px-4 py-4">
      <div className="flex items-start gap-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]"><Map className="size-4" /></span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.plan.title')}</h3>
            <span className="rounded-full border border-[var(--nova-border)] bg-[var(--director-panel)] px-2 py-0.5 text-[9px] font-medium text-[var(--nova-text-faint)]">{t(planningEnabled ? 'directorPanel.planning.enabled' : 'directorPanel.planning.disabled')}</span>
          </div>
          <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t(planningEnabled ? 'directorPanel.plan.description' : 'directorPanel.plan.disabledDescription')}</p>
        </div>
      </div>

      {markdown ? (
        <div className="mt-4 rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] p-4">
          <ThemedMarkdownRenderer content={markdown} className="text-xs leading-6 text-[var(--nova-text-muted)]" />
          {updatedAt ? <p className="mt-4 border-t border-[var(--nova-border-soft)] pt-3 text-[10px] text-[var(--nova-text-faint)]">{t('directorPanel.plan.updatedAt', { time: updatedAt })}</p> : null}
        </div>
      ) : (
        <div className="mt-4 rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--director-panel)] px-5 py-10 text-center">
          <p className="text-xs font-medium text-[var(--nova-text-muted)]">{t('directorPanel.plan.empty')}</p>
          <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.plan.emptyHint')}</p>
        </div>
      )}
    </section>
  )
}

function formatUpdatedAt(value: string | undefined, locale: string) {
  if (!value) return ''
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? '' : parsed.toLocaleString(locale)
}
