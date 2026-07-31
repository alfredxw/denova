import type { BranchSummary, Snapshot, StoryDirector, StorySummary } from '../types'
import { useInteractiveStore } from '../stores/interactive-store'
import { DirectorConsole } from './director-console/DirectorConsole'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from './story-state/display-preference'

interface DirectorPanelProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  branchId?: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
  stateDisplayPreference?: StoryStateDisplayPreference
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
}

export function DirectorPanel({ storyId, story, storyDirectors = [], onDirectorChange, onReplyTargetCharsChange, branchId, branches, snapshot, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onSwitchBranch, onOpenBranchTimeline }: DirectorPanelProps) {
  const setSubmode = useInteractiveStore((state) => state.setSubmode)
  const effectiveBranchId = branchId || snapshot?.branch_id || ''

  return (
    <DirectorConsole
      storyId={storyId}
      story={story}
      storyDirectors={storyDirectors}
      onDirectorChange={onDirectorChange}
      onReplyTargetCharsChange={onReplyTargetCharsChange}
      branchId={effectiveBranchId}
      branches={branches}
      snapshot={snapshot}
      stateError={snapshot?.current_turn?.state_error || ''}
      stateDisplayPreference={stateDisplayPreference}
      onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
      directorStatus={snapshot?.director_plan_status}
      onOpenBackstage={() => setSubmode('director')}
      onSwitchBranch={onSwitchBranch}
      onOpenBranchTimeline={onOpenBranchTimeline}
    />
  )
}

function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}
