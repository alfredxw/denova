import { useCallback, useEffect, useRef, useState } from 'react'
import { Gauge, GripHorizontal, GripVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { useShallow } from 'zustand/react/shallow'
import { readFile } from '@/lib/api'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, getStoryDirectors, selectInteractiveStory, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import { BranchTimeline } from './BranchTimeline'
import { DirectorBackstage } from './director-backstage/DirectorBackstage'
import { DirectorPanel } from './DirectorPanel'
import { SettingPanel, type SettingPanelMode } from './SettingPanel'
import { StoryPicker } from './StoryPicker'
import { StoryStage } from './StoryStage'
import {
  readStoryStateDisplayPreference,
  writeStoryStateDisplayPreference,
  type StoryStateDisplayPreference,
} from './story-state/display-preference'
import { novaEase, panelPresence, subtlePresence } from '@/features/motion/motion-tokens'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost } from '@/components/layout/mobile-pane-host'
import type { ImagePreset, InteractiveTurnPersistedEvent, Snapshot, StoryDirector, StoryImageSettings, StorySummary } from '../types'
import { INTERACTIVE_OPENING_PRESET_PATH, INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, LEGACY_INTERACTIVE_OPENING_PRESET_PATH, parseBookOpeningPresets, type BookOpeningPreset, type StoryCreateInput } from '../opening'

interface InteractiveLayoutProps {
  workspace?: string
  active?: boolean
  imagePresets?: ImagePreset[]
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  loreEmpty?: boolean
  onRequestLoreInit?: () => void
  rightPanelVisible?: boolean
  onToggleRightPanel?: () => void
}

const SNAPSHOT_POLL_INTERVAL_MS = 1000

export function InteractiveLayout({ workspace, active = true, imagePresets = [], onImagePresetsChange, loreEmpty = false, onRequestLoreInit, rightPanelVisible = true, onToggleRightPanel }: InteractiveLayoutProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const {
    stories,
    tellers,
    storyDirectors,
    branches,
    snapshot,
    currentStoryId,
    currentBranchId,
    submode,
    setStories,
    setTellers,
    setStoryDirectors,
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
    storyDirectors: state.storyDirectors,
    branches: state.branches,
    snapshot: state.snapshot,
    currentStoryId: state.currentStoryId,
    currentBranchId: state.currentBranchId,
    submode: state.submode,
    setStories: state.setStories,
    setTellers: state.setTellers,
    setStoryDirectors: state.setStoryDirectors,
    setBranches: state.setBranches,
    setSnapshot: state.setSnapshot,
    applyTurnPersisted: state.applyTurnPersisted,
    setCurrentStoryId: state.setCurrentStoryId,
    setCurrentBranchId: state.setCurrentBranchId,
    setSubmode: state.setSubmode,
    resetWorkspaceState: state.resetWorkspaceState,
  })))
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const currentTeller = tellers.find((teller) => teller.id === currentStory?.story_teller_id)
  const styleSceneSuggestions = Array.from(new Set((currentTeller?.style_rules || []).map((rule) => rule.scene.trim()).filter((scene) => scene && !isGlobalStyleSceneName(scene))))
  const currentBranchSnapshot = snapshot?.story_id === currentStoryId && snapshot.branch_id === currentBranchId ? snapshot : null
  const storyIndexRequestSeqRef = useRef(0)
  const snapshotStoryIdRef = useRef('')
  const snapshotRequestSeqRef = useRef(0)
  const storySelectionQueueRef = useRef<Promise<void>>(Promise.resolve())
  const lastStableSnapshotRef = useRef<Snapshot | null>(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotLoadFailed, setSnapshotLoadFailed] = useState(false)
  const [mobileSnapshotOpen, setMobileSnapshotOpen] = useState(false)
  const [storyStateDisplayPreference, setStoryStateDisplayPreference] = useState(readStoryStateDisplayPreference)
  const [bookOpeningPresets, setBookOpeningPresets] = useState<BookOpeningPreset[]>([])

  if (currentBranchSnapshot) {
    lastStableSnapshotRef.current = currentBranchSnapshot
  }
  const fallbackSnapshot = lastStableSnapshotRef.current?.story_id === currentStoryId ? lastStableSnapshotRef.current : null
  const snapshotPending = !snapshotLoadFailed && Boolean(currentStoryId) && !currentBranchSnapshot && (snapshotLoading || !snapshot || snapshot.story_id !== currentStoryId || snapshot.branch_id !== currentBranchId)
  const displaySnapshot = currentBranchSnapshot ?? (snapshotPending ? fallbackSnapshot : null)

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  const reloadStories = useCallback(async (preferredStory?: StorySummary) => {
    const requestSeq = storyIndexRequestSeqRef.current + 1
    storyIndexRequestSeqRef.current = requestSeq
    const index = await getInteractiveStories()
    if (requestSeq !== storyIndexRequestSeqRef.current) return
    setStories(mergePreferredStory(index.stories || [], preferredStory), preferredStory?.id || index.current_story_id)
  }, [setStories])

  const reloadBookOpeningPreset = useCallback(async () => {
    if (!workspace) {
      setBookOpeningPresets([])
      return
    }
    try {
      const data = await readFile(INTERACTIVE_OPENING_PRESET_PATH)
      setBookOpeningPresets(parseBookOpeningPresets(data.content || ''))
    } catch {
      try {
        const legacy = await readFile(LEGACY_INTERACTIVE_OPENING_PRESET_PATH)
        setBookOpeningPresets(parseBookOpeningPresets(legacy.content || ''))
      } catch {
        setBookOpeningPresets([])
      }
    }
  }, [workspace])

  const reloadSnapshot = useCallback(
    async (branchOverride?: string, storyOverride?: string, options?: { silent?: boolean }) => {
      const silent = options?.silent === true
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
        const [nextSnapshot, nextBranches] = await Promise.all([getInteractiveSnapshot(storyId, branchId), getInteractiveBranches(storyId)])
        if (requestSeq !== snapshotRequestSeqRef.current) return
        setSnapshot(nextSnapshot)
        setBranches(nextBranches)
        return nextSnapshot
      } catch (error) {
        if (requestSeq === snapshotRequestSeqRef.current) {
          console.error('[interactive-layout] 刷新互动快照失败', error)
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
    storyIndexRequestSeqRef.current += 1
    snapshotRequestSeqRef.current += 1
    snapshotStoryIdRef.current = ''
    if (workspace !== undefined) {
      resetWorkspaceState()
      if (!workspace) return
    }
    void Promise.all([reloadStories(), getInteractiveTellers().then(setTellers), getStoryDirectors().then(setStoryDirectors)])
  }, [reloadStories, resetWorkspaceState, setStoryDirectors, setTellers, workspace])

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
    const directorStatus = snapshot?.director_plan_status?.status || ''
    const directorPending = directorStatus === 'running' || (directorStatus === 'waiting_opening' && (snapshot?.turns?.length || 0) > 0)
    if (!active || !branchID || (snapshot?.current_turn?.state_status !== 'pending' && !directorPending)) return
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
        void reloadSnapshot(branchID, undefined, { silent: true }).finally(schedule)
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
  }, [active, reloadSnapshot, snapshot?.branch_id, snapshot?.current_turn?.id, snapshot?.current_turn?.state_status, snapshot?.director_plan_status?.status, snapshot?.turns?.length])

  useEffect(() => {
    if (!isMobile || submode !== 'story') setMobileSnapshotOpen(false)
  }, [isMobile, submode])

  const handleCreateStory = async (input: StoryCreateInput) => {
    const story = await createInteractiveStory(input)
    setCurrentStoryId(story.id)
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, story), story.id)
    await reloadStories(story)
  }

  const handleStorySelect = useCallback((storyId: string) => {
    if (!storyId || storyId === useInteractiveStore.getState().currentStoryId) return
    setCurrentStoryId(storyId)
    const persisted = storySelectionQueueRef.current
      .catch(() => undefined)
      .then(() => selectInteractiveStory(storyId))
    storySelectionQueueRef.current = persisted
    void persisted.catch((error) => {
      console.error('[interactive-layout] 持久化当前故事线失败', { storyId, error })
    })
  }, [setCurrentStoryId])

  const handleDeleteStories = async (storyIds: string[]) => {
    const uniqueStoryIds = Array.from(new Set(storyIds.filter(Boolean)))
    if (uniqueStoryIds.length === 0) return

    console.info('[interactive-layout] 开始删除故事线', { count: uniqueStoryIds.length, storyIds: uniqueStoryIds })
    const results = await Promise.allSettled(uniqueStoryIds.map((storyId) => deleteInteractiveStory(storyId)))
    await reloadStories()
    const failed = results.flatMap((result, index) => result.status === 'rejected' ? [{ storyId: uniqueStoryIds[index], reason: result.reason }] : [])
    if (failed.length > 0) {
      console.error('[interactive-layout] 删除故事线失败', { requested: uniqueStoryIds.length, failed })
      const reason = failed[0].reason
      throw reason instanceof Error ? reason : new Error(String(reason))
    }
    console.info('[interactive-layout] 故事线删除完成', { count: uniqueStoryIds.length })
  }

  const handleStorySetupUpdate = async (input: StoryCreateInput) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, {
      title: input.title,
      origin: input.origin,
      story_teller_id: input.story_teller_id,
      story_director_id: input.story_director_id,
      module_refs: input.module_refs,
      reply_target_chars: input.reply_target_chars,
      choice_count: input.choice_count,
      image_settings: input.image_settings,
      state_schema_policy: input.state_schema_policy,
    })
    await reloadStories()
    await reloadSnapshot(undefined, currentStoryId, { silent: true })
  }

  const handleDirectorChange = async (directorId: string) => {
    if (!currentStoryId) return
    const director = storyDirectors.find((item) => item.id === directorId)
    await updateInteractiveStory(currentStoryId, {
      story_director_id: directorId,
      story_teller_id: storyDirectorNarrativeStyleId(director, tellers, currentStory?.story_teller_id),
    })
    await reloadStories()
    await reloadSnapshot(undefined, currentStoryId, { silent: true })
  }

  const handleReplyTargetCharsChange = async (replyTargetChars: number) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, {
      reply_target_chars: replyTargetChars,
    })
    await reloadStories()
  }

  const handleImageSettingsChange = async (imageSettings: StoryImageSettings) => {
    if (!currentStoryId) return
    await updateInteractiveStory(currentStoryId, {
      image_settings: imageSettings,
    })
    await reloadStories()
  }

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

  const handleTurnPersisted = useCallback((event: InteractiveTurnPersistedEvent) => {
    return applyTurnPersisted(event) || undefined
  }, [applyTurnPersisted])

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

  const settingMode: SettingPanelMode = submode === 'story' || submode === 'timeline' || submode === 'director' ? 'lore' : submode
  const settingsWorkspaceVisible = submode !== 'story' && submode !== 'timeline' && submode !== 'director'
  const contentKey = settingsWorkspaceVisible ? `settings:${settingMode}` : submode
  const directorPanelVisible = isMobile ? mobileSnapshotOpen : rightPanelVisible
  const storyStage = (
    <StoryStage
      workspace={workspace}
      styleSceneSuggestions={styleSceneSuggestions}
      stories={stories}
      story={currentStory}
      tellers={tellers}
      storyDirectors={storyDirectors}
      imagePresets={imagePresets}
      storyId={currentStoryId}
      branchId={currentBranchId}
      snapshot={displaySnapshot}
      snapshotLoading={snapshotPending}
      loreEmpty={loreEmpty}
      bookOpeningPresets={bookOpeningPresets}
      directorPanelVisible={directorPanelVisible}
      stateDisplayPreference={storyStateDisplayPreference}
      onStorySelect={handleStorySelect}
      onStoryCreate={handleCreateStory}
      onStorySetupUpdate={handleStorySetupUpdate}
      onStoryDelete={handleDeleteStories}
      onDirectorChange={handleDirectorChange}
      onReplyTargetCharsChange={handleReplyTargetCharsChange}
      onImageSettingsChange={handleImageSettingsChange}
      onRequestLoreInit={onRequestLoreInit}
      onOpenDirectorConfig={() => {
        setSubmode('teller')
        setMobileSnapshotOpen(false)
      }}
      onToggleDirectorPanel={isMobile ? () => setMobileSnapshotOpen((open) => !open) : onToggleRightPanel}
      onOpenDirectorState={openDirectorState}
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
              {settingsWorkspaceVisible ? (
                <SettingPanel mode={settingMode} workspace={workspace} presetUsageMode="game" tellers={tellers} storyDirectors={storyDirectors} imagePresets={imagePresets} onTellersChange={setTellers} onStoryDirectorsChange={setStoryDirectors} onImagePresetsChange={onImagePresetsChange} />
              ) : submode === 'director' ? (
                <DirectorBackstage storyId={currentStoryId} branchId={currentBranchId} snapshot={displaySnapshot} loading={snapshotPending} onSnapshotRefresh={() => reloadSnapshot(currentBranchId, currentStoryId, { silent: true })} />
              ) : submode === 'timeline' ? (
                <BranchTimeline snapshot={displaySnapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} fill variant="workspace" onBackToStory={() => setSubmode('story')} headerControls={<StoryPicker stories={stories} currentStoryId={currentStoryId} onSelect={handleStorySelect} onCreate={() => undefined} onDeleteStories={handleDeleteStories} hideCreate />} />
              ) : isMobile ? (
                <MobilePaneHost
                  panes={[{
                    id: 'director-panel',
                    title: t('directorPanel.title'),
                    side: 'right',
                    icon: <Gauge className="h-4 w-4" />,
                    content: <DirectorPanel storyId={currentStoryId} story={currentStory} storyDirectors={storyDirectors} onDirectorChange={handleDirectorChange} onReplyTargetCharsChange={handleReplyTargetCharsChange} branchId={currentBranchId} snapshot={displaySnapshot} stateDisplayPreference={storyStateDisplayPreference} onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange} />,
                  }]}
                  closeLabel={t('common.close')}
                  openPaneId={mobileSnapshotOpen ? 'director-panel' : null}
                  onOpenPaneChange={(id) => setMobileSnapshotOpen(id === 'director-panel')}
                  className="relative flex min-h-0 flex-1"
                >
                  {storyStage}
                </MobilePaneHost>
              ) : (
                <Group id="nova-interactive-horizontal" defaultLayout={readStoredLayout('nova-interactive-horizontal')} onLayoutChanged={(layout) => storeLayout('nova-interactive-horizontal', layout)} orientation="horizontal" className="min-h-0 flex-1">
                  <Panel id="story-stage" minSize="240px" className="min-w-0">
                    {storyStage}
                  </Panel>
                  {rightPanelVisible && (
                    <>
                      <InteractiveResizeHandle direction="vertical" label={t('interactiveLayout.resizeDirectorPanel')} />
                      <Panel id="snapshot" defaultSize="320px" minSize="180px" maxSize="45%" className="min-w-0">
                        <motion.div className="h-full min-h-0" variants={subtlePresence} initial="initial" animate="animate" transition={{ duration: 0.16, ease: novaEase }}>
                          <DirectorPanel storyId={currentStoryId} story={currentStory} storyDirectors={storyDirectors} onDirectorChange={handleDirectorChange} onReplyTargetCharsChange={handleReplyTargetCharsChange} branchId={currentBranchId} snapshot={displaySnapshot} stateDisplayPreference={storyStateDisplayPreference} onStateDisplayPreferenceChange={handleStoryStateDisplayPreferenceChange} />
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

function isGlobalStyleSceneName(scene: string) {
  const normalized = scene.trim().toLowerCase()
  return normalized === '全局' || normalized === 'global'
}

function storyDirectorNarrativeStyleId(director: StoryDirector | undefined, tellers: { id: string }[], fallbackTellerId = '') {
  if (director?.module_refs?.narrative_style_disabled !== true && director?.module_refs?.narrative_style_id) {
    return director.module_refs.narrative_style_id
  }
  return tellers[0]?.id || fallbackTellerId || 'classic'
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
