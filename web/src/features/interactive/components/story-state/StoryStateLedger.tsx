import { useEffect, useMemo, useState } from 'react'
import { AlignLeft, AlertCircle, ChevronDown, ChevronUp, CircleCheck, EyeOff, Gauge, Globe2, Loader2, Package, PanelRight, Sparkles, Tag } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia } from '@/components/ui/empty'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { Snapshot } from '../../types'
import { ChangesSummary } from './ChangesSummary'
import type { StoryStateDisplayPreference } from './display-preference'
import { applyStoryStateLayout, readStoryStateLayouts, writeStoryStateTemplateLayout, type StoryStateLayouts, type StoryStateTemplateLayout } from './layout-preference'
import { LedgerFieldView } from './ledger-fields'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  buildLedgerGroups,
  buildStoryStateModel,
  humanizeStateKey,
  splitLedgerGroupsForPreview,
  visibleActorTraits,
  type LedgerFieldEntry,
  type LedgerFieldGroup,
  type StoryStateChange,
} from './model'
import { StateDisplayPreferenceMenu } from './StateDisplayPreferenceMenu'
import { StateLayoutEditor } from './StateLayoutEditor'

const WORLD_STATE_TAB = '__world_state__'

type StoryStatePanelMode = 'collapsed' | 'preview' | 'expanded'

const PANEL_MODE_BY_PREFERENCE: Record<StoryStateDisplayPreference, StoryStatePanelMode> = {
  preview: 'preview',
  expanded: 'expanded',
  collapsed: 'collapsed',
  'director-only': 'collapsed',
}

interface StoryStateLedgerProps {
  snapshot: Snapshot | null
  displayPreference: StoryStateDisplayPreference
  onDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onOpenDirectorState?: () => void
}

interface StateLedgerPresentation {
  id: string
  name: string
  templateId: string
  groups: LedgerFieldGroup[]
  traits: ReturnType<typeof visibleActorTraits>
}

/**
 * StoryStateLedger is the compact state panel pinned after the latest prose.
 * Fields lay out as bordered group sections on one page. Schema hints provide
 * the fallback grouping, while a story + template UI preference controls the
 * final section and field order. Preview mode shows the first two ordered
 * sections with a "show all" affordance; the turn's state delta surfaces once in the
 * summary row plus per-field change chips.
 */
export function StoryStateLedger({ snapshot, displayPreference, onDisplayPreferenceChange, onOpenDirectorState }: StoryStateLedgerProps) {
  const { t } = useTranslation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const actorLedgers = useMemo(() => model.actors.map(([actorId, actor]) => buildActorLedger(actorId, actor, snapshot, model.changes)), [model.actors, model.changes, snapshot])
  const worldLedger = useMemo(() => buildWorldLedger(model.worldFacts, model.changes), [model.changes, model.worldFacts])
  const actorTabs = useMemo(() => actorLedgers.map((ledger) => ({ id: ledger.id, name: ledger.name })), [actorLedgers])
  const hasWorldFacts = model.worldFacts.length > 0
  const [selectedTab, setSelectedTab] = useState(actorTabs[0]?.id || WORLD_STATE_TAB)
  const storyId = snapshot?.story_id || ''
  const [layoutState, setLayoutState] = useState<{ storyId: string; layouts: StoryStateLayouts }>(() => ({ storyId, layouts: readStoryStateLayouts(storyId) }))
  const [layoutEditorOpen, setLayoutEditorOpen] = useState(false)
  const turnKey = `${snapshot?.story_id || ''}:${snapshot?.branch_id || ''}:${snapshot?.current_turn?.id || ''}`
  const [panelMode, setPanelMode] = useState<StoryStatePanelMode>(PANEL_MODE_BY_PREFERENCE[displayPreference])
  const layouts = layoutState.storyId === storyId ? layoutState.layouts : {}
  const selectedLedger = selectedTab === WORLD_STATE_TAB
    ? worldLedger
    : actorLedgers.find((ledger) => ledger.id === selectedTab)

  useEffect(() => {
    if (selectedTab === WORLD_STATE_TAB && hasWorldFacts) return
    if (actorTabs.some((actor) => actor.id === selectedTab)) return
    setSelectedTab(actorTabs[0]?.id || WORLD_STATE_TAB)
  }, [actorTabs, hasWorldFacts, selectedTab])

  useEffect(() => {
    setPanelMode(PANEL_MODE_BY_PREFERENCE[displayPreference])
  }, [displayPreference, turnKey])

  useEffect(() => {
    setLayoutState({ storyId, layouts: readStoryStateLayouts(storyId) })
    setLayoutEditorOpen(false)
  }, [storyId])

  if (!model.hasState || displayPreference === 'director-only') return null

  const collapsed = panelMode === 'collapsed'

  return (
    <Collapsible
      open={!collapsed}
      onOpenChange={(nextOpen) => setPanelMode(nextOpen ? 'preview' : 'collapsed')}
      asChild
    >
      <section
        aria-label={t('storyStage.state.current')}
        data-state-panel-mode={panelMode}
        className="story-state-ledger mt-3 overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--story-state-canvas)]"
      >
        <header className="flex h-10 min-w-0 items-center gap-2 px-2.5">
          <StatusIndicator status={snapshot?.current_turn?.state_status} />
          <div className="flex min-w-0 flex-1 items-baseline gap-2">
            <h2 className="shrink-0 text-[13px] font-semibold tracking-tight text-[var(--nova-text)]">{t('storyStage.state.current')}</h2>
            <p className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{turnStatusLabel(snapshot, t)}</p>
          </div>
          <StateDisplayPreferenceMenu
            value={displayPreference}
            onChange={onDisplayPreferenceChange}
            onCustomizeLayout={selectedLedger?.groups.length ? () => setLayoutEditorOpen(true) : undefined}
            compact
          />
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
          {model.changes.length > 0 ? (
            <ChangesSummary changes={model.changes} actors={model.actors} schema={snapshot?.actor_state_schema} />
          ) : null}
          <Tabs value={selectedTab} onValueChange={setSelectedTab} className="gap-0">
            <StateEntityTabs actors={actorTabs} showWorld={hasWorldFacts} />
            {actorLedgers.map((ledger) => (
              <TabsContent
                key={ledger.id}
                value={ledger.id}
                forceMount
                hidden={selectedTab !== ledger.id}
                className="mt-0"
              >
                <ActorLedgerBody
                  ledger={ledger}
                  layout={layouts[ledger.templateId]}
                  panelMode={panelMode === 'expanded' ? 'expanded' : 'preview'}
                  onPanelModeChange={setPanelMode}
                />
              </TabsContent>
            ))}
            {hasWorldFacts ? (
              <TabsContent
                value={WORLD_STATE_TAB}
                forceMount
                hidden={selectedTab !== WORLD_STATE_TAB}
                className="mt-0"
              >
                <WorldLedgerBody
                  ledger={worldLedger}
                  layout={layouts[worldLedger.templateId]}
                  panelMode={panelMode === 'expanded' ? 'expanded' : 'preview'}
                  onPanelModeChange={setPanelMode}
                />
              </TabsContent>
            ) : null}
          </Tabs>
        </CollapsibleContent>
        {selectedLedger ? (
          <StateLayoutEditor
            open={layoutEditorOpen}
            title={selectedLedger.id === WORLD_STATE_TAB ? t('storyStage.state.world') : selectedLedger.name}
            groups={selectedLedger.groups}
            value={layouts[selectedLedger.templateId]}
            onOpenChange={setLayoutEditorOpen}
            onChange={(layout) => {
              const next = { ...layouts, [selectedLedger.templateId]: layout }
              setLayoutState({ storyId, layouts: next })
              writeStoryStateTemplateLayout(storyId, selectedLedger.templateId, layout)
            }}
            onReset={() => {
              const next = { ...layouts }
              delete next[selectedLedger.templateId]
              setLayoutState({ storyId, layouts: next })
              writeStoryStateTemplateLayout(storyId, selectedLedger.templateId, null)
            }}
          />
        ) : null}
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

function StateEntityTabs({ actors, showWorld }: { actors: Array<{ id: string; name: string }>; showWorld: boolean }) {
  const { t } = useTranslation()
  if (actors.length === 1 && !showWorld) return null
  return (
    <div className="story-state-ledger__tabs-scroll overflow-x-auto overflow-y-hidden px-2.5 pb-1.5">
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
        {showWorld ? (
          <TabsTrigger
            value={WORLD_STATE_TAB}
            className="min-w-20 flex-none"
          >
            <Globe2 data-icon="inline-start" />
            <span>{t('storyStage.state.world')}</span>
          </TabsTrigger>
        ) : null}
      </TabsList>
    </div>
  )
}

function buildActorLedger(actorId: string, actor: Record<string, unknown>, snapshot: Snapshot | null, changes: StoryStateChange[]): StateLedgerPresentation {
  const template = actorTemplate(actor, snapshot?.actor_state_schema)
  const entries: LedgerFieldEntry[] = actorFieldEntries(actor, template?.fields).map(({ field, value }) => ({
    id: field.id || field.path || field.name,
    label: field.name,
    field,
    value: value ?? field.default ?? null,
  }))
  const rawTemplateId = typeof actor.template_id === 'string' ? actor.template_id.trim() : ''
  return {
    id: actorId,
    name: actorName(actorId, actor),
    templateId: template?.id || rawTemplateId || `actor:${actorId}`,
    groups: buildLedgerGroups(entries, changes.filter((change) => change.actorId === actorId)),
    traits: visibleActorTraits(actor),
  }
}

function buildWorldLedger(facts: Array<[string, unknown]>, changes: StoryStateChange[]): StateLedgerPresentation {
  // Record-valued facts (e.g. the story-context object) are exploded one
  // level so each nested value routes to its own renderer and group instead
  // of flattening into one unreadable mega-row.
  const entries: LedgerFieldEntry[] = facts.flatMap(([key, value]) => {
    if (isRecordValue(value)) {
      return Object.entries(value).map(([nestedKey, nestedValue]) => ({
        id: `${key}.${nestedKey}`,
        label: humanizeStateKey(nestedKey),
        value: nestedValue,
      }))
    }
    return [{ id: key, label: humanizeStateKey(key), value }]
  })
  return {
    id: WORLD_STATE_TAB,
    name: 'world',
    templateId: WORLD_STATE_TAB,
    groups: buildLedgerGroups(entries, changes.filter((change) => !change.actorId)),
    traits: [],
  }
}

function ActorLedgerBody({ ledger, layout, panelMode, onPanelModeChange }: { ledger: StateLedgerPresentation; layout?: StoryStateTemplateLayout; panelMode: 'preview' | 'expanded'; onPanelModeChange: (mode: StoryStatePanelMode) => void }) {
  const { t } = useTranslation()
  const groups = applyStoryStateLayout(ledger.groups, layout)

  return (
    <div>
      {ledger.traits.length > 0 ? <ActorTraits traits={ledger.traits} /> : null}
      {groups.length > 0
        ? <LedgerSections groups={groups} mode={panelMode} onModeChange={onPanelModeChange} />
        : <StateSectionEmpty label={t('storyStage.state.actorEmpty')} />}
    </div>
  )
}

function WorldLedgerBody({ ledger, layout, panelMode, onPanelModeChange }: { ledger: StateLedgerPresentation; layout?: StoryStateTemplateLayout; panelMode: 'preview' | 'expanded'; onPanelModeChange: (mode: StoryStatePanelMode) => void }) {
  const { t } = useTranslation()
  const groups = applyStoryStateLayout(ledger.groups, layout)
  if (groups.length === 0) return <StateSectionEmpty label={t('storyStage.state.worldEmpty')} />
  return <LedgerSections groups={groups} mode={panelMode} onModeChange={onPanelModeChange} />
}

/**
 * LedgerSections lays groups out as visually distinct blocks on one page. In
 * preview mode only the first two ordered sections show, with a mode toggle that
 * reveals the rest without any height-clamped tricks.
 */
function LedgerSections({ groups, mode, onModeChange }: { groups: LedgerFieldGroup[]; mode: 'preview' | 'expanded'; onModeChange: (mode: StoryStatePanelMode) => void }) {
  const { t } = useTranslation()
  const { preview, hidden } = useMemo(() => splitLedgerGroupsForPreview(groups), [groups])
  const expanded = mode === 'expanded'
  // Keep the already-visible preview sections anchored in place. Sections
  // revealed by the user's action append after them even when their schema
  // order originally placed them above the preview set.
  const visibleGroups = expanded ? [...preview, ...hidden] : preview
  const decorated = groups.length > 1
  return (
    <div className="story-state-ledger__sections">
      {visibleGroups.map((group) => (
        <LedgerSectionBlock key={group.key} group={group} decorated={decorated} />
      ))}
      {!expanded && hidden.length > 0 ? (
        <button
          type="button"
          className="story-state-ledger__mode-toggle"
          onClick={() => onModeChange('expanded')}
        >
          <ChevronDown aria-hidden="true" className="size-3.5" />
          {t('storyStage.state.expandAll', { count: hidden.length })}
        </button>
      ) : null}
      {expanded && hidden.length > 0 ? (
        <button
          type="button"
          className="story-state-ledger__mode-toggle"
          onClick={() => onModeChange('preview')}
        >
          <ChevronUp aria-hidden="true" className="size-3.5" />
          {t('storyStage.state.collapseToPreview')}
        </button>
      ) : null}
    </div>
  )
}

function LedgerSectionBlock({ group, decorated }: { group: LedgerFieldGroup; decorated: boolean }) {
  const { t } = useTranslation()
  const label = group.custom ? group.key : t(`storyStage.state.group.${group.key}`)
  return (
    <section aria-label={label} data-decorated={decorated || undefined} className="story-state-ledger__section">
      {decorated ? (
        <header className="story-state-ledger__section-header">
          <LedgerGroupIcon group={group} />
          <h3 className="story-state-ledger__section-title">{label}</h3>
          <span className="story-state-ledger__section-count">{group.fields.length}</span>
        </header>
      ) : null}
      <LedgerGroupGrid group={group} />
    </section>
  )
}

function LedgerGroupIcon({ group }: { group: LedgerFieldGroup }) {
  const className = 'story-state-ledger__section-icon'
  if (group.custom) return <Tag aria-hidden="true" className={className} />
  switch (group.key) {
    case 'overview':
      return <Gauge aria-hidden="true" className={className} />
    case 'holdings':
      return <Package aria-hidden="true" className={className} />
    case 'details':
      return <AlignLeft aria-hidden="true" className={className} />
    case 'spoiler':
      return <EyeOff aria-hidden="true" className={className} />
    default:
      return <Tag aria-hidden="true" className={className} />
  }
}

function LedgerGroupGrid({ group }: { group: LedgerFieldGroup }) {
  return (
    <div className="story-state-ledger__grid" data-group={group.custom ? 'custom' : group.key}>
      {group.fields.map((item) => <LedgerFieldView key={item.id} item={item} />)}
    </div>
  )
}

function ActorTraits({ traits }: { traits: ReturnType<typeof visibleActorTraits> }) {
  return (
    <div className="flex min-w-0 flex-wrap gap-1 border-b border-[var(--nova-border-soft)] px-2.5 py-1.5">
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

function turnStatusLabel(snapshot: Snapshot | null, t: ReturnType<typeof useTranslation>['t']) {
  const turnId = snapshot?.current_turn?.id
  const matchedIndex = turnId ? snapshot?.turns.findIndex((turn) => turn.id === turnId) ?? -1 : -1
  const turn = matchedIndex >= 0 ? matchedIndex + 1 : Math.max(snapshot?.turns.length || 0, turnId ? 1 : 0)
  if (snapshot?.current_turn?.state_status === 'pending') return t('storyStage.state.syncing', { turn })
  if (snapshot?.current_turn?.state_status === 'failed') return t('storyStage.state.failed', { turn })
  return t('storyStage.state.updatedTurn', { turn })
}

function isRecordValue(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
