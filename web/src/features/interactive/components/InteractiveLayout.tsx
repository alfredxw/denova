import { useCallback, useEffect, useRef, useState, type KeyboardEventHandler, type PointerEventHandler } from 'react'
import { Gauge, GripHorizontal, GripVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Panel } from 'react-resizable-panels'
import { toast } from 'sonner'
import { useShallow } from 'zustand/react/shallow'
import { readOptionalProjectFile, type LoreItem } from '@/lib/api'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getGamePlanningTemplates, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, selectInteractiveStory, switchInteractiveBranch, updateInteractiveBranchPlan, updateInteractiveStory } from '../api'
import { branchPlanSnapshotAfterUpdate } from '../branch-plan-snapshot'
import { useInteractiveStore } from '../stores/interactive-store'
import { BranchTimeline } from './BranchTimeline'
import { DirectorPanel } from './DirectorPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import { CreateBranchDialog } from './branching/CreateBranchDialog'
import type { BranchCreationSource } from './branching/model'
import {
  readStoryStateDisplayPreference,
  writeStoryStateDisplayPreference,
  type StoryStateDisplayPreference,
} from './story-state/display-preference'
import { novaEase, panelPresence } from '@/features/motion/motion-tokens'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost } from '@/components/layout/mobile-pane-host'
import { CollapsiblePanelSeparator, CollapsibleResizablePanel, PanelMotionGroup } from '@/components/layout/panel-motion'
import { usePersistedPanelLayout } from '@/components/layout/use-persisted-panel-layout'
import type { ImagePreset, InteractiveStoryUpdateInput, InteractiveTurnPersistedEvent, Snapshot, StorySummary } from '../types'
import { INTERACTIVE_OPENING_PRESET_PATH, INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, LEGACY_INTERACTIVE_OPENING_PRESET_PATH, parseBookOpeningPresets, type BookOpeningPreset, type StoryCreateInput } from '../opening'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from '../narrative-style'
import { LoadingState } from '@/components/common/LoadingState'

interface InteractiveLayoutProps {
  projectId?: string
  workspace?: string
  active?: boolean
  recentNarrativeStyleID?: string
  narrativeStyleLoading?: boolean
  onNarrativeStyleChange?: (id: string) => void | Promise<unknown>
  imagePresets?: ImagePreset[]
  loreEmpty?: boolean
  loreItems?: LoreItem[]
  onRequestLoreInit?: () => void
  onOpenPresets?: () => void
  rightPanelVisible?: boolean
  onToggleRightPanel?: () => void
}

const SNAPSHOT_POLL_INTERVAL_MS = 1000

export function InteractiveLayout({ projectId = '', workspace, active = true, recentNarrativeStyleID = DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleLoading = false, onNarrativeStyleChange, imagePresets = [], loreEmpty = false, loreItems = [], onRequestLoreInit, onOpenPresets, rightPanelVisible = true, onToggleRightPanel }: InteractiveLayoutProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const {
    stories,
    tellers,
    planningTemplates,
    branches,
    snapshot,
    currentStoryId,
    currentBranchId,
    submode,
    setStories,
    setTellers,
    setPlanningTemplates,
    setBranches,
    setSnapshot,
    applyTurnPersisted,
    setCurrentStoryId,
    setCurrentBranchId,
    setSubmode,
    resetWorkspaceState,
  } = useInteractiveStore(useShallow((state) => ({
    stories: state.stories,
    tellers: state.tellers,
    planningTemplates: state.planningTemplates,
    branches: state.branches,
    snapshot: state.snapshot,
    currentStoryId: state.currentStoryId,
    currentBranchId: state.currentBranchId,
    submode: state.submode,
    setStories: state.setStories,
    setTellers: state.setTellers,
    setPlanningTemplates: state.setPlanningTemplates,
    setBranches: state.setBranches,
    setSnapshot: state.setSnapshot,
    applyTurnPersisted: state.applyTurnPersisted,
    setCurrentStoryId: state.setCurrentStoryId,
    setCurrentBranchId: state.setCurrentBranchId,
    setSubmode: state.setSubmode,
    resetWorkspaceState: state.resetWorkspaceState,
  })))
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const currentTeller = resolveNarrativeStyle(tellers, currentStory?.story_teller_id)
  const styleSceneSuggestions = Array.from(new Set((currentTeller?.style_rules || []).map((rule) => rule.scene.trim()).filter((scene) => scene && !isGlobalStyleSceneName(scene))))
  const currentBranchSnapshot = snapshot?.story_id === currentStoryId && snapshot.branch_id === currentBranchId ? snapshot : null
  const storyIndexRequestSeqRef = useRef(0)
  const snapshotStoryIdRef = useRef('')
  const snapshotRequestSeqRef = useRef(0)
  const storySelectionQueueRef = useRef<Promise<void>>(Promise.resolve())
  const lastStableSnapshotRef = useRef<Snapshot | null>(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotLoadFailed, setSnapshotLoadFailed] = useState(false)
  const [storyIndexLoading, setStoryIndexLoading] = useState(true)
  const [mobileSnapshotOpen, setMobileSnapshotOpen] = useState(false)
  const [storyStateDisplayPreference, setStoryStateDisplayPreference] = useState(readStoryStateDisplayPreference)
  const [bookOpeningPresets, setBookOpeningPresets] = useState<BookOpeningPreset[]>([])
  const [branchCreationSource, setBranchCreationSource] = useState<BranchCreationSource | null>(null)
  const storyPanelLayout = usePersistedPanelLayout({
    storageKey: 'nova-interactive-horizontal',
    panelIds: ['story-stage', 'snapshot'],
  })

  if (currentBranchSnapshot) {
    lastStableSnapshotRef.current = currentBranchSnapshot
  }
  const fallbackSnapshot = lastStableSnapshotRef.current?.story_id === currentStoryId ? lastStableSnapshotRef.current : null
  const snapshotPending = !snapshotLoadFailed && Boolean(currentStoryId) && !currentBranchSnapshot && (snapshotLoading || !snapshot || snapshot.story_id !== currentStoryId || snapshot.branch_id !== currentBranchId)
  const displaySnapshot = currentBranchSnapshot ?? (snapshotPending ? fallbackSnapshot : null)
  const currentStageKey = `${workspace || 'current'}:${currentStoryId || 'none'}:${currentBranchId || displaySnapshot?.branch_id || 'main'}`
  const branchPlanRunActive = useInteractiveStore((state) => {
    const run = state.storyStageRuns[currentStageKey]
    return Boolean(run && (run.streaming || run.runtime.phase !== 'idle'))
  })
  const branchPlanEditingDisabled = !currentBranchSnapshot || branchPlanRunActive

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  useEffect(() => {
    setBranchCreationSource(null)
  }, [currentStoryId])

  const reloadStories = useCallback(async (preferredStory?: StorySummary) => {
    const requestSeq = storyIndexRequestSeqRef.current + 1
    storyIndexRequestSeqRef.current = requestSeq
    const index = await getInteractiveStories()
    if (requestSeq !== storyIndexRequestSeqRef.current) return
    setStories(mergePreferredStory(index.stories || [], preferredStory), preferredStory?.id || index.current_story_id)
  }, [setStories])

  const reloadBookOpeningPreset = useCallback(async () => {
    if (!projectId) {
      setBookOpeningPresets([])
      return
    }
    try {
      const data = await readOptionalProjectFile(projectId, INTERACTIVE_OPENING_PRESET_PATH)
      if (data) {
        setBookOpeningPresets(parseBookOpeningPresets(data.content || ''))
        return
      }
      const legacy = await readOptionalProjectFile(projectId, LEGACY_INTERACTIVE_OPENING_PRESET_PATH)
      setBookOpeningPresets(legacy ? parseBookOpeningPresets(legacy.content || '') : [])
    } catch {
      setBookOpeningPresets([])
    }
  }, [projectId])

  const reloadSnapshot = useCallback(
    async (branchOverride?: string, storyOverride?: string, options?: { silent?: boolean; includeBranches?: boolean }) => {
      const silent = options?.silent === true
      const includeBranches = options?.includeBranches !== false
      const requestSeq = snapshotRequestSeqRef.current + 1
      snapshotRequestSeqRef.current = requestSeq
      const storyId = storyOverride || currentStoryId
      if (!storyId) {
        if (!silent) {
          setSnapshotLoading(false)
          setSnapshot(null)
        }
        return
      }
      if (!silent) {
        setSnapshotLoading(true)
        setSnapshotLoadFailed(false)
      }
      const branchId = branchOverride ?? (snapshotStoryIdRef.current === storyId || currentBranchId !== 'main' ? currentBranchId : '')
      try {
        const [nextSnapshot, nextBranches] = await Promise.all([
          getInteractiveSnapshot(storyId, branchId),
          includeBranches ? getInteractiveBranches(storyId) : Promise.resolve(null),
        ])
        if (requestSeq !== snapshotRequestSeqRef.current) return
        setSnapshot(nextSnapshot)
        if (nextBranches) setBranches(nextBranches)
        return nextSnapshot
      } catch (error) {
        if (requestSeq === snapshotRequestSeqRef.current) {
          console.error('[interactive-layout] Failed to refresh interactive snapshot', error)
          if (!silent) setSnapshotLoadFailed(true)
        }
        if (silent) return
        throw error
      } finally {
        if (!silent && requestSeq === snapshotRequestSeqRef.current) setSnapshotLoading(false)
      }
    },
    [currentBranchId, currentStoryId, setBranches, setSnapshot],
  )

  useEffect(() => {
    let cancelled = false
    storyIndexRequestSeqRef.current += 1
    snapshotRequestSeqRef.current += 1
    snapshotStoryIdRef.current = ''
    if (workspace !== undefined) {
      resetWorkspaceState()
      if (!workspace) {
        setStoryIndexLoading(false)
        return () => { cancelled = true }
      }
    }
    setStoryIndexLoading(true)
    void Promise.all([
      reloadStories(),
      getInteractiveTellers().then(setTellers),
      getGamePlanningTemplates().then(setPlanningTemplates),
    ])
      .catch((error) => {
        if (!cancelled) console.error('[InteractiveLayout.tsx] failed to load the interactive workspace index', { workspace, error })
      })
      .finally(() => {
        if (!cancelled) setStoryIndexLoading(false)
      })
    return () => { cancelled = true }
  }, [reloadStories, resetWorkspaceState, setPlanningTemplates, setTellers, workspace])

  useEffect(() => {
    void reloadBookOpeningPreset()
    const onPresetUpdated = () => void reloadBookOpeningPreset()
    window.addEventListener(INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, onPresetUpdated)
    return () => window.removeEventListener(INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, onPresetUpdated)
  }, [reloadBookOpeningPreset])

  useEffect(() => {
    if (!active) return
    void reloadSnapshot()
  }, [active, currentStoryId, reloadSnapshot])

  useEffect(() => {
    const branchID = snapshot?.branch_id
    const storyID = snapshot?.story_id
    const statePending = snapshot?.current_turn?.state_status === 'pending'
    if (!active || !storyID || !branchID || !statePending) return
    let cancelled = false
    let timer: number | null = null
    const clearTimer = () => {
      if (timer === null) return
      window.clearTimeout(timer)
      timer = null
    }
    const schedule = () => {
      clearTimer()
      if (cancelled || document.visibilityState !== 'visible') return
      timer = window.setTimeout(() => {
        timer = null
        void reloadSnapshot(branchID, storyID, { silent: true, includeBranches: false }).finally(schedule)
      }, SNAPSHOT_POLL_INTERVAL_MS)
    }
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') schedule()
      else clearTimer()
    }
    schedule()
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      cancelled = true
      clearTimer()
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [active, reloadSnapshot, snapshot?.branch_id, snapshot?.current_turn?.id, snapshot?.current_turn?.state_status, snapshot?.story_id])

  useEffect(() => {
    if (!isMobile || submode !== 'story') setMobileSnapshotOpen(false)
  }, [isMobile, submode])

  const handleCreateStory = async (input: StoryCreateInput) => {
    const story = await createInteractiveStory(input)
    setCurrentStoryId(story.id)
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, story), story.id)
    await reloadStories(story)
    return story
  }

  const handleStorySelect = useCallback((storyId: string) => {
    if (!storyId || storyId === useInteractiveStore.getState().currentStoryId) return
    setCurrentStoryId(storyId)
    const persisted = storySelectionQueueRef.current
      .catch(() => undefined)
      .then(() => selectInteractiveStory(storyId))
    storySelectionQueueRef.current = persisted
    void persisted.catch((error) => {
      console.error('[interactive-layout] Failed to persist the active story', { storyId, error })
    })
  }, [setCurrentStoryId])

  const handleDeleteStories = async (storyIds: string[]) => {
    const uniqueStoryIds = Array.from(new Set(storyIds.filter(Boolean)))
    if (uniqueStoryIds.length === 0) return

    console.info('[interactive-layout] Deleting stories', { count: uniqueStoryIds.length, storyIds: uniqueStoryIds })
    const results = await Promise.allSettled(uniqueStoryIds.map((storyId) => deleteInteractiveStory(storyId)))
    await reloadStories()
    const failed = results.flatMap((result, index) => result.status === 'rejected' ? [{ storyId: uniqueStoryIds[index], reason: result.reason }] : [])
    if (failed.length > 0) {
      console.error('[interactive-layout] Failed to delete stories', { requested: uniqueStoryIds.length, failed })
      const reason = failed[0].reason
      throw reason instanceof Error ? reason : new Error(String(reason))
    }
    console.info('[interactive-layout] Stories deleted', { count: uniqueStoryIds.length })
  }

  const handleRenameStory = async (storyId: string, title: string) => {
    console.info('[interactive-layout] Renaming story', { storyId })
    const updated = await updateInteractiveStory(storyId, { title })
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, updated), updated.id)
    await reloadStories(updated)
    console.info('[interactive-layout] Story renamed', { storyId })
  }

  const handleStorySetupUpdate = async (input: StoryCreateInput) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, {
      title: input.title,
      origin: input.origin,
      protagonist: input.protagonist,
      story_teller_id: input.story_teller_id,
      planning_template_id: input.planning_template_id,
      planning_mode: input.planning_mode,
      module_refs: input.module_refs,
      reply_target_chars: input.reply_target_chars,
      choice_count: input.choice_count,
      image_settings: input.image_settings,
      check_settings: input.check_settings,
      opening: input.opening,
      state_schema_policy: input.state_schema_policy,
    })
    await reloadStories()
    await reloadSnapshot(undefined, currentStoryId, { silent: true })
  }

  const handlePlanningTemplateChange = async (templateId: string) => {
    if (!currentStoryId) return
    const updated = await updateInteractiveStory(currentStoryId, { planning_template_id: templateId })
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, updated), updated.id)
    await reloadStories(updated)
    await reloadSnapshot(undefined, currentStoryId, { silent: true })
  }

  const handleStoryUpdate = async (input: InteractiveStoryUpdateInput) => {
    if (!currentStoryId) return
    const updated = await updateInteractiveStory(currentStoryId, input)
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, updated), updated.id)
    await reloadStories(updated)
    if (input.module_refs || input.state_schema_policy) {
      await reloadSnapshot(undefined, currentStoryId, { silent: true })
    }
  }

  const handleBranchPlanUpdate = useCallback(async (markdown: string, baseRevision: string) => {
    const storyId = currentStoryId || useInteractiveStore.getState().currentStoryId
    const branchId = currentBranchId || useInteractiveStore.getState().currentBranchId
    if (!storyId || !branchId) throw new Error(t('directorPanel.plan.missingContext'))
    console.info('[interactive-layout] Updating branch plan', { storyId, branchId, baseRevision })
    const result = await updateInteractiveBranchPlan(storyId, branchId, {
      markdown,
      base_revision: baseRevision,
    })
    const currentState = useInteractiveStore.getState()
    const nextSnapshot = branchPlanSnapshotAfterUpdate({
      currentStoryId: currentState.currentStoryId,
      currentBranchId: currentState.currentBranchId,
      snapshot: currentState.snapshot,
      updatedStoryId: storyId,
      updatedBranchId: branchId,
      result,
    })
    if (nextSnapshot) {
      // Supersede an older snapshot request only while this branch is still
      // selected. A save response from the branch we just left must never
      // cancel the new branch's in-flight snapshot.
      snapshotRequestSeqRef.current += 1
      setSnapshotLoading(false)
      setSnapshot(nextSnapshot)
    }
    console.info('[interactive-layout] Branch plan updated', {
      storyId,
      branchId,
      revision: result.branch_plan.revision,
    })
  }, [currentBranchId, currentStoryId, setSnapshot, t])

  const handleStoryStateDisplayPreferenceChange = useCallback((value: StoryStateDisplayPreference) => {
    setStoryStateDisplayPreference(value)
    writeStoryStateDisplayPreference(value)
  }, [])

  const openDirectorState = useCallback(() => {
    if (isMobile) {
      setMobileSnapshotOpen(true)
      return
    }
    if (!rightPanelVisible) onToggleRightPanel?.()
  }, [isMobile, onToggleRightPanel, rightPanelVisible])

  const openBranchTimeline = useCallback(() => {
    setMobileSnapshotOpen(false)
    setSubmode('timeline')
  }, [setSubmode])

  const handleTurnPersisted = useCallback((event: InteractiveTurnPersistedEvent) => {
    const nextSnapshot = applyTurnPersisted(event) || undefined
    const persistedStory = useInteractiveStore.getState().stories.find((story) => story.id === event.story_id)
    if (persistedStory?.title_source === 'pending') {
      void reloadStories().catch((error) => {
        console.error('[interactive-layout] Failed to refresh the generated story title', { storyId: event.story_id, error })
      })
    }
    return nextSnapshot
  }, [applyTurnPersisted, reloadStories])

  const handleStoryStageDone = useCallback((options?: { silent?: boolean }) => {
    return reloadSnapshot(undefined, undefined, options)
  }, [reloadSnapshot])

  const handleSwitchBranch = async (branchId: string) => {
    const storyId = currentStoryId || useInteractiveStore.getState().currentStoryId || snapshot?.story_id
    if (!storyId) return
    await switchInteractiveBranch(storyId, branchId)
    setCurrentBranchId(branchId)
    await reloadSnapshot(branchId, storyId)
  }

  const handleCreateBranch = async (turnId: string, title: string, customAgentId?: string) => {
    const storyId = currentStoryId || useInteractiveStore.getState().currentStoryId
    if (!storyId) throw new Error(t('branchTimeline.createUnavailable'))
    const branch = await createInteractiveBranch(storyId, {
      parent_event_id: turnId,
      title,
      ...(customAgentId !== undefined ? { custom_agent_id: customAgentId } : {}),
    })
    setCurrentBranchId(branch.id)
    await reloadSnapshot(branch.id, storyId)
    toast.success(t('branchTimeline.createdAndSwitched', { name: branch.title || title }))
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

  if (storyIndexLoading) {
    return (
      <LoadingState
        label={t('interactiveLayout.loading')}
        className="h-full min-h-0 bg-[var(--nova-bg)]"
      />
    )
  }

  const contentKey = submode
  const directorPanelVisible = isMobile ? mobileSnapshotOpen : rightPanelVisible
  const storyStage = (
    <StoryStage
      projectId={projectId}
      workspace={workspace}
      styleSceneSuggestions={styleSceneSuggestions}
      stories={stories}
      story={currentStory}
      tellers={tellers}
      planningTemplates={planningTemplates}
      imagePresets={imagePresets}
      recentNarrativeStyleID={recentNarrativeStyleID}
      narrativeStyleLoading={narrativeStyleLoading}
      storyId={currentStoryId}
      branchId={currentBranchId}
      snapshot={displaySnapshot}
      snapshotLoading={snapshotPending}
      loreEmpty={loreEmpty}
      loreItems={loreItems}
      bookOpeningPresets={bookOpeningPresets}
      directorPanelVisible={directorPanelVisible}
      stateDisplayPreference={storyStateDisplayPreference}
      onStorySelect={handleStorySelect}
      onStoryCreate={handleCreateStory}
      onStorySetupUpdate={handleStorySetupUpdate}
      onNarrativeStyleChange={onNarrativeStyleChange}
      onStoryDelete={handleDeleteStories}
      onStoryRename={handleRenameStory}
      onRequestLoreInit={onRequestLoreInit}
      onOpenDirectorConfig={() => {
        onOpenPresets?.()
        setMobileSnapshotOpen(false)
      }}
      onToggleDirectorPanel={isMobile ? () => setMobileSnapshotOpen((open) => !open) : onToggleRightPanel}
      onOpenDirectorState={openDirectorState}
      onRequestCreateBranch={setBranchCreationSource}
      onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange}
      onTurnPersisted={handleTurnPersisted}
      onDone={handleStoryStageDone}
    />
  )
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <motion.div key={contentKey} variants={panelPresence} initial="initial" animate="animate" transition={{ duration: 0.2, ease: novaEase }} className="flex min-h-0 flex-1 flex-col">
              {submode === 'timeline' ? (
                <BranchTimeline projectId={projectId} snapshot={displaySnapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} fill variant="workspace" onBackToStory={() => setSubmode('story')} headerControls={<StoryPicker stories={stories} currentStoryId={currentStoryId} onSelect={handleStorySelect} onCreate={() => undefined} onDeleteStories={handleDeleteStories} onRenameStory={handleRenameStory} hideCreate />} />
              ) : isMobile ? (
                <MobilePaneHost
                  panes={[{
                    id: 'director-panel',
                    title: t('directorPanel.title'),
                    side: 'right',
                    icon: <Gauge className="h-4 w-4" />,
                    content: (
                      <DirectorPanel
                        storyId={currentStoryId}
                        story={currentStory}
                        planningTemplates={planningTemplates}
                        tellers={tellers}
                        imagePresets={imagePresets}
                        onPlanningTemplateChange={handlePlanningTemplateChange}
                        onStoryUpdate={handleStoryUpdate}
                        onOpenPresets={onOpenPresets}
                        branchId={currentBranchId}
                        branches={branches}
                        snapshot={displaySnapshot}
                        branchPlanEditingDisabled={branchPlanEditingDisabled}
                        onBranchPlanUpdate={handleBranchPlanUpdate}
                        stateDisplayPreference={storyStateDisplayPreference}
                        onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange}
                        onSwitchBranch={handleSwitchBranch}
                        onOpenBranchTimeline={openBranchTimeline}
                      />
                    ),
                  }]}
                  closeLabel={t('common.close')}
                  openPaneId={mobileSnapshotOpen ? 'director-panel' : null}
                  onOpenPaneChange={(id) => setMobileSnapshotOpen(id === 'director-panel')}
                  className="relative flex min-h-0 flex-1"
                >
                  {storyStage}
                </MobilePaneHost>
              ) : (
                <PanelMotionGroup
                  id="nova-interactive-horizontal"
                  defaultLayout={storyPanelLayout.defaultLayout}
                  onLayoutChanged={(layout) => {
                    if (rightPanelVisible) storyPanelLayout.persistUserLayout(layout)
                  }}
                  orientation="horizontal"
                  className="min-h-0 flex-1"
                >
                  <Panel id="story-stage" minSize="240px" className="min-w-0">
                    {storyStage}
                  </Panel>
                  <InteractiveResizeHandle
                    visible={rightPanelVisible}
                    direction="vertical"
                    label={t('interactiveLayout.resizeDirectorPanel')}
                    {...storyPanelLayout.resizeHandleIntentProps}
                  />
                  <CollapsibleResizablePanel
                    id="snapshot"
                    visible={rightPanelVisible}
                    side="right"
                    defaultSize="320px"
                    minSize="240px"
                    maxSize="45%"
                    className="min-w-[240px]"
                  >
                    <DirectorPanel
                      storyId={currentStoryId}
                      story={currentStory}
                      planningTemplates={planningTemplates}
                      tellers={tellers}
                      imagePresets={imagePresets}
                      onPlanningTemplateChange={handlePlanningTemplateChange}
                      onStoryUpdate={handleStoryUpdate}
                      onOpenPresets={onOpenPresets}
                      branchId={currentBranchId}
                      branches={branches}
                      snapshot={displaySnapshot}
                      branchPlanEditingDisabled={branchPlanEditingDisabled}
                      onBranchPlanUpdate={handleBranchPlanUpdate}
                      stateDisplayPreference={storyStateDisplayPreference}
                      onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange}
                      onSwitchBranch={handleSwitchBranch}
                      onOpenBranchTimeline={openBranchTimeline}
                    />
                  </CollapsibleResizablePanel>
                </PanelMotionGroup>
              )}
            </motion.div>
          </div>
        </div>
      </div>
      <CreateBranchDialog
        projectId={projectId}
        source={branchCreationSource}
        onClose={() => setBranchCreationSource(null)}
        onCreate={(source, title, customAgentId) => handleCreateBranch(source.turnId, title, customAgentId)}
      />
    </div>
  )
}

function isGlobalStyleSceneName(scene: string) {
  const normalized = scene.trim().toLowerCase()
  return normalized === '全局' || normalized === 'global'
}

function mergePreferredStory(stories: StorySummary[], preferredStory?: StorySummary) {
  if (!preferredStory) return stories
  let found = false
  const nextStories = stories.map((story) => {
    if (story.id !== preferredStory.id) return story
    found = true
    return preferredStory
  })
  return found ? nextStories : [preferredStory, ...nextStories]
}

function InteractiveResizeHandle({
  direction,
  label,
  prominent = false,
  visible = true,
  onPointerDownCapture,
  onKeyDownCapture,
}: {
  direction: 'horizontal' | 'vertical'
  label: string
  prominent?: boolean
  visible?: boolean
  onPointerDownCapture?: PointerEventHandler<HTMLElement>
  onKeyDownCapture?: KeyboardEventHandler<HTMLElement>
}) {
  const Icon = direction === 'vertical' ? GripVertical : GripHorizontal
  const className = direction === 'vertical' ? 'nova-resize-handle group -mx-1 flex w-3 cursor-col-resize items-center justify-center bg-transparent transition-colors' : `nova-resize-handle group ${prominent ? '-my-0.5 h-4' : '-my-1 h-3'} flex cursor-row-resize items-center justify-center bg-transparent transition-colors`

  return (
    <CollapsiblePanelSeparator
      visible={visible}
      aria-label={label}
      className={className}
      onPointerDownCapture={onPointerDownCapture}
      onKeyDownCapture={onKeyDownCapture}
    >
      <span className={`flex items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)] shadow-[0_4px_14px_rgba(0,0,0,0.22)] transition-colors group-hover:border-[var(--nova-active)] group-data-[resize-handle-active]:border-[var(--nova-active)] group-data-[resize-handle-active]:text-[var(--nova-text)] ${direction === 'vertical' ? 'h-9 w-2.5' : 'h-2.5 w-16'}`}>
        <Icon className={direction === 'vertical' ? 'h-3.5 w-3.5' : 'h-3 w-3'} aria-hidden="true" />
      </span>
    </CollapsiblePanelSeparator>
  )
}
