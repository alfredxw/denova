import { useCallback, useEffect, useRef } from 'react'
import { BookMarked, Database, GitBranch, GripHorizontal, GripVertical, MessageSquareText, PanelLeft, PanelRight, SlidersHorizontal } from 'lucide-react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import type { InteractiveSubmode } from '../types'
import { BranchTimeline } from './BranchTimeline'
import { SettingPanel, type SettingPanelMode } from './SettingPanel'
import { SnapshotPanel } from './SnapshotPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import { TellerPicker } from './TellerPicker'

interface InteractiveLayoutProps {
  workspace?: string
  leftPanelVisible?: boolean
  rightPanelVisible?: boolean
  onToggleLeftPanel?: () => void
  onToggleRightPanel?: () => void
}

export function InteractiveLayout({
  workspace,
  leftPanelVisible = true,
  rightPanelVisible = true,
  onToggleLeftPanel,
  onToggleRightPanel,
}: InteractiveLayoutProps) {
  const {
    stories, tellers, branches, snapshot, currentStoryId, currentBranchId, submode,
    setStories, setTellers, setBranches, setSnapshot, setCurrentStoryId, setCurrentBranchId, setSubmode, resetWorkspaceState,
  } = useInteractiveStore()
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const currentBranchSnapshot = snapshot?.story_id === currentStoryId && snapshot.branch_id === currentBranchId ? snapshot : null
  const snapshotStoryIdRef = useRef('')
  const snapshotRequestSeqRef = useRef(0)

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  const reloadStories = useCallback(async () => {
    const index = await getInteractiveStories()
    setStories(index.stories || [], index.current_story_id)
  }, [setStories])

  const reloadSnapshot = useCallback(async (branchOverride?: string, storyOverride?: string) => {
    const requestSeq = snapshotRequestSeqRef.current + 1
    snapshotRequestSeqRef.current = requestSeq
    const storyId = storyOverride || currentStoryId
    if (!storyId) {
      setSnapshot(null)
      return
    }
    const branchId = branchOverride ?? (snapshotStoryIdRef.current === storyId ? currentBranchId : '')
    const [nextSnapshot, nextBranches] = await Promise.all([
      getInteractiveSnapshot(storyId, branchId),
      getInteractiveBranches(storyId),
    ])
    if (requestSeq !== snapshotRequestSeqRef.current) return
    setSnapshot(nextSnapshot)
    setBranches(nextBranches)
  }, [currentBranchId, currentStoryId, setBranches, setSnapshot])

  useEffect(() => {
    snapshotRequestSeqRef.current += 1
    snapshotStoryIdRef.current = ''
    if (workspace !== undefined) {
      resetWorkspaceState()
      if (!workspace) return
    }
    void Promise.all([reloadStories(), getInteractiveTellers().then(setTellers)])
  }, [reloadStories, resetWorkspaceState, setTellers, workspace])

  useEffect(() => {
    void reloadSnapshot()
  }, [currentStoryId])

  useEffect(() => {
    if (snapshot?.current_turn?.state_status !== 'pending') return
    const timer = window.setInterval(() => {
      void reloadSnapshot(snapshot.branch_id)
    }, 1000)
    return () => window.clearInterval(timer)
  }, [reloadSnapshot, snapshot?.branch_id, snapshot?.current_turn?.id, snapshot?.current_turn?.state_status])

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
    const storyId = currentStoryId || useInteractiveStore.getState().currentStoryId || snapshot?.story_id
    if (!storyId) return
    await switchInteractiveBranch(storyId, branchId)
    setCurrentBranchId(branchId)
    await reloadSnapshot(branchId, storyId)
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
    if (branchId === currentBranchId) {
      setCurrentBranchId('main')
    }
    await reloadSnapshot(branchId === currentBranchId ? 'main' : undefined)
    await reloadStories()
  }

  const mainTabs: Array<{ value: InteractiveSubmode; label: string; icon: typeof MessageSquareText }> = [
    { value: 'story', label: '剧情', icon: MessageSquareText },
    { value: 'timeline', label: '剧情路线图', icon: GitBranch },
    { value: 'lore', label: '资料库', icon: Database },
    { value: 'creator', label: '创作者', icon: BookMarked },
    { value: 'teller', label: '讲述者', icon: SlidersHorizontal },
  ]
  const settingMode: SettingPanelMode = submode === 'story' || submode === 'timeline' ? 'lore' : submode
  const settingsWorkspaceVisible = submode !== 'story' && submode !== 'timeline'
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
          <aside className={`nova-sidebar flex shrink-0 flex-col gap-1 border-r p-3 transition-[width] duration-500 ease-[var(--nova-ease)] ${leftPanelVisible ? 'w-64' : 'w-16'}`} aria-label="互动页面切换">
            {leftPanelVisible && (
              <div className="mb-2 flex flex-col gap-3 border-b border-[var(--nova-border)] pb-4">
                <StoryPicker layout="sidebar" stories={stories} currentStoryId={currentStoryId} tellers={tellers} onSelect={setCurrentStoryId} onCreate={handleCreateStory} onDelete={handleDeleteStory} />
                <TellerPicker layout="sidebar" story={currentStory} tellers={tellers} onChange={handleTellerChange} />
              </div>
            )}
            <div className="flex min-h-0 flex-1 flex-col gap-1">
              {mainTabs.map((item) => {
                const Icon = item.icon
                const active = submode === item.value
                return (
                  <button
                    key={item.value}
                    type="button"
                    onClick={() => setSubmode(item.value)}
                    className={`nova-nav-item flex h-10 items-center text-left text-xs ${
                      leftPanelVisible ? 'gap-3 px-4' : 'justify-center px-0'
                    } ${active ? 'is-active' : ''}`}
                    aria-current={active ? 'page' : undefined}
                    aria-label={item.label}
                    title={item.label}
                  >
                    <Icon className="h-4 w-4 shrink-0" />
                    {leftPanelVisible && <span className="truncate font-medium">{item.label}</span>}
                  </button>
                )
              })}
            </div>
            <div className="mt-2 flex flex-col gap-1 border-t border-[var(--nova-border)] pt-3">
              <button
                type="button"
                className={`nova-nav-item flex h-10 items-center text-xs ${
                  leftPanelVisible ? 'gap-3 px-4' : 'justify-center px-0'
                } ${rightPanelVisible ? 'is-active' : ''}`}
                onClick={onToggleRightPanel}
                title={rightPanelVisible ? '隐藏场景记忆' : '显示场景记忆'}
                aria-label={rightPanelVisible ? '隐藏场景记忆' : '显示场景记忆'}
              >
                <PanelRight className="h-4 w-4 shrink-0" />
                {leftPanelVisible && <span className="truncate font-medium">场景记忆</span>}
              </button>
              <button
                type="button"
                className={`nova-nav-item flex h-10 items-center text-xs disabled:cursor-not-allowed disabled:opacity-50 ${
                  leftPanelVisible ? 'gap-3 px-4' : 'justify-center px-0'
                }`}
                onClick={onToggleLeftPanel}
                disabled={!onToggleLeftPanel}
                title={leftPanelVisible ? '收起左侧导航' : '展开左侧导航'}
                aria-label={leftPanelVisible ? '收起左侧导航' : '展开左侧导航'}
              >
                <PanelLeft className={`h-4 w-4 shrink-0 transition-transform ${leftPanelVisible ? '' : 'rotate-180'}`} />
                {leftPanelVisible && <span className="truncate font-medium">收起导航</span>}
              </button>
              {leftPanelVisible && (
                <div className="mt-2 flex items-center gap-2 px-4 py-1 text-[11px] text-[var(--nova-text-faint)]">
                  <span className={`h-2 w-2 rounded-full ${snapshot?.current_turn?.state_status === 'pending' ? 'bg-[var(--nova-accent)]' : 'bg-[var(--nova-accent-green)]'}`} />
                  <span className="truncate">{snapshot?.current_turn?.state_status === 'pending' ? '场景同步中' : '已同步'}</span>
                </div>
              )}
            </div>
          </aside>
          <div className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            {settingsWorkspaceVisible ? (
              <SettingPanel
                mode={settingMode}
                workspace={workspace}
                tellers={tellers}
                onTellersChange={setTellers}
              />
            ) : (
              submode === 'timeline' ? (
                <BranchTimeline
                  snapshot={snapshot}
                  branches={branches}
                  currentBranchId={currentBranchId}
                  onSwitchBranch={handleSwitchBranch}
                  onCreateBranch={handleCreateBranch}
                  onDeleteBranch={handleDeleteBranch}
                  fill
                  variant="workspace"
                  onBackToStory={() => setSubmode('story')}
                />
              ) : (
                <Group
                  id="nova-interactive-horizontal"
                  defaultLayout={readStoredLayout('nova-interactive-horizontal')}
                  onLayoutChanged={(layout) => storeLayout('nova-interactive-horizontal', layout)}
                  orientation="horizontal"
                  className="min-h-0 flex-1"
                >
                  <Panel id="story-stage" minSize="240px" className="min-w-0">
                    <StoryStage workspace={workspace} storyId={currentStoryId} branchId={currentBranchId} snapshot={currentBranchSnapshot} onDone={reloadSnapshot} />
                  </Panel>
                  {rightPanelVisible && (
                    <>
                      <InteractiveResizeHandle direction="vertical" label="调整场景记忆宽度" />
                      <Panel id="snapshot" defaultSize="320px" minSize="180px" maxSize="45%" className="min-w-0">
                        <SnapshotPanel snapshot={currentBranchSnapshot} />
                      </Panel>
                    </>
                  )}
                </Group>
              )
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function InteractiveResizeHandle({ direction, label, prominent = false }: { direction: 'horizontal' | 'vertical'; label: string; prominent?: boolean }) {
  const Icon = direction === 'vertical' ? GripVertical : GripHorizontal
  const className = direction === 'vertical'
    ? 'nova-resize-handle group -mx-1 flex w-3 cursor-col-resize items-center justify-center bg-transparent transition-colors'
    : `nova-resize-handle group ${prominent ? '-my-0.5 h-4' : '-my-1 h-3'} flex cursor-row-resize items-center justify-center bg-transparent transition-colors`

  return (
    <Separator aria-label={label} className={className}>
      <span className={`flex items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)] shadow-[0_4px_14px_rgba(0,0,0,0.22)] transition-colors group-hover:border-[var(--nova-active)] group-data-[resize-handle-active]:border-[var(--nova-active)] group-data-[resize-handle-active]:text-[var(--nova-text)] ${direction === 'vertical' ? 'h-9 w-2.5' : 'h-2.5 w-16'}`}>
        <Icon className={direction === 'vertical' ? 'h-3.5 w-3.5' : 'h-3 w-3'} aria-hidden="true" />
      </span>
    </Separator>
  )
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
