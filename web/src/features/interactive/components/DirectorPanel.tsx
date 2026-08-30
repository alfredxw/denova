import type { BranchSummary, Snapshot, StoryDirector, StoryPlanningMode, StorySummary } from '../types'
import { DirectorConsole } from './director-console/DirectorConsole'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from './story-state/display-preference'

interface DirectorPanelProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  onPlanningModeChange?: (mode: StoryPlanningMode) => void | Promise<void>
  branchId?: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  stateDisplayPreference?: StoryStateDisplayPreference
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
}

export function DirectorPanel({ storyId, story, storyDirectors = [], onDirectorChange, onReplyTargetCharsChange, onPlanningModeChange, branchId, branches, snapshot, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onSwitchBranch, onOpenBranchTimeline }: DirectorPanelProps) {
  const effectiveBranchId = branchId || snapshot?.branch_id || ''

  return (
    <DirectorConsole
      storyId={storyId}
      story={story}
      storyDirectors={storyDirectors}
      onDirectorChange={onDirectorChange}
      onReplyTargetCharsChange={onReplyTargetCharsChange}
      onPlanningModeChange={onPlanningModeChange}
      branchId={effectiveBranchId}
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
