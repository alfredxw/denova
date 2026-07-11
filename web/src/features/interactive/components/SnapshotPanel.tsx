import { Activity, Compass, Copy, Flag, MapPin, Package, Plus, RefreshCw, Sparkles, Tag, UserRoundCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { Snapshot } from '../types'

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

export function SnapshotPanel({ snapshot, loading = false }: { snapshot: Snapshot | null; loading?: boolean }) {
  const { t } = useTranslation()
  const state = snapshot?.state || {}
  const onStage = asArray(state.on_stage)
  const events = asArray(state.events)
  const characters = state.characters && typeof state.characters === 'object'
    ? Object.entries(state.characters as Record<string, unknown>)
    : []
  const scene = isPlainObject(state.scene) ? state.scene : {}
  const inventory = isPlainObject(state.inventory) ? state.inventory : null
  const resources = isPlainObject(state.resources) ? state.resources : null
  const worldFlags = asArray(state.world_flags)
  const rules = asArray(state.rules)
  const threads = asArray(state.threads)
  const actionSpace = asArray(state.action_space)
  const location = pickString(state, ['location', 'place', 'scene', '地点'])
    || pickString(scene, ['location', 'place', 'name', '地点', '场景'])
  const time = pickString(state, ['time', 'moment', '时间'])
  const pov = pickString(state, ['pov', 'viewpoint', '视角'])
  const sceneEntries = Object.entries(scene).filter(([key]) => !SCENE_METRIC_KEYS.has(key))
  const stateStatus = snapshot?.current_turn?.state_status
  const directorPlan = snapshot?.director_plan
  const directorStatus = snapshot?.director_plan_status
  const directorMetadata = directorPlan?.metadata
  const directorRun = directorStatus || directorMetadata?.last_run
  const ruleResolution = snapshot?.current_turn?.rule_resolution
  const ruleRequest = ruleResolution?.request
  const ruleResult = ruleResolution?.result
  const stateChanges = ruleResult?.state_changes || []
  const stateConsumption = ruleResolution?.state_consumption
  const terminalCandidate = ruleResolution?.terminal_candidate
  const terminalOutcome = snapshot?.current_turn?.terminal_outcome
  const hasRuleAudit = !!ruleResolution || !!terminalOutcome

  return (
    <aside className="nova-sidebar flex h-full min-w-0 flex-col border-l p-4">
      <div className="mb-3 flex h-8 items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('snapshot.title')}</h2>
          <div className="text-[11px] text-[var(--nova-text-faint)]">{t('snapshot.subtitle')}</div>
        </div>
        <div className="flex items-center gap-1.5">
          {loading ? <Badge variant="outline" className="gap-1 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]"><RefreshCw className="h-3 w-3 animate-spin" />{t('common.loading')}</Badge> : null}
          {stateStatus === 'pending' ? <Badge variant="outline" className="border-[var(--nova-accent)]/40 bg-[var(--nova-accent)]/10 text-[var(--nova-accent)]">{t('snapshot.syncing')}</Badge> : null}
          {stateStatus === 'failed' ? <Badge variant="outline" className="border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]">{t('snapshot.syncFailed')}</Badge> : null}
          <Badge variant="outline" className="border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">{formatBranchName(snapshot?.branch_id, t)}</Badge>
        </div>
      </div>
      {stateStatus === 'failed' && snapshot?.current_turn?.state_error ? (
        <div className="mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">
          {snapshot.current_turn.state_error}
        </div>
      ) : null}
      <ScrollArea className="min-h-0 flex-1 pr-1">
        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <MapPin className="h-3.5 w-3.5" />
            {t('snapshot.currentScene')}
          </div>
          <div className="grid grid-cols-[repeat(auto-fit,minmax(72px,1fr))] gap-2">
            <SnapshotMetric label={t('snapshot.field.place')} value={location || t('snapshot.noRecord')} />
            <SnapshotMetric label={t('snapshot.field.time')} value={time || t('snapshot.noRecord')} />
            <SnapshotMetric label={t('snapshot.field.pov')} value={pov || t('snapshot.noRecord')} />
          </div>
          {sceneEntries.length ? (
            <div className="mt-3 border-t border-[var(--nova-border)] pt-3">
              <StateValue value={Object.fromEntries(sceneEntries)} />
            </div>
          ) : null}
        </section>

        <section className={panelSectionClass}>
          <div className="mb-2 text-xs font-semibold text-[var(--nova-text-muted)]">{t('snapshot.sceneNotes')}</div>
          <div className={`${panelCardClass} px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]`}>
            {t('snapshot.sceneNotesPlaceholder')}
          </div>
        </section>

        {directorStatus || directorPlan ? (
          <section className={panelSectionClass}>
            <div className={sectionTitleClass}>
              <Sparkles className="h-3.5 w-3.5" />
              {t('snapshot.director.title')}
            </div>
            <div className="grid grid-cols-[repeat(auto-fit,minmax(72px,1fr))] gap-2">
              <SnapshotMetric label={t('snapshot.director.status')} value={directorRun?.status || t('snapshot.noRecord')} />
              <SnapshotMetric label={t('snapshot.director.docs')} value={directorStatus ? `${directorStatus.completed_docs}/${directorStatus.planned_docs}` : String(Object.keys(directorMetadata?.docs || {}).length || 1)} />
              <SnapshotMetric label={t('snapshot.director.branchPlanningTurns')} value={String(directorMetadata?.branch_planning_turns || 5)} />
            </div>
            {directorRun ? (
              <div className={`${panelCardClass} mt-3 p-2 text-xs text-[var(--nova-text-muted)]`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.director.lastRun')}</div>
                <StateValue value={compactRecord({
                  status: directorRun.status,
                  summary: directorRun.summary,
                  error: directorRun.error,
                  source_turn_id: directorRun.source_turn_id,
                  updated_at: directorRun.updated_at,
                })} />
              </div>
            ) : null}
          </section>
        ) : null}

        {hasRuleAudit ? (
          <section className={panelSectionClass}>
            <div className={sectionTitleClass}>
              <Activity className="h-3.5 w-3.5" />
              {t('snapshot.ruleAudit.title')}
            </div>
            <div className="grid grid-cols-[repeat(auto-fit,minmax(72px,1fr))] gap-2">
              <SnapshotMetric label={t('snapshot.ruleAudit.intent')} value={ruleRequest?.intent || t('snapshot.noRecord')} />
              <SnapshotMetric label={t('snapshot.ruleAudit.difficulty')} value={ruleRequest?.difficulty || t('snapshot.noRecord')} />
              <SnapshotMetric label={t('snapshot.ruleAudit.outcome')} value={ruleResult?.outcome || t('snapshot.noRecord')} />
            </div>
            {ruleRequest ? (
              <div className={`${panelCardClass} mt-3 p-2 text-xs text-[var(--nova-text-muted)]`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.ruleAudit.request')}</div>
                <StateValue value={compactRecord({
                  action: ruleRequest.action,
                  challenge: ruleRequest.challenge,
                  cost: ruleRequest.cost,
                  state: ruleRequest.state,
                  rule: ruleRequest.rule,
                  bonuses: ruleRequest.bonuses,
                })} />
              </div>
            ) : null}
            {ruleRequest?.adjudication ? (
              <div className={`${panelCardClass} mt-3 p-2 text-xs text-[var(--nova-text-muted)]`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.ruleAudit.adjudication')}</div>
                <StateValue value={compactRecord({
                  reason: ruleRequest.adjudication.reason,
                  stakes: ruleRequest.adjudication.stakes,
                  difficulty_reason: ruleRequest.adjudication.difficulty_reason,
                  roll_mode_reason: ruleRequest.adjudication.roll_mode_reason,
									state_refs: ruleRequest.adjudication.state_refs,
                })} />
              </div>
            ) : null}
            <div className="mt-3 space-y-2 text-xs text-[var(--nova-text-muted)]">
              {ruleResult ? (
                <article className={`${panelCardClass} p-2`}>
                  <div className="mb-1 flex items-start justify-between gap-2">
                    <div className="min-w-0 font-medium text-[var(--nova-text)]">{ruleResult.label || ruleRequest?.challenge || t('snapshot.ruleAudit.result')}</div>
                    <Badge variant="outline" className={`h-5 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 text-[10px] ${ruleOutcomeClass(ruleResult.outcome)}`}>
                      {ruleResult.outcome}
                    </Badge>
                  </div>
                  <StateValue value={compactRecord({
                    dice: ruleResult.dice,
                    roll_mode: ruleResult.roll_mode,
                    rolls: ruleResult.rolls,
                    kept_roll: ruleResult.kept_roll,
                    base_target: ruleResult.base_target,
                    bonus_total: ruleResult.bonus_total,
                    bonus_details: ruleResult.bonus_details,
                    target: ruleResult.target,
                    total: ruleResult.total,
                    result: ruleResult.result,
                    constraints: ruleResult.constraints,
                  })} />
                </article>
              ) : <div className="text-xs text-[var(--nova-text-muted)]">{t('snapshot.ruleAudit.noResults')}</div>}
            </div>
            {ruleResolution?.rule_constraints?.length ? (
              <div className="mt-3 text-xs text-[var(--nova-text-muted)]">
                <LabeledList label={t('snapshot.ruleAudit.constraints')} items={ruleResolution.rule_constraints} empty={t('snapshot.ruleAudit.noConstraints')} />
              </div>
            ) : null}
            {stateChanges.length ? (
              <div className="mt-3 text-xs text-[var(--nova-text-muted)]">
                <LabeledList label={t('snapshot.ruleAudit.stateChanges')} items={stateChanges} empty={t('snapshot.ruleAudit.noStateOps')} />
              </div>
            ) : null}
            {stateConsumption ? (
              <div className={`${panelCardClass} mt-3 p-2 text-xs text-[var(--nova-text-muted)]`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.ruleAudit.stateConsumption')}</div>
                <StateValue value={compactRecord({
                  status: stateConsumption.status,
                  mode: stateConsumption.mode,
                  applied_actor_ops: stateConsumption.applied_actor_ops,
                  warnings: stateConsumption.warnings,
                })} />
              </div>
            ) : null}
            {terminalCandidate || terminalOutcome ? (
              <div className={`${panelCardClass} mt-3 p-2 text-xs text-[var(--nova-text-muted)]`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.ruleAudit.terminal')}</div>
                <StateValue value={compactRecord({
                  candidate_type: terminalCandidate?.type,
                  candidate_reason: terminalCandidate?.reason,
                  check_id: terminalCandidate?.check_id,
                  final_type: terminalOutcome?.type,
                  final_reason: terminalOutcome?.reason,
                  final_narrative_summary: terminalOutcome?.final_narrative_summary,
                  restart_suggestions: terminalOutcome?.restart_suggestions,
                })} />
              </div>
            ) : null}
          </section>
        ) : null}

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <UserRoundCheck className="h-3.5 w-3.5" />
            {t('snapshot.onStage')}
          </div>
          <div className="flex flex-wrap gap-1.5 text-sm text-[var(--nova-text-muted)]">
            {onStage.length ? onStage.map((name) => <Badge key={String(name)} className="border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]" variant="secondary">{String(name)}</Badge>) : t('snapshot.noOnStage')}
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Compass className="h-3.5 w-3.5" />
            {t('snapshot.actionSpace')}
          </div>
          <CompactList items={actionSpace} empty={t('snapshot.noActionSpace')} />
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Activity className="h-3.5 w-3.5" />
            {t('snapshot.characterStates')}
          </div>
          <div className="space-y-2 text-xs text-[var(--nova-text-muted)]">
            {characters.length ? characters.map(([name, state]) => (
              <div key={name} className={`${panelCardClass} p-2`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{name}</div>
                <StateValue value={state} />
              </div>
            )) : t('snapshot.noCharacterStates')}
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Package className="h-3.5 w-3.5" />
            {t('snapshot.inventoryResources')}
          </div>
          <div className="space-y-2 text-xs text-[var(--nova-text-muted)]">
            {inventory ? (
              <div className={`${panelCardClass} p-2`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.field.inventory')}</div>
                <StateValue value={inventory} />
              </div>
            ) : null}
            {resources ? (
              <div className={`${panelCardClass} p-2`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{t('snapshot.field.resources')}</div>
                <StateValue value={resources} />
              </div>
            ) : null}
            {!inventory && !resources ? t('snapshot.noInventoryResources') : null}
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Flag className="h-3.5 w-3.5" />
            {t('snapshot.rulesThreads')}
          </div>
          <div className="space-y-3 text-xs text-[var(--nova-text-muted)]">
            <LabeledList label={t('snapshot.worldRules')} items={[...worldFlags, ...rules]} empty={t('snapshot.noWorldRules')} />
            <LabeledList label={t('snapshot.unresolvedThreads')} items={threads} empty={t('snapshot.noThreads')} />
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Sparkles className="h-3.5 w-3.5" />
            {t('snapshot.keyEvents')}
          </div>
          <div className="space-y-2 text-xs text-[var(--nova-text-muted)]">
            {events.length ? events.map((event, index) => (
              <EventItem key={index} event={event} index={index} />
            )) : t('snapshot.noKeyEvents')}
          </div>
        </section>

        <section className={`${panelSectionClass} mb-0`}>
          <div className="mb-3 text-xs font-semibold text-[var(--nova-text-muted)]">{t('snapshot.quickActions')}</div>
          <div className="grid grid-cols-2 gap-2">
            <InspectorAction icon={Plus} label={t('snapshot.newScene')} />
            <InspectorAction icon={Copy} label={t('snapshot.copyLink')} />
            <InspectorAction icon={Compass} label={t('snapshot.addBranch')} />
            <InspectorAction icon={Tag} label={t('snapshot.setTags')} />
          </div>
        </section>
      </ScrollArea>
    </aside>
  )
}

function InspectorAction({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <button type="button" className="nova-nav-item flex h-9 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]">
      <Icon className="h-3.5 w-3.5" />
      <span>{label}</span>
    </button>
  )
}

function SnapshotMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-2">
      <div className="text-[10px] text-[var(--nova-text-faint)]">{label}</div>
      <div className="truncate text-xs font-medium text-[var(--nova-text)]" title={value}>{value}</div>
    </div>
  )
}

function pickString(source: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function StateValue({ value }: { value: unknown }) {
  const { t } = useTranslation()
  if (isPlainObject(value)) {
    const entries = Object.entries(value)
    if (!entries.length) return <div className="text-[var(--nova-text-faint)]">{t('snapshot.noRecord')}</div>
    return (
      <dl className="grid gap-1.5">
        {entries.map(([key, item]) => (
          <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
            <dt className="truncate text-[var(--nova-text-faint)]" title={formatLabel(key, t)}>{formatLabel(key, t)}</dt>
            <dd className="min-w-0 text-[var(--nova-text-muted)]">{renderReadableValue(item, t)}</dd>
          </div>
        ))}
      </dl>
    )
  }
  return <div className="whitespace-pre-wrap text-[var(--nova-text-muted)]">{formatScalar(value, t)}</div>
}

function EventItem({ event, index }: { event: unknown; index: number }) {
  const { t } = useTranslation()
  if (!isPlainObject(event)) {
    return (
      <div className={`${panelCardClass} p-2 text-[var(--nova-text-muted)]`}>
        {formatScalar(event, t)}
      </div>
    )
  }

  const title = pickEventTitle(event, index, t)
  const description = pickString(event, ['description', 'summary', 'content', 'text', 'event', '事件', '描述'])
  const detailEntries = Object.entries(event).filter(([key]) => !EVENT_PRIMARY_KEYS.has(key))

  return (
    <article className={`${panelCardClass} p-2`}>
      <div className="mb-1 flex items-start justify-between gap-2">
        <div className="min-w-0 font-medium text-[var(--nova-text)]">{title}</div>
        {typeof event.type === 'string' && event.type.trim() ? (
          <Badge variant="outline" className="h-5 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 text-[10px] text-[var(--nova-text-muted)]">
            {event.type.trim()}
          </Badge>
        ) : null}
      </div>
      {description ? <div className="mb-2 whitespace-pre-wrap text-[var(--nova-text-muted)]">{description}</div> : null}
      {detailEntries.length ? (
        <dl className="grid gap-1.5">
          {detailEntries.map(([key, value]) => (
            <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
              <dt className="truncate text-[var(--nova-text-faint)]" title={formatLabel(key, t)}>{formatLabel(key, t)}</dt>
              <dd className="min-w-0 text-[var(--nova-text-muted)]">{renderReadableValue(value, t)}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </article>
  )
}

function CompactList({ items, empty }: { items: unknown[]; empty: string }) {
  const { t } = useTranslation()
  if (!items.length) return <div className="text-xs text-[var(--nova-text-muted)]">{empty}</div>
  return (
    <div className="space-y-1.5 text-xs text-[var(--nova-text-muted)]">
      {items.map((item, index) => (
        <div key={index} className={`${panelCardClass} px-2 py-1.5`}>
          {renderReadableValue(item, t)}
        </div>
      ))}
    </div>
  )
}

function LabeledList({ label, items, empty }: { label: string; items: unknown[]; empty: string }) {
  return (
    <div>
      <div className="mb-1 text-[10px] font-medium text-[var(--nova-text-faint)]">{label}</div>
      <CompactList items={items} empty={empty} />
    </div>
  )
}

function renderReadableValue(value: unknown, t: (key: string, options?: Record<string, unknown>) => string): ReactNode {
  if (Array.isArray(value)) {
    if (!value.length) return <span className="text-[var(--nova-text-faint)]">{t('snapshot.empty')}</span>
    if (value.every((item) => !isPlainObject(item) && !Array.isArray(item))) {
      return (
        <div className="flex flex-wrap gap-1">
          {value.map((item, index) => (
            <Badge key={index} variant="secondary" className="border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
              {formatScalar(item, t)}
            </Badge>
          ))}
        </div>
      )
    }
    return (
      <div className="space-y-1">
        {value.map((item, index) => (
          <div key={index} className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1">
            {renderReadableValue(item, t)}
          </div>
        ))}
      </div>
    )
  }
  if (isPlainObject(value)) {
    const entries = Object.entries(value)
    if (!entries.length) return <span className="text-[var(--nova-text-faint)]">{t('snapshot.empty')}</span>
    return (
      <div className="space-y-1">
        {entries.map(([key, item]) => (
          <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-1">
            <span className="text-[var(--nova-text-faint)]">{formatLabel(key, t)}</span>
            <span className="min-w-0">{renderReadableValue(item, t)}</span>
          </div>
        ))}
      </div>
    )
  }
  return <span className="whitespace-pre-wrap">{formatScalar(value, t)}</span>
}

function pickEventTitle(event: Record<string, unknown>, index: number, t: (key: string, options?: Record<string, unknown>) => string) {
  const title = pickString(event, ['title', 'name', 'flag', 'event', '事件名'])
  if (title) return title
  return t('snapshot.eventFallback', { index: index + 1 })
}

function formatLabel(key: string, t: (key: string) => string) {
  const normalized = key.trim()
  const directKey = `snapshot.field.${normalized}`
  const directLabel = t(directKey)
  if (directLabel !== directKey) return directLabel
  if (/^[a-z][a-z0-9_]*$/i.test(normalized) && normalized.includes('_')) {
    return normalized.split('_').map((part) => {
      const partKey = `snapshot.field.${part}`
      const partLabel = t(partKey)
      return partLabel === partKey ? part : partLabel
    }).join(' ')
  }
  return normalized
}

function formatScalar(value: unknown, t: (key: string) => string) {
  if (value === null || value === undefined) return t('snapshot.noRecord')
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

function compactRecord(source: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(source).filter(([, value]) => {
    if (value === null || value === undefined) return false
    if (typeof value === 'string') return value.trim() !== ''
    if (Array.isArray(value)) return value.length > 0
    return true
  }))
}

function ruleOutcomeClass(outcome: string) {
  if (outcome.includes('success')) return 'text-[var(--nova-success)]'
  if (outcome.includes('failure') || outcome === 'error') return 'text-[var(--nova-danger)]'
  return 'text-[var(--nova-text-muted)]'
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function formatBranchName(branchId: string | undefined, t: (key: string) => string) {
  if (!branchId || branchId === 'main') return t('branchTimeline.mainBranch')
  if (/^branch[_-]?\d+$/i.test(branchId)) return `${t('branchTimeline.title')} ${branchId.replace(/^branch[_-]?/i, '')}`
  return branchId
}

const EVENT_PRIMARY_KEYS = new Set(['title', 'name', 'flag', 'event', '事件名', 'description', 'summary', 'content', 'text', '事件', '描述', 'type'])
const SCENE_METRIC_KEYS = new Set(['location', 'place', 'name', '地点', '场景'])

const panelSectionClass = 'mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3'
const panelCardClass = 'rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]'
const sectionTitleClass = 'mb-3 flex items-center gap-2 text-xs font-semibold text-[var(--nova-text-muted)]'
