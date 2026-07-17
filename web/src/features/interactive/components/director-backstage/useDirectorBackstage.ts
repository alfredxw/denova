import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ContextAnalysis } from '@/lib/api'
import { analyzeInteractiveDirectorContext, getInteractiveDirector, rebuildInteractiveDirector, rerollInteractiveRuleResolution, runInteractiveDirector, updateInteractiveDirector } from '../../api'
import type { DirectorPlan, DirectorPlanDocs, DirectorPlanStatus, Snapshot } from '../../types'
import { extractDirectorDisplayEvents, isMissingDirectorPlanError } from '../director-console/utils'

// 导演台（主区子模式）的数据编排：计划文档的拉取/保存/重建、手动运行、
// 上下文分析、规则重掷。原右栏 DirectorConsole 的逻辑整体迁移至此。
export function useDirectorBackstage({ storyId, branchId, snapshot, revealed, onReveal, onSnapshotRefresh }: {
  storyId?: string
  branchId: string
  snapshot: Snapshot | null
  revealed: boolean
  onReveal: () => void
  onSnapshotRefresh?: () => void | Promise<unknown>
}) {
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

  useEffect(() => {
    setDirectorPlan(snapshot?.director_plan || null)
    setDraftDocs(snapshot?.director_plan?.docs || null)
  }, [snapshot?.director_plan, snapshot?.director_plan?.metadata?.revision])

  useEffect(() => {
    if (snapshot?.director_plan_status) setManualDirectorStatus(null)
  }, [snapshot?.director_plan_status?.revision, snapshot?.director_plan_status?.status, snapshot?.director_plan_status?.updated_at])

  useEffect(() => {
    if (!revealed || !storyId) return
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
          console.info('[interactive-director-backstage] director plan missing for branch', { storyId, branchId, error: err })
        } else {
          console.error('[interactive-director-backstage] load director plan failed', err)
        }
        setDirectorError(err instanceof Error ? err.message : t('snapshot.director.loadFailed'))
      })
      .finally(() => {
        if (!cancelled) setPlanLoading(false)
      })
    return () => { cancelled = true }
  }, [branchId, revealed, storyId, t])

  const rebuildDirector = async (resetEvents = false) => {
    if (!storyId || rebuilding) return
    setRebuilding(true)
    setDirectorError('')
    try {
      const plan = await rebuildInteractiveDirector(storyId, branchId, { resetEvents })
      setDirectorPlan(plan)
      setDraftDocs(plan.docs)
      onReveal()
      await onSnapshotRefresh?.()
    } catch (err) {
      console.error('[interactive-director-backstage] rebuild director failed', err)
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
      console.error('[interactive-director-backstage] save director plan failed', err)
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
      console.error('[interactive-director-backstage] retry director failed', err)
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
      console.error('[interactive-director-backstage] analyze director context failed', err)
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
      console.error('[interactive-director-backstage] reroll rules failed', err)
      setRuleError(err instanceof Error ? err.message : t('snapshot.ruleAudit.rerollFailed'))
    } finally {
      setRerolling(false)
    }
  }

  return {
    rebuilding,
    planLoading,
    savingPlan,
    retryingDirector,
    rerolling,
    directorError,
    ruleError,
    contextAnalysisOpen,
    setContextAnalysisOpen,
    contextAnalysisLoading,
    contextAnalysisError,
    contextAnalysis,
    directorPlan,
    draftDocs,
    setDraftDocs,
    directorMetadata,
    directorStatus,
    directorDisplayEvents,
    hasDirectorRun,
    hasRuleAudit,
    ruleResolution,
    terminalOutcome,
    canAnalyzeDirectorContext,
    rebuildDirector,
    saveDirectorPlan,
    runDirectorPlan,
    openDirectorContextAnalysis,
    rerollRules,
  }
}

export type DirectorBackstageModel = ReturnType<typeof useDirectorBackstage>
