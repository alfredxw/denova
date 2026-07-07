import { Activity, Gauge } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Snapshot } from '../../types'
import { StateValue, SyncBadge } from './shared'

export function StateView({ snapshot, stateFacts, syncStatus, syncError }: { snapshot: Snapshot | null; stateFacts: Array<[string, unknown]>; syncStatus?: string; syncError?: string }) {
  const { t } = useTranslation()
  const turn = snapshot?.current_turn
  return (
    <div className="space-y-3">
      <section className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
        <div className="mb-2 flex min-w-0 items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
            <Gauge className="h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-blue)]" />
            <span className="truncate">{t('memoryPanel.currentState')}</span>
          </div>
          <SyncBadge status={syncStatus} error={syncError} loading={syncStatus === 'pending'} />
        </div>
        {turn?.state_error || syncError ? <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{turn?.state_error || syncError}</div> : null}
        {stateFacts.length ? (
          <div className="space-y-2">
            {stateFacts.map(([key, value]) => (
              <StateFactCard key={key} label={key} value={value} />
            ))}
          </div>
        ) : (
          <div className="flex min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.stateEmpty')}</div>
        )}
      </section>
      {turn?.state_delta ? (
        <section className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex min-w-0 items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
            <Activity className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
            <span className="truncate">{t('memoryPanel.stateDelta')}</span>
          </div>
          <StateValue value={turn.state_delta} />
        </section>
      ) : null}
    </div>
  )
}

function StateFactCard({ label, value }: { label: string; value: unknown }) {
  return (
    <article className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <div className="mb-1 truncate text-[11px] font-medium text-[var(--nova-text-faint)]" title={label}>{label}</div>
      <StateValue value={value} />
    </article>
  )
}
