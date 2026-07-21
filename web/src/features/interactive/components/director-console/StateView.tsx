import { useEffect, useMemo, useState } from 'react'
import { Activity, ArrowRight, ChevronDown, ChevronRight, Sparkles, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { ActorStateField, ActorStateSchemaSnapshot, Snapshot } from '../../types'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  actorTraits,
  humanizeStateKey,
  splitStoryStateFacts,
  stateChanges,
  statePathLabel,
  type StoryStateChange,
} from '../story-state/model'
import { StateValue } from './shared'
import type { StatePanelTab } from './types'

const CHANGES_PREVIEW_COUNT = 3

// 状态感知栏：只做「现在」的核心浏览，按 section 渲染一个分区——
// 本回合变化 / 角色紧凑行（默认展开主角，行内可展开）/ 世界与场景。
// 展示偏好设置在控制台 header 信息条；状态结构机械信息在主区导演台。
export function StateView({ snapshot, stateFacts, syncError, section }: { snapshot: Snapshot | null; stateFacts: Array<[string, unknown]>; syncError?: string; section: StatePanelTab }) {
  const { t } = useTranslation()
  const turn = snapshot?.current_turn
  const { actors, worldFacts } = useMemo(() => splitStoryStateFacts(stateFacts), [stateFacts])
  const [showAllChanges, setShowAllChanges] = useState(false)
  const [expandedActorIds, setExpandedActorIds] = useState<string[]>(() => defaultExpandedActors(actors))
  const actorIdsKey = actors.map(([actorId]) => actorId).join('|')

  // 角色集合变化（切故事/分支）时重置为默认展开；同组角色逐回合更新不打断展开状态。
  const [prevActorIdsKey, setPrevActorIdsKey] = useState(actorIdsKey)
  if (prevActorIdsKey !== actorIdsKey) {
    setPrevActorIdsKey(actorIdsKey)
    setExpandedActorIds(defaultExpandedActors(actors))
  }

  useEffect(() => {
    setShowAllChanges(false)
  }, [turn?.id])

  const changes = useMemo(() => stateChanges(turn?.state_delta), [turn?.state_delta])
  const visibleChanges = showAllChanges ? changes : changes.slice(0, CHANGES_PREVIEW_COUNT)
  const actorNames = useMemo(() => new Map(actors.map(([actorId, actor]) => [actorId, actorName(actorId, actor)])), [actors])

  const toggleActor = (actorId: string) => {
    setExpandedActorIds((current) => current.includes(actorId) ? current.filter((id) => id !== actorId) : [...current, actorId])
  }

  return (
    <div className="min-w-0 space-y-5">
      {turn?.state_error || syncError ? (
        <div className="rounded-[10px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-5 text-[var(--nova-danger)]">
          {turn?.state_error || syncError}
        </div>
      ) : null}

      {section === 'changes' ? (
        changes.length > 0 ? (
          <section aria-labelledby="director-state-change-title">
            <SectionHeading id="director-state-change-title" icon={<Activity className="h-3.5 w-3.5" />} title={t('directorPanel.stateDelta')} hint={t('directorPanel.stateDeltaHint')} />
            <ol className="mt-3 space-y-2">
              {visibleChanges.map((change) => (
                <StateChangeRow key={change.id} change={change} actorName={change.actorId ? actorNames.get(change.actorId) : undefined} />
              ))}
            </ol>
            {changes.length > CHANGES_PREVIEW_COUNT ? (
              <Button type="button" variant="ghost" size="xs" className="mt-2 w-full gap-1 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]" onClick={() => setShowAllChanges((value) => !value)}>
                <ChevronDown className={`h-3.5 w-3.5 transition-transform ${showAllChanges ? 'rotate-180' : ''}`} />
                {showAllChanges ? t('directorPanel.stateDeltaShowLess') : t('directorPanel.stateDeltaShowAll', { count: changes.length })}
              </Button>
            ) : null}
          </section>
        ) : (
          <SectionEmpty text={t('directorPanel.stateDeltaEmpty')} />
        )
      ) : null}

      {section === 'actors' ? (
        <section className="min-w-0">
          {actors.length === 0 ? (
            <StateEmpty />
          ) : (
            <div className="min-w-0">
              <div className="flex min-w-0 items-center justify-between gap-2 px-0.5">
                <span className="truncate text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--nova-text-faint)]">{t('directorPanel.actorCue')}</span>
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t('directorPanel.actorCount', { count: actors.length })}</span>
              </div>
              <div className="mt-1.5 space-y-1.5">
                {actors.map(([actorId, actor]) => (
                  <ActorRow
                    key={actorId}
                    actorId={actorId}
                    actor={actor}
                    schema={snapshot?.actor_state_schema}
                    expanded={expandedActorIds.includes(actorId)}
                    onToggle={() => toggleActor(actorId)}
                  />
                ))}
              </div>
            </div>
          )}
        </section>
      ) : null}

      {section === 'world' ? (
        worldFacts.length > 0 ? (
          <section aria-labelledby="director-world-state-title">
            <SectionHeading id="director-world-state-title" icon={<Sparkles className="h-3.5 w-3.5" />} title={t('directorPanel.worldState')} hint={t('directorPanel.worldStateHint')} />
            <div className="director-state-grid mt-3 grid grid-cols-1 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
              {worldFacts.map(([key, value]) => (
                <StateFact key={key} label={humanizeStateKey(key)} value={value} />
              ))}
            </div>
          </section>
        ) : (
          <SectionEmpty text={t('directorPanel.worldStateEmpty')} />
        )
      ) : null}
    </div>
  )
}

function SectionEmpty({ text }: { text: string }) {
  return (
    <div className="flex min-h-[160px] items-center justify-center rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--director-panel)] px-6 text-center text-xs leading-5 text-[var(--nova-text-faint)]">
      {text}
    </div>
  )
}

// Actor 紧凑行：折叠时一行展示名字与最多两个数值 meter，展开后显示词条与全部字段。
function ActorRow({ actorId, actor, schema, expanded, onToggle }: { actorId: string; actor: Record<string, unknown>; schema?: ActorStateSchemaSnapshot; expanded: boolean; onToggle: () => void }) {
  const { t } = useTranslation()
  const name = actorName(actorId, actor)
  const template = actorTemplate(actor, schema)
  const fields = actorFieldEntries(actor, template?.fields)
  const traits = actorTraits(actor)
  const meters = fields.filter(({ field, value }) => meterPercent(field, value) !== null).slice(0, 2)

  return (
    <article aria-label={name} className="min-w-0 overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <button type="button" aria-expanded={expanded} className="flex w-full min-w-0 items-center gap-2 px-3 py-2.5 text-left transition-colors hover:bg-[var(--nova-hover)]" onClick={onToggle}>
        <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`} />
        <span className="min-w-0 truncate text-xs font-medium text-[var(--nova-text)]">{name}</span>
        <span className="ml-auto flex shrink-0 items-center gap-2.5">
          {meters.map(({ field, value }) => (
            <MiniMeter key={field.name} field={field} value={value} />
          ))}
        </span>
      </button>
      {expanded ? (
        <div className="border-t border-[var(--nova-border)] px-3 pb-3">
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
          <div className="pt-3">
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
        </div>
      ) : null}
    </article>
  )
}

function meterPercent(field: ActorStateField, value: unknown): number | null {
  if (typeof value !== 'number' || typeof field.min !== 'number' || typeof field.max !== 'number' || field.max <= field.min) return null
  return Math.min(100, Math.max(0, ((value - field.min) / (field.max - field.min)) * 100))
}

function MiniMeter({ field, value }: { field: ActorStateField; value: unknown }) {
  const percent = meterPercent(field, value)
  if (percent === null || typeof value !== 'number') return null
  return (
    <span className="flex items-center gap-1.5" title={`${field.name} ${value}/${field.max}`}>
      <span className="max-w-[52px] truncate text-[9px] text-[var(--nova-text-faint)]">{field.name}</span>
      <span className="h-1 w-9 overflow-hidden rounded-full bg-[var(--nova-surface-3)]" aria-hidden="true">
        <span className="block h-full rounded-full bg-[var(--director-live)]" style={{ width: `${percent}%` }} />
      </span>
      <span className="font-mono text-[10px] tabular-nums text-[var(--nova-text-muted)]">{value}</span>
    </span>
  )
}

function ActorField({ field, value }: { field: ActorStateField; value: unknown }) {
  const percent = meterPercent(field, value)
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
      {percent !== null ? (
        <div className="mt-2 h-1 overflow-hidden rounded-full bg-[var(--nova-surface-3)]" aria-hidden="true">
          <div className="h-full rounded-full bg-[var(--director-live)] transition-[width] duration-300 motion-reduce:transition-none" style={{ width: `${percent}%` }} />
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

function defaultExpandedActors(actors: Array<[string, Record<string, unknown>]>): string[] {
  const protagonist = actors.find(([, actor]) => actor.role === 'protagonist')
  const first = protagonist?.[0] || actors[0]?.[0] || ''
  return first ? [first] : []
}
