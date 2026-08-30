import { useEffect, useMemo, useState } from 'react'
import type { BranchSummary, ImagePreset, InteractiveStoryUpdateInput, Snapshot, StoryDirector, StoryDirectorModuleRefs, StorySummary, Teller } from '../../types'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'
import { BranchPreview } from './BranchPreview'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { DirectorConsoleTabs } from './DirectorConsoleTabs'
import { OverviewView } from './OverviewView'
import { readStoredDirectorConsoleTab, writeStoredDirectorConsoleTab } from './persistence'
import { StoryTuningView } from './StoryTuningView'
import type { DirectorConsoleTab } from './types'
import { stateEntries } from './utils'

export interface DirectorConsoleProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  tellers?: Teller[]
  imagePresets?: ImagePreset[]
  onDirectorChange?: (directorId: string) => void | Promise<void>
  onStoryUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
  branchId: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  stateError?: string
  stateDisplayPreference: StoryStateDisplayPreference
  onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
}

export function DirectorConsole({
  storyId,
  story,
  storyDirectors = [],
  tellers = [],
  imagePresets = [],
  onDirectorChange,
  onStoryUpdate,
  onOpenPresets,
  branchId,
  branches,
  snapshot,
  stateError,
  stateDisplayPreference,
  onStateDisplayPreferenceChange,
  onSwitchBranch,
  onOpenBranchTimeline,
}: DirectorConsoleProps) {
  const [activeTab, setActiveTab] = useState<DirectorConsoleTab>(() => readStoredDirectorConsoleTab(storyId) || 'overview')

  useEffect(() => {
    setActiveTab(readStoredDirectorConsoleTab(storyId) || 'overview')
  }, [storyId])

  const changeTab = (tab: DirectorConsoleTab) => {
    setActiveTab(tab)
    writeStoredDirectorConsoleTab(storyId, tab)
  }

  const stateFacts = useMemo(() => stateEntries(snapshot?.state), [snapshot?.state])
  const branchesCount = useMemo(() => {
    const branchIds = new Set([...branches, ...(snapshot?.graph?.branches || [])].map((branch) => branch.id))
    if (branchId) branchIds.add(branchId)
    return branchIds.size
  }, [branchId, branches, snapshot?.graph?.branches])
  const director = storyDirectors.find((item) => item.id === story?.story_director_id)
  const overrideCount = storyOverrideCount(story, director?.module_refs)
  const turnCount = story?.turn_count ?? ((snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0))
  let activeView
  if (activeTab === 'routes') {
    activeView = <BranchPreview branches={branches} currentBranchId={branchId} snapshot={snapshot} onSwitchBranch={onSwitchBranch} onOpenTimeline={onOpenBranchTimeline} />
  } else if (activeTab === 'tuning') {
    activeView = (
      <StoryTuningView
        story={story}
        directors={storyDirectors}
        tellers={tellers}
        imagePresets={imagePresets}
        stateDisplayPreference={stateDisplayPreference}
        onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
        onDirectorChange={onDirectorChange}
        onUpdate={onStoryUpdate}
        onOpenPresets={onOpenPresets}
      />
    )
  } else {
    activeView = (
      <OverviewView
        snapshot={snapshot}
        stateFacts={stateFacts}
        stateError={stateError}
        plan={snapshot?.branch_plan}
        planningEnabled={story?.planning_mode === 'enabled'}
        stateDisplayPreference={stateDisplayPreference}
        onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
      />
    )
  }

  return (
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader branchId={branchId} turnCount={turnCount} story={story} storyDirectors={storyDirectors} overrideCount={overrideCount} />
      <DirectorConsoleTabs activeTab={activeTab} onChange={changeTab} branchesCount={branchesCount} />
      <div className="min-h-0 flex-1 overflow-hidden">{activeView}</div>
    </aside>
  )
}

function storyOverrideCount(story: StorySummary | undefined, presetRefs: StoryDirectorModuleRefs | undefined): number {
  if (!story) return 0
  const refs = story.module_refs
  let count = 0
  if (refs) {
    if (moduleDiffers(refs.narrative_style_id, refs.narrative_style_disabled, presetRefs?.narrative_style_id, presetRefs?.narrative_style_disabled)) count += 1
    if (moduleDiffers(refs.rule_system_id, refs.rule_system_disabled, presetRefs?.rule_system_id, presetRefs?.rule_system_disabled)) count += 1
    if (moduleDiffers(refs.actor_state_id, refs.actor_state_disabled, presetRefs?.actor_state_id, presetRefs?.actor_state_disabled)) count += 1
    if (moduleDiffers(refs.image_preset_id, refs.image_preset_disabled, presetRefs?.image_preset_id, presetRefs?.image_preset_disabled)) count += 1
    if (refs.event_packages_disabled !== presetRefs?.event_packages_disabled || !sameStrings(refs.event_package_ids, presetRefs?.event_package_ids)) count += 1
  }
  if ((story.check_settings?.difficulty_shift || 0) !== 0) count += 1
  if ((story.check_settings?.roll_modifier || 0) !== 0) count += 1
  return count
}

function moduleDiffers(id: string | undefined, disabled: boolean | undefined, presetID: string | undefined, presetDisabled: boolean | undefined): boolean {
  return id !== presetID || Boolean(disabled) !== Boolean(presetDisabled)
}

function sameStrings(left: string[] | undefined, right: string[] | undefined): boolean {
  const leftValues = left || []
  const rightValues = right || []
  return leftValues.length === rightValues.length && leftValues.every((value, index) => value === rightValues[index])
}
