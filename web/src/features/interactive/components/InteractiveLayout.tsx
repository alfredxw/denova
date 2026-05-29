import { useCallback, useEffect, useRef } from 'react'
import { GripHorizontal, GripVertical } from 'lucide-react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import { BranchTimeline } from './BranchTimeline'
import { SettingPanel, type SettingPanelMode } from './SettingPanel'
import { SnapshotPanel } from './SnapshotPanel'
import { StoryStage } from './StoryStage'

interface InteractiveLayoutProps {
  workspace?: string
  rightPanelVisible?: boolean
}

export function InteractiveLayout({
  workspace,
  rightPanelVisible = true,
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

  const settingMode: SettingPanelMode = submode === 'story' || submode === 'timeline' ? 'lore' : submode
  const settingsWorkspaceVisible = submode !== 'story' && submode !== 'timeline'
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
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
                    <StoryStage
                      workspace={workspace}
                      stories={stories}
                      story={currentStory}
                      tellers={tellers}
                      storyId={currentStoryId}
                      branchId={currentBranchId}
                      snapshot={currentBranchSnapshot}
                      onStorySelect={setCurrentStoryId}
                      onStoryCreate={handleCreateStory}
                      onStoryDelete={handleDeleteStory}
                      onTellerChange={handleTellerChange}
                      onDone={reloadSnapshot}
                    />
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
