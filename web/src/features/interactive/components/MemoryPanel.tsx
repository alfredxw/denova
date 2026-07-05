import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Activity, Archive, Brain, Edit3, Eye, Loader2, RefreshCw, RotateCcw, Save, Search, ShieldAlert, Sparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MessageList } from '@/components/Chat/MessageList'
import { useAgentEventStream } from '@/hooks/useAgentEventStream'
import { generateStoryMemoryStream, getInteractiveDirector, getStoryMemory, rebuildInteractiveDirector, rerollInteractiveRuleResolution, runInteractiveDirector, updateInteractiveDirector } from '../api'
import type { DirectorPlan, DirectorPlanDocs, DirectorPlanRunStatus, Snapshot, StoryMemoryRecord, StoryMemoryState, StoryMemoryStructure } from '../types'

type MemoryPanelTab = 'memory' | 'director'
type MemoryPanelView = 'content' | 'generation'

interface MemoryPanelProps {
  storyId?: string
  branchId?: string
  snapshot: Snapshot | null
  loading?: boolean
  refreshKey?: string | number
  onOpenMemoryManager?: () => void
  onSnapshotRefresh?: () => void | Promise<unknown>
}

const allStructuresId = '__all__'

export function MemoryPanel({ storyId, branchId, snapshot, loading = false, refreshKey, onOpenMemoryManager, onSnapshotRefresh }: MemoryPanelProps) {
  const { t } = useTranslation()
  const [memory, setMemory] = useState<StoryMemoryState | null>(null)
  const [memoryLoading, setMemoryLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [showArchived, setShowArchived] = useState(false)
  const [selectedStructureId, setSelectedStructureId] = useState(allStructuresId)
  const [panelTab, setPanelTab] = useState<MemoryPanelTab>('memory')
  const [view, setView] = useState<MemoryPanelView>('content')
  const [directorRevealed, setDirectorRevealed] = useState(false)
  const autoGenerateTurnKeyRef = useRef('')

  const effectiveBranchId = branchId || snapshot?.branch_id || ''
  const turnSyncStatus = snapshot?.current_turn?.memory_status || snapshot?.current_turn?.state_status || ''
  const syncStatus = turnSyncStatus === 'pending' || turnSyncStatus === 'failed' ? turnSyncStatus : memory?.sync_status || turnSyncStatus
  const syncError = snapshot?.current_turn?.memory_error || snapshot?.current_turn?.state_error || memory?.sync_error || ''

  useEffect(() => {
    setPanelTab('memory')
    setDirectorRevealed(false)
  }, [effectiveBranchId, storyId])

  const loadMemory = useCallback(async () => {
    if (!storyId) {
      setMemory(null)
      return
    }
    setMemoryLoading(true)
    setError('')
    try {
      const next = await getStoryMemory(storyId, effectiveBranchId, showArchived)
      setMemory(next)
      setSelectedStructureId((current) => {
        if (current === allStructuresId || next.structures.some((structure) => structure.id === current)) return current
        return allStructuresId
      })
    } catch (err) {
      console.error('[interactive-memory-panel] load failed', err)
      setError(err instanceof Error ? err.message : t('memoryPanel.loadFailed'))
    } finally {
      setMemoryLoading(false)
    }
  }, [effectiveBranchId, showArchived, storyId, t])

  const { messages: generateMessages, setMessages: setGenerateMessages, isStreaming: generating, activityContent: generateActivity, consumeAgentStream, resetStreamingState, setAbortController, abortLocalStream } = useAgentEventStream({
    onEvent: (event, data) => {
      if (event.event !== 'story_memory_result') return
      setGenerateMessages(prev => [...prev, {
        role: 'system',
        content: t('memoryPanel.generateDone', {
          patches: readNumber(data.patches),
          records: readNumber(data.records),
        }),
      }])
      void loadMemory()
    },
  })

  useEffect(() => {
    void loadMemory()
  }, [loadMemory, refreshKey])

  const structures = useMemo(() => (memory?.structures || []).filter((structure) => storyMemoryEnabled(structure.enabled)), [memory?.structures])
  const filteredRecords = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const enabledStructureIds = new Set(structures.map((structure) => structure.id))
    const source = (memory?.records || []).filter((record) => enabledStructureIds.has(record.structure_id))
    if (!needle) return source
    return source.filter((record) => {
      const structure = structures.find((item) => item.id === record.structure_id)
      return storyMemorySearchText(record, structure).toLowerCase().includes(needle)
    })
  }, [memory?.records, query, structures])
  const structureRecordCounts = useMemo(() => {
    const counts = new Map<string, number>()
    filteredRecords.forEach((record) => counts.set(record.structure_id, (counts.get(record.structure_id) || 0) + 1))
    return counts
  }, [filteredRecords])
  const visibleStructures = useMemo(() => {
    if (selectedStructureId === allStructuresId) return structures
    return structures.filter((structure) => structure.id === selectedStructureId)
  }, [selectedStructureId, structures])

  const runStoryMemoryGenerate = useCallback(async (source: 'manual' | 'auto' = 'manual') => {
    if (!storyId || generating) return
    if (source === 'manual') setView('generation')
    resetStreamingState()
    setGenerateMessages([{ role: 'user', content: source === 'auto' ? t('memoryPanel.autoGenerateRequest') : t('memoryPanel.generateRequest') }])
    const controller = new AbortController()
    setAbortController(controller)
    try {
      const stream = await generateStoryMemoryStream(storyId, effectiveBranchId, source, controller.signal)
      await consumeAgentStream(stream)
      await loadMemory()
    } catch (err) {
      console.error('[interactive-memory-panel] generate stream failed', err)
      setGenerateMessages(prev => [...prev, { role: 'error', content: err instanceof Error ? err.message : t('memoryPanel.generateFailed') }])
      resetStreamingState()
    }
  }, [consumeAgentStream, effectiveBranchId, generating, loadMemory, resetStreamingState, setAbortController, setGenerateMessages, storyId, t])

  useEffect(() => {
    const turn = snapshot?.current_turn
    if (!storyId || !effectiveBranchId || !turn?.id || turn.memory_status !== 'pending' || generating) return
    const turnKey = `${storyId}:${effectiveBranchId}:${turn.id}`
    if (autoGenerateTurnKeyRef.current === turnKey) return
    autoGenerateTurnKeyRef.current = turnKey
    void runStoryMemoryGenerate('auto')
  }, [effectiveBranchId, generating, runStoryMemoryGenerate, snapshot?.current_turn?.id, snapshot?.current_turn?.memory_status, storyId])

  return (
    <aside className="flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="shrink-0 border-b border-[var(--nova-border)] px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <div data-testid="memory-panel-icon" className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)]" aria-label={panelTab === 'director' ? t('snapshot.director.title') : t('memoryPanel.title')} title={panelTab === 'director' ? t('snapshot.director.title') : t('memoryPanel.title')}>
              {panelTab === 'director' ? <Sparkles className="h-4 w-4" /> : <Brain className="h-4 w-4" />}
            </div>
            <div className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('memoryPanel.sidebarTabs')}>
              <PanelTabButton active={panelTab === 'memory'} onClick={() => setPanelTab('memory')}>
                <Brain className="h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0 truncate">{t('memoryPanel.tab.memory')}</span>
              </PanelTabButton>
              <PanelTabButton active={panelTab === 'director'} onClick={() => setPanelTab('director')}>
                <Sparkles className="h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0 truncate">{t('memoryPanel.tab.director')}</span>
              </PanelTabButton>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {panelTab === 'memory' ? <SyncBadge status={syncStatus} error={syncError} loading={loading || memoryLoading} /> : null}
          </div>
        </div>
        {panelTab === 'memory' ? view === 'content' ? (
          <div className="mt-3 flex items-center gap-2">
            <MemoryViewSwitch view={view} onChange={setView} />
            <label className="flex min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text-muted)]">
              <Search className="h-3.5 w-3.5 shrink-0" />
              <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('memoryPanel.search')} className="min-w-0 flex-1 bg-transparent text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]" />
            </label>
            <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={showArchived ? t('memoryPanel.hideArchived') : t('memoryPanel.showArchived')} onClick={() => setShowArchived((value) => !value)}>
              {showArchived ? <RotateCcw className="h-4 w-4" /> : <Archive className="h-4 w-4" />}
            </button>
            <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('memoryPanel.generate')} onClick={() => void runStoryMemoryGenerate()} disabled={generating || !storyId}>
              {generating ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            </button>
            <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('memoryPanel.openManager')} onClick={onOpenMemoryManager} disabled={!onOpenMemoryManager}>
              <Edit3 className="h-4 w-4" />
            </button>
          </div>
        ) : (
          <div className="mt-3 flex items-center gap-2">
            <MemoryViewSwitch view={view} onChange={setView} />
            <div className="flex min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text-muted)]">
              <Brain className="h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0 truncate">{t('memoryPanel.generateLog')}</span>
            </div>
            {generating ? (
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('memoryPanel.abortGenerate')} onClick={abortLocalStream}>
                <X className="h-4 w-4" />
              </button>
            ) : (
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('memoryPanel.generate')} onClick={() => void runStoryMemoryGenerate()} disabled={!storyId}>
                <RefreshCw className="h-4 w-4" />
              </button>
            )}
          </div>
        ) : null}
        {error && <p className="mt-2 text-xs text-[var(--nova-danger)]">{error}</p>}
      </div>

      {panelTab === 'director' ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          {directorRevealed ? (
            <NarrativeOrchestrationSummary
              storyId={storyId}
              branchId={effectiveBranchId}
              snapshot={snapshot}
              onSnapshotRefresh={onSnapshotRefresh}
              emptyFallback={<div className="flex min-h-[180px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.directorEmpty')}</div>}
            />
          ) : (
            <DirectorSpoilerGate onReveal={() => setDirectorRevealed(true)} />
          )}
        </div>
      ) : view === 'generation' ? (
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-4 py-3">
          {generateMessages.length > 0 || generating ? (
            <MessageList
              messages={generateMessages}
              isStreaming={generating}
              activityContent={generateActivity}
              scrollResetKey={`${storyId || ''}:${effectiveBranchId}`}
              bottomPaddingClassName="pb-3"
              messageStyle={{ fontSize: '12px', lineHeight: 1.55 }}
              collapseTraceBeforeAssistant
            />
          ) : (
            <div className="flex h-full min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.generateEmpty')}</div>
          )}
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          <div className="-mx-1 mb-3 overflow-x-auto px-1" aria-label={t('memoryPanel.structureTabs')} data-testid="memory-panel-structure-tabs">
            <div className="flex w-max min-w-full gap-1">
              <StructureTab
                active={selectedStructureId === allStructuresId}
                label={t('memoryPanel.allStructures')}
                count={filteredRecords.length}
                onClick={() => setSelectedStructureId(allStructuresId)}
              />
              {structures.map((structure) => (
                <StructureTab
                  key={structure.id}
                  active={selectedStructureId === structure.id}
                  label={structure.name || structure.id}
                  count={structureRecordCounts.get(structure.id) || 0}
                  onClick={() => setSelectedStructureId(structure.id)}
                />
              ))}
            </div>
          </div>
          {memoryLoading ? (
            <div className="flex min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.loading')}</div>
          ) : filteredRecords.length === 0 ? (
            <div className="flex min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{query.trim() ? t('memoryPanel.noMatches') : t('memoryPanel.empty')}</div>
          ) : (
            <div className="space-y-4">
              {visibleStructures.map((structure) => {
                const records = filteredRecords.filter((record) => record.structure_id === structure.id)
                if (records.length === 0) {
                  if (selectedStructureId === allStructuresId) return null
                  return (
                    <section key={structure.id} className="space-y-2">
                      <MemoryStructureHeader structure={structure} count={0} />
                      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-3 py-6 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.tableEmpty')}</div>
                    </section>
                  )
                }
                return (
                  <section key={structure.id} className="space-y-2">
                    <MemoryStructureHeader structure={structure} count={records.length} />
                    <div className="space-y-2">
                      {records.map((record) => (
                        <MemoryRecordCard key={record.id} record={record} structure={structure} />
                      ))}
                    </div>
                  </section>
                )
              })}
            </div>
          )}
        </div>
      )}
    </aside>
  )
}

function MemoryViewSwitch({ view, onChange }: { view: MemoryPanelView; onChange: (view: MemoryPanelView) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('memoryPanel.panelSwitch')}>
      <button
        type="button"
        onClick={() => onChange('content')}
        className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'content' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
      >
        {t('memoryPanel.view.content')}
      </button>
      <button
        type="button"
        onClick={() => onChange('generation')}
        className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'generation' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
      >
        {t('memoryPanel.view.generation')}
      </button>
    </div>
  )
}

function PanelTabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      className={`flex h-6 max-w-[128px] min-w-0 items-center gap-1.5 rounded-[6px] px-2 text-[11px] transition-colors ${active ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
      aria-pressed={active}
      onClick={onClick}
    >
      {children}
    </button>
  )
}

function DirectorSpoilerGate({ onReveal }: { onReveal: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[260px] items-center justify-center">
      <section className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-4 text-center">
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]">
          <ShieldAlert className="h-5 w-5" />
        </div>
        <h3 className="mt-3 text-sm font-semibold text-[var(--nova-text)]">{t('memoryPanel.directorSpoilerTitle')}</h3>
        <p className="mt-2 text-xs leading-5 text-[var(--nova-text-muted)]">{t('memoryPanel.directorSpoilerDescription')}</p>
        <button
          type="button"
          className="mt-4 inline-flex h-8 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 text-xs font-medium text-[var(--nova-text)] transition-colors hover:border-[var(--nova-accent)]"
          onClick={onReveal}
        >
          <Eye className="h-3.5 w-3.5" />
          {t('memoryPanel.directorReveal')}
        </button>
      </section>
    </div>
  )
}

function NarrativeOrchestrationSummary({ storyId, branchId, snapshot, onSnapshotRefresh, emptyFallback = null }: { storyId?: string; branchId?: string; snapshot: Snapshot | null; onSnapshotRefresh?: () => void | Promise<unknown>; emptyFallback?: ReactNode }) {
  const { t } = useTranslation()
  const [rebuilding, setRebuilding] = useState(false)
  const [planLoading, setPlanLoading] = useState(false)
  const [savingPlan, setSavingPlan] = useState(false)
  const [retryingDirector, setRetryingDirector] = useState(false)
  const [rerolling, setRerolling] = useState(false)
  const [directorError, setDirectorError] = useState('')
  const [ruleError, setRuleError] = useState('')
  const [directorPlan, setDirectorPlan] = useState<DirectorPlan | null>(snapshot?.director_plan || null)
  const [draftDocs, setDraftDocs] = useState<DirectorPlanDocs | null>(snapshot?.director_plan?.docs || null)
  const ruleResolution = snapshot?.current_turn?.rule_resolution
  const ruleRequest = ruleResolution?.request
  const ruleResult = ruleResolution?.result
  const terminalCandidate = ruleResolution?.terminal_candidate
  const terminalOutcome = snapshot?.current_turn?.terminal_outcome
  const hasRuleAudit = !!ruleResolution || !!terminalOutcome
  const effectiveBranchId = branchId || snapshot?.branch_id || ''
  const directorMetadata = directorPlan?.metadata
  const directorStatus = snapshot?.director_plan_status || directorMetadata?.last_run

  useEffect(() => {
    setDirectorPlan(snapshot?.director_plan || null)
    setDraftDocs(snapshot?.director_plan?.docs || null)
  }, [snapshot?.director_plan, snapshot?.director_plan?.metadata?.revision])

  useEffect(() => {
    if (!storyId) return
    let cancelled = false
    setPlanLoading(true)
    setDirectorError('')
    getInteractiveDirector(storyId, effectiveBranchId)
      .then((plan) => {
        if (cancelled) return
        setDirectorPlan(plan)
        setDraftDocs(plan.docs)
      })
      .catch((err) => {
        if (cancelled) return
        console.error('[interactive-memory-panel] load director plan failed', err)
        setDirectorError(err instanceof Error ? err.message : t('snapshot.director.loadFailed'))
      })
      .finally(() => {
        if (!cancelled) setPlanLoading(false)
      })
    return () => { cancelled = true }
  }, [effectiveBranchId, storyId, t])

  if (!directorPlan && !hasRuleAudit && !planLoading) return emptyFallback

  const rebuildDirector = async () => {
    if (!storyId || rebuilding) return
    setRebuilding(true)
    setDirectorError('')
    try {
      const plan = await rebuildInteractiveDirector(storyId, effectiveBranchId)
      setDirectorPlan(plan)
      setDraftDocs(plan.docs)
      await onSnapshotRefresh?.()
    } catch (err) {
      console.error('[interactive-memory-panel] rebuild director failed', err)
      setDirectorError(err instanceof Error ? err.message : t('snapshot.director.rebuildFailed'))
    } finally {
      setRebuilding(false)
    }
  }

  const saveDirectorPlan = async () => {
    if (!storyId || !draftDocs || !directorPlan || !directorMetadata?.revision || savingPlan) return
    setSavingPlan(true)
    setDirectorError('')
    try {
      const plan = await updateInteractiveDirector(storyId, {
        branch_id: effectiveBranchId,
        docs: draftDocs,
        base_revision: directorMetadata.revision,
        summary: t('snapshot.director.savedSummary'),
      })
      setDirectorPlan(plan)
      setDraftDocs(plan.docs)
      await onSnapshotRefresh?.()
    } catch (err) {
      console.error('[interactive-memory-panel] save director plan failed', err)
      setDirectorError(err instanceof Error ? err.message : t('snapshot.director.saveFailed'))
    } finally {
      setSavingPlan(false)
    }
  }

  const retryDirectorPlan = async () => {
    if (!storyId || retryingDirector) return
    setRetryingDirector(true)
    setDirectorError('')
    try {
      await runInteractiveDirector(storyId, effectiveBranchId)
      await onSnapshotRefresh?.()
    } catch (err) {
      console.error('[interactive-memory-panel] retry director failed', err)
      setDirectorError(err instanceof Error ? err.message : t('storyStage.director.retryFailed'))
    } finally {
      setRetryingDirector(false)
    }
  }

  const rerollRules = async () => {
    const resolutionId = ruleResolution?.id
    const turnId = snapshot?.current_turn?.id
    if (!storyId || !resolutionId || rerolling) return
    setRerolling(true)
    setRuleError('')
    try {
      await rerollInteractiveRuleResolution(storyId, resolutionId, { branch_id: branchId, turn_id: turnId })
      await onSnapshotRefresh?.()
    } catch (err) {
      console.error('[interactive-memory-panel] reroll rules failed', err)
      setRuleError(err instanceof Error ? err.message : t('snapshot.ruleAudit.rerollFailed'))
    } finally {
      setRerolling(false)
    }
  }

  return (
    <div className="mb-3 space-y-2">
      {directorPlan || planLoading ? (
        <section className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-2 text-xs font-semibold text-[var(--nova-text)]">
            <div className="flex min-w-0 items-center gap-2">
              <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{t('snapshot.director.title')}</span>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              {directorStatus?.status === 'failed' ? (
                <button type="button" className="nova-icon-button flex h-6 w-6 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('storyStage.director.retry')} title={t('storyStage.director.retry')} onClick={() => void retryDirectorPlan()} disabled={!storyId || retryingDirector}>
                  {retryingDirector ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
                </button>
              ) : null}
              <button type="button" className="nova-icon-button flex h-6 w-6 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={savingPlan ? t('common.saving') : t('common.save')} title={savingPlan ? t('common.saving') : t('common.save')} onClick={() => void saveDirectorPlan()} disabled={!storyId || !draftDocs || !directorPlan || savingPlan}>
                {savingPlan ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
              </button>
              <button type="button" className="nova-icon-button flex h-6 w-6 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} title={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} onClick={() => void rebuildDirector()} disabled={!storyId || rebuilding}>
                {rebuilding ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
              </button>
            </div>
          </div>
          {directorError ? <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{directorError}</div> : null}
          {planLoading && !directorPlan ? <div className="text-xs text-[var(--nova-text-muted)]">{t('common.loading')}</div> : null}
          {directorPlan ? (
            <>
              <div className="flex flex-wrap gap-1.5">
                <MemoryChip>{`${t('snapshot.director.status')}: ${directorMetadata?.last_run?.status || t('snapshot.noRecord')}`}</MemoryChip>
                <MemoryChip>{`${t('snapshot.director.docs')}: ${Object.keys(directorMetadata?.docs || {}).length || 1}`}</MemoryChip>
                <MemoryChip>{`${t('snapshot.director.branchPlanningTurns')}: ${directorMetadata?.branch_planning_turns || 5}`}</MemoryChip>
              </div>
              {directorMetadata?.last_run ? <DirectorRunSummary run={directorMetadata.last_run} /> : null}
              {draftDocs ? (
                <div className="mt-3 space-y-2">
                  <DirectorPlanTextarea label={t('snapshot.director.plan')} value={draftDocs.plan} onChange={(value) => setDraftDocs({ ...draftDocs, plan: value })} />
                </div>
              ) : null}
            </>
          ) : null}
        </section>
      ) : null}

      {hasRuleAudit ? (
        <section className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-2 text-xs font-semibold text-[var(--nova-text)]">
            <div className="flex min-w-0 items-center gap-2">
              <Activity className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{t('snapshot.ruleAudit.title')}</span>
            </div>
            {ruleResolution?.id ? (
              <button type="button" className="nova-icon-button flex h-6 w-6 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={rerolling ? t('snapshot.ruleAudit.rerolling') : t('snapshot.ruleAudit.reroll')} title={rerolling ? t('snapshot.ruleAudit.rerolling') : t('snapshot.ruleAudit.reroll')} onClick={() => void rerollRules()} disabled={!storyId || rerolling}>
                <RefreshCw className={`h-3.5 w-3.5 ${rerolling ? 'animate-spin' : ''}`} />
              </button>
            ) : null}
          </div>
          {ruleError ? <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{ruleError}</div> : null}
          <div className="flex flex-wrap gap-1.5">
            <MemoryChip>{ruleRequest?.intent || t('snapshot.noRecord')}</MemoryChip>
            <MemoryChip>{`${t('snapshot.ruleAudit.difficulty')}: ${ruleRequest?.difficulty || t('snapshot.noRecord')}`}</MemoryChip>
            <MemoryChip>{`${t('snapshot.ruleAudit.outcome')}: ${ruleResult?.outcome || t('snapshot.noRecord')}`}</MemoryChip>
          </div>
          {ruleRequest?.challenge || ruleRequest?.cost || ruleRequest?.state ? (
            <div className="mt-2 space-y-1 text-xs leading-5 text-[var(--nova-text-muted)]">
              {ruleRequest.challenge ? <InfoLine label={t('snapshot.field.challenge')} value={ruleRequest.challenge} /> : null}
              {ruleRequest.cost ? <InfoLine label={t('snapshot.field.cost')} value={ruleRequest.cost} /> : null}
              {ruleRequest.state ? <InfoLine label={t('snapshot.field.state')} value={ruleRequest.state} /> : null}
            </div>
          ) : null}
          {ruleResult ? (
            <div className="mt-2 space-y-1.5">
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs">
                <div className="flex items-center justify-between gap-2">
                  <span className="min-w-0 truncate text-[var(--nova-text)]">{ruleResult.label || ruleRequest?.challenge || t('snapshot.ruleAudit.result')}</span>
                  <span className={ruleOutcomeClass(ruleResult.outcome)}>{ruleResult.outcome}</span>
                </div>
                <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">
                  {[ruleResult.dice, ruleResult.roll_mode, ruleResult.rolls?.length ? `${t('snapshot.field.rolls')}: ${ruleResult.rolls.join(', ')}` : '', Number.isFinite(ruleResult.kept_roll) ? `${t('snapshot.field.kept_roll')}: ${ruleResult.kept_roll}` : '', Number.isFinite(ruleResult.bonus_total) ? `${t('snapshot.field.bonus_total')}: ${ruleResult.bonus_total}` : '', Number.isFinite(ruleResult.total) ? `${t('snapshot.field.total')}: ${ruleResult.total}` : ''].filter(Boolean).join(' · ')}
                </div>
                {ruleResult.result ? <div className="mt-1 text-[var(--nova-text-muted)]">{ruleResult.result}</div> : null}
              </div>
            </div>
          ) : null}
          {terminalCandidate || terminalOutcome ? (
            <div className="mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">
              {terminalOutcome?.reason || terminalCandidate?.reason || terminalOutcome?.type || terminalCandidate?.type}
            </div>
          ) : null}
        </section>
      ) : null}
    </div>
  )
}

function InfoLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
      <span className="truncate text-[var(--nova-text-faint)]" title={label}>{label}</span>
      <span className="min-w-0 break-words text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{value}</span>
    </div>
  )
}

function DirectorRunSummary({ run }: { run: DirectorPlanRunStatus }) {
  const { t } = useTranslation()
  const failed = run.status === 'failed'
  return (
    <div className={`mt-2 rounded-[var(--nova-radius)] border px-2 py-1.5 text-xs ${failed ? 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]'}`}>
      <div className="mb-1 flex min-w-0 items-center justify-between gap-2">
        <span className="truncate font-medium text-[var(--nova-text)]">{t('snapshot.director.lastRun')}</span>
        {run.status ? <span className={`shrink-0 ${failed ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}>{run.status}</span> : null}
      </div>
      {run.summary ? <div className="break-words leading-5 [overflow-wrap:anywhere]">{run.summary}</div> : null}
      {run.error ? <div className="mt-1 break-words text-[11px] leading-5 [overflow-wrap:anywhere]">{run.error}</div> : null}
    </div>
  )
}

function DirectorPlanTextarea({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-medium text-[var(--nova-text-faint)]">{label}</span>
      <textarea
        className="min-h-[132px] w-full resize-y rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-2 font-mono text-[11px] leading-5 text-[var(--nova-text)] outline-none transition-colors focus:border-[var(--nova-accent)]"
        value={value}
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}

function ruleOutcomeClass(outcome: string) {
  if (outcome.includes('success')) return 'shrink-0 text-[var(--nova-success)]'
  if (outcome.includes('failure') || outcome === 'error') return 'shrink-0 text-[var(--nova-danger)]'
  return 'shrink-0 text-[var(--nova-text-muted)]'
}

function StructureTab({ active, label, count, onClick }: { active: boolean; label: string; count: number; onClick: () => void }) {
  return (
    <button
      type="button"
      className={`inline-flex h-7 max-w-[168px] shrink-0 items-center gap-1 rounded-[var(--nova-radius)] border px-2 text-[11px] transition-colors ${active ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
      aria-label={`${label} ${count}`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span className="min-w-0 truncate">{label}</span>
      <span className="shrink-0 text-[10px] opacity-70">{count}</span>
    </button>
  )
}

function MemoryStructureHeader({ structure, count }: { structure: StoryMemoryStructure; count: number }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-w-0 items-center justify-between gap-2">
      <div className="min-w-0">
        <h3 className="truncate text-xs font-semibold text-[var(--nova-text)]">{structure.name || structure.id}</h3>
        {structure.description && <p className="mt-0.5 line-clamp-1 break-words text-[11px] text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{structure.description}</p>}
      </div>
      <span className="shrink-0 rounded-full border border-[var(--nova-border)] px-2 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('memoryPanel.recordCount', { count })}</span>
    </div>
  )
}

function MemoryRecordCard({ record, structure }: { record: StoryMemoryRecord; structure: StoryMemoryStructure }) {
  const { t } = useTranslation()
  const enabledFields = structure.fields.filter((field) => storyMemoryEnabled(field.enabled))
  const fields = enabledFields.length ? enabledFields : [{ id: 'value', name: t('storyMemory.value'), order: 10 }]
  const displayFields = fields.filter((field) => recordFieldValue(record, field.id).trim()).slice(0, 4)
  const visibleFields = displayFields.length > 0 ? displayFields : fields.slice(0, 1)
  return (
    <article className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 ${record.archived ? 'opacity-55' : ''}`}>
      <div className="min-w-0">
        <h4 className="break-words text-sm font-medium text-[var(--nova-text)] [overflow-wrap:anywhere]">{storyMemoryRecordTitle(record, structure, t('storyMemory.untitled'))}</h4>
        <div className="mt-1 flex flex-wrap gap-1.5">
          {record.manual && <MemoryChip>{t('storyMemory.manual')}</MemoryChip>}
          {record.inherited_from && <MemoryChip>{t('storyMemory.inherited')}</MemoryChip>}
          {record.archived && <MemoryChip>{t('memoryPanel.archived')}</MemoryChip>}
          {record.updated_at && <MemoryChip>{`${t('storyMemory.updated')} ${formatShortDate(record.updated_at)}`}</MemoryChip>}
        </div>
      </div>
      <div className="mt-2 space-y-2">
        {visibleFields.map((field) => (
          <section key={field.id} className="min-w-0">
            <div className="mb-0.5 truncate text-[11px] font-medium text-[var(--nova-text-muted)]">{field.name || field.id}</div>
            <p className="line-clamp-4 whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{recordFieldValue(record, field.id) || t('storyMemory.noValue')}</p>
          </section>
        ))}
      </div>
    </article>
  )
}

function SyncBadge({ status, error, loading }: { status?: string; error?: string; loading?: boolean }) {
  const { t } = useTranslation()
  if (loading || status === 'pending') {
    return (
      <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-[var(--nova-border)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
        <Loader2 className="h-3 w-3 animate-spin" />
        {t('memoryPanel.syncing')}
      </span>
    )
  }
  if (status === 'failed') {
    return <span className="inline-flex max-w-[120px] shrink-0 truncate rounded-full border border-[var(--nova-danger)] px-2 py-1 text-[11px] text-[var(--nova-danger)]" title={error}>{t('memoryPanel.failed')}</span>
  }
  return <span className="inline-flex shrink-0 rounded-full border border-[var(--nova-border)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">{t('memoryPanel.ready')}</span>
}

function MemoryChip({ children }: { children: string }) {
  return <span className="max-w-full truncate rounded-full border border-[var(--nova-border)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">{children}</span>
}

function readNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function storyMemoryEnabled(value?: boolean) {
  return value !== false
}

function storyMemorySearchText(record: StoryMemoryRecord, structure?: StoryMemoryStructure) {
  return [
    structure?.name,
    structure?.description,
    record.key,
    ...Object.values(record.values || {}),
  ].filter(Boolean).join('\n')
}

function storyMemoryRecordTitle(record: StoryMemoryRecord, structure: StoryMemoryStructure, fallback: string) {
  if (record.key?.trim()) return record.key.trim()
  const keyField = structure.key_field_id ? record.values?.[structure.key_field_id]?.trim() : ''
  if (keyField) return keyField
  const firstValue = structure.fields.map((field) => record.values?.[field.id]?.trim()).find(Boolean)
  return firstValue || structure.name || fallback
}

function recordFieldValue(record: StoryMemoryRecord, fieldId: string) {
  return record.values?.[fieldId] || ''
}

function formatShortDate(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString()
}
