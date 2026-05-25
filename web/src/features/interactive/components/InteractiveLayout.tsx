import { useCallback, useEffect, useRef } from 'react'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import type { InteractiveSubmode } from '../types'
import { BranchTimeline } from './BranchTimeline'
import { SettingPanel } from './SettingPanel'
import { SnapshotPanel } from './SnapshotPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import { TellerPicker } from './TellerPicker'

export function InteractiveLayout() {
  const {
    stories, tellers, branches, snapshot, currentStoryId, currentBranchId, submode,
    setStories, setTellers, setBranches, setSnapshot, setCurrentStoryId, setCurrentBranchId, setSubmode,
  } = useInteractiveStore()
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const snapshotStoryIdRef = useRef('')

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  const reloadStories = useCallback(async () => {
    const index = await getInteractiveStories()
    setStories(index.stories || [], index.current_story_id)
  }, [setStories])

  const reloadSnapshot = useCallback(async (branchOverride?: string) => {
    if (!currentStoryId) {
      setSnapshot(null)
      return
    }
    const branchId = branchOverride ?? (snapshotStoryIdRef.current === currentStoryId ? currentBranchId : '')
    const [nextSnapshot, nextBranches] = await Promise.all([
      getInteractiveSnapshot(currentStoryId, branchId),
      getInteractiveBranches(currentStoryId),
    ])
    setSnapshot(nextSnapshot)
    setBranches(nextBranches)
  }, [currentBranchId, currentStoryId, setBranches, setSnapshot])

  useEffect(() => {
    void Promise.all([reloadStories(), getInteractiveTellers().then(setTellers)])
  }, [reloadStories, setTellers])

  useEffect(() => {
    void reloadSnapshot()
  }, [reloadSnapshot])

  const handleCreateStory = async (input: { title: string; origin: string; story_teller_id: string }) => {
    const story = await createInteractiveStory(input)
    await reloadStories()
    setCurrentStoryId(story.id)
  }

  const handleDeleteStory = async (storyId: string) => {
    await deleteInteractiveStory(storyId)
    await reloadStories()
  }

  const handleTellerChange = async (tellerId: string) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, { story_teller_id: tellerId })
    await reloadStories()
  }

  const handleSwitchBranch = async (branchId: string) => {
    if (!currentStoryId) return
    await switchInteractiveBranch(currentStoryId, branchId)
    setCurrentBranchId(branchId)
    await reloadSnapshot(branchId)
  }

  const handleCreateBranch = async (turnId: string) => {
    if (!currentStoryId) return
    const branch = await createInteractiveBranch(currentStoryId, { parent_event_id: turnId, title: `分支 ${branches.length + 1}` })
    setCurrentBranchId(branch.id)
    await reloadSnapshot(branch.id)
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#18191b] p-3 text-[#d7dbe2]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-[#30343b] bg-[#18191b] shadow-[0_12px_36px_rgba(0,0,0,0.22)]">
        <div className="relative flex h-[42px] shrink-0 items-center gap-3 border-b border-[#30343b] bg-[#202226] px-3">
          <StoryPicker stories={stories} currentStoryId={currentStoryId} tellers={tellers} onSelect={setCurrentStoryId} onCreate={handleCreateStory} onDelete={handleDeleteStory} />
          <TellerPicker story={currentStory} tellers={tellers} onChange={handleTellerChange} />
          <Badge variant="outline" className="border-[#30343b] bg-[#2a2d34] text-[#aab2c0]">{currentStory ? `${currentStory.events} events` : '未选择故事'}</Badge>
          <Tabs className="ml-auto" value={submode} onValueChange={(value) => setSubmode(value as InteractiveSubmode)}>
            <TabsList className="h-8 bg-[#252831]">
              <TabsTrigger value="story" className="px-3 text-xs">story</TabsTrigger>
              <TabsTrigger value="setting" className="px-3 text-xs">setting</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
        <div className="flex min-h-0 flex-1">
          <SettingPanel />
          <StoryStage storyId={currentStoryId} branchId={currentBranchId} snapshot={snapshot} onDone={reloadSnapshot} />
          <SnapshotPanel snapshot={snapshot} />
        </div>
        <BranchTimeline snapshot={snapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} />
      </div>
    </div>
  )
}
