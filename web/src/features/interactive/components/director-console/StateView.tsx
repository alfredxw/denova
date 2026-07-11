import { useEffect, useMemo, useState } from 'react'
import { Activity, ArrowRight, Sparkles, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ActorStateField, ActorStateSchemaSnapshot, ActorTraitInstance, Snapshot, TurnEvent } from '../../types'
import { ActorTabs } from './ActorTabs'
import { StateValue } from './shared'

type ActorEntry = [string, Record<string, unknown>]
type StateChange = {
  id: string
  actorId?: string
  path: string
  op: string
  value?: unknown
  reason?: string
}

export function StateView({ snapshot, stateFacts, syncError }: { snapshot: Snapshot | null; stateFacts: Array<[string, unknown]>; syncStatus?: string; syncError?: string }) {
  const { t } = useTranslation()
  const turn = snapshot?.current_turn
  const stateObjects = useMemo(() => actorEntries(stateFacts), [stateFacts])
  const actors = useMemo(() => stateObjects.filter(([actorId, actor]) => isActorLike(actorId, actor)), [stateObjects])
  const otherFacts = stateFacts.filter(([key]) => key !== 'actors')
  const worldFacts = useMemo<Array<[string, unknown]>>(() => [
    ...otherFacts,
    ...stateObjects
      .filter(([actorId, actor]) => !isActorLike(actorId, actor))
      .map(([actorId, actor]): [string, unknown] => [actorName(actorId, actor), stateObjectValue(actor)]),
  ], [otherFacts, stateObjects])
  const [selectedActorId, setSelectedActorId] = useState(actors[0]?.[0] || '')

  useEffect(() => {
    if (actors.some(([actorId]) => actorId === selectedActorId)) return
    setSelectedActorId(actors[0]?.[0] || '')
  }, [actors, selectedActorId])

  const changes = useMemo(() => stateChanges(turn?.state_delta), [turn?.state_delta])
  const actorNames = useMemo(() => new Map(actors.map(([actorId, actor]) => [actorId, actorName(actorId, actor)])), [actors])
  const hasState = stateObjects.length > 0 || otherFacts.length > 0
  const selectedActor = actors.find(([actorId]) => actorId === selectedActorId)

  return (
    <div className="min-w-0 space-y-5">
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
                <span className="truncate text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--nova-text-faint)]">{t('memoryPanel.actorCue')}</span>
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t('memoryPanel.actorCount', { count: actors.length })}</span>
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
          <SectionHeading id="director-world-state-title" icon={<Sparkles className="h-3.5 w-3.5" />} title={t('memoryPanel.worldState')} hint={t('memoryPanel.worldStateHint')} />
          <div className="director-state-grid mt-3 grid grid-cols-1 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
            {worldFacts.map(([key, value]) => (
              <StateFact key={key} label={humanizeStateKey(key)} value={value} />
            ))}
          </div>
        </section>
      ) : null}

      {changes.length > 0 ? (
        <section aria-labelledby="director-state-change-title">
          <SectionHeading id="director-state-change-title" icon={<Activity className="h-3.5 w-3.5" />} title={t('memoryPanel.stateDelta')} hint={t('memoryPanel.stateDeltaHint')} />
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
  const templateId = stringValue(actor.template_id)
  const template = schema?.system.templates?.find((item) => item.id === templateId)
  const fields = actorFieldEntries(actor, template?.fields)
  const traits = Array.isArray(actor.traits)
    ? actor.traits.filter(isActorTrait).filter((trait) => trait.visibility !== 'hidden')
    : []

  return (
    <article aria-label={name} className="relative min-w-0 pt-2">
      {traits.length > 0 ? (
        <div className="border-b border-[var(--nova-border)] py-2.5">
          <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--nova-text-faint)]">{t('memoryPanel.actorTraits')}</div>
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
          <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--nova-text-faint)]">{t('memoryPanel.actorFields')}</span>
          <span className="text-[10px] text-[var(--nova-text-faint)]">{t('memoryPanel.fieldCount', { count: fields.length })}</span>
        </div>
        {fields.length > 0 ? (
          <div className="director-state-grid grid grid-cols-1 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
            {fields.map(({ field, value }) => (
              <ActorField key={field.name} field={field} value={value} />
            ))}
          </div>
        ) : (
          <div className="rounded-[10px] border border-dashed border-[var(--nova-border)] px-3 py-7 text-center text-xs text-[var(--nova-text-faint)]">{t('memoryPanel.actorFieldsEmpty')}</div>
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

function StateChangeRow({ change, actorName }: { change: StateChange; actorName?: string }) {
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
      <p className="mt-3 text-xs font-medium text-[var(--nova-text-muted)]">{t('memoryPanel.stateEmpty')}</p>
      <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('memoryPanel.stateEmptyHint')}</p>
    </div>
  )
}

function actorEntries(stateFacts: Array<[string, unknown]>): ActorEntry[] {
  const actors = stateFacts.find(([key]) => key === 'actors')?.[1]
  if (!isRecord(actors)) return []
  return Object.entries(actors)
    .filter((entry): entry is ActorEntry => isRecord(entry[1]))
    .sort(([leftId, left], [rightId, right]) => actorPriority(leftId, left) - actorPriority(rightId, right))
}

function actorPriority(actorId: string, actor: Record<string, unknown>) {
  if (isLeadActor(actorId, actor)) return 0
  return 1
}

function isLeadActor(actorId: string, actor: Record<string, unknown>) {
  const candidates = [actorId, stringValue(actor.role), stringValue(actor.template_id)].map((value) => value.toLowerCase())
  return candidates.some((value) => value === 'protagonist' || value === 'lead' || value === 'player' || value === '主角')
}

function isActorLike(actorId: string, actor: Record<string, unknown>) {
  const role = stringValue(actor.role).toLowerCase()
  const templateId = stringValue(actor.template_id).toLowerCase()
  if (['story_context', 'world', 'scene', 'global', 'faction', 'base', 'instance', 'location', 'clock', 'quest'].some((marker) => role === marker || templateId === marker)) return false
  if (role) return true
  if (isLeadActor(actorId, actor)) return true
  const identity = `${actorId} ${templateId}`.toLowerCase()
  return ['important_character', 'opponent', 'supporting', 'character', 'npc', 'enemy', 'monster'].some((marker) => identity.includes(marker))
}

function actorName(actorId: string, actor: Record<string, unknown>) {
  return stringValue(actor.name) || actorId
}

function stateObjectValue(actor: Record<string, unknown>) {
  if (isRecord(actor.state)) return actor.state
  return Object.fromEntries(Object.entries(actor).filter(([key]) => !['name', 'role', 'template_id', 'traits'].includes(key)))
}

function actorFieldEntries(actor: Record<string, unknown>, schemaFields: ActorStateField[] | undefined) {
  const state = isRecord(actor.state) ? actor.state : {}
  const visibleFields = (schemaFields || [])
    .filter((field) => field.visibility !== 'hidden')
    .slice()
    .sort((left, right) => (left.order || 0) - (right.order || 0) || left.name.localeCompare(right.name))
  if (schemaFields !== undefined) {
    return visibleFields.map((field) => ({
      field,
      value: state[field.name] ?? (field.id ? state[field.id] : undefined) ?? (field.path ? state[field.path] : undefined),
    }))
  }

  const directState = Object.fromEntries(Object.entries(actor).filter(([key]) => !['name', 'role', 'template_id', 'state', 'traits'].includes(key)))
  return Object.entries({ ...directState, ...state }).map(([name, value], index) => ({
    field: { name: humanizeStateKey(name), type: inferredFieldType(value), order: index * 10 } satisfies ActorStateField,
    value,
  }))
}

function stateChanges(delta: TurnEvent['state_delta']): StateChange[] {
  if (!delta) return []
  const actorChanges: StateChange[] = (delta.actor_ops || []).map((op, index) => ({
    id: `actor:${op.actor_id}:${op.field_id}:${index}`,
    actorId: op.actor_id,
    path: op.field_id,
    op: op.op,
    value: op.value,
    reason: op.reason,
  }))
  const sharedChanges: StateChange[] = (delta.ops || []).map((op, index) => ({
    id: `state:${op.path}:${index}`,
    path: op.path,
    op: op.op,
    value: op.value,
    reason: op.reason,
  }))
  return [...actorChanges, ...sharedChanges]
}

function changeVerb(op: string, t: ReturnType<typeof useTranslation>['t']) {
  const normalized = op.toLowerCase()
  if (normalized === 'set' || normalized === 'replace') return t('memoryPanel.stateChange.set')
  if (normalized === 'add' || normalized === 'increment' || normalized === 'append') return t('memoryPanel.stateChange.add')
  if (normalized === 'remove' || normalized === 'delete' || normalized === 'unset') return t('memoryPanel.stateChange.remove')
  return op
}

function statePathLabel(path: string) {
  const parts = path.split('.').filter(Boolean)
  const useful = parts[0] === 'actors' && parts.length > 3 ? parts.slice(3) : parts
  return useful.map(humanizeStateKey).join(' / ')
}

function inferredFieldType(value: unknown) {
  if (Array.isArray(value)) return 'list'
  if (value === null) return 'object'
  if (typeof value === 'boolean') return 'bool'
  return typeof value
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

function humanizeStateKey(value: string) {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/^\w/, (letter) => letter.toUpperCase())
}
