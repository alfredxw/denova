import { useEffect, useRef, useState } from 'react'
import type { Snapshot, StoryDirector, StorySummary } from '../types'
import { DirectorConsole } from './director-console/DirectorConsole'
import { readStoredConsoleTab, readStoredDirectorRevealed, writeStoredConsoleTab, writeStoredDirectorRevealed } from './director-console/persistence'
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
  const [activeTab, setActiveTab] = useState<ConsoleTab>(() => readStoredConsoleTab(storyId) || 'state')
  const [directorRevealed, setDirectorRevealed] = useState(() => readStoredDirectorRevealed(storyId))
  const effectiveBranchId = branchId || snapshot?.branch_id || ''

  // tab 与揭示态按故事持久化：切分支、切面板、刷新页面都不丢失；仅切故事时恢复该故事各自的偏好。
  useEffect(() => {
    setActiveTab(readStoredConsoleTab(storyId) || 'state')
    setDirectorRevealed(readStoredDirectorRevealed(storyId))
  }, [storyId])

  const storyIdRef = useRef(storyId)
  storyIdRef.current = storyId

  const changeTab = (tab: ConsoleTab) => {
    setActiveTab(tab)
    writeStoredConsoleTab(storyId, tab)
  }

  const revealDirector = () => {
    setDirectorRevealed(true)
    writeStoredDirectorRevealed(storyId, true)
  }

  useEffect(() => {
    const openState = () => {
      setActiveTab('state')
      writeStoredConsoleTab(storyIdRef.current, 'state')
    }
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
      stateError={snapshot?.current_turn?.state_error || ''}
      stateDisplayPreference={stateDisplayPreference}
      onStateDisplayPreferenceChange={onStateDisplayPreferenceChange}
      activeTab={activeTab}
      onTabChange={changeTab}
      directorRevealed={directorRevealed}
      onRevealDirector={revealDirector}
      onSnapshotRefresh={onSnapshotRefresh}
    />
  )
}

function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}
