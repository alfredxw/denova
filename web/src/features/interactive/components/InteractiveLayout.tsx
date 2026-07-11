import { useCallback, useEffect, useRef, useState } from 'react'
import { Brain, GripHorizontal, GripVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import type { Layout } from 'react-resizable-panels'
import { readFile } from '@/lib/api'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveBranch, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, getStoryDirectors, switchInteractiveBranch, updateInteractiveStory } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import { BranchTimeline } from './BranchTimeline'
import { MemoryPanel } from './MemoryPanel'
import { SettingPanel, type SettingPanelMode } from './SettingPanel'
import { StoryPicker } from './StoryPicker'
import { StoryMemoryView } from './StoryMemoryView'
import { StoryStage } from './StoryStage'
import { novaEase, panelPresence, subtlePresence } from '@/features/motion/motion-tokens'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost } from '@/components/layout/mobile-pane-host'
import type { ImagePreset, InteractiveTurnPersistedEvent, Snapshot, StoryDirector, StoryImageSettings, StorySummary } from '../types'
import { INTERACTIVE_OPENING_PRESET_PATH, INTERACTIVE_OPENING_PRESET_UPDATED_EVENT, LEGACY_INTERACTIVE_OPENING_PRESET_PATH, parseBookOpeningPresets, type BookOpeningPreset, type StoryCreateInput } from '../opening'

interface InteractiveLayoutProps {
  workspace?: string
  imagePresets?: ImagePreset[]
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  loreEmpty?: boolean
  onRequestLoreInit?: () => void
  rightPanelVisible?: boolean
  onToggleRightPanel?: () => void
}

export function InteractiveLayout({ workspace, imagePresets = [], onImagePresetsChange, loreEmpty = false, onRequestLoreInit, rightPanelVisible = true, onToggleRightPanel }: InteractiveLayoutProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const { stories, tellers, storyDirectors, branches, snapshot, currentStoryId, currentBranchId, submode, setStories, setTellers, setStoryDirectors, setBranches, setSnapshot, applyTurnPersisted, setCurrentStoryId, setCurrentBranchId, setSubmode, resetWorkspaceState } = useInteractiveStore()
  const currentStory = stories.find((story) => story.id === currentStoryId)
  const currentTeller = tellers.find((teller) => teller.id === currentStory?.story_teller_id)
  const styleSceneSuggestions = Array.from(new Set((currentTeller?.style_rules || []).map((rule) => rule.scene.trim()).filter((scene) => scene && !isGlobalStyleSceneName(scene))))
  const currentBranchSnapshot = snapshot?.story_id === currentStoryId && snapshot.branch_id === currentBranchId ? snapshot : null
  const storyIndexRequestSeqRef = useRef(0)
  const snapshotStoryIdRef = useRef('')
  const snapshotRequestSeqRef = useRef(0)
  const lastStableSnapshotRef = useRef<Snapshot | null>(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotLoadFailed, setSnapshotLoadFailed] = useState(false)
  const [mobileSnapshotOpen, setMobileSnapshotOpen] = useState(false)
  const [bookOpeningPresets, setBookOpeningPresets] = useState<BookOpeningPreset[]>([])
  const [presetFocus, setPresetFocus] = useState<{ nonce: number; kind: 'memory-structure'; id?: string } | undefined>()

  if (currentBranchSnapshot) {
    lastStableSnapshotRef.current = currentBranchSnapshot
  }
  const fallbackSnapshot = lastStableSnapshotRef.current?.story_id === currentStoryId ? lastStableSnapshotRef.current : null
  const snapshotPending = !snapshotLoadFailed && Boolean(currentStoryId) && !currentBranchSnapshot && (snapshotLoading || !snapshot || snapshot.story_id !== currentStoryId || snapshot.branch_id !== currentBranchId)
  const displaySnapshot = currentBranchSnapshot ?? (snapshotPending ? fallbackSnapshot : null)

  useEffect(() => {
    snapshotStoryIdRef.current = snapshot?.story_id || ''
  }, [snapshot?.story_id])

  useEffect(() => {
    const openPresets = (event: Event) => {
      const detail = (event as CustomEvent<{ kind?: 'memory-structure'; id?: string }>).detail
      if (detail?.kind !== 'memory-structure') return
      setPresetFocus({ nonce: Date.now(), kind: detail.kind, id: detail.id })
      setSubmode('teller')
      setMobileSnapshotOpen(false)
    }
    window.addEventListener('nova:interactive-open-preset', openPresets)
    return () => window.removeEventListener('nova:interactive-open-preset', openPresets)
  }, [setSubmode])

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
    void reloadSnapshot()
  }, [currentStoryId])

  useEffect(() => {
    const branchID = snapshot?.branch_id
    const directorStatus = snapshot?.director_plan_status?.status || ''
    const directorPending = directorStatus === 'running' || (directorStatus === 'waiting_opening' && (snapshot?.turns?.length || 0) > 0)
    const memoryPending = snapshot?.current_turn?.memory_status === 'pending' || snapshot?.current_turn?.memory_status === 'running'
    if (!branchID || (snapshot?.current_turn?.state_status !== 'pending' && !memoryPending && !directorPending)) return
    const timer = window.setInterval(() => {
      void reloadSnapshot(branchID)
    }, 1000)
    return () => window.clearInterval(timer)
  }, [reloadSnapshot, snapshot?.branch_id, snapshot?.current_turn?.id, snapshot?.current_turn?.memory_status, snapshot?.current_turn?.state_status, snapshot?.director_plan_status?.status, snapshot?.turns?.length])

  useEffect(() => {
    if (!isMobile) setMobileSnapshotOpen(false)
  }, [isMobile])

  const handleCreateStory = async (input: StoryCreateInput) => {
    const story = await createInteractiveStory(input)
    setCurrentStoryId(story.id)
    setStories(mergePreferredStory(useInteractiveStore.getState().stories, story), story.id)
    await reloadStories(story)
  }

  const handleDeleteStory = async (storyId: string) => {
    await deleteInteractiveStory(storyId)
    await reloadStories()
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

  const settingMode: SettingPanelMode = submode === 'story' || submode === 'timeline' || submode === 'memory' ? 'lore' : submode
  const settingsWorkspaceVisible = submode !== 'story' && submode !== 'timeline' && submode !== 'memory'
  const contentKey = settingsWorkspaceVisible ? `settings:${settingMode}` : submode
  const sceneMemoryVisible = isMobile ? mobileSnapshotOpen : rightPanelVisible
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
      sceneMemoryVisible={sceneMemoryVisible}
      onStorySelect={setCurrentStoryId}
      onStoryCreate={handleCreateStory}
      onStoryDelete={handleDeleteStory}
      onDirectorChange={handleDirectorChange}
      onReplyTargetCharsChange={handleReplyTargetCharsChange}
      onImageSettingsChange={handleImageSettingsChange}
      onRequestLoreInit={onRequestLoreInit}
      onOpenDirectorConfig={() => {
        setSubmode('teller')
        setMobileSnapshotOpen(false)
      }}
      onToggleSceneMemory={isMobile ? () => setMobileSnapshotOpen((open) => !open) : onToggleRightPanel}
      onTurnPersisted={handleTurnPersisted}
      onDone={handleStoryStageDone}
    />
  )
  const openMemoryManager = () => {
    setSubmode('memory')
    setMobileSnapshotOpen(false)
  }
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div data-testid="interactive-shell" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]">
        <div className="flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <motion.div key={contentKey} variants={panelPresence} initial="initial" animate="animate" transition={{ duration: 0.2, ease: novaEase }} className="flex min-h-0 flex-1 flex-col">
              {submode === 'memory' ? (
                <StoryMemoryView storyId={currentStoryId} branchId={currentBranchId} branches={branches} onBackToStory={() => setSubmode('story')} />
              ) : settingsWorkspaceVisible ? (
                <SettingPanel mode={settingMode} workspace={workspace} presetFocus={presetFocus} presetUsageMode="game" tellers={tellers} storyDirectors={storyDirectors} imagePresets={imagePresets} onTellersChange={setTellers} onStoryDirectorsChange={setStoryDirectors} onImagePresetsChange={onImagePresetsChange} />
              ) : submode === 'timeline' ? (
                <BranchTimeline snapshot={displaySnapshot} branches={branches} currentBranchId={currentBranchId} onSwitchBranch={handleSwitchBranch} onCreateBranch={handleCreateBranch} onDeleteBranch={handleDeleteBranch} fill variant="workspace" onBackToStory={() => setSubmode('story')} headerControls={<StoryPicker stories={stories} currentStoryId={currentStoryId} tellers={tellers} storyDirectors={storyDirectors} onSelect={setCurrentStoryId} onCreate={handleCreateStory} onDelete={handleDeleteStory} />} />
              ) : isMobile ? (
                <MobilePaneHost
                  panes={[{
                    id: 'scene-memory',
                    title: t('memoryPanel.title'),
                    side: 'right',
                    icon: <Brain className="h-4 w-4" />,
                    content: <MemoryPanel storyId={currentStoryId} branchId={currentBranchId} snapshot={displaySnapshot} loading={snapshotPending} refreshKey={`${displaySnapshot?.current_turn?.id || ''}:${displaySnapshot?.current_turn?.memory_status || ''}:${displaySnapshot?.current_turn?.state_status || ''}`} onOpenMemoryManager={openMemoryManager} onSnapshotRefresh={() => reloadSnapshot(currentBranchId, currentStoryId, { silent: true })} />,
                  }]}
                  closeLabel={t('common.close')}
                  openPaneId={mobileSnapshotOpen ? 'scene-memory' : null}
                  onOpenPaneChange={(id) => setMobileSnapshotOpen(id === 'scene-memory')}
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
                      <InteractiveResizeHandle direction="vertical" label={t('interactiveLayout.resizeSceneMemory')} />
                      <Panel id="snapshot" defaultSize="320px" minSize="180px" maxSize="45%" className="min-w-0">
                        <motion.div className="h-full min-h-0" variants={subtlePresence} initial="initial" animate="animate" transition={{ duration: 0.16, ease: novaEase }}>
                          <MemoryPanel storyId={currentStoryId} branchId={currentBranchId} snapshot={displaySnapshot} loading={snapshotPending} refreshKey={`${displaySnapshot?.current_turn?.id || ''}:${displaySnapshot?.current_turn?.memory_status || ''}:${displaySnapshot?.current_turn?.state_status || ''}`} onOpenMemoryManager={openMemoryManager} onSnapshotRefresh={() => reloadSnapshot(currentBranchId, currentStoryId, { silent: true })} />
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
