import { useCallback, useEffect, useRef, useState } from 'react'
import { GripHorizontal, GripVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import { BranchTimeline } from './BranchTimeline'
import { SettingPanel, type SettingPanelMode } from './SettingPanel'
import { SnapshotPanel } from './SnapshotPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import { novaEase, panelPresence, subtlePresence } from '@/features/motion/motion-tokens'
import type { Snapshot } from '../types'

interface InteractiveLayoutProps {
  workspace?: string
  styleSuggestions?: string[]
  loreEmpty?: boolean
  onRequestLoreInit?: () => void
  rightPanelVisible?: boolean
  onToggleRightPanel?: () => void
}

export function InteractiveLayout({ workspace, styleSuggestions = [], loreEmpty = false, onRequestLoreInit, rightPanelVisible = true, onToggleRightPanel }: InteractiveLayoutProps) {
  const { t } = useTranslation()
  const { stories, tellers, branches, snapshot, currentStoryId, currentBranchId, submode, setStories, setTellers, setBranches, setSnapshot, setCurrentStoryId, setCurrentBranchId, setSubmode, resetWorkspaceState } = useInteractiveStore()
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const currentBranchSnapshot = snapshot?.story_id === currentStoryId && snapshot.branch_id === currentBranchId ? snapshot : null
  const snapshotStoryIdRef = useRef('')
  const snapshotRequestSeqRef = useRef(0)
  const lastStableSnapshotRef = useRef<Snapshot | null>(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotLoadFailed, setSnapshotLoadFailed] = useState(false)

  if (currentBranchSnapshot) {
    lastStableSnapshotRef.current = currentBranchSnapshot
  }
  const fallbackSnapshot = lastStableSnapshotRef.current?.story_id === currentStoryId ? lastStableSnapshotRef.current : null
  const snapshotPending = !snapshotLoadFailed && Boolean(currentStoryId) && !currentBranchSnapshot && (snapshotLoading || !snapshot || snapshot.story_id !== currentStoryId || snapshot.branch_id !== currentBranchId)
  const displaySnapshot = currentBranchSnapshot ?? (snapshotPending ? fallbackSnapshot : null)

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  const reloadStories = useCallback(async () => {
    const index = await getInteractiveStories()
    setStories(index.stories || [], index.current_story_id)
  }, [setStories])

  const reloadSnapshot = useCallback(
    async (branchOverride?: string, storyOverride?: string) => {
      const requestSeq = snapshotRequestSeqRef.current + 1
      snapshotRequestSeqRef.current = requestSeq
      const storyId = storyOverride || currentStoryId
      if (!storyId) {
        setSnapshotLoading(false)
        setSnapshot(null)
        return
      }
      setSnapshotLoading(true)
      setSnapshotLoadFailed(false)
      const branchId = branchOverride ?? (snapshotStoryIdRef.current === storyId ? currentBranchId : '')
      try {
        const [nextSnapshot, nextBranches] = await Promise.all([getInteractiveSnapshot(storyId, branchId), getInteractiveBranches(storyId)])
        if (requestSeq !== snapshotRequestSeqRef.current) return
        setSnapshot(nextSnapshot)
        setBranches(nextBranches)
        return nextSnapshot
      } catch (error) {
        if (requestSeq === snapshotRequestSeqRef.current) {
          console.error('[interactive-layout] 刷新互动快照失败', error)
          setSnapshotLoadFailed(true)
        }
        throw error
      } finally {
        if (requestSeq === snapshotRequestSeqRef.current) setSnapshotLoading(false)
      }
    },
    [currentBranchId, currentStoryId, setBranches, setSnapshot],
  )

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

  const handleCreateStory = async (input: { title: string; origin: string; story_teller_id: string; reply_target_chars: number }) => {
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

  const handleReplyTargetCharsChange = async (replyTargetChars: number) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, {
      reply_target_chars: replyTargetChars,
    })
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
    const branch = await createInteractiveBranch(currentStoryId, {
      parent_event_id: turnId,
      title,
    })
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
  const contentKey = settingsWorkspaceVisible ? `settings:${settingMode}` : submode === 'timeline' ? 'timeline' : 'story'
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <motion.div key={contentKey} variants={panelPresence} initial="initial" animate="animate" transition={{ duration: 0.2, ease: novaEase }} className="flex min-h-0 flex-1 flex-col">
              {settingsWorkspaceVisible ? (
                <SettingPanel mode={settingMode} workspace={workspace} tellers={tellers} onTellersChange={setTellers} />
              ) : submode === 'timeline' ? (
                <BranchTimeline snapshot={displaySnapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} fill variant="workspace" onBackToStory={() => setSubmode('story')} headerControls={<StoryPicker stories={stories} currentStoryId={currentStoryId} tellers={tellers} onSelect={setCurrentStoryId} onCreate={handleCreateStory} onDelete={handleDeleteStory} />} />
              ) : (
                <Group id="nova-interactive-horizontal" defaultLayout={readStoredLayout('nova-interactive-horizontal')} onLayoutChanged={(layout) => storeLayout('nova-interactive-horizontal', layout)} orientation="horizontal" className="min-h-0 flex-1">
                  <Panel id="story-stage" minSize="240px" className="min-w-0">
                    <StoryStage workspace={workspace} styleSuggestions={styleSuggestions} stories={stories} story={currentStory} tellers={tellers} storyId={currentStoryId} branchId={currentBranchId} snapshot={displaySnapshot} snapshotLoading={snapshotPending} loreEmpty={loreEmpty} sceneMemoryVisible={rightPanelVisible} onStorySelect={setCurrentStoryId} onStoryCreate={handleCreateStory} onStoryDelete={handleDeleteStory} onTellerChange={handleTellerChange} onReplyTargetCharsChange={handleReplyTargetCharsChange} onRequestLoreInit={onRequestLoreInit} onToggleSceneMemory={onToggleRightPanel} onDone={reloadSnapshot} />
                  </Panel>
                  {rightPanelVisible && (
                    <>
                      <InteractiveResizeHandle direction="vertical" label={t('interactiveLayout.resizeSceneMemory')} />
                      <Panel id="snapshot" defaultSize="320px" minSize="180px" maxSize="45%" className="min-w-0">
                        <motion.div className="h-full min-h-0" variants={subtlePresence} initial="initial" animate="animate" transition={{ duration: 0.16, ease: novaEase }}>
                          <SnapshotPanel snapshot={displaySnapshot} loading={snapshotPending} />
                        </motion.div>
                      </Panel>
                    </>
                  )}
                </Group>
              )}
            </motion.div>
          </div>
        </div>
      </div>
    </div>
  )
}

function InteractiveResizeHandle({ direction, label, prominent = false }: { direction: 'horizontal' | 'vertical'; label: string; prominent?: boolean }) {
  const Icon = direction === 'vertical' ? GripVertical : GripHorizontal
  const className = direction === 'vertical' ? 'nova-resize-handle group -mx-1 flex w-3 cursor-col-resize items-center justify-center bg-transparent transition-colors' : `nova-resize-handle group ${prominent ? '-my-0.5 h-4' : '-my-1 h-3'} flex cursor-row-resize items-center justify-center bg-transparent transition-colors`

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
