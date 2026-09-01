import { useEffect, useMemo, useState } from 'react'
import type {
  BranchSummary,
  GamePlanningTemplate,
  ImagePreset,
  InteractiveStoryUpdateInput,
  Snapshot,
  StorySummary,
  Teller,
} from '../../types'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'
import { BranchPreview } from './BranchPreview'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { DirectorConsoleTabs } from './DirectorConsoleTabs'
import { OverviewView } from './OverviewView'
import { readStoredDirectorConsoleTab, writeStoredDirectorConsoleTab } from './persistence'
import { StoryTuningView } from './StoryTuningView'
import type { DirectorConsoleTab } from './types'

export interface DirectorConsoleProps {
  storyId?: string
  story?: StorySummary
  planningTemplates?: GamePlanningTemplate[]
  tellers?: Teller[]
  imagePresets?: ImagePreset[]
  onPlanningTemplateChange?: (templateId: string) => void | Promise<void>
  onStoryUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
  branchId: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  branchPlanEditingDisabled?: boolean
  onBranchPlanUpdate?: (markdown: string, baseRevision: string) => void | Promise<void>
  stateError?: string
  stateDisplayPreference: StoryStateDisplayPreference
  onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
}

export function DirectorConsole({
  storyId,
  story,
  planningTemplates = [],
  tellers = [],
  imagePresets = [],
  onPlanningTemplateChange,
  onStoryUpdate,
  onOpenPresets,
  branchId,
  branches,
  snapshot,
  branchPlanEditingDisabled = false,
  onBranchPlanUpdate,
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

  const branchesCount = useMemo(() => {
    const branchIds = new Set([...branches, ...(snapshot?.graph?.branches || [])].map((branch) => branch.id))
    if (branchId) branchIds.add(branchId)
    return branchIds.size
  }, [branchId, branches, snapshot?.graph?.branches])
  const turnCount = story?.turn_count ?? ((snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0))
  let activeView
  if (activeTab === 'routes') {
    activeView = (
      <BranchPreview
        branches={branches}
        currentBranchId={branchId}
        snapshot={snapshot}
        onSwitchBranch={onSwitchBranch}
        onOpenTimeline={onOpenBranchTimeline}
      />
    )
  } else if (activeTab === 'controls') {
    activeView = (
      <StoryTuningView
        story={story}
        planningTemplates={planningTemplates}
        tellers={tellers}
        imagePresets={imagePresets}
        stateDisplayPreference={stateDisplayPreference}
        onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
        onPlanningTemplateChange={onPlanningTemplateChange}
        onUpdate={onStoryUpdate}
        onOpenPresets={onOpenPresets}
      />
    )
  } else {
    activeView = (
      <OverviewView
        key={branchId}
        snapshot={snapshot}
        stateError={stateError}
        plan={snapshot?.branch_plan}
        planningEnabled={story?.planning_mode === 'enabled'}
        branchPlanEditingDisabled={branchPlanEditingDisabled}
        onBranchPlanUpdate={onBranchPlanUpdate}
      />
    )
  }

  return (
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader
        branchId={branchId}
        turnCount={turnCount}
        story={story}
        planningTemplates={planningTemplates}
      />
      <DirectorConsoleTabs activeTab={activeTab} onChange={changeTab} branchesCount={branchesCount} />
      <div className="min-h-0 flex-1 overflow-hidden">{activeView}</div>
    </aside>
  )
}
