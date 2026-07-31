import { useEffect, useMemo, useState } from 'react'
import type { BranchSummary, Snapshot, StoryDirector, StorySummary } from '../../types'
import { splitStoryStateFacts, stateChanges } from '../story-state/model'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { readStoredDirectorConsoleTab, writeStoredDirectorConsoleTab } from './persistence'
import { DirectorConsoleTabs } from './DirectorConsoleTabs'
import { BranchPreview } from './BranchPreview'
import { StateView } from './StateView'
import type { DirectorConsoleTab, DirectorStatusLike } from './types'
import { stateEntries } from './utils'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'

export interface DirectorConsoleProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  branchId: string
  branches: BranchSummary[]
  snapshot: Snapshot | null
	stateError?: string
	stateDisplayPreference: StoryStateDisplayPreference
	onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  directorStatus?: DirectorStatusLike
  onOpenBackstage: () => void
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenBranchTimeline: () => void
}

// 右栏 = 状态感知栏：header 标题行内含导演台入口（状态+打开一体），
// 信息条默认展示导演/字数（行内编辑）/展示偏好，并提供变化、角色、世界与分支四个分区。
export function DirectorConsole({
  storyId,
  story,
  storyDirectors = [],
  onDirectorChange,
  onReplyTargetCharsChange,
  branchId,
  branches,
  snapshot,
	stateError,
	stateDisplayPreference,
	onStateDisplayPreferenceChange,
  directorStatus,
  onOpenBackstage,
  onSwitchBranch,
  onOpenBranchTimeline,
}: DirectorConsoleProps) {
  const [activeTab, setActiveTab] = useState<DirectorConsoleTab>(() => readStoredDirectorConsoleTab(storyId) || 'actors')

  // 分区选择按故事持久化；切故事时恢复该故事各自的上次选择。
  useEffect(() => {
    setActiveTab(readStoredDirectorConsoleTab(storyId) || 'actors')
  }, [storyId])

  const changeTab = (tab: DirectorConsoleTab) => {
    setActiveTab(tab)
    writeStoredDirectorConsoleTab(storyId, tab)
  }

  const stateFacts = useMemo(() => stateEntries(snapshot?.state), [snapshot?.state])
  const { actors, archivedActors, worldFacts } = useMemo(() => splitStoryStateFacts(stateFacts), [stateFacts])
  const changesCount = useMemo(() => stateChanges(snapshot?.current_turn?.state_delta).length, [snapshot?.current_turn?.state_delta])
  const branchesCount = useMemo(() => {
    const branchIds = new Set([...branches, ...(snapshot?.graph?.branches || [])].map((branch) => branch.id))
    if (branchId) branchIds.add(branchId)
    return branchIds.size
  }, [branchId, branches, snapshot?.graph?.branches])

  return (
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader branchId={branchId} turnCount={(snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0)} story={story} storyDirectors={storyDirectors} onDirectorChange={onDirectorChange} onReplyTargetCharsChange={onReplyTargetCharsChange} stateDisplayPreference={stateDisplayPreference} onStateDisplayPreferenceChange={onStateDisplayPreferenceChange} directorStatus={directorStatus} onOpenBackstage={onOpenBackstage} />
      <DirectorConsoleTabs activeTab={activeTab} onChange={changeTab} changesCount={changesCount} actorsCount={actors.length + archivedActors.length} worldCount={worldFacts.length} branchesCount={branchesCount} />
      {activeTab === 'branches' ? (
        <div className="min-h-0 flex-1 overflow-hidden">
          <BranchPreview branches={branches} currentBranchId={branchId} snapshot={snapshot} onSwitchBranch={onSwitchBranch} onOpenTimeline={onOpenBranchTimeline} />
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-hidden px-4 py-4">
          <div className="director-console__scroll h-full min-h-0 overflow-y-auto pb-4 pr-1">
            <StateView snapshot={snapshot} stateFacts={stateFacts} syncError={stateError} section={activeTab} />
          </div>
        </div>
      )}
    </aside>
  )
}
