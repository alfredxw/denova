import { Activity, Gauge, Sparkles, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ActorTraitInstance, Snapshot } from '../../types'
import { StateValue, SyncBadge } from './shared'

export function StateView({ snapshot, stateFacts, syncStatus, syncError }: { snapshot: Snapshot | null; stateFacts: Array<[string, unknown]>; syncStatus?: string; syncError?: string }) {
  const { t } = useTranslation()
  const turn = snapshot?.current_turn
  const actorFacts = actorEntries(stateFacts)
  const otherFacts = stateFacts.filter(([key]) => key !== 'actors')
  const hasState = actorFacts.length > 0 || otherFacts.length > 0
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
        {hasState ? (
          <div className="space-y-2">
            {actorFacts.length ? (
              <div className="space-y-2">
                <div className="px-0.5 text-[11px] font-medium text-[var(--nova-text-faint)]">
                  {t('memoryPanel.actorState')}
                </div>
                {actorFacts.map(([actorId, actor]) => (
                  <ActorStateCard key={actorId} actorId={actorId} actor={actor} />
                ))}
              </div>
            ) : null}
            {otherFacts.map(([key, value]) => (
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

function ActorStateCard({ actorId, actor }: { actorId: string; actor: Record<string, unknown> }) {
  const { t } = useTranslation()
  const name = stringValue(actor.name) || actorId
  const role = stringValue(actor.role)
  const templateId = stringValue(actor.template_id)
  const fields = isRecord(actor.state) ? actor.state : {}
  const traits = Array.isArray(actor.traits)
    ? actor.traits.filter(isActorTrait).filter((trait) => trait.visibility !== 'hidden')
    : []

  return (
    <article className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2.5">
      <div className="flex min-w-0 items-start gap-2">
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <UserRound className="h-3.5 w-3.5 text-[var(--nova-accent-blue)]" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="truncate text-xs font-semibold text-[var(--nova-text)]">{name}</span>
            {role ? <StateMetaBadge>{role}</StateMetaBadge> : null}
          </div>
          <div className="mt-0.5 truncate font-mono text-[10px] text-[var(--nova-text-faint)]">
            {templateId ? t('memoryPanel.actorTemplate', { template: templateId }) : actorId}
          </div>
        </div>
      </div>

      <div className="mt-2 border-t border-[var(--nova-border)] pt-2">
        <div className="mb-1.5 flex items-center gap-1.5 text-[10px] font-medium text-[var(--nova-text-faint)]">
          <Sparkles className="h-3 w-3" />
          {t('memoryPanel.actorTraits')}
        </div>
        {traits.length ? (
          <div className="flex flex-wrap gap-1.5">
            {traits.map((trait) => (
              <span
                key={`${trait.pool_id}:${trait.trait_id}`}
                title={trait.summary || trait.name}
                className="max-w-full truncate rounded-full border border-[var(--nova-accent-blue)]/20 bg-[var(--nova-accent-blue)]/8 px-2 py-1 text-[10px] text-[var(--nova-text-muted)]"
              >
                {trait.name}
              </span>
            ))}
          </div>
        ) : (
          <div className="text-[10px] text-[var(--nova-text-faint)]">{t('memoryPanel.actorTraitsEmpty')}</div>
        )}
      </div>

      {Object.keys(fields).length ? (
        <div className="mt-2 border-t border-[var(--nova-border)] pt-2">
          <div className="mb-1 text-[10px] font-medium text-[var(--nova-text-faint)]">{t('memoryPanel.actorFields')}</div>
          <StateValue value={fields} />
        </div>
      ) : null}
    </article>
  )
}

function StateMetaBadge({ children }: { children: React.ReactNode }) {
  return <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[9px] text-[var(--nova-text-faint)]">{children}</span>
}

function actorEntries(stateFacts: Array<[string, unknown]>): Array<[string, Record<string, unknown>]> {
  const actors = stateFacts.find(([key]) => key === 'actors')?.[1]
  if (!isRecord(actors)) return []
  return Object.entries(actors).filter((entry): entry is [string, Record<string, unknown>] => isRecord(entry[1]))
}

function isActorTrait(value: unknown): value is ActorTraitInstance {
  return isRecord(value)
    && typeof value.pool_id === 'string'
    && typeof value.trait_id === 'string'
    && typeof value.name === 'string'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function StateFactCard({ label, value }: { label: string; value: unknown }) {
  return (
    <article className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <div className="mb-1 truncate text-[11px] font-medium text-[var(--nova-text-faint)]" title={label}>{label}</div>
      <StateValue value={value} />
    </article>
  )
}
