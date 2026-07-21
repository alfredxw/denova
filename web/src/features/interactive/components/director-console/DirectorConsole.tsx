import { useEffect, useMemo, useState } from 'react'
import type { Snapshot, StoryDirector, StorySummary } from '../../types'
import { splitStoryStateFacts, stateChanges } from '../story-state/model'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { readStoredStatePanelTab, writeStoredStatePanelTab } from './persistence'
import { StatePanelTabs } from './StatePanelTabs'
import { StateView } from './StateView'
import type { DirectorStatusLike, StatePanelTab } from './types'
import { stateEntries } from './utils'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'

export interface DirectorConsoleProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  branchId: string
  snapshot: Snapshot | null
	stateError?: string
	stateDisplayPreference: StoryStateDisplayPreference
	onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  directorStatus?: DirectorStatusLike
  onOpenBackstage: () => void
}

// 右栏 = 状态感知栏：header 标题行内含导演台入口（状态+打开一体），
// 信息条默认展示导演/字数（行内编辑）/展示偏好 + 变化 / 角色 / 世界三个分区 tab。
export function DirectorConsole({
  storyId,
  story,
  storyDirectors = [],
  onDirectorChange,
  onReplyTargetCharsChange,
  branchId,
  snapshot,
	stateError,
	stateDisplayPreference,
	onStateDisplayPreferenceChange,
  directorStatus,
  onOpenBackstage,
}: DirectorConsoleProps) {
  const [activeTab, setActiveTab] = useState<StatePanelTab>(() => readStoredStatePanelTab(storyId) || 'actors')

  // 分区选择按故事持久化；切故事时恢复该故事各自的上次选择。
  useEffect(() => {
    setActiveTab(readStoredStatePanelTab(storyId) || 'actors')
  }, [storyId])

  const changeTab = (tab: StatePanelTab) => {
    setActiveTab(tab)
    writeStoredStatePanelTab(storyId, tab)
  }

  const stateFacts = useMemo(() => stateEntries(snapshot?.state), [snapshot?.state])
  const { actors, archivedActors, worldFacts } = useMemo(() => splitStoryStateFacts(stateFacts), [stateFacts])
  const changesCount = useMemo(() => stateChanges(snapshot?.current_turn?.state_delta).length, [snapshot?.current_turn?.state_delta])

  return (
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader branchId={branchId} turnCount={(snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0)} story={story} storyDirectors={storyDirectors} onDirectorChange={onDirectorChange} onReplyTargetCharsChange={onReplyTargetCharsChange} stateDisplayPreference={stateDisplayPreference} onStateDisplayPreferenceChange={onStateDisplayPreferenceChange} directorStatus={directorStatus} onOpenBackstage={onOpenBackstage} />
      <StatePanelTabs activeTab={activeTab} onChange={changeTab} changesCount={changesCount} actorsCount={actors.length + archivedActors.length} worldCount={worldFacts.length} />
      <div className="min-h-0 flex-1 overflow-hidden px-4 py-4">
        <div className="director-console__scroll h-full min-h-0 overflow-y-auto pb-4 pr-1">
          <StateView snapshot={snapshot} stateFacts={stateFacts} syncError={stateError} section={activeTab} />
        </div>
      </div>
    </aside>
  )
}
