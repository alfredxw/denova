import type { BranchSummary, ImagePreset, InteractiveStoryUpdateInput, Snapshot, StoryDirector, StorySummary, Teller } from '../types'
import { DirectorConsole } from './director-console/DirectorConsole'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from './story-state/display-preference'

interface DirectorPanelProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  tellers?: Teller[]
  imagePresets?: ImagePreset[]
  onDirectorChange?: (directorId: string) => void | Promise<void>
  onStoryUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
  branchId?: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  stateDisplayPreference?: StoryStateDisplayPreference
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
}

export function DirectorPanel({ storyId, story, storyDirectors = [], tellers = [], imagePresets = [], onDirectorChange, onStoryUpdate, onOpenPresets, branchId, branches, snapshot, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onSwitchBranch, onOpenBranchTimeline }: DirectorPanelProps) {
  return (
    <DirectorConsole
      storyId={storyId}
      story={story}
      storyDirectors={storyDirectors}
      tellers={tellers}
      imagePresets={imagePresets}
      onDirectorChange={onDirectorChange}
      onStoryUpdate={onStoryUpdate}
      onOpenPresets={onOpenPresets}
      branchId={branchId || snapshot?.branch_id || ''}
      branches={branches}
      snapshot={snapshot}
      stateError={snapshot?.current_turn?.state_error || ''}
      stateDisplayPreference={stateDisplayPreference}
      onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
      onSwitchBranch={onSwitchBranch}
      onOpenBranchTimeline={onOpenBranchTimeline}
    />
  )
}

function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}
