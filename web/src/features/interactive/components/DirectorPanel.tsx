import { useEffect, useState } from 'react'
import type { Snapshot, StoryDirector, StorySummary } from '../types'
import { DirectorConsole } from './director-console/DirectorConsole'
import type { ConsoleTab } from './director-console/types'
import { DEFAULT_STORY_STATE_DISPLAY, OPEN_DIRECTOR_STATE_EVENT, type StoryStateDisplayPreference } from './story-state/display-preference'

interface DirectorPanelProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  branchId?: string
  snapshot: Snapshot | null
  loading?: boolean
  stateDisplayPreference?: StoryStateDisplayPreference
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onSnapshotRefresh?: () => void | Promise<unknown>
}

export function DirectorPanel({ storyId, story, storyDirectors = [], onDirectorChange, onReplyTargetCharsChange, branchId, snapshot, loading = false, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onSnapshotRefresh }: DirectorPanelProps) {
  const [activeTab, setActiveTab] = useState<ConsoleTab>('state')
  const [directorRevealed, setDirectorRevealed] = useState(false)
  const effectiveBranchId = branchId || snapshot?.branch_id || ''

  useEffect(() => {
    setActiveTab('state')
    setDirectorRevealed(false)
  }, [effectiveBranchId, storyId])

  useEffect(() => {
    const openState = () => setActiveTab('state')
    window.addEventListener(OPEN_DIRECTOR_STATE_EVENT, openState)
    return () => window.removeEventListener(OPEN_DIRECTOR_STATE_EVENT, openState)
  }, [])

  return (
    <DirectorConsole
      storyId={storyId}
      story={story}
      storyDirectors={storyDirectors}
      onDirectorChange={onDirectorChange}
      onReplyTargetCharsChange={onReplyTargetCharsChange}
      branchId={effectiveBranchId}
      snapshot={snapshot}
      loading={loading}
      stateStatus={snapshot?.current_turn?.state_status || ''}
      stateError={snapshot?.current_turn?.state_error || ''}
      stateDisplayPreference={stateDisplayPreference}
      onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      directorRevealed={directorRevealed}
      onRevealDirector={() => setDirectorRevealed(true)}
      onSnapshotRefresh={onSnapshotRefresh}
    />
  )
}

function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}
