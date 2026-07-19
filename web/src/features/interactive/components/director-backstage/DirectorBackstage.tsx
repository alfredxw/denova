import { useEffect, useState } from 'react'
import { AlertCircle, ArrowLeft, CheckCircle2, Clapperboard, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import { Button } from '@/components/ui/button'
import type { Snapshot } from '../../types'
import { useInteractiveStore } from '../../stores/interactive-store'
import { DirectorDocsCard } from '../director-console/DirectorDocsCard'
import { DirectorGate } from '../director-console/DirectorGate'
import { DirectorProcessCard } from '../director-console/DirectorProcessCard'
import { DirectorStatusCard } from '../director-console/DirectorStatusCard'
import { EventRuntimeCard } from '../director-console/EventRuntimeCard'
import { readStoredDirectorRevealed, writeStoredDirectorRevealed } from '../director-console/persistence'
import { RuleAuditCard } from '../director-console/RuleAuditCard'
import { StateSchemaCard } from '../director-console/StateSchemaCard'
import { directorStatusLabel } from '../director-console/utils'
import { useDirectorBackstage } from './useDirectorBackstage'

// 导演台：主区全屏子模式，承载所有幕后内容（节拍表、事件编排、执行过程、
// 规则审计、状态结构），让重内容摆脱窄侧栏。防剧透揭示一次、按故事持久化。
export function DirectorBackstage({ storyId, branchId, snapshot, loading = false, onSnapshotRefresh }: {
  storyId?: string
  branchId: string
  snapshot: Snapshot | null
  loading?: boolean
  onSnapshotRefresh?: () => void | Promise<unknown>
}) {
  const { t } = useTranslation()
  const setSubmode = useInteractiveStore((state) => state.setSubmode)
  const [revealed, setRevealed] = useState(() => readStoredDirectorRevealed(storyId))

  useEffect(() => {
    setRevealed(readStoredDirectorRevealed(storyId))
  }, [storyId])

  const reveal = () => {
    setRevealed(true)
    writeStoredDirectorRevealed(storyId, true)
  }

  const model = useDirectorBackstage({ storyId, branchId, snapshot, revealed, onReveal: reveal, onSnapshotRefresh })
  const {
    rebuilding, planLoading, savingPlan, retryingDirector, rerolling,
    directorError, ruleError,
    contextAnalysisOpen, setContextAnalysisOpen, contextAnalysisLoading, contextAnalysisError, contextAnalysis,
    directorPlan, draftDocs, setDraftDocs, directorMetadata, directorStatus, directorDisplayEvents,
    hasDirectorRun, hasRuleAudit, ruleResolution, terminalOutcome, canAnalyzeDirectorContext,
    rebuildDirector, saveDirectorPlan, runDirectorPlan, openDirectorContextAnalysis, rerollRules,
  } = model

  const busy = loading || planLoading || retryingDirector
  const gated = !revealed && hasDirectorRun
  const turnCount = (snapshot?.turns || []).length || (snapshot?.current_turn ? 1 : 0)

  return (
    <section data-testid="director-backstage" className="director-console flex h-full min-h-0 flex-col bg-[var(--director-canvas)] text-[var(--nova-text)]">
      <header className="shrink-0 border-b border-[var(--nova-border)] px-5 py-4">
        <div className="flex min-w-0 items-center gap-3">
          <Button variant="outline" size="xs" className="nova-nav-item shrink-0 gap-1.5 border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={() => setSubmode('story')}>
            <ArrowLeft className="h-3.5 w-3.5" />
            {t('branchTimeline.backToStory')}
          </Button>
          <div className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]" aria-hidden="true">
            <Clapperboard className="h-4.5 w-4.5" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-[9px] font-semibold uppercase tracking-[0.2em] text-[var(--nova-text-faint)]">{t('directorPanel.consoleEyebrow')}</p>
            <h2 className="director-console__display min-w-0 truncate text-base font-semibold leading-6 text-[var(--nova-text)]">{t('directorPanel.consoleTitle')}</h2>
            <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[9px] text-[var(--nova-text-faint)]">
              <span className="truncate">{t('directorPanel.branch', { branch: branchId || 'main' })}</span>
              <span aria-hidden="true">/</span>
              <span className="shrink-0">{t('directorPanel.turnCount', { count: turnCount })}</span>
            </div>
          </div>
          <DirectorStatusPill status={directorStatus?.status || ''} loading={busy} label={directorStatusLabel(directorStatus, busy, t)} />
        </div>
      </header>

      {!storyId ? (
        <div className="flex flex-1 items-center justify-center px-6 text-center text-xs leading-5 text-[var(--nova-text-faint)]">{t('directorPanel.backstageEmpty')}</div>
      ) : gated ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
          <div className="mx-auto max-w-2xl space-y-3">
            <DirectorStatusCard
              storyId={storyId}
              hasDirectorRun={hasDirectorRun}
              status={directorStatus}
              metadata={directorMetadata}
              loading={busy}
              running={retryingDirector}
              analyzing={contextAnalysisLoading}
              canAnalyze={canAnalyzeDirectorContext}
              error={directorError}
              onRun={() => void runDirectorPlan()}
              onAnalyze={openDirectorContextAnalysis}
            />
            {hasRuleAudit ? (
              <RuleAuditCard ruleResolution={ruleResolution} terminalOutcome={terminalOutcome} error={ruleError} rerolling={rerolling} onReroll={() => void rerollRules()} />
            ) : null}
            <DirectorGate onReveal={reveal} />
          </div>
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
          {directorError ? <div className="mb-3 rounded-[10px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-5 text-[var(--nova-danger)]">{directorError}</div> : null}
          <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,0.85fr)]">
            <DirectorDocsCard
              storyId={storyId}
              directorPlan={directorPlan}
              draftDocs={draftDocs}
              onDraftDocsChange={setDraftDocs}
              directorStatus={directorStatus}
              loading={busy}
              saving={savingPlan}
              rebuilding={rebuilding}
              onSave={() => void saveDirectorPlan()}
              onRebuild={() => void rebuildDirector()}
            />
            <div className="space-y-4">
              <DirectorStatusCard
                storyId={storyId}
                hasDirectorRun={hasDirectorRun}
                status={directorStatus}
                metadata={directorMetadata}
                loading={busy}
                running={retryingDirector}
                analyzing={contextAnalysisLoading}
                canAnalyze={canAnalyzeDirectorContext}
                error=""
                onRun={() => void runDirectorPlan()}
                onAnalyze={openDirectorContextAnalysis}
              />
              {hasDirectorRun ? (
                <EventRuntimeCard status={directorStatus} metadata={directorMetadata} busy={busy || rebuilding} onEvaluate={() => void runDirectorPlan(true)} onReset={() => void rebuildDirector(true)} />
              ) : null}
              {hasRuleAudit ? (
                <RuleAuditCard ruleResolution={ruleResolution} terminalOutcome={terminalOutcome} error={ruleError} rerolling={rerolling} onReroll={() => void rerollRules()} />
              ) : null}
              <DirectorProcessCard status={directorStatus} metadata={directorMetadata} loading={busy} displayEvents={directorDisplayEvents} />
              <StateSchemaCard snapshot={snapshot} />
            </div>
          </div>
        </div>
      )}

      <ContextAnalysisDialog
        open={contextAnalysisOpen}
        loading={contextAnalysisLoading}
        error={contextAnalysisError}
        analysis={contextAnalysis}
        onOpenChange={setContextAnalysisOpen}
        title={t('directorPanel.directorContextAnalysis')}
        description={t('directorPanel.directorContextAnalysisDescription')}
      />
    </section>
  )
}

function DirectorStatusPill({ status, loading, label }: { status: string; loading: boolean; label: string }) {
  const running = loading || status === 'running'
  const failed = status === 'failed'
  return (
    <span className={`flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] ${failed ? 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]' : 'border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--nova-text-muted)]'}`}>
      {running ? <Loader2 className="h-3 w-3 animate-spin" /> : failed ? <AlertCircle className="h-3 w-3" /> : <CheckCircle2 className="h-3 w-3 text-[var(--nova-accent-green)]" />}
      {label}
    </span>
  )
}
