import { useEffect, useMemo, useState } from 'react'
import { AlertCircle, ChevronDown, ChevronUp, CircleCheck, Globe2, Loader2, PanelRight, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia } from '@/components/ui/empty'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { Snapshot } from '../../types'
import { ChangesSummary } from './ChangesSummary'
import type { StoryStateDisplayPreference } from './display-preference'
import { LedgerFieldView } from './ledger-fields'
import {
  actorFieldEntries,
  actorName,
  actorTemplate,
  buildLedgerGroups,
  buildStoryStateModel,
  humanizeStateKey,
  visibleActorTraits,
  type LedgerFieldEntry,
  type LedgerFieldGroup,
  type StoryStateChange,
} from './model'
import { StateDisplayPreferenceMenu } from './StateDisplayPreferenceMenu'

const WORLD_STATE_TAB = '__world_state__'

interface StoryStateLedgerProps {
  snapshot: Snapshot | null
  displayPreference: StoryStateDisplayPreference
  onDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onOpenDirectorState?: () => void
}

/**
 * StoryStateLedger is the compact state panel pinned after the latest prose.
 * Fields are clustered into switchable groups (template-declared or inferred
 * from value shape) and rendered on a dense auto-fill grid; the turn's state
 * delta surfaces once in the summary row plus per-field change chips.
 */
export function StoryStateLedger({ snapshot, displayPreference, onDisplayPreferenceChange, onOpenDirectorState }: StoryStateLedgerProps) {
  const { t } = useTranslation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const actorTabs = useMemo(() => model.actors.map(([actorId, actor]) => ({ id: actorId, name: actorName(actorId, actor) })), [model.actors])
  const hasWorldFacts = model.worldFacts.length > 0
  const [selectedTab, setSelectedTab] = useState(actorTabs[0]?.id || WORLD_STATE_TAB)
  const turnKey = `${snapshot?.story_id || ''}:${snapshot?.branch_id || ''}:${snapshot?.current_turn?.id || ''}`
  const [open, setOpen] = useState(displayPreference !== 'collapsed')

  useEffect(() => {
    if (selectedTab === WORLD_STATE_TAB && hasWorldFacts) return
    if (actorTabs.some((actor) => actor.id === selectedTab)) return
    setSelectedTab(actorTabs[0]?.id || WORLD_STATE_TAB)
  }, [actorTabs, hasWorldFacts, selectedTab])

  useEffect(() => {
    setOpen(displayPreference !== 'collapsed')
  }, [displayPreference, turnKey])

  if (!model.hasState || displayPreference === 'director-only') return null

  return (
    <Collapsible open={open} onOpenChange={setOpen} asChild>
      <section
        aria-label={t('storyStage.state.current')}
        className="story-state-ledger mt-3 overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--story-state-canvas)]"
      >
        <header className="flex h-10 min-w-0 items-center gap-2 px-2.5">
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
              aria-label={open ? t('storyStage.state.collapse') : t('storyStage.state.expand')}
              title={open ? t('storyStage.state.collapse') : t('storyStage.state.expand')}
            >
              {open ? <ChevronUp data-icon="inline-start" /> : <ChevronDown data-icon="inline-start" />}
            </Button>
          </CollapsibleTrigger>
        </header>

        <CollapsibleContent>
          {model.changes.length > 0 ? (
            <ChangesSummary changes={model.changes} actors={model.actors} schema={snapshot?.actor_state_schema} />
          ) : null}
          <Tabs value={selectedTab} onValueChange={setSelectedTab} className="gap-0">
            <StateEntityTabs actors={actorTabs} showWorld={hasWorldFacts} />
            {model.actors.map(([actorId, actor]) => (
              <TabsContent key={actorId} value={actorId} className="mt-0">
                <ActorLedgerBody
                  actor={actor}
                  snapshot={snapshot}
                  changes={model.changes.filter((change) => change.actorId === actorId)}
                />
              </TabsContent>
            ))}
            {hasWorldFacts ? (
              <TabsContent value={WORLD_STATE_TAB} className="mt-0">
                <WorldLedgerBody
                  facts={model.worldFacts}
                  changes={model.changes.filter((change) => !change.actorId)}
                />
              </TabsContent>
            ) : null}
          </Tabs>
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

function ActorLedgerBody({ actor, snapshot, changes }: { actor: Record<string, unknown>; snapshot: Snapshot | null; changes: StoryStateChange[] }) {
  const { t } = useTranslation()
  const template = actorTemplate(actor, snapshot?.actor_state_schema)
  const entries: LedgerFieldEntry[] = actorFieldEntries(actor, template?.fields).map(({ field, value }) => ({
    id: field.id || field.path || field.name,
    label: field.name,
    field,
    value: value ?? field.default ?? null,
  }))
  const groups = buildLedgerGroups(entries, changes)
  const traits = visibleActorTraits(actor)

  return (
    <div>
      {traits.length > 0 ? <ActorTraits traits={traits} /> : null}
      {groups.length > 0 ? <LedgerGroupTabs groups={groups} /> : <StateSectionEmpty label={t('storyStage.state.actorEmpty')} />}
    </div>
  )
}

function WorldLedgerBody({ facts, changes }: { facts: Array<[string, unknown]>; changes: StoryStateChange[] }) {
  const { t } = useTranslation()
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
  const groups = buildLedgerGroups(entries, changes)
  if (groups.length === 0) return <StateSectionEmpty label={t('storyStage.state.worldEmpty')} />
  return <LedgerGroupTabs groups={groups} />
}

/** LedgerGroupTabs renders one dense grid per group; a single group skips the tab bar. */
function LedgerGroupTabs({ groups }: { groups: LedgerFieldGroup[] }) {
  const { t } = useTranslation()
  const [selectedGroup, setSelectedGroup] = useState(groups[0]?.key || '')
  const activeGroup = groups.some((group) => group.key === selectedGroup) ? selectedGroup : groups[0]?.key || ''

  if (groups.length === 1) {
    return <LedgerGroupGrid group={groups[0]} />
  }

  return (
    <Tabs value={activeGroup} onValueChange={setSelectedGroup} className="gap-0">
      <div className="story-state-ledger__tabs-scroll overflow-x-auto overflow-y-hidden border-b border-[var(--nova-border-soft)] px-2.5">
        <TabsList
          variant="line"
          aria-label={t('storyStage.state.groups')}
          className="h-8 w-max max-w-none justify-start gap-3 p-0"
        >
          {groups.map((group) => (
            <TabsTrigger
              key={group.key}
              value={group.key}
              className="h-8 min-w-12 max-w-40 flex-none rounded-none px-1 text-[11px] after:bottom-0"
            >
              <span className="truncate">{group.custom ? group.key : t(`storyStage.state.group.${group.key}`)}</span>
              <span className="ml-1 shrink-0 text-[10px] text-[var(--nova-text-faint)]">{group.fields.length}</span>
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      {groups.map((group) => (
        <TabsContent key={group.key} value={group.key} className="mt-0">
          <LedgerGroupGrid group={group} />
        </TabsContent>
      ))}
    </Tabs>
  )
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
