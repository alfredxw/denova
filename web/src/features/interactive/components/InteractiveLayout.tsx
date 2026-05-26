import { useCallback, useEffect, useRef } from 'react'
import { BookOpen, CheckCircle2, Layers3, Lightbulb, PenLine, ScrollText } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import type { InteractiveSubmode } from '../types'
import { BranchTimeline } from './BranchTimeline'
import { SettingPanel } from './SettingPanel'
import { SnapshotPanel } from './SnapshotPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import { TellerPicker } from './TellerPicker'

interface InteractiveLayoutProps {
  leftPanelVisible?: boolean
  rightPanelVisible?: boolean
}

export function InteractiveLayout({
  leftPanelVisible = true,
  rightPanelVisible = true,
}: InteractiveLayoutProps) {
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

  const handleCreateBranch = async (turnId: string, title: string) => {
    if (!currentStoryId) return
    const branch = await createInteractiveBranch(currentStoryId, { parent_event_id: turnId, title })
    setCurrentBranchId(branch.id)
    await reloadSnapshot(branch.id)
  }

  const handleDeleteBranch = async (branchId: string) => {
    if (!currentStoryId) return
    await deleteInteractiveBranch(currentStoryId, branchId)
    if (branchId === currentBranchId) setCurrentBranchId('main')
    await reloadSnapshot(branchId === currentBranchId ? 'main' : undefined)
    await reloadStories()
  }

  const workflow = [
    { label: '灵感', icon: Lightbulb },
    { label: '设定', icon: ScrollText },
    { label: '大纲', icon: Layers3 },
    { label: '章节', icon: BookOpen },
    { label: '正文', icon: PenLine, active: submode === 'story' },
    { label: '检查', icon: CheckCircle2 },
  ]

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#15171a] p-3 text-[#d7dbe2]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-[#2f3540] bg-[#17191d] shadow-[0_18px_48px_rgba(0,0,0,0.26)]">
        <div className="relative flex min-h-[64px] shrink-0 flex-wrap items-center gap-3 border-b border-[#2f3540] bg-[#1d2026] px-4 py-3">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <StoryPicker stories={stories} currentStoryId={currentStoryId} tellers={tellers} onSelect={setCurrentStoryId} onCreate={handleCreateStory} onDelete={handleDeleteStory} />
            <TellerPicker story={currentStory} tellers={tellers} onChange={handleTellerChange} />
            <Badge variant="outline" className="h-7 border-[#3a414d] bg-[#252a33] px-2.5 text-[#aab2c0]">{currentStory ? `${currentStory.events} 个事件` : '未选择故事'}</Badge>
          </div>
          <nav className="flex items-center gap-1 rounded-lg border border-[#303743] bg-[#171a20] p-1" aria-label="创作流程">
            {workflow.map((item) => {
              const Icon = item.icon
              return (
                <button
                  key={item.label}
                  type="button"
                  className={`flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition ${item.active ? 'bg-[#2d6fb8] text-white shadow-[0_6px_18px_rgba(45,111,184,0.28)]' : 'text-[#8f98a8] hover:bg-[#232832] hover:text-[#d9dee7]'}`}
                >
                  <Icon className="h-3.5 w-3.5" />
                  {item.label}
                </button>
              )
            })}
          </nav>
          <Tabs value={submode} onValueChange={(value) => setSubmode(value as InteractiveSubmode)}>
            <TabsList className="h-8 bg-[#252a33]">
              <TabsTrigger value="story" className="px-3 text-xs">故事</TabsTrigger>
              <TabsTrigger value="setting" className="px-3 text-xs">设定</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
        <div className="flex min-h-0 flex-1">
          {leftPanelVisible && <SettingPanel />}
          <StoryStage storyId={currentStoryId} branchId={currentBranchId} snapshot={snapshot} onDone={reloadSnapshot} />
          {rightPanelVisible && <SnapshotPanel snapshot={snapshot} />}
        </div>
        <BranchTimeline snapshot={snapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} />
      </div>
    </div>
  )
}
