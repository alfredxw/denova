import { useMemo, type ReactNode } from 'react'
import { Activity, AlertCircle, Brain, CheckCircle2, Clock3, Eye, FileText, Loader2, RefreshCw, ScrollText, ShieldAlert, Sparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MessageList } from '@/components/Chat/MessageList'
import { Button } from '@/components/ui/button'
import type { ChatMessage } from '@/lib/api'
import { chatMessagesToAgentUIMessages } from '@/lib/agent-legacy-message'
import type { AgentUIMessage } from '@/lib/agent-ui'
import type { DirectorPlanMetadata, TurnDisplayEvent } from '../../types'
import type { DirectorStatusLike } from './types'
import { directorPlanTotals, directorStatusFallback, directorStatusLabel, displayEventToChatMessage, formatBytes, formatShortDate } from './utils'

export function DirectorRunView({
  storyId,
  hasDirectorRun,
  directorStatus,
  directorMetadata,
  directorDisplayEvents,
  loading,
  retrying,
  contextAnalysisLoading,
  canAnalyzeDirectorContext,
  directorError,
  processRevealed,
  generateMessages,
  generating,
  generateActivity,
  onRevealProcess,
  onRun,
  onAnalyze,
  onGenerateMemory,
  onAbortGenerate,
}: {
  storyId?: string
  hasDirectorRun: boolean
  directorStatus?: DirectorStatusLike
  directorMetadata?: DirectorPlanMetadata
  directorDisplayEvents: TurnDisplayEvent[]
  loading: boolean
  retrying: boolean
  contextAnalysisLoading: boolean
  canAnalyzeDirectorContext: boolean
  directorError: string
  processRevealed: boolean
  generateMessages: AgentUIMessage[]
  generating: boolean
  generateActivity: string
  onRevealProcess: () => void
  onRun: () => void
  onAnalyze: () => void
  onGenerateMemory: () => void
  onAbortGenerate: () => void
}) {
  return (
    <div className="space-y-3">
      {hasDirectorRun ? (
        <DirectorRunStatusCard
          status={directorStatus}
          metadata={directorMetadata}
          loading={loading}
          error={directorError}
        />
      ) : (
        <DirectorEmptyState error={directorError} running={retrying} />
      )}
      <DirectorProcessPanel
        storyId={storyId}
        status={directorStatus}
        metadata={directorMetadata}
        loading={loading}
        retrying={retrying}
        contextAnalysisLoading={contextAnalysisLoading}
        canAnalyzeDirectorContext={canAnalyzeDirectorContext}
        displayEvents={directorDisplayEvents}
        revealed={processRevealed}
        generateMessages={generateMessages}
        generating={generating}
        generateActivity={generateActivity}
        onReveal={onRevealProcess}
        onRun={onRun}
        onAnalyze={onAnalyze}
        onGenerateMemory={onGenerateMemory}
        onAbortGenerate={onAbortGenerate}
      />
    </div>
  )
}

function DirectorRunStatusCard({ status, metadata, loading, error }: { status?: DirectorStatusLike; metadata?: DirectorPlanMetadata; loading: boolean; error?: string }) {
  const { t } = useTranslation()
  const currentStatus = loading && !status?.status ? 'loading' : status?.status || ''
  const running = currentStatus === 'running' || currentStatus === 'loading'
  const failed = currentStatus === 'failed'
  const totals = directorPlanTotals(status, metadata)
  const summary = error || status?.error || status?.summary || directorStatusFallback(currentStatus, t)
  const updatedAt = status?.updated_at || metadata?.updated_at || ''
  const statusIcon = failed ? <AlertCircle className="h-4 w-4" /> : running ? <Loader2 className="h-4 w-4 animate-spin" /> : <CheckCircle2 className="h-4 w-4" />

  return (
    <section data-testid="director-run-summary" className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <div className={`h-0.5 w-full ${running ? 'animate-pulse bg-[var(--director-brass)]' : failed ? 'bg-[var(--nova-danger)]' : 'bg-[var(--director-live)]'}`} />
      <div className="p-3">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border bg-[var(--nova-surface)] ${failed ? 'border-[var(--nova-danger-border)] text-[var(--nova-danger)]' : running ? 'border-[var(--nova-accent-blue)]/40 text-[var(--nova-accent-blue)]' : 'border-[var(--nova-border)] text-[var(--nova-accent-green)]'}`}>
                {statusIcon}
              </span>
              <div className="min-w-0">
                <h3 className="director-console__display truncate text-base font-semibold text-[var(--nova-text)]">{t('memoryPanel.run.statusTitle')}</h3>
                <p className="mt-0.5 truncate text-[9px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{directorStatusLabel(status, loading, t)}</p>
              </div>
            </div>
            <p className="mt-3 break-words text-xs leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{summary}</p>
            {status?.decision?.mode ? (
              <div className="mt-2 flex flex-wrap items-center gap-1.5 text-[10px] text-[var(--nova-text-muted)]">
                <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 font-medium text-[var(--nova-text)]">
                  {t(`memoryPanel.planDecision.${status.decision.mode}`, { defaultValue: status.decision.mode })}
                </span>
                {status.decision.triggers?.slice(0, 3).map((trigger) => (
                  <span key={trigger} className="rounded-full bg-[var(--nova-hover)] px-2 py-0.5">{trigger}</span>
                ))}
              </div>
            ) : null}
          </div>
          {updatedAt ? (
            <span className="shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {formatShortDate(updatedAt)}
            </span>
          ) : null}
        </div>

        <div className="director-run-metrics mt-3 grid grid-cols-1 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
          <RunMetric icon={<FileText className="h-3.5 w-3.5" />} label={t('snapshot.director.docs')} value={`${totals.completed}/${totals.planned}`} />
          <RunMetric icon={<Clock3 className="h-3.5 w-3.5" />} label={t('snapshot.director.branchPlanningTurns')} value={String(metadata?.branch_planning_turns || 5)} />
          <RunMetric icon={<Activity className="h-3.5 w-3.5" />} label={t('memoryPanel.run.visibleBytes')} value={`${formatBytes(totals.visibleBytes)} / ${formatBytes(totals.totalBytes)}`} />
        </div>
      </div>
    </section>
  )
}

function DirectorProcessPanel({
  storyId,
  status,
  metadata,
  loading,
  retrying,
  contextAnalysisLoading,
  canAnalyzeDirectorContext,
  displayEvents,
  revealed,
  generateMessages,
  generating,
  generateActivity,
  onReveal,
  onRun,
  onAnalyze,
  onGenerateMemory,
  onAbortGenerate,
}: {
  storyId?: string
  status?: DirectorStatusLike
  metadata?: DirectorPlanMetadata
  loading: boolean
  retrying: boolean
  contextAnalysisLoading: boolean
  canAnalyzeDirectorContext: boolean
  displayEvents: TurnDisplayEvent[]
  revealed: boolean
  generateMessages: AgentUIMessage[]
  generating: boolean
  generateActivity: string
  onReveal: () => void
  onRun: () => void
  onAnalyze: () => void
  onGenerateMemory: () => void
  onAbortGenerate: () => void
}) {
  const { t } = useTranslation()
  const process = useDirectorProcessMessages({ status, metadata, loading, displayEvents, generateMessages, generating, generateActivity })
  return (
    <section data-testid="director-process-panel" className="rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] p-3">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
            <Activity className="h-3.5 w-3.5 shrink-0 text-[var(--director-brass)]" />
            <span className="director-console__display truncate text-[15px]">{t('memoryPanel.process.title')}</span>
          </div>
          <p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{t('memoryPanel.process.description')}</p>
        </div>
      </div>

      <div className="director-process-actions mt-3 grid grid-cols-1 gap-2">
        <ProcessActionButton
          icon={retrying ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
          label={retrying ? t('memoryPanel.directorManualRunning') : t('memoryPanel.directorManualRun')}
          onClick={onRun}
          disabled={!storyId || retrying}
        />
        <ProcessActionButton
          icon={contextAnalysisLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <ScrollText className="h-3.5 w-3.5" />}
          label={contextAnalysisLoading ? t('chat.contextAnalysis.loading') : t('memoryPanel.directorContextAnalysis')}
          onClick={onAnalyze}
          disabled={!canAnalyzeDirectorContext || contextAnalysisLoading}
        />
        <ProcessActionButton
          icon={generating ? <X className="h-3.5 w-3.5" /> : <Brain className="h-3.5 w-3.5" />}
          label={generating ? t('memoryPanel.abortGenerate') : t('memoryPanel.generate')}
          onClick={generating ? onAbortGenerate : onGenerateMemory}
          disabled={!storyId}
        />
      </div>

      <div className="mt-3">
        {!revealed ? (
          <DirectorProcessGate onReveal={onReveal} />
        ) : process.messages.length > 0 || process.streaming ? (
          <div className="flex h-[320px] min-h-[240px] flex-col overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
            <MessageList
              messages={process.messages}
              isStreaming={process.streaming}
              activityContent={process.activityContent}
              scrollResetKey={process.scrollKey}
              bottomPaddingClassName="pb-3"
              messageStyle={{ fontSize: '12px', lineHeight: 1.55 }}
              collapseTraceBeforeAssistant
            />
          </div>
        ) : (
          <div className="flex min-h-[180px] items-center justify-center rounded-[10px] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs leading-5 text-[var(--nova-text-muted)]">{t('memoryPanel.process.empty')}</div>
        )}
      </div>
    </section>
  )
}

function useDirectorProcessMessages({
  status,
  metadata,
  loading,
  displayEvents,
  generateMessages,
  generating,
  generateActivity,
}: {
  status?: DirectorStatusLike
  metadata?: DirectorPlanMetadata
  loading: boolean
  displayEvents: TurnDisplayEvent[]
  generateMessages: AgentUIMessage[]
  generating: boolean
  generateActivity: string
}) {
  const { t } = useTranslation()
  return useMemo(() => {
    const currentStatus = loading && !status?.status ? 'loading' : status?.status || ''
    const running = currentStatus === 'running' || currentStatus === 'loading'
    const hasDirectorSignal = Boolean(currentStatus || status || metadata || displayEvents.length)
    const totals = directorPlanTotals(status, metadata)
    const summary = status?.error || status?.summary || directorStatusFallback(currentStatus, t)
    const updatedAt = status?.updated_at || metadata?.updated_at || ''
    const progress = t('memoryPanel.directorChat.planProgress', {
      completed: totals.completed,
      planned: totals.planned,
      visible: formatBytes(totals.visibleBytes),
      total: formatBytes(totals.totalBytes),
      turns: metadata?.branch_planning_turns || 5,
    })
    const meta = updatedAt ? t('memoryPanel.directorChat.updatedAt', { time: formatShortDate(updatedAt) }) : currentStatus || t('snapshot.noRecord')
    const toolStatus = currentStatus === 'failed' ? 'error' : running ? 'running' : 'success'
    const showFileTool = ['running', 'ready', 'failed', 'conflict'].includes(currentStatus)
    const persistedMessages = displayEvents.map((event, index) => displayEventToChatMessage(event, `director-event-${index}`))
    const fileToolMessages: ChatMessage[] = persistedMessages.length > 0
      ? persistedMessages
      : showFileTool
        ? [{
            id: 'director-run-tool',
            role: 'tool_call',
            name: 'edit_file',
            status: toolStatus,
            args: JSON.stringify({ file_path: 'director.md' }),
            result: toolStatus === 'success' ? progress : '',
            created_at: updatedAt,
          }]
        : []
    const directorMessages: ChatMessage[] = hasDirectorSignal ? [
      {
        id: 'director-run-request',
        role: 'user',
        content: t('memoryPanel.directorChat.request'),
      },
      {
        id: 'director-run-thinking',
        role: 'thinking',
        content: summary,
        streaming: running,
        created_at: updatedAt,
      },
      ...fileToolMessages,
      {
        id: 'director-run-result',
        role: currentStatus === 'failed' ? 'error' : 'assistant',
        content: `${summary}\n\n${t('snapshot.director.plan')}: ${progress}\n${meta}`,
        streaming: running,
        created_at: updatedAt,
      },
    ] : []
    const messages = generateMessages.length > 0
      ? [
          ...chatMessagesToAgentUIMessages(directorMessages),
          ...chatMessagesToAgentUIMessages([{ id: 'memory-generation-section', role: 'system', content: t('memoryPanel.process.memorySection') }]),
          ...generateMessages,
        ]
      : chatMessagesToAgentUIMessages(directorMessages)
    return {
      messages,
      streaming: running || generating,
      activityContent: generating ? generateActivity : running ? summary : '',
      scrollKey: `director-process:${metadata?.revision || ''}:${currentStatus}:${updatedAt}:${generateMessages.length}:${generating ? 'generating' : 'idle'}`,
    }
  }, [displayEvents, generateActivity, generateMessages, generating, loading, metadata, status, t])
}

function DirectorProcessGate({ onReveal }: { onReveal: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[200px] items-center justify-center rounded-[10px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-5 text-center">
      <div className="max-w-[24rem]">
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]">
          <ShieldAlert className="h-5 w-5" />
        </div>
        <h3 className="director-console__display mt-3 text-base font-semibold text-[var(--nova-text)]">{t('memoryPanel.process.spoilerTitle')}</h3>
        <p className="mt-2 text-xs leading-5 text-[var(--nova-text-muted)]">{t('memoryPanel.process.spoilerDescription')}</p>
        <Button type="button" size="sm" variant="outline" className="mt-4 gap-2 rounded-[9px] border-[var(--director-brass)] bg-[color-mix(in_srgb,var(--director-brass)_10%,var(--nova-surface))] text-[var(--nova-text)] hover:bg-[color-mix(in_srgb,var(--director-brass)_16%,var(--nova-surface))]" onClick={onReveal}>
          <Eye className="h-3.5 w-3.5" />
          {t('memoryPanel.process.reveal')}
        </Button>
      </div>
    </div>
  )
}

function ProcessActionButton({ icon, label, disabled, onClick }: { icon: ReactNode; label: string; disabled?: boolean; onClick: () => void }) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="min-w-0 justify-start gap-2 rounded-[9px] border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45"
      disabled={disabled}
      onClick={onClick}
    >
      {icon}
      <span className="min-w-0 truncate">{label}</span>
    </Button>
  )
}

function RunMetric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className="min-w-0 bg-[var(--nova-surface)] px-2.5 py-2">
      <div className="flex min-w-0 items-center gap-1.5 text-[10px] text-[var(--nova-text-faint)]">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <div className="mt-1 truncate text-xs font-medium text-[var(--nova-text)]" title={value}>{value}</div>
    </div>
  )
}

function DirectorEmptyState({ error, running }: { error?: string; running?: boolean }) {
  const { t } = useTranslation()
  return (
    <section className="flex min-h-[240px] flex-col items-center justify-center rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--director-panel)] px-4 py-6 text-center">
      <div className="flex h-10 w-10 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
        {running ? <Loader2 className="h-5 w-5 animate-spin" /> : <Sparkles className="h-5 w-5" />}
      </div>
      <h3 className="director-console__display mt-3 text-base font-semibold text-[var(--nova-text)]">{t('memoryPanel.directorEmpty')}</h3>
      <p className="mt-2 max-w-[24rem] text-xs leading-5 text-[var(--nova-text-muted)]">{t('memoryPanel.directorManualRunHint')}</p>
      {error ? <div className="mt-3 w-full rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{error}</div> : null}
    </section>
  )
}
