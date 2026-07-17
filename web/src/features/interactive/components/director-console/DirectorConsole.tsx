import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import type { ContextAnalysis } from '@/lib/api'
import { analyzeInteractiveDirectorContext, getInteractiveDirector, rebuildInteractiveDirector, rerollInteractiveRuleResolution, runInteractiveDirector, updateInteractiveDirector } from '../../api'
import type { DirectorPlan, DirectorPlanDocs, DirectorPlanStatus, Snapshot, StoryDirector, StorySummary } from '../../types'
import { ConsoleTabs } from './ConsoleTabs'
import { DirectorConsoleHeader } from './DirectorConsoleHeader'
import { DirectorView } from './DirectorView'
import { StateView } from './StateView'
import type { ConsoleTab } from './types'
import { extractDirectorDisplayEvents, isMissingDirectorPlanError, stateEntries } from './utils'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'

export interface DirectorConsoleProps {
  storyId?: string
  story?: StorySummary
  storyDirectors?: StoryDirector[]
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  branchId: string
  snapshot: Snapshot | null
  loading: boolean
	stateError?: string
	stateDisplayPreference: StoryStateDisplayPreference
	onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
  activeTab: ConsoleTab
  onTabChange: (tab: ConsoleTab) => void
  directorRevealed: boolean
  onRevealDirector: () => void
  onSnapshotRefresh?: () => void | Promise<unknown>
}

export function DirectorConsole({
  storyId,
  story,
  storyDirectors = [],
  onDirectorChange,
  onReplyTargetCharsChange,
  branchId,
  snapshot,
  loading,
	stateError,
	stateDisplayPreference,
	onStateDisplayPreferenceChange,
  activeTab,
  onTabChange,
  directorRevealed,
  onRevealDirector,
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
          console.info('[interactive-director-panel] director plan missing for branch', { storyId, branchId, error: err })
        } else {
          console.error('[interactive-director-panel] load director plan failed', err)
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
      console.error('[interactive-director-panel] rebuild director failed', err)
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
      console.error('[interactive-director-panel] save director plan failed', err)
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
      console.error('[interactive-director-panel] retry director failed', err)
      setDirectorError(err instanceof Error ? err.message : t('storyStage.director.retryFailed'))
    } finally {
      setRetryingDirector(false)
    }
  }

  const analyzeDirectorContext = async () => {
    if (!storyId || !currentTurnId) {
      setContextAnalysis(null)
      setContextAnalysisError(t('directorPanel.directorContextAnalysisUnavailable'))
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
      console.error('[interactive-director-panel] analyze director context failed', err)
      setContextAnalysisError(err instanceof Error ? err.message : t('directorPanel.directorContextAnalysisFailed'))
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
      console.error('[interactive-director-panel] reroll rules failed', err)
      setRuleError(err instanceof Error ? err.message : t('snapshot.ruleAudit.rerollFailed'))
    } finally {
      setRerolling(false)
    }
  }

  return (
    <aside className="director-console flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <DirectorConsoleHeader branchId={branchId} turnCount={(snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0)} story={story} storyDirectors={storyDirectors} onDirectorChange={onDirectorChange} onReplyTargetCharsChange={onReplyTargetCharsChange} stateDisplayPreference={stateDisplayPreference} onStateDisplayPreferenceChange={onStateDisplayPreferenceChange} />
      <ConsoleTabs activeTab={activeTab} onChange={onTabChange} stateCount={actorCount} directorStatus={directorStatus} directorActive={rebuilding || retryingDirector} />
      {activeTab === 'director' && directorError ? <div className="mx-4 mt-3 rounded-[10px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-5 text-[var(--nova-danger)]">{directorError}</div> : null}
      <div className="min-h-0 flex-1 overflow-hidden px-4 py-4">
        <div className="director-console__scroll h-full min-h-0 overflow-y-auto pb-4 pr-1">
          {activeTab === 'state' ? (
            <StateView snapshot={snapshot} stateFacts={stateFacts} syncError={stateError} />
          ) : (
            <DirectorView
              storyId={storyId}
              snapshot={snapshot}
              onSnapshotRefresh={onSnapshotRefresh}
              revealed={directorRevealed}
              onReveal={onRevealDirector}
              hasDirectorRun={hasDirectorRun}
              directorStatus={directorStatus}
              directorMetadata={directorMetadata}
              directorPlan={directorPlan}
              draftDocs={draftDocs}
              onDraftDocsChange={setDraftDocs}
              loading={loading || planLoading || retryingDirector}
              running={retryingDirector}
              rebuilding={rebuilding}
              saving={savingPlan}
              directorError={directorError}
              directorDisplayEvents={directorDisplayEvents}
              analyzing={contextAnalysisLoading}
              canAnalyze={canAnalyzeDirectorContext}
              onRun={() => void runDirectorPlan()}
              onAnalyze={openDirectorContextAnalysis}
              onEvaluateEvent={() => void runDirectorPlan(true)}
              onResetEvents={() => void rebuildDirector(true)}
              onSave={() => void saveDirectorPlan()}
              onRebuild={() => void rebuildDirector()}
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
        title={t('directorPanel.directorContextAnalysis')}
        description={t('directorPanel.directorContextAnalysisDescription')}
      />
    </aside>
  )
}
