import type { Snapshot, StoryDirector, StorySummary } from '../types'
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
  snapshot: Snapshot | null
  stateDisplayPreference?: StoryStateDisplayPreference
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
}

export function DirectorPanel({ storyId, story, storyDirectors = [], onDirectorChange, onReplyTargetCharsChange, branchId, snapshot, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange }: DirectorPanelProps) {
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
      snapshot={snapshot}
      stateError={snapshot?.current_turn?.state_error || ''}
      stateDisplayPreference={stateDisplayPreference}
      onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
      directorStatus={snapshot?.director_plan_status}
      onOpenBackstage={() => setSubmode('director')}
    />
  )
}

function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}
