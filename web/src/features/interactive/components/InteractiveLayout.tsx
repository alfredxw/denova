import { useCallback, useEffect, useRef, useState } from 'react'
import { BookOpen, CheckCircle2, Layers3, Lightbulb, PenLine, ScrollText } from 'lucide-react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout, PanelImperativeHandle, PanelSize } from 'react-resizable-panels'
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
  const snapshotRequestSeqRef = useRef(0)
  const [timelineExpanded, setTimelineExpanded] = useState(false)
  const timelinePanelRef = useRef<PanelImperativeHandle | null>(null)
  const timelineExpandedHeightRef = useRef(360)

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  const reloadStories = useCallback(async () => {
    const index = await getInteractiveStories()
    setStories(index.stories || [], index.current_story_id)
  }, [setStories])

  const reloadSnapshot = useCallback(async (branchOverride?: string) => {
    const requestSeq = snapshotRequestSeqRef.current + 1
    snapshotRequestSeqRef.current = requestSeq
    if (!currentStoryId) {
      setSnapshot(null)
      return
    }
    const branchId = branchOverride ?? (snapshotStoryIdRef.current === currentStoryId ? currentBranchId : '')
    const [nextSnapshot, nextBranches] = await Promise.all([
      getInteractiveSnapshot(currentStoryId, branchId),
      getInteractiveBranches(currentStoryId),
    ])
    if (requestSeq !== snapshotRequestSeqRef.current) return
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
    setSnapshot(null)
    await reloadSnapshot(branchId)
  }

  const handleCreateBranch = async (turnId: string, title: string) => {
    if (!currentStoryId) return
    const branch = await createInteractiveBranch(currentStoryId, { parent_event_id: turnId, title })
    setCurrentBranchId(branch.id)
    setSnapshot(null)
    await reloadSnapshot(branch.id)
  }

  const handleDeleteBranch = async (branchId: string) => {
    if (!currentStoryId) return
    await deleteInteractiveBranch(currentStoryId, branchId)
    if (branchId === currentBranchId) {
      setCurrentBranchId('main')
      setSnapshot(null)
    }
    await reloadSnapshot(branchId === currentBranchId ? 'main' : undefined)
    await reloadStories()
  }

  const handleTimelineExpandedChange = (nextExpanded: boolean) => {
    setTimelineExpanded(nextExpanded)
    window.requestAnimationFrame(() => {
      if (nextExpanded) {
        timelinePanelRef.current?.resize(`${Math.max(280, timelineExpandedHeightRef.current)}px`)
      } else {
        timelinePanelRef.current?.resize('52px')
      }
    })
  }

  const handleTimelineResize = (size: PanelSize) => {
    if (timelineExpanded) timelineExpandedHeightRef.current = Math.max(280, size.inPixels)
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
        <Group
          id="nova-interactive-main-vertical"
          defaultLayout={readStoredLayout('nova-interactive-main-vertical')}
          onLayoutChanged={(layout) => storeLayout('nova-interactive-main-vertical', layout)}
          orientation="vertical"
          className="min-h-0 flex-1"
        >
          <Panel id="stage-area" minSize="240px" className="min-h-0">
            <Group
              id="nova-interactive-horizontal"
              defaultLayout={readStoredLayout('nova-interactive-horizontal')}
              onLayoutChanged={(layout) => storeLayout('nova-interactive-horizontal', layout)}
              orientation="horizontal"
              className="min-h-0"
            >
              {leftPanelVisible && (
                <>
                  <Panel id="setting" defaultSize="280px" minSize="180px" maxSize="45%" className="min-w-0">
                    <SettingPanel />
                  </Panel>
                  <InteractiveResizeHandle direction="vertical" label="调整互动资料库宽度" />
                </>
              )}
              <Panel id="story-stage" minSize="240px" className="min-w-0">
                <StoryStage storyId={currentStoryId} branchId={currentBranchId} snapshot={snapshot} onDone={reloadSnapshot} />
              </Panel>
              {rightPanelVisible && (
                <>
                  <InteractiveResizeHandle direction="vertical" label="调整场景记忆宽度" />
                  <Panel id="snapshot" defaultSize="320px" minSize="180px" maxSize="45%" className="min-w-0">
                    <SnapshotPanel snapshot={snapshot} />
                  </Panel>
                </>
              )}
            </Group>
          </Panel>
          <InteractiveResizeHandle direction="horizontal" label="调整剧情路线图高度" />
          <Panel
            id="branch-timeline"
            defaultSize="52px"
            minSize={timelineExpanded ? '180px' : '52px'}
            maxSize="60%"
            className="min-h-0"
            onResize={handleTimelineResize}
            panelRef={timelinePanelRef}
          >
            <BranchTimeline
              snapshot={snapshot}
              branches={branches}
              currentBranchId={currentBranchId}
              onSwitchBranch={handleSwitchBranch}
              onCreateBranch={handleCreateBranch}
              onDeleteBranch={handleDeleteBranch}
              expanded={timelineExpanded}
              fill
              onExpandedChange={handleTimelineExpandedChange}
            />
          </Panel>
        </Group>
      </div>
    </div>
  )
}

function InteractiveResizeHandle({ direction, label }: { direction: 'horizontal' | 'vertical'; label: string }) {
  const className = direction === 'vertical'
    ? '-mx-1 w-2 cursor-col-resize bg-transparent transition-colors hover:bg-[#2f7dd3]/40 data-[resize-handle-active]:bg-[#2f7dd3]/60'
    : '-my-1 h-2 cursor-row-resize bg-transparent transition-colors hover:bg-[#2f7dd3]/40 data-[resize-handle-active]:bg-[#2f7dd3]/60'

  return <Separator aria-label={label} className={className} />
}

function readStoredLayout(key: string): Layout | undefined {
  if (typeof window === 'undefined') return undefined
  const value = window.localStorage.getItem(key)
  if (!value) return undefined
  try {
    return JSON.parse(value) as Layout
  } catch {
    return undefined
  }
}

function storeLayout(key: string, layout: Layout) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(key, JSON.stringify(layout))
}
