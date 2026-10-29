import { GameStories } from '@/features/platform/GameStories'
import loadable from '@loadable/component'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { toast } from '@/lib/toast'
import { useShallow } from 'zustand/react/shallow'
import { readOptionalProjectFile, type LoreItem } from '@/lib/api'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getGamePlanningTemplates, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, selectInteractiveStory, switchInteractiveBranch, updateInteractiveBranchPlan, updateInteractiveTurnPresentation, updateInteractiveStory } from '../api'
import type { TurnPresentationPatch } from '../types'
import { branchPlanSnapshotAfterUpdate } from '../branch-plan-snapshot'
import { useInteractiveStore } from '../stores/interactive-store'
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
import { StoryWorkspace } from './StoryWorkspace'
import type { ImagePreset, InteractiveStoryUpdateInput, InteractiveTurnPersistedEvent, Snapshot, StorySummary } from '../types'
import { INTERACTIVE_OPENING_PRESET_PATH, INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, LEGACY_INTERACTIVE_OPENING_PRESET_PATH, parseBookOpeningPresets, type BookOpeningPreset, type StoryCreateInput } from '../opening'
import { resolveNarrativeStyle } from '../narrative-style'
import { LoadingState } from '@/components/common/LoadingState'

interface InteractiveLayoutProps {
  projectId?: string
  workspace?: string
  active?: boolean
  imagePresets?: ImagePreset[]
  loreEmpty?: boolean
  loreItems?: LoreItem[]
  onRequestLoreInit?: () => void
  onOpenPresets?: () => void
  rightPanelVisible?: boolean
  onToggleRightPanel?: () => void
}

const SNAPSHOT_POLL_INTERVAL_MS = 1000
const BranchTimeline = loadable(() => import('./BranchTimeline'), { resolveComponent: module => module.BranchTimeline })

export function InteractiveLayout({ projectId = '', workspace, active = true, imagePresets = [], loreEmpty = false, loreItems = [], onRequestLoreInit, onOpenPresets, rightPanelVisible = true, onToggleRightPanel }: InteractiveLayoutProps) {
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
  const snapshotRequestSeqRef = useRef(0)
  const storySelectionQueueRef = useRef<Promise<void>>(Promise.resolve())
  const lastStableSnapshotRef = useRef<Snapshot | null>(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotLoadFailed, setSnapshotLoadFailed] = useState(false)
  const [storyIndexLoading, setStoryIndexLoading] = useState(true)
  const [resourcesLoading, setResourcesLoading] = useState(true)
  const [loadedWorkspace, setLoadedWorkspace] = useState(workspace)
  const [mobileSnapshotOpen, setMobileSnapshotOpen] = useState(false)
  const [storyStateDisplayPreference, setStoryStateDisplayPreference] = useState(readStoryStateDisplayPreference)
  const [bookOpeningPresets, setBookOpeningPresets] = useState<BookOpeningPreset[]>([])
  const [branchCreationSource, setBranchCreationSource] = useState<BranchCreationSource | null>(null)

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
      // Branch actions reload explicitly. Restoring a branch from the response
      // must not change this callback and trigger a second initial snapshot.
      const current = useInteractiveStore.getState()
      const storyId = storyOverride || current.currentStoryId
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
      const branchId = branchOverride ?? (current.snapshot?.story_id === storyId || current.currentBranchId !== 'main' ? current.currentBranchId : '')
      try {
        const [nextSnapshot, nextBranches] = await Promise.all([
          getInteractiveSnapshot(storyId, branchId).then(next => {
            if (requestSeq !== snapshotRequestSeqRef.current) return
            // The current story can be read while the branch picker is still loading.
            setSnapshot(next)
            if (!silent) setSnapshotLoading(false)
            return next
          }),
          includeBranches ? getInteractiveBranches(storyId) : Promise.resolve(null),
        ])
        if (requestSeq !== snapshotRequestSeqRef.current) return
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
    [setBranches, setSnapshot],
  )

  useEffect(() => {
    let cancelled = false
    storyIndexRequestSeqRef.current += 1
    snapshotRequestSeqRef.current += 1
    if (workspace !== undefined) {
      resetWorkspaceState()
      if (!workspace) {
        setLoadedWorkspace(workspace)
        setStoryIndexLoading(false)
        setResourcesLoading(false)
        return () => { cancelled = true }
      }
    }
    setStoryIndexLoading(true)
    setResourcesLoading(true)
    // Resolve the selected story immediately so its snapshot overlaps catalog reads.
    // Keep setup gated until both catalogs settle, preserving creation defaults.
    void reloadStories()
      .catch((error) => {
        if (!cancelled) console.error('[InteractiveLayout.tsx] failed to load the interactive workspace index', { workspace, error })
      })
      .finally(() => {
        if (!cancelled) {
          setLoadedWorkspace(workspace)
          setStoryIndexLoading(false)
        }
      })
    void Promise.allSettled([getInteractiveTellers(), getGamePlanningTemplates()]).then(([styles, templates]) => {
      if (cancelled) return
      if (styles.status === 'fulfilled') setTellers(styles.value)
      else console.error('[interactive-layout] Failed to load narrative styles', { workspace, error: styles.reason })
      if (templates.status === 'fulfilled') setPlanningTemplates(templates.value)
      else console.error('[interactive-layout] Failed to load planning templates', { workspace, error: templates.reason })
      setResourcesLoading(false)
    })
    return () => {
      cancelled = true
      storyIndexRequestSeqRef.current += 1
      snapshotRequestSeqRef.current += 1
    }
  }, [reloadStories, resetWorkspaceState, setPlanningTemplates, setTellers, workspace])

  useEffect(() => {
    void reloadBookOpeningPreset()
    const onPresetUpdated = () => void reloadBookOpeningPreset()
    window.addEventListener(INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, onPresetUpdated)
    return () => window.removeEventListener(INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, onPresetUpdated)
  }, [reloadBookOpeningPreset])

  useEffect(() => {
    if (!active || storyIndexLoading || loadedWorkspace !== workspace) return
    void reloadSnapshot()
  }, [active, currentStoryId, loadedWorkspace, reloadSnapshot, storyIndexLoading, workspace])

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
    if (!active || !isMobile || submode !== 'story') setMobileSnapshotOpen(false)
  }, [active, isMobile, submode])

  const handleCreateStory = async (input: StoryCreateInput) => {
    const story = await createInteractiveStory(input)
    setCurrentStoryId(story.id)
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, story), story.id)
    await reloadStories(story)
    return story
  }

  const handleStorySelect = useCallback((storyId: string) => {
    setMobileSnapshotOpen(false)
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
      presentation_settings: input.presentation_settings,
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
    // A completed save belongs to its original story, even after navigation.
    if (useInteractiveStore.getState().currentStoryId !== currentStoryId) return
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, updated), updated.id)
    await reloadStories(updated)
    if (input.module_refs || input.state_schema_policy) {
      await reloadSnapshot(undefined, currentStoryId, { silent: true })
    }
  }

  const handlePresentationChange = async (turnId: string, patch: TurnPresentationPatch) => {
    if (!currentStoryId || !currentBranchId) throw new Error(t('storyStage.presentation.updateFailed'))
    await updateInteractiveTurnPresentation(currentStoryId, turnId, { branch_id: currentBranchId, presentation: patch })
    const current = useInteractiveStore.getState()
    // A completed save must not replace the snapshot of a newly selected story.
    if (current.currentStoryId !== currentStoryId || current.currentBranchId !== currentBranchId) return
    await reloadSnapshot(currentBranchId, currentStoryId, { silent: true })
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

  // Workspace props change before the shared story store is reset by its effect.
  // Never bind the previous story to the incoming Project during that render.
  if (storyIndexLoading || resourcesLoading || loadedWorkspace !== workspace) {
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
      active={active}
      projectId={projectId}
      workspace={workspace}
      styleSceneSuggestions={styleSceneSuggestions}
      stories={stories}
      story={currentStory}
      tellers={tellers}
      planningTemplates={planningTemplates}
      imagePresets={imagePresets}
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
      onStoryDelete={handleDeleteStories}
      onStoryRename={handleRenameStory}
      onRequestLoreInit={onRequestLoreInit}
      onOpenDirectorConfig={() => {
        onOpenPresets?.()
        setMobileSnapshotOpen(false)
      }}
      onToggleDirectorPanel={isMobile ? () => setMobileSnapshotOpen((open) => !open) : onToggleRightPanel}
      onRequestCreateBranch={setBranchCreationSource}
      onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange}
      onTurnPersisted={handleTurnPersisted}
      onDone={handleStoryStageDone}
    />
  )
  return (
    <GameStories key={projectId || 'current'} projectId={projectId ?? ''} active={active} builtinPicker={{
      stories, currentStoryId, onSelect: handleStorySelect, onCreate: () => undefined,
      onDeleteStories: handleDeleteStories, onRenameStory: handleRenameStory,
    }}>
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <motion.div key={contentKey} variants={panelPresence} initial="initial" animate="animate" transition={{ duration: 0.2, ease: novaEase }} className="flex min-h-0 flex-1 flex-col">
              {submode === 'timeline' ? (
                <BranchTimeline fallback={<LoadingState label={t('common.loading')} />} projectId={projectId} snapshot={displaySnapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} fill variant="workspace" onBackToStory={() => setSubmode('story')} headerControls={<StoryPicker stories={stories} currentStoryId={currentStoryId} onSelect={handleStorySelect} onCreate={() => undefined} onDeleteStories={handleDeleteStories} onRenameStory={handleRenameStory} hideCreate />} />
              ) : (
                <StoryWorkspace
                  rightPanelVisible={rightPanelVisible}
                  onToggleRightPanel={onToggleRightPanel}
                  mobileConsoleOpen={mobileSnapshotOpen}
                  onMobileConsoleOpenChange={setMobileSnapshotOpen}
                  story={storyStage}
                  console={<DirectorPanel
                      projectId={projectId}
                      loreItems={loreItems}
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
                      onPresentationChange={handlePresentationChange}
                      onBranchPlanUpdate={handleBranchPlanUpdate}
                      stateDisplayPreference={storyStateDisplayPreference}
                      onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange}
                      onSwitchBranch={handleSwitchBranch}
                      onOpenBranchTimeline={openBranchTimeline}
                    />}
                />
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
    </GameStories>
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
