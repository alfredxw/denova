import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { AlertCircle, ChevronDown, ChevronUp, CircleCheck, Globe2, Loader2, PanelRight, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia } from '@/components/ui/empty'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import type { ActorStateField, Snapshot } from '../../types'
import { StateValue } from '../director-console/shared'
import type { StoryStateDisplayPreference } from './display-preference'
import { StateDisplayPreferenceMenu } from './StateDisplayPreferenceMenu'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  buildStoryStateModel,
  humanizeStateKey,
  statePathLabel,
  visibleActorTraits,
  type StoryStateChange,
} from './model'

const WORLD_STATE_TAB = '__world_state__'

type StoryStatePanelMode = 'collapsed' | 'preview' | 'expanded'

const PANEL_MODE_BY_PREFERENCE: Record<StoryStateDisplayPreference, StoryStatePanelMode> = {
  preview: 'preview',
  expanded: 'expanded',
  collapsed: 'collapsed',
  'director-only': 'collapsed',
}

type ActorFieldEntry = ReturnType<typeof actorFieldEntries>[number]
type BoundedNumericFieldEntry = ActorFieldEntry & {
  field: ActorStateField & { min: number; max: number }
  value: number
}
type StateFieldLayout = 'compact' | 'wide' | 'structured'

interface LedgerStateField {
  id: string
  label: string
  value: unknown
  fieldPath: string
  numeric: boolean
  changes: StoryStateChange[]
  layout: StateFieldLayout
}

interface StoryStateLedgerProps {
  snapshot: Snapshot | null
  displayPreference: StoryStateDisplayPreference
  onDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onOpenDirectorState?: () => void
}

export function StoryStateLedger({ snapshot, displayPreference, onDisplayPreferenceChange, onOpenDirectorState }: StoryStateLedgerProps) {
  const { t } = useTranslation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const actorTabs = useMemo(() => model.actors.map(([actorId, actor]) => ({ id: actorId, name: actorName(actorId, actor) })), [model.actors])
  const [selectedTab, setSelectedTab] = useState(actorTabs[0]?.id || WORLD_STATE_TAB)
  const turnKey = `${snapshot?.story_id || ''}:${snapshot?.branch_id || ''}:${snapshot?.current_turn?.id || ''}`
  const [panelMode, setPanelMode] = useState<StoryStatePanelMode>(PANEL_MODE_BY_PREFERENCE[displayPreference])
  const [previewOverflowing, setPreviewOverflowing] = useState(false)
  const previewViewportRef = useRef<HTMLDivElement>(null)
  const previewContentRef = useRef<HTMLDivElement>(null)
  const contentId = useId()

  useEffect(() => {
    if (selectedTab === WORLD_STATE_TAB || actorTabs.some((actor) => actor.id === selectedTab)) return
    setSelectedTab(actorTabs[0]?.id || WORLD_STATE_TAB)
  }, [actorTabs, selectedTab])

  useEffect(() => {
    setPanelMode(PANEL_MODE_BY_PREFERENCE[displayPreference])
  }, [displayPreference, turnKey])

  useLayoutEffect(() => {
    if (panelMode !== 'preview') {
      setPreviewOverflowing(false)
      return
    }

    const viewport = previewViewportRef.current
    const content = previewContentRef.current
    if (!viewport || !content) return

    const updateOverflow = () => {
      setPreviewOverflowing(content.scrollHeight > viewport.clientHeight + 1)
    }

    updateOverflow()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(updateOverflow)
    observer.observe(viewport)
    observer.observe(content)
    return () => observer.disconnect()
  }, [panelMode, selectedTab, turnKey])

  if (!model.hasState || displayPreference === 'director-only') return null

  const collapsed = panelMode === 'collapsed'
  const open = !collapsed

  return (
    <Collapsible
      open={open}
      onOpenChange={(nextOpen) => setPanelMode(nextOpen ? 'preview' : 'collapsed')}
      asChild
    >
      <section
        aria-label={t('storyStage.state.current')}
        data-state-panel-mode={panelMode}
        className="story-state-ledger mt-3 overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--story-state-canvas)]"
      >
        <header className="flex h-11 min-w-0 items-center gap-2 border-b border-[var(--nova-border)] px-2.5">
          <StatusIndicator status={snapshot?.current_turn?.state_status} />
          <div className="flex min-w-0 flex-1 items-baseline gap-2">
            <h2 className="shrink-0 text-[13px] font-semibold tracking-tight text-[var(--nova-text)]">{t('storyStage.state.current')}</h2>
            <p className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{turnStatusLabel(snapshot, t)}</p>
          </div>
          <StateDisplayPreferenceMenu value={displayPreference} onChange={onDisplayPreferenceChange} compact />
          {onOpenDirectorState ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onOpenDirectorState}
              title={t('storyStage.state.openDirector')}
              aria-label={t('storyStage.state.openDirector')}
            >
              <PanelRight data-icon="inline-start" />
              <span className="story-state-ledger__director-label">{t('storyStage.state.openDirector')}</span>
            </Button>
          ) : null}
          <CollapsibleTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={collapsed ? t('storyStage.state.expand') : t('storyStage.state.collapse')}
              title={collapsed ? t('storyStage.state.expand') : t('storyStage.state.collapse')}
            >
              {collapsed ? <ChevronDown data-icon="inline-start" /> : <ChevronUp data-icon="inline-start" />}
            </Button>
          </CollapsibleTrigger>
        </header>

        <CollapsibleContent>
          <div className="story-state-ledger__content-shell" data-panel-mode={panelMode}>
            <div
              ref={previewViewportRef}
              id={contentId}
              className="story-state-ledger__content-viewport"
            >
              <div ref={previewContentRef} className="story-state-ledger__content">
                <Tabs value={selectedTab} onValueChange={setSelectedTab} className="gap-0">
                  <StateEntityTabs actors={actorTabs} />
                  {model.actors.map(([actorId, actor]) => (
                    <TabsContent key={actorId} value={actorId} className="mt-0">
                      <ActorLedger
                        actor={actor}
                        snapshot={snapshot}
                        changes={model.changes.filter((change) => change.actorId === actorId)}
                      />
                    </TabsContent>
                  ))}
                  <TabsContent value={WORLD_STATE_TAB} className="mt-0">
                    <WorldLedger
                      facts={model.worldFacts}
                      changes={model.changes.filter((change) => !change.actorId)}
                    />
                  </TabsContent>
                </Tabs>
              </div>
            </div>
            {panelMode === 'preview' && previewOverflowing ? (
              <div className="story-state-ledger__preview-action">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  aria-controls={contentId}
                  aria-expanded="false"
                  onClick={() => setPanelMode('expanded')}
                >
                  <ChevronDown data-icon="inline-start" />
                  {t('storyStage.state.expandAll')}
                </Button>
              </div>
            ) : null}
            {panelMode === 'expanded' ? (
              <div className="story-state-ledger__expanded-action">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  aria-controls={contentId}
                  aria-expanded="true"
                  onClick={() => setPanelMode('preview')}
                >
                  <ChevronUp data-icon="inline-start" />
                  {t('storyStage.state.collapseToPreview')}
                </Button>
              </div>
            ) : null}
          </div>
        </CollapsibleContent>
      </section>
    </Collapsible>
  )
}

function StatusIndicator({ status }: { status?: 'pending' | 'ready' | 'failed' }) {
  const { t } = useTranslation()
  if (status === 'pending') {
    return (
      <span
        aria-label={t('storyStage.state.syncingShort')}
        title={t('storyStage.state.syncingShort')}
        className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-[var(--story-state-pending-soft)] text-[var(--story-state-pending)]"
      >
        <Loader2 aria-hidden="true" className="size-3.5 animate-spin motion-reduce:animate-none" />
      </span>
    )
  }
  if (status === 'failed') {
    return (
      <span
        aria-label={t('storyStage.state.failedShort')}
        title={t('storyStage.state.failedShort')}
        className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-[var(--story-state-negative-soft)] text-[var(--story-state-negative)]"
      >
        <AlertCircle aria-hidden="true" className="size-3.5" />
      </span>
    )
  }
  return (
    <span
      aria-label={t('storyStage.state.readyShort')}
      title={t('storyStage.state.readyShort')}
      className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-[var(--story-state-positive-soft)] text-[var(--story-state-positive)]"
    >
      <CircleCheck aria-hidden="true" className="size-3.5" />
    </span>
  )
}

function StateEntityTabs({ actors }: { actors: Array<{ id: string; name: string }> }) {
  const { t } = useTranslation()
  return (
    <div className="story-state-ledger__tabs-scroll overflow-x-auto overflow-y-hidden border-b border-[var(--nova-border)] px-2.5 py-1.5">
      <TabsList
        aria-label={t('storyStage.state.tabs')}
        className="story-state-ledger__tabs-list w-max max-w-none justify-start"
      >
        {actors.map((actor) => (
          <TabsTrigger
            key={actor.id}
            value={actor.id}
            title={actor.name}
            className="min-w-20 max-w-40 flex-none"
          >
            <span className="truncate">{actor.name}</span>
          </TabsTrigger>
        ))}
        <TabsTrigger
          value={WORLD_STATE_TAB}
          className="min-w-20 flex-none"
        >
          <Globe2 data-icon="inline-start" />
          <span>{t('storyStage.state.world')}</span>
        </TabsTrigger>
      </TabsList>
    </div>
  )
}

function ActorLedger({ actor, snapshot, changes }: { actor: Record<string, unknown>; snapshot: Snapshot | null; changes: StoryStateChange[] }) {
  const { t } = useTranslation()
  const template = actorTemplate(actor, snapshot?.actor_state_schema)
  const fields = actorFieldEntries(actor, template?.fields)
  const traits = visibleActorTraits(actor)
  const metricFields = fields.filter(isBoundedNumericFieldEntry)
  const detailFields = fields.filter((entry) => !isBoundedNumericFieldEntry(entry))
  const ledgerFields = detailFields.map(({ field, value }): LedgerStateField => {
    const resolvedValue = value ?? field.default ?? null
    const fieldPath = actorFieldPaths(field)[0] || field.name
    const fieldChanges = actorFieldChanges(changes, field)
    return {
      id: field.id || field.path || field.name,
      label: field.name,
      value: resolvedValue,
      fieldPath,
      numeric: typeof resolvedValue === 'number',
      changes: fieldChanges,
      layout: stateFieldLayout(resolvedValue, fieldChanges),
    }
  })

  return (
    <div>
      {traits.length > 0 ? <ActorTraits traits={traits} /> : null}
      {metricFields.length > 0 ? <NumericStateMetrics entries={metricFields} changes={changes} /> : null}
      {ledgerFields.length > 0 ? <StateFieldCollection items={ledgerFields} /> : null}
      {metricFields.length === 0 && detailFields.length === 0 ? <StateSectionEmpty label={t('storyStage.state.actorEmpty')} /> : null}
    </div>
  )
}

function NumericStateMetrics({ entries, changes }: { entries: BoundedNumericFieldEntry[]; changes: StoryStateChange[] }) {
  const { t } = useTranslation()
  return (
    <div role="group" aria-label={t('storyStage.state.numericStatus')} className="story-state-ledger__metric-grid story-state-ledger__flow-grid">
      {entries.map(({ field, value }) => (
        <NumericStateMetric
          key={field.id || field.path || field.name}
          field={field}
          value={value}
          fieldPath={actorFieldPaths(field)[0] || field.name}
          changes={actorFieldChanges(changes, field)}
        />
      ))}
    </div>
  )
}

function NumericStateMetric({ field, value, fieldPath, changes }: { field: BoundedNumericFieldEntry['field']; value: number; fieldPath: string; changes: StoryStateChange[] }) {
  const { t } = useTranslation()
  const progress = normalizedProgress(value, field.min, field.max)
  const valueLabel = `${formatMetricNumber(value)} / ${formatMetricNumber(field.max)}`
  return (
    <section data-state-metric className="min-w-0 bg-[var(--story-state-panel)] px-2.5 py-1.5">
      <div className="mb-1 flex min-w-0 items-baseline justify-between gap-2">
        <h4 className="truncate text-[11px] font-medium text-[var(--nova-text-faint)]" title={field.name}>{field.name}</h4>
        <span className="shrink-0 font-mono text-[11px] font-semibold tabular-nums text-[var(--nova-text)]">{valueLabel}</span>
      </div>
      <Progress
        value={progress}
        aria-label={t('storyStage.state.metricProgress', {
          label: field.name,
          value: formatMetricNumber(value),
          min: formatMetricNumber(field.min),
          max: formatMetricNumber(field.max),
        })}
        aria-valuetext={valueLabel}
        className="story-state-ledger__metric-progress h-1.5"
      />
      <InlineFieldChanges changes={changes} fieldPath={fieldPath} numeric variant="metric" />
    </section>
  )
}

function ActorTraits({ traits }: { traits: ReturnType<typeof visibleActorTraits> }) {
  return (
    <div className="flex min-w-0 flex-wrap gap-1 border-b border-[var(--nova-border)] bg-[var(--story-state-panel)] px-2.5 py-1.5">
      {traits.map((trait) => (
        <Badge
          key={`${trait.pool_id}:${trait.trait_id}`}
          variant="secondary"
          title={trait.summary || trait.name}
          className="max-w-32 truncate"
        >
          {trait.name}
        </Badge>
      ))}
    </div>
  )
}

function WorldLedger({ facts, changes }: { facts: Array<[string, unknown]>; changes: StoryStateChange[] }) {
  const { t } = useTranslation()

  if (facts.length === 0) return <StateSectionEmpty label={t('storyStage.state.worldEmpty')} />

  const ledgerFields = facts.map(([key, value]): LedgerStateField => {
    const fieldChanges = worldFieldChanges(changes, key)
    return {
      id: key,
      label: humanizeStateKey(key),
      value,
      fieldPath: key,
      numeric: typeof value === 'number',
      changes: fieldChanges,
      layout: stateFieldLayout(value, fieldChanges),
    }
  })

  return <StateFieldCollection items={ledgerFields} />
}

function StateFieldCollection({ items }: { items: LedgerStateField[] }) {
  const groups: Record<StateFieldLayout, LedgerStateField[]> = { compact: [], wide: [], structured: [] }
  items.forEach((item) => groups[item.layout].push(item))

  return (
    <>
      {(['compact', 'wide'] as const).map((layout) => groups[layout].length > 0 ? (
        <div
          key={layout}
          data-state-field-group={layout}
          className={cn(
            'story-state-ledger__field-grid',
            layout === 'compact' ? 'story-state-ledger__flow-grid' : 'story-state-ledger__stack-grid',
          )}
        >
          {groups[layout].map((item) => <StateField key={item.id} item={item} />)}
        </div>
      ) : null)}
      {groups.structured.length > 0 ? (
        <StructuredStateTabs items={groups.structured} />
      ) : null}
    </>
  )
}

function StructuredStateTabs({ items }: { items: LedgerStateField[] }) {
  const { t } = useTranslation()
  const [selectedTab, setSelectedTab] = useState(items[0]?.id || '')
  const activeTab = items.some((item) => item.id === selectedTab) ? selectedTab : items[0]?.id || ''

  return (
    <Tabs value={activeTab} onValueChange={setSelectedTab} className="gap-0">
      <div className="story-state-ledger__tabs-scroll overflow-x-auto overflow-y-hidden border-b border-[var(--nova-border-soft)] bg-[var(--story-state-panel)] px-3">
        <TabsList
          variant="line"
          aria-label={t('storyStage.state.structuredTabs')}
          className="story-state-ledger__subtabs-list h-8 w-max max-w-none justify-start p-0"
        >
          {items.map((item) => (
            <TabsTrigger
              key={item.id}
              value={item.id}
              title={item.label}
              className="h-8 min-w-16 max-w-40 flex-none rounded-none px-2 text-[11px] after:bottom-0"
            >
              <span className="truncate">{item.label}</span>
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      {items.map((item) => (
        <TabsContent key={item.id} value={item.id} className="mt-0">
          <div data-state-field-group="structured" className="story-state-ledger__structured-panel">
            <StateField item={item} hideLabel />
          </div>
        </TabsContent>
      ))}
    </Tabs>
  )
}

function StateField({ item, hideLabel = false }: { item: LedgerStateField; hideLabel?: boolean }) {
  const { label, value, fieldPath, numeric, changes, layout } = item
  return (
    <section
      aria-label={hideLabel ? label : undefined}
      data-state-field
      data-state-field-layout={layout}
      className={cn(
        'min-w-0 bg-[var(--story-state-panel)] transition-colors hover:bg-[var(--story-state-field-hover)]',
        layout === 'compact' ? 'px-2.5 py-1.5' : 'px-3 py-2',
      )}
    >
      <div className={cn(layout === 'compact' && 'grid grid-cols-[minmax(64px,auto)_minmax(0,1fr)] items-baseline gap-3')}>
        {hideLabel ? null : (
          <h4 className={cn('truncate text-[11px] font-medium text-[var(--nova-text-faint)]', layout === 'wide' && 'mb-0.5')} title={label}>{label}</h4>
        )}
        <div className={cn('min-w-0', layout === 'compact' && 'text-right')}>
          <StateValue value={value} />
        </div>
      </div>
      <InlineFieldChanges changes={changes} fieldPath={fieldPath} numeric={numeric} />
    </section>
  )
}

function stateFieldLayout(value: unknown, changes: StoryStateChange[]): StateFieldLayout {
  if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
    return 'structured'
  }
  if (Array.isArray(value)) {
    return value.some((item) => typeof item === 'object' && item !== null) ? 'structured' : 'wide'
  }

  const simpleValueLength = typeof value === 'string'
    ? value.trim().length
    : 0
  const usesWideLayout = simpleValueLength >= 20
    || changes.some((change) => (change.reason?.trim().length || 0) >= 40)
  return usesWideLayout ? 'wide' : 'compact'
}

function InlineFieldChanges({ changes, fieldPath, numeric, variant = 'field' }: { changes: StoryStateChange[]; fieldPath: string; numeric: boolean; variant?: 'field' | 'metric' }) {
  const { t } = useTranslation()
  if (changes.length === 0) return null
  return (
    <ul
      aria-label={t('storyStage.state.fieldChanges')}
      className={cn(
        'flex flex-col gap-0.5',
        variant === 'field' ? 'mt-1 border-l-2 border-[var(--nova-border)] pl-2' : 'mt-1',
      )}
    >
      {changes.map((change) => {
        const relativeLabel = relativeChangeLabel(change.path, fieldPath)
        const tone = changeTone(change, numeric)
        return (
          <li key={change.id} className={cn('flex min-w-0 flex-wrap items-baseline gap-x-1.5', variant === 'field' ? 'text-[10px] leading-4' : 'text-[9px] leading-3.5')}>
            {relativeLabel ? <span className="font-medium text-[var(--nova-text-muted)]">{relativeLabel}</span> : null}
            <span className={cn('font-medium', tone === 'positive' && 'font-mono tabular-nums text-[var(--story-state-positive)]', tone === 'negative' && 'font-mono tabular-nums text-[var(--story-state-negative)]', tone === 'neutral' && 'text-[var(--nova-text-faint)]')}>
              {inlineChangeLabel(change, numeric, t)}
            </span>
            {change.reason ? <span title={change.reason} className={cn('min-w-0 text-[var(--nova-text-faint)]', variant === 'field' ? 'line-clamp-2' : 'truncate')}>{change.reason}</span> : null}
          </li>
        )
      })}
    </ul>
  )
}

function StateSectionEmpty({ label }: { label: string }) {
  return (
    <Empty className="min-h-20">
      <EmptyHeader>
        <EmptyMedia variant="icon"><Sparkles /></EmptyMedia>
        <EmptyDescription>{label}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

function isBoundedNumericFieldEntry(entry: ActorFieldEntry): entry is BoundedNumericFieldEntry {
  return entry.field.type === 'number'
    && typeof entry.value === 'number'
    && Number.isFinite(entry.value)
    && typeof entry.field.min === 'number'
    && Number.isFinite(entry.field.min)
    && typeof entry.field.max === 'number'
    && Number.isFinite(entry.field.max)
    && entry.field.max > entry.field.min
}

function normalizedProgress(value: number, min: number, max: number) {
  return Math.min(100, Math.max(0, ((value - min) / (max - min)) * 100))
}

function formatMetricNumber(value: number) {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(2)))
}

function actorFieldChanges(changes: StoryStateChange[], field: ActorStateField) {
  const paths = actorFieldPaths(field)
  return changes.filter((change) => paths.some((path) => sameFieldPath(change.path, path)))
}

function actorFieldPaths(field: ActorStateField) {
  return [field.id, field.path, field.name]
    .filter((value): value is string => typeof value === 'string' && value.trim() !== '')
}

function sameFieldPath(left: string, right: string) {
  const normalizedLeft = humanizeStateKey(left.trim()).toLocaleLowerCase()
  const normalizedRight = humanizeStateKey(right.trim()).toLocaleLowerCase()
  return normalizedLeft === normalizedRight
}

function worldFieldChanges(changes: StoryStateChange[], fieldPath: string) {
  return changes.filter((change) => change.path === fieldPath || change.path.startsWith(`${fieldPath}.`))
}

function relativeChangeLabel(changePath: string, fieldPath: string) {
  if (sameFieldPath(changePath, fieldPath)) return ''
  const relative = changePath.startsWith(`${fieldPath}.`) ? changePath.slice(fieldPath.length + 1) : ''
  return relative ? statePathLabel(relative) : ''
}

function changeTone(change: StoryStateChange, numeric: boolean): 'positive' | 'negative' | 'neutral' {
  const op = change.op.trim().toLowerCase()
  const delta = numericChangeDelta(change, numeric)
  if (delta !== null) {
    if (delta > 0) return 'positive'
    if (delta < 0) return 'negative'
  }
  if (['push', 'append', 'add'].includes(op)) return 'positive'
  if (['pull', 'remove', 'delete', 'unset'].includes(op)) return 'negative'
  return 'neutral'
}

function inlineChangeLabel(change: StoryStateChange, numeric: boolean, t: ReturnType<typeof useTranslation>['t']) {
  const op = change.op.trim().toLowerCase()
  const delta = numericChangeDelta(change, numeric)
  if (delta !== null) {
    return `${delta >= 0 ? '+' : ''}${delta}`
  }
  const value = inlineValue(change.value)
  if (['set', 'replace', 'merge'].includes(op)) return t('storyStage.state.changeUpdatedInline')
  if (['push', 'append', 'add'].includes(op)) return value ? t('storyStage.state.changeAddInline', { value }) : t('storyStage.state.changeUpdatedInline')
  if (['pull', 'remove', 'delete'].includes(op)) return value ? t('storyStage.state.changeRemoveInline', { value }) : t('storyStage.state.changeRemovedInline')
  if (op === 'unset') return t('storyStage.state.changeRemovedInline')
  return t('storyStage.state.changeUpdatedInline')
}

function isIncrementOperation(op: string) {
  return op === 'inc' || op === 'increment' || op === 'decrement'
}

function numericChangeDelta(change: StoryStateChange, numeric: boolean) {
  const op = change.op.trim().toLowerCase()
  if (!numeric || !isIncrementOperation(op) || typeof change.value !== 'number') return null
  return op === 'decrement' ? -Math.abs(change.value) : change.value
}

function inlineValue(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value) && value.every((item) => item === null || ['string', 'number', 'boolean'].includes(typeof item))) {
    return value.map((item) => item === null ? '' : String(item)).filter(Boolean).join('、')
  }
  return ''
}

function turnStatusLabel(snapshot: Snapshot | null, t: ReturnType<typeof useTranslation>['t']) {
  const turnId = snapshot?.current_turn?.id
  const matchedIndex = turnId ? snapshot?.turns.findIndex((turn) => turn.id === turnId) ?? -1 : -1
  const turn = matchedIndex >= 0 ? matchedIndex + 1 : Math.max(snapshot?.turns.length || 0, turnId ? 1 : 0)
  if (snapshot?.current_turn?.state_status === 'pending') return t('storyStage.state.syncing', { turn })
  if (snapshot?.current_turn?.state_status === 'failed') return t('storyStage.state.failed', { turn })
  return t('storyStage.state.updatedTurn', { turn })
}
