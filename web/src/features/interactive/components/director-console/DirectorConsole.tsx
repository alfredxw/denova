import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import type { ContextAnalysis } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { analyzeInteractiveDirectorContext, getInteractiveDirector, rebuildInteractiveDirector, rerollInteractiveRuleResolution, runInteractiveDirector, updateInteractiveDirector } from '../../api'
import type { DirectorPlan, DirectorPlanDocs, DirectorPlanStatus, Snapshot, StoryMemoryRecord, StoryMemoryStructure } from '../../types'
import { ConsoleTabs } from './ConsoleTabs'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { DirectorRunView } from './DirectorRunView'
import { MemoryView } from './MemoryView'
import { PlanView } from './PlanView'
import { StateView } from './StateView'
import type { ConsoleTab } from './types'
import { extractDirectorDisplayEvents, isMissingDirectorPlanError, stateEntries } from './utils'

export interface DirectorConsoleProps {
  storyId?: string
  branchId: string
  snapshot: Snapshot | null
  loading: boolean
  memoryLoading: boolean
  memoryError: string
	stateStatus?: string
	stateError?: string
	memoryStatus?: string
	memorySyncError?: string
  activeTab: ConsoleTab
  onTabChange: (tab: ConsoleTab) => void
  directorRevealed: boolean
  onRevealDirector: () => void
  structures: StoryMemoryStructure[]
  filteredRecords: StoryMemoryRecord[]
  visibleStructures: StoryMemoryStructure[]
  structureRecordCounts: Map<string, number>
  selectedStructureId: string
  onSelectStructure: (structureId: string) => void
  query: string
  onQueryChange: (value: string) => void
  generateMessages: AgentUIMessage[]
  generating: boolean
  generateActivity: string
  onGenerateMemory: () => void
  onAbortGenerate: () => void
  onOpenMemoryManager?: () => void
  onSnapshotRefresh?: () => void | Promise<unknown>
}

export function DirectorConsole({
  storyId,
  branchId,
  snapshot,
  loading,
  memoryLoading,
  memoryError,
	stateStatus,
	stateError,
	memoryStatus,
	memorySyncError,
  activeTab,
  onTabChange,
  directorRevealed,
  onRevealDirector,
  structures,
  filteredRecords,
  visibleStructures,
  structureRecordCounts,
  selectedStructureId,
  onSelectStructure,
  query,
  onQueryChange,
  generateMessages,
  generating,
  generateActivity,
  onGenerateMemory,
  onAbortGenerate,
  onOpenMemoryManager,
  onSnapshotRefresh,
}: DirectorConsoleProps) {
  const { t } = useTranslation()
  const [rebuilding, setRebuilding] = useState(false)
  const [planLoading, setPlanLoading] = useState(false)
  const [savingPlan, setSavingPlan] = useState(false)
  const [retryingDirector, setRetryingDirector] = useState(false)
  const [rerolling, setRerolling] = useState(false)
  const [directorError, setDirectorError] = useState('')
  const [ruleError, setRuleError] = useState('')
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [directorPlan, setDirectorPlan] = useState<DirectorPlan | null>(snapshot?.director_plan || null)
  const [draftDocs, setDraftDocs] = useState<DirectorPlanDocs | null>(snapshot?.director_plan?.docs || null)
  const [manualDirectorStatus, setManualDirectorStatus] = useState<DirectorPlanStatus | null>(null)
  const [processRevealed, setProcessRevealed] = useState(false)

  const ruleResolution = snapshot?.current_turn?.rule_resolution
  const terminalOutcome = snapshot?.current_turn?.terminal_outcome
  const hasRuleAudit = !!ruleResolution || !!terminalOutcome
  const directorMetadata = directorPlan?.metadata
  const directorStatus = manualDirectorStatus || snapshot?.director_plan_status || directorMetadata?.last_run
  const directorDisplayEvents = useMemo(
    () => extractDirectorDisplayEvents(snapshot, directorStatus),
    [directorStatus?.source_turn_id, snapshot?.current_turn?.display_events, snapshot?.turns],
  )
  const currentTurnId = snapshot?.current_turn?.id || ''
  const canAnalyzeDirectorContext = Boolean(storyId && currentTurnId)
  const hasDirectorRun = Boolean(directorPlan || directorStatus || directorMetadata?.last_run || planLoading || retryingDirector)
  const stateFacts = useMemo(() => stateEntries(snapshot?.state), [snapshot?.state])
  const actorCount = useMemo(() => {
    const actors = stateFacts.find(([key]) => key === 'actors')?.[1]
    return actors && typeof actors === 'object' && !Array.isArray(actors) ? Object.keys(actors).length : 0
  }, [stateFacts])

  useEffect(() => {
    setProcessRevealed(false)
  }, [branchId, storyId])

  useEffect(() => {
    setDirectorPlan(snapshot?.director_plan || null)
    setDraftDocs(snapshot?.director_plan?.docs || null)
  }, [snapshot?.director_plan, snapshot?.director_plan?.metadata?.revision])

  useEffect(() => {
    if (snapshot?.director_plan_status) setManualDirectorStatus(null)
  }, [snapshot?.director_plan_status?.revision, snapshot?.director_plan_status?.status, snapshot?.director_plan_status?.updated_at])

  useEffect(() => {
    if (!directorRevealed || !storyId) return
    let cancelled = false
    setPlanLoading(true)
    setDirectorError('')
    getInteractiveDirector(storyId, branchId)
      .then((plan) => {
        if (cancelled) return
        setDirectorPlan(plan)
        setDraftDocs(plan.docs)
      })
      .catch((err) => {
        if (cancelled) return
        if (isMissingDirectorPlanError(err)) {
          console.info('[interactive-memory-panel] director plan missing for branch', { storyId, branchId, error: err })
        } else {
          console.error('[interactive-memory-panel] load director plan failed', err)
        }
        setDirectorError(err instanceof Error ? err.message : t('snapshot.director.loadFailed'))
      })
      .finally(() => {
        if (!cancelled) setPlanLoading(false)
      })
    return () => { cancelled = true }
  }, [branchId, directorRevealed, storyId, t])

	const rebuildDirector = async (resetEvents = false) => {
    if (!storyId || rebuilding) return
    setRebuilding(true)
    setDirectorError('')
    try {
		const plan = await rebuildInteractiveDirector(storyId, branchId, { resetEvents })
      setDirectorPlan(plan)
      setDraftDocs(plan.docs)
      onRevealDirector()
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
        branch_id: branchId,
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

	const runDirectorPlan = async (forceEventEvaluation = false) => {
    if (!storyId || retryingDirector) return
    setRetryingDirector(true)
    setDirectorError('')
    try {
		const status = await runInteractiveDirector(storyId, branchId, { forceEventEvaluation })
      setManualDirectorStatus(status)
      await onSnapshotRefresh?.()
    } catch (err) {
      console.error('[interactive-memory-panel] retry director failed', err)
      setDirectorError(err instanceof Error ? err.message : t('storyStage.director.retryFailed'))
    } finally {
      setRetryingDirector(false)
    }
  }

  const analyzeDirectorContext = async () => {
    if (!storyId || !currentTurnId) {
      setContextAnalysis(null)
      setContextAnalysisError(t('memoryPanel.directorContextAnalysisUnavailable'))
      return
    }
    setContextAnalysisLoading(true)
    setContextAnalysisError(null)
    setContextAnalysis(null)
    try {
      setContextAnalysis(await analyzeInteractiveDirectorContext(storyId, {
        branch_id: branchId,
        turn_id: currentTurnId,
      }))
    } catch (err) {
      console.error('[interactive-memory-panel] analyze director context failed', err)
      setContextAnalysisError(err instanceof Error ? err.message : t('memoryPanel.directorContextAnalysisFailed'))
    } finally {
      setContextAnalysisLoading(false)
    }
  }

  const openDirectorContextAnalysis = () => {
    setContextAnalysisOpen(true)
    void analyzeDirectorContext()
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
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader branchId={branchId} turnCount={(snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0)} />
      <ConsoleTabs activeTab={activeTab} onChange={onTabChange} stateCount={actorCount} memoryCount={filteredRecords.length} />
      {activeTab === 'plan' && directorError ? <div className="mx-4 mt-3 rounded-[10px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-5 text-[var(--nova-danger)]">{directorError}</div> : null}
      <div className="min-h-0 flex-1 overflow-hidden px-4 py-4">
        <div className="director-console__scroll h-full min-h-0 overflow-y-auto pb-4 pr-1">
          {activeTab === 'run' ? (
            <DirectorRunView
              storyId={storyId}
              hasDirectorRun={hasDirectorRun}
              directorStatus={directorStatus}
              directorMetadata={directorMetadata}
              loading={loading || planLoading || retryingDirector}
              retrying={retryingDirector}
              contextAnalysisLoading={contextAnalysisLoading}
              canAnalyzeDirectorContext={canAnalyzeDirectorContext}
              directorError={directorError}
              directorDisplayEvents={directorDisplayEvents}
              processRevealed={processRevealed}
              onRevealProcess={() => setProcessRevealed(true)}
              generateMessages={generateMessages}
              generating={generating}
              generateActivity={generateActivity}
              onGenerateMemory={onGenerateMemory}
              onAbortGenerate={onAbortGenerate}
              onRun={() => void runDirectorPlan()}
              onAnalyze={openDirectorContextAnalysis}
            />
          ) : activeTab === 'state' ? (
            <StateView snapshot={snapshot} stateFacts={stateFacts} syncStatus={stateStatus} syncError={stateError} />
          ) : activeTab === 'memory' ? (
            <MemoryView
				loadError={memoryError}
				memoryStatus={memoryStatus}
				memorySyncError={memorySyncError}
              memoryLoading={memoryLoading}
              structures={structures}
              filteredRecords={filteredRecords}
              visibleStructures={visibleStructures}
              structureRecordCounts={structureRecordCounts}
              selectedStructureId={selectedStructureId}
              onSelectStructure={onSelectStructure}
              query={query}
              onQueryChange={onQueryChange}
              onOpenMemoryManager={onOpenMemoryManager}
            />
          ) : (
            <PlanView
              storyId={storyId}
              directorRevealed={directorRevealed}
              onRevealDirector={onRevealDirector}
              directorPlan={directorPlan}
              draftDocs={draftDocs}
              onDraftDocsChange={setDraftDocs}
              directorStatus={directorStatus}
              directorMetadata={directorMetadata}
              loading={planLoading || retryingDirector}
              rebuilding={rebuilding}
              saving={savingPlan}
              onSave={() => void saveDirectorPlan()}
				onRebuild={() => void rebuildDirector()}
				onEvaluateEvent={() => void runDirectorPlan(true)}
				onResetEvents={() => void rebuildDirector(true)}
              hasRuleAudit={hasRuleAudit}
              ruleResolution={ruleResolution}
              terminalOutcome={terminalOutcome}
              ruleError={ruleError}
              rerolling={rerolling}
              onReroll={() => void rerollRules()}
            />
          )}
        </div>
      </div>
      <ContextAnalysisDialog
        open={contextAnalysisOpen}
        loading={contextAnalysisLoading}
        error={contextAnalysisError}
        analysis={contextAnalysis}
        onOpenChange={setContextAnalysisOpen}
        title={t('memoryPanel.directorContextAnalysis')}
        description={t('memoryPanel.directorContextAnalysisDescription')}
      />
    </aside>
  )
}
