import { useEffect, useMemo, useState } from 'react'
import { Activity, ArrowRight, Sparkles, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ActorStateField, ActorStateSchemaSnapshot, Snapshot } from '../../types'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from '../story-state/display-preference'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  humanizeStateKey,
  splitStoryStateFacts,
  stateChanges,
  statePathLabel,
  visibleActorTraits,
  type StoryStateChange,
} from '../story-state/model'
import { ActorTabs } from './ActorTabs'
import { StateValue } from './shared'
import { StateSchemaOverview } from './StateSchemaOverview'

export function StateView({ storyId, snapshot, stateFacts, syncError, displayPreference = DEFAULT_STORY_STATE_DISPLAY, onDisplayPreferenceChange = noopDisplayPreferenceChange, onSnapshotRefresh }: { storyId?: string; snapshot: Snapshot | null; stateFacts: Array<[string, unknown]>; syncStatus?: string; syncError?: string; displayPreference?: StoryStateDisplayPreference; onDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void; onSnapshotRefresh?: () => void | Promise<unknown> }) {
  const { t } = useTranslation()
  const turn = snapshot?.current_turn
  const { actors, worldFacts } = useMemo(() => splitStoryStateFacts(stateFacts), [stateFacts])
  const [selectedActorId, setSelectedActorId] = useState(actors[0]?.[0] || '')

  useEffect(() => {
    if (actors.some(([actorId]) => actorId === selectedActorId)) return
    setSelectedActorId(actors[0]?.[0] || '')
  }, [actors, selectedActorId])

  const changes = useMemo(() => stateChanges(turn?.state_delta), [turn?.state_delta])
  const actorNames = useMemo(() => new Map(actors.map(([actorId, actor]) => [actorId, actorName(actorId, actor)])), [actors])
  const hasState = actors.length > 0 || worldFacts.length > 0
  const selectedActor = actors.find(([actorId]) => actorId === selectedActorId)

  return (
    <div className="min-w-0 space-y-5">
      <section className="flex min-w-0 items-center gap-3 rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-3 py-2.5">
        <div className="min-w-0 flex-1">
          <h3 className="text-[11px] font-semibold text-[var(--nova-text)]">{t('storyStage.state.stageDisplay')}</h3>
          <p className="mt-0.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('storyStage.state.stageDisplayHint')}</p>
        </div>
        <StateDisplayPreferenceMenu value={displayPreference} onChange={onDisplayPreferenceChange} compact />
      </section>
      <StateSchemaOverview storyId={storyId} schema={snapshot?.actor_state_schema} initialization={snapshot?.state_schema_initialization} canReview={!snapshot || (snapshot.graph?.branches.length ?? 0) <= 1} onRefresh={onSnapshotRefresh} />
      <section className="min-w-0">
        {turn?.state_error || syncError ? (
          <div className="mb-3 rounded-[10px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-5 text-[var(--nova-danger)]">
            {turn?.state_error || syncError}
          </div>
        ) : null}

        {!hasState ? (
          <StateEmpty />
        ) : actors.length > 0 ? (
          <div className="min-w-0">
            <div className="flex min-w-0 flex-col gap-1.5">
              <div className="flex min-w-0 items-center justify-between gap-2 px-0.5">
                <span className="truncate text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--nova-text-faint)]">{t('directorPanel.actorCue')}</span>
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t('directorPanel.actorCount', { count: actors.length })}</span>
              </div>
              <ActorTabs
                actors={actors.map(([actorId, actor]) => ({ id: actorId, name: actorName(actorId, actor) }))}
                value={selectedActorId}
                onValueChange={setSelectedActorId}
              />
            </div>

            {selectedActor ? <ActorStateSheet actorId={selectedActor[0]} actor={selectedActor[1]} schema={snapshot?.actor_state_schema} /> : null}
          </div>
        ) : null}
      </section>

      {worldFacts.length > 0 ? (
        <section aria-labelledby="director-world-state-title">
          <SectionHeading id="director-world-state-title" icon={<Sparkles className="h-3.5 w-3.5" />} title={t('directorPanel.worldState')} hint={t('directorPanel.worldStateHint')} />
          <div className="director-state-grid mt-3 grid grid-cols-1 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
            {worldFacts.map(([key, value]) => (
              <StateFact key={key} label={humanizeStateKey(key)} value={value} />
            ))}
          </div>
        </section>
      ) : null}

      {changes.length > 0 ? (
        <section aria-labelledby="director-state-change-title">
          <SectionHeading id="director-state-change-title" icon={<Activity className="h-3.5 w-3.5" />} title={t('directorPanel.stateDelta')} hint={t('directorPanel.stateDeltaHint')} />
          <ol className="mt-3 space-y-2">
            {changes.map((change) => (
              <StateChangeRow key={change.id} change={change} actorName={change.actorId ? actorNames.get(change.actorId) : undefined} />
            ))}
          </ol>
        </section>
      ) : null}
    </div>
  )
}

function ActorStateSheet({ actorId, actor, schema }: { actorId: string; actor: Record<string, unknown>; schema?: ActorStateSchemaSnapshot }) {
  const { t } = useTranslation()
  const name = actorName(actorId, actor)
  const template = actorTemplate(actor, schema)
  const fields = actorFieldEntries(actor, template?.fields)
  const traits = visibleActorTraits(actor)

  return (
    <article aria-label={name} className="relative min-w-0 pt-2">
      {traits.length > 0 ? (
        <div className="border-b border-[var(--nova-border)] py-2.5">
          <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--nova-text-faint)]">{t('directorPanel.actorTraits')}</div>
          <div className="flex flex-wrap gap-1.5">
            {traits.map((trait) => (
              <span
                key={`${trait.pool_id}:${trait.trait_id}`}
                title={trait.summary || trait.name}
                className="max-w-full truncate rounded-full border border-[color-mix(in_srgb,var(--director-brass)_35%,var(--nova-border))] bg-[color-mix(in_srgb,var(--director-brass)_9%,transparent)] px-2.5 py-1 text-[10px] text-[var(--nova-text-muted)]"
              >
                {trait.name}
              </span>
            ))}
          </div>
        </div>
      ) : null}

      <div className="py-3">
        <div className="mb-2 flex items-center justify-between gap-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--nova-text-faint)]">{t('directorPanel.actorFields')}</span>
          <span className="text-[10px] text-[var(--nova-text-faint)]">{t('directorPanel.fieldCount', { count: fields.length })}</span>
        </div>
        {fields.length > 0 ? (
          <div className="director-state-grid grid grid-cols-1 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
            {fields.map(({ field, value }) => (
              <ActorField key={field.name} field={field} value={value} />
            ))}
          </div>
        ) : (
          <div className="rounded-[10px] border border-dashed border-[var(--nova-border)] px-3 py-7 text-center text-xs text-[var(--nova-text-faint)]">{t('directorPanel.actorFieldsEmpty')}</div>
        )}
      </div>
    </article>
  )
}

function ActorField({ field, value }: { field: ActorStateField; value: unknown }) {
  const numericValue = typeof value === 'number' ? value : null
  const hasMeter = numericValue !== null && typeof field.min === 'number' && typeof field.max === 'number' && field.max > field.min
  const meterValue = hasMeter ? Math.min(100, Math.max(0, ((numericValue - field.min!) / (field.max! - field.min!)) * 100)) : 0
  return (
    <section className="min-w-0 bg-[var(--director-panel)] px-3 py-2.5">
      <div className="mb-1.5 flex min-w-0 items-start justify-between gap-2">
        <div className="min-w-0">
          <h5 className="truncate text-[11px] font-medium text-[var(--nova-text-muted)]" title={field.name}>{field.name}</h5>
          {field.description ? <p className="mt-0.5 line-clamp-1 text-[9px] leading-3.5 text-[var(--nova-text-faint)]">{field.description}</p> : null}
        </div>
        <span className="shrink-0 font-mono text-[8px] uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{field.type}</span>
      </div>
      <StateValue value={value ?? field.default ?? null} />
      {hasMeter ? (
        <div className="mt-2 h-1 overflow-hidden rounded-full bg-[var(--nova-surface-3)]" aria-hidden="true">
          <div className="h-full rounded-full bg-[var(--director-live)] transition-[width] duration-300 motion-reduce:transition-none" style={{ width: `${meterValue}%` }} />
        </div>
      ) : null}
    </section>
  )
}

function StateFact({ label, value }: { label: string; value: unknown }) {
  return (
    <article className="min-w-0 bg-[var(--director-panel)] px-3 py-2.5">
      <h4 className="mb-1.5 truncate text-[10px] font-medium text-[var(--nova-text-faint)]" title={label}>{label}</h4>
      <StateValue value={value} />
    </article>
  )
}

function StateChangeRow({ change, actorName }: { change: StoryStateChange; actorName?: string }) {
  const { t } = useTranslation()
  return (
    <li className="grid grid-cols-[8px_minmax(0,1fr)] gap-3 [&:last-child_.state-change-line]:hidden">
      <div className="relative flex justify-center pt-2">
        <span className="z-10 h-2 w-2 rounded-full border-2 border-[var(--nova-surface)] bg-[var(--director-live)]" />
        <span className="state-change-line absolute bottom-[-14px] top-3 w-px bg-[var(--nova-border)]" />
      </div>
      <div className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-3 py-2.5">
        <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-[11px]">
          {actorName ? <span className="font-semibold text-[var(--nova-text)]">{actorName}</span> : null}
          {actorName ? <ArrowRight className="h-3 w-3 text-[var(--nova-text-faint)]" /> : null}
          <span className="min-w-0 break-words text-[var(--nova-text-muted)]">{statePathLabel(change.path)}</span>
          <span className="rounded-full bg-[var(--nova-surface-3)] px-1.5 py-0.5 text-[9px] text-[var(--nova-text-faint)]">{changeVerb(change.op, t)}</span>
        </div>
        {change.value !== undefined ? <div className="mt-1.5"><StateValue value={change.value} /></div> : null}
        {change.reason ? <p className="mt-1.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">{change.reason}</p> : null}
      </div>
    </li>
  )
}

function SectionHeading({ id, icon, title, hint }: { id: string; icon: React.ReactNode; title: string; hint: string }) {
  return (
    <div className="px-0.5">
      <div className="flex items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
        <span className="text-[var(--director-brass)]">{icon}</span>
        <h3 id={id}>{title}</h3>
      </div>
      <p className="mt-1 text-[11px] leading-4 text-[var(--nova-text-faint)]">{hint}</p>
    </div>
  )
}

function StateEmpty() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[220px] flex-col items-center justify-center rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--director-panel)] px-6 text-center">
      <span className="flex h-10 w-10 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]"><UserRound className="h-4 w-4" /></span>
      <p className="mt-3 text-xs font-medium text-[var(--nova-text-muted)]">{t('directorPanel.stateEmpty')}</p>
      <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.stateEmptyHint')}</p>
    </div>
  )
}

function changeVerb(op: string, t: ReturnType<typeof useTranslation>['t']) {
  const normalized = op.toLowerCase()
  if (normalized === 'set' || normalized === 'replace') return t('directorPanel.stateChange.set')
  if (normalized === 'add' || normalized === 'increment' || normalized === 'append') return t('directorPanel.stateChange.add')
  if (normalized === 'remove' || normalized === 'delete' || normalized === 'unset') return t('directorPanel.stateChange.remove')
  return op
}

function noopDisplayPreferenceChange() {}
