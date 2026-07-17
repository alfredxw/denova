import type { DirectorPlan, DirectorPlanDocs, DirectorPlanMetadata, RuleResolution, Snapshot, TerminalOutcome, TurnDisplayEvent } from '../../types'
import { DirectorDocsCard } from './DirectorDocsCard'
import { DirectorGate } from './DirectorGate'
import { DirectorProcessCard } from './DirectorProcessCard'
import { DirectorStatusCard } from './DirectorStatusCard'
import { EventRuntimeCard } from './EventRuntimeCard'
import { RuleAuditCard } from './RuleAuditCard'
import { StateSchemaCard } from './StateSchemaCard'
import type { DirectorStatusLike } from './types'

export interface DirectorViewProps {
  storyId?: string
  snapshot: Snapshot | null
  onSnapshotRefresh?: () => void | Promise<unknown>
  revealed: boolean
  onReveal: () => void
  hasDirectorRun: boolean
  directorStatus?: DirectorStatusLike
  directorMetadata?: DirectorPlanMetadata
  directorPlan: DirectorPlan | null
  draftDocs: DirectorPlanDocs | null
  onDraftDocsChange: (docs: DirectorPlanDocs) => void
  loading: boolean
  running: boolean
  rebuilding: boolean
  saving: boolean
  directorError: string
  directorDisplayEvents: TurnDisplayEvent[]
  analyzing: boolean
  canAnalyze: boolean
  onRun: () => void
  onAnalyze: () => void
  onEvaluateEvent: () => void
  onResetEvents: () => void
  onSave: () => void
  onRebuild: () => void
  hasRuleAudit: boolean
  ruleResolution?: RuleResolution
  terminalOutcome?: TerminalOutcome
  ruleError: string
  rerolling: boolean
  onReroll: () => void
}

// 导演 tab：常驻概览（运行状态 + 规则审计）对所有人可见；
// 节拍表、事件编排与执行过程等可能剧透的内容统一收在一道防剧透门后。
// 还没有任何导演运行记录时没有剧透可防，直接展示空态。
export function DirectorView({
  storyId,
  snapshot,
  onSnapshotRefresh,
  revealed,
  onReveal,
  hasDirectorRun,
  directorStatus,
  directorMetadata,
  directorPlan,
  draftDocs,
  onDraftDocsChange,
  loading,
  running,
  rebuilding,
  saving,
  directorError,
  directorDisplayEvents,
  analyzing,
  canAnalyze,
  onRun,
  onAnalyze,
  onEvaluateEvent,
  onResetEvents,
  onSave,
  onRebuild,
  hasRuleAudit,
  ruleResolution,
  terminalOutcome,
  ruleError,
  rerolling,
  onReroll,
}: DirectorViewProps) {
  const gated = !revealed && hasDirectorRun
  return (
    <div className="space-y-3">
      <DirectorStatusCard
        storyId={storyId}
        hasDirectorRun={hasDirectorRun}
        status={directorStatus}
        metadata={directorMetadata}
        loading={loading}
        running={running}
        analyzing={analyzing}
        canAnalyze={canAnalyze}
        error={directorError}
        onRun={onRun}
        onAnalyze={onAnalyze}
      />
      {hasRuleAudit ? (
        <RuleAuditCard ruleResolution={ruleResolution} terminalOutcome={terminalOutcome} error={ruleError} rerolling={rerolling} onReroll={onReroll} />
      ) : null}
      {gated ? (
        <DirectorGate onReveal={onReveal} />
      ) : (
        <>
          {hasDirectorRun ? (
            <EventRuntimeCard status={directorStatus} metadata={directorMetadata} busy={loading || rebuilding} onEvaluate={onEvaluateEvent} onReset={onResetEvents} />
          ) : null}
          <DirectorDocsCard
            storyId={storyId}
            directorPlan={directorPlan}
            draftDocs={draftDocs}
            onDraftDocsChange={onDraftDocsChange}
            directorStatus={directorStatus}
            loading={loading}
            saving={saving}
            rebuilding={rebuilding}
            onSave={onSave}
            onRebuild={onRebuild}
          />
          <DirectorProcessCard status={directorStatus} metadata={directorMetadata} loading={loading} displayEvents={directorDisplayEvents} />
        </>
      )}
      <StateSchemaCard storyId={storyId} snapshot={snapshot} onRefresh={onSnapshotRefresh} />
    </div>
  )
}
