import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { CheckCircle2, ClipboardCheck, Circle, Loader2, Pencil, RefreshCw, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChatMessage, ChatPlanAction, ContextCompactionChatMessage, ProposedPlanChatMessage } from '@/lib/api'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { Button } from '@/components/ui/button'
import { Plan, PlanContent, PlanHeader } from '@/components/ai-elements/plan'
import { planDisplayContent } from '@/lib/plan-mode'
import { MarkdownContent } from './message-content'

const planThinkingPreviewStaleMs = 3500

export function ContextCompactionBlock({ message }: { message: ContextCompactionChatMessage }) {
  const { t } = useTranslation()
  const status = message.status || 'running'
  const isRunning = status === 'running'
  const summary = (message.content || '').trim()
  const summaryScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: isRunning || message.streaming === true,
    resetKey: `${message.id || message.created_at || 'context-compaction'}:summary`,
    contentKey: `${status}:${message.phase || ''}:${summary.length}`,
  })

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)] backdrop-blur">
        <div className="flex min-w-0 items-start gap-2 px-3 py-2.5">
          <span
            className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]"
            aria-label={t(`chat.contextCompaction.status.${status}`)}
          >
            {isRunning ? (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
            ) : status === 'success' ? (
              <CheckCircle2 className="h-3.5 w-3.5 text-[var(--nova-accent-green)]" />
            ) : (
              <Circle className="h-3.5 w-3.5 text-[var(--nova-danger)]" />
            )}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <span className="font-medium text-[var(--nova-text)]">{t('chat.contextCompaction.title')}</span>
              <span className={`rounded-full border px-1.5 py-0.5 text-[10px] ${status === 'error' ? 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}>
                {t(`chat.contextCompaction.status.${status}`)}
              </span>
              {message.epoch ? (
                <span className="font-mono text-[10px] text-[var(--nova-text-faint)]">epoch {message.epoch}</span>
              ) : null}
              {message.attempt && message.attempt > 1 ? (
                <span className="font-mono text-[10px] text-[var(--nova-text-faint)]">{t('chat.contextCompaction.attempt', { count: message.attempt })}</span>
              ) : null}
            </div>
            <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-[var(--nova-text-faint)]">
              <span>{t(`chat.contextCompaction.phase.${message.phase || 'pre_run'}`)}</span>
            </div>
          </div>
        </div>
        <div
          ref={summaryScrollLock.ref}
          onScroll={summaryScrollLock.onScroll}
          onWheel={summaryScrollLock.onWheel}
          onKeyDown={summaryScrollLock.onKeyDown}
          data-nova-scroll-lock="context-compaction-summary"
          className="min-w-0 max-w-full max-h-40 overflow-x-hidden overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 text-[11px] leading-relaxed text-[var(--nova-text-muted)] whitespace-pre-wrap [overflow-anchor:none] [overflow-wrap:anywhere]"
        >
          {summary || (isRunning ? t('chat.contextCompaction.waiting') : t('chat.contextCompaction.empty'))}
        </div>
      </div>
    </div>
  )
}

export function ProposedPlanBlock({ projectId, message, highlightDialogue, onApprove, onContinue, onExit, onLayoutChange }: { projectId: string; message: ProposedPlanChatMessage; highlightDialogue?: boolean; onApprove?: (message: ChatMessage) => void; onContinue?: (message: ChatMessage) => void; onExit?: () => void; onLayoutChange?: () => void }) {
  const { t } = useTranslation()
  const display = planDisplayContent(message.content || '')
  const [localAction, setLocalAction] = useState<ChatPlanAction | undefined>(message.plan_action)
  const planAction = message.plan_action || localAction
  useEffect(() => {
    if (message.plan_action) setLocalAction(message.plan_action)
  }, [message.plan_action])
  if (message.status === 'running' && !(message.content || '').trim()) {
    return (
      <PlanShell icon={<Loader2 className="h-3.5 w-3.5 animate-spin" />} title={t('chat.plan.proposalTitle')} badge={t('chat.plan.generatingBadge')}>
        <PlanPendingBlock text={t('chat.plan.generatingProposal')} preview={message.thinking_preview} />
      </PlanShell>
    )
  }
  return (
    <PlanShell icon={<ClipboardCheck className="h-3.5 w-3.5" />} title={t('chat.plan.proposalTitle')} badge={t('chat.plan.proposalBadge')}>
      <div className="flex max-h-[min(680px,calc(100vh-220px))] min-h-0 flex-col">
        <div className="chat-agent-message min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1 text-sm leading-6 text-[var(--nova-text)] [scrollbar-gutter:stable]">
          <MarkdownContent
            content={display}
            highlightDialogue={highlightDialogue === true}
            projectId={projectId}
            streaming={message.streaming === true}
          />
        </div>
        {planAction ? (
          <PlanActionStatus text={planActionStatusText(t, planAction)} />
        ) : (
          <div className="mt-3 flex shrink-0 flex-wrap justify-end gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] pt-3">
            <Button type="button" size="xs" variant="outline" className="gap-1.5" onClick={() => {
              setLocalAction('continue')
              onLayoutChange?.()
              onContinue?.(message)
            }}>
              <Pencil className="h-3.5 w-3.5" />
              {t('chat.plan.continueDiscussion')}
            </Button>
            <Button type="button" size="xs" variant="outline" className="gap-1.5" onClick={() => {
              setLocalAction('exited')
              onLayoutChange?.()
              onExit?.()
            }}>
              <X className="h-3.5 w-3.5" />
              {t('chat.plan.exit')}
            </Button>
            <Button type="button" size="xs" className="gap-1.5" onClick={() => {
              setLocalAction('approved')
              onLayoutChange?.()
              onApprove?.(message)
            }}>
              <CheckCircle2 className="h-3.5 w-3.5" />
              {t('chat.plan.approve')}
            </Button>
          </div>
        )}
      </div>
    </PlanShell>
  )
}

function PlanPendingBlock({ text, preview }: { text: string; preview?: string }) {
  const { t } = useTranslation()
  const [visiblePreview, setVisiblePreview] = useState(preview || '')
  useEffect(() => {
    if (!preview) {
      setVisiblePreview('')
      return undefined
    }
    setVisiblePreview(preview)
    const timer = window.setTimeout(() => {
      setVisiblePreview((current) => current === preview ? '' : current)
    }, planThinkingPreviewStaleMs)
    return () => window.clearTimeout(timer)
  }, [preview])

  return (
    <div className="rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 text-xs text-[var(--nova-text-muted)]">
      <div className="flex items-center gap-2">
        <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-[var(--nova-text-muted)]" />
        <span>{text}</span>
      </div>
      {visiblePreview && (
        <div className="mt-1 flex min-w-0 items-center gap-1 text-[11px] text-[var(--nova-text-faint)]">
          <span className="shrink-0">{t('chat.plan.thinkingPreviewLabel')}</span>
          <span className="min-w-0 truncate">{visiblePreview}</span>
        </div>
      )}
    </div>
  )
}

function PlanActionStatus({ text }: { text: string }) {
  return (
    <div className="mt-3 flex shrink-0 items-center gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] pt-3 text-xs text-[var(--nova-text-muted)]">
      <CheckCircle2 className="h-3.5 w-3.5 text-[var(--nova-accent)]" />
      <span>{text}</span>
    </div>
  )
}

function planActionStatusText(t: ReturnType<typeof useTranslation>['t'], action: ChatPlanAction) {
  switch (action) {
    case 'approved':
      return t('chat.plan.approvedStatus')
    case 'continue':
      return t('chat.plan.continueStatus')
    case 'exited':
      return t('chat.plan.exitedStatus')
  }
}

function PlanShell({ icon, title, badge, children }: { icon: ReactNode; title: string; badge?: string; children: ReactNode }) {
  return (
    <div className="flex justify-start">
      <Plan defaultOpen className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)] backdrop-blur">
        <PlanHeader className="flex-row items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2.5">
          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            {icon}
          </span>
          <span className="min-w-0 flex-1 text-sm font-medium text-[var(--nova-text)]">{title}</span>
          {badge && <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{badge}</span>}
        </PlanHeader>
        <PlanContent className="px-3 py-3">
          {children}
        </PlanContent>
      </Plan>
    </div>
  )
}
