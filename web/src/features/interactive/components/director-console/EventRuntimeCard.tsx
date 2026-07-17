import { Activity, RotateCcw, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { DirectorPlanMetadata } from '../../types'
import type { DirectorStatusLike } from './types'

// 事件编排运行态：仅在有导演运行记录且已揭示剧透内容后展示（活跃事件本身可能剧透）。
export function EventRuntimeCard({ status, metadata, busy, onEvaluate, onReset }: { status?: DirectorStatusLike; metadata?: DirectorPlanMetadata; busy: boolean; onEvaluate: () => void; onReset: () => void }) {
  const { t } = useTranslation()
  const runtime = status?.event_runtime || metadata?.event_runtime
  const opportunity = status?.event_opportunity || metadata?.last_run?.event_opportunity
  const decisions = runtime?.recent_decisions || []
  const lastDecision = decisions.length ? decisions[decisions.length - 1].decision : undefined
  const active = runtime?.active
  return (
    <section className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <div className="flex flex-col items-stretch gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]"><Activity className="h-3.5 w-3.5" /></span>
          <div className="min-w-0">
            <h3 className="director-console__display text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.events.title')}</h3>
            <p className="mt-1 break-all text-[10px] text-[var(--nova-text-faint)]">{active ? active.event_ref : t('directorPanel.events.noActive')}</p>
          </div>
        </div>
        <div className="flex w-full flex-wrap gap-1">
          <Button type="button" variant="outline" size="xs" className="h-7 flex-1 gap-1 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)]" disabled={busy} onClick={onEvaluate}><Zap className="h-3 w-3" />{t('directorPanel.events.evaluate')}</Button>
          <Button type="button" variant="outline" size="xs" className="h-7 flex-1 gap-1 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)]" disabled={busy} onClick={onReset}><RotateCcw className="h-3 w-3" />{t('directorPanel.events.reset')}</Button>
        </div>
      </div>
      <div className="grid gap-2 px-3 py-3 text-[11px] leading-5 text-[var(--nova-text-muted)] sm:grid-cols-3">
        <div><span className="text-[var(--nova-text-faint)]">{t('directorPanel.events.stage')}：</span>{active?.stage || '-'}</div>
        <div><span className="text-[var(--nova-text-faint)]">{t('directorPanel.events.opportunity')}：</span>{opportunity?.kind || 'none'}{opportunity?.due ? ' · due' : ''}</div>
        <div><span className="text-[var(--nova-text-faint)]">{t('directorPanel.events.lastDecision')}：</span>{lastDecision?.mode || '-'}</div>
      </div>
      {active?.summary ? <p className="break-words border-t border-[var(--nova-border)] px-3 py-2 text-xs leading-5 text-[var(--nova-text)]">{active.summary}</p> : null}
    </section>
  )
}
