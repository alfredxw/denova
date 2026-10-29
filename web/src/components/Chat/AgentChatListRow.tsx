import { useLayoutEffect, useState, type CSSProperties, type ReactNode, type RefCallback } from 'react'
import { motion, useReducedMotionConfig } from 'motion/react'
import { useTranslation } from 'react-i18next'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration, ChatMessage } from '@/lib/api'
import { agentViewNavigationAnchor, isAgentTraceView, type AgentExecutionTiming, type AgentMessageView, type AgentPartRef } from '@/lib/agent-message-view'
import { listItem, novaEase } from '@/features/motion/motion-tokens'
import { useMotionIntensity } from '@/features/motion/motion-preferences'
import { cn } from '@/lib/utils'
import { AgentActivityShimmer, MessageItem, type AssistantMessagePresentation } from './MessageItem'
import { AgentMessageItem } from './AgentMessageItem'
import { AgentExecutionProcess } from './AgentExecutionProcess'
import { VirtualizedMessageRow, useVirtualizedMessageState } from './VirtualizedMessageState'
import { AgentRunActions } from './AgentRunActions'

export type AgentChatListItem =
  | { kind: 'empty'; key: string }
  | { kind: 'typing'; key: string }
  | { kind: 'activity'; key: string; content: string; runId?: string }
  | { kind: 'clear'; key: string; createdAt?: string }
  | { kind: 'message'; key: string; view: AgentMessageView; sourceIndex: number; processKey?: string; depth?: number; ownerRunId?: string; navigationAnchor?: string }
  | { kind: 'legacy-message'; key: string; message: ChatMessage; sourceIndex: number; openView?: AgentMessageView; processKey?: string; depth?: number; ownerRunId?: string; navigationAnchor?: string }
  | { kind: 'process'; key: string; views: AgentMessageView[]; runId: string; running: boolean; expanded: boolean; showTiming: boolean; navigationAnchor: string }
  | { kind: 'run-actions'; key: string; runId: string }
  | { kind: 'attachment'; key: string; runId: string; content: ReactNode }

export function AgentChatListRow({ projectId, item, nextItem, animateAttachment = false, executionTimings, isStreaming, tailFollowActive, onProcessExpandedChange, subAgentPresentation, highlightDialogue, messageStyle, contentClassName, canMutateMessage, onEditMessage, onEditAssistantReply, onCreateBranch, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onReadAloud, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onApprovePlan, onContinuePlan, onExitPlanMode, onResolveAsk, onInteractiveCardLayoutChange, streamingRowRef, syncStreamingTailLayout }: {
  projectId?: string
  item: AgentChatListItem
  executionTimings: ReadonlyMap<string, AgentExecutionTiming>
  nextItem?: AgentChatListItem
  animateAttachment?: boolean
  isStreaming: boolean
  tailFollowActive: boolean
  onProcessExpandedChange?: (key: string, expanded: boolean, navigationAnchor: string) => void | Promise<void>
  subAgentPresentation: 'card' | 'content'
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  contentClassName?: string
  canMutateMessage?: (view: AgentMessageView) => boolean
  onEditMessage?: (view: AgentMessageView) => void
  onEditAssistantReply?: (view: AgentMessageView) => void
  onCreateBranch?: (view: AgentMessageView) => void
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onReadAloud?: (message: AgentMessageView) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onResolveAsk?: (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => Promise<AgentAskResolution>
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  streamingRowRef?: RefCallback<HTMLDivElement>
  syncStreamingTailLayout?: () => void
}) {
  const { t } = useTranslation()
  const isLast = !nextItem
  // Consecutive trace rows share the spacing used inside AgentExecutionProcess.
  const continuesTrace = item.kind === 'message' && isAgentTraceView(item.view)
    && nextItem?.kind === 'message' && isAgentTraceView(nextItem.view)
  const turnAnchor = chatListItemNavigationAnchor(item)
  const renderMessageView = (view: AgentMessageView, key?: string, assistantPresentation: AssistantMessagePresentation = 'message') => {
    const mutationsAllowed = canMutateMessage?.(view) !== false
    return (
      <AgentMessageItem
        projectId={projectId}
        key={key}
        view={view}
        assistantPresentation={assistantPresentation}
        highlightDialogue={highlightDialogue}
        messageStyle={messageStyle}
        onEditMessage={isStreaming || !mutationsAllowed ? undefined : onEditMessage}
        onEditAssistantReply={isStreaming || !mutationsAllowed ? undefined : onEditAssistantReply}
        onCreateBranch={isStreaming ? undefined : onCreateBranch}
        onRegenerateMessage={isStreaming || !mutationsAllowed ? undefined : onRegenerateMessage}
        onSwitchMessageVersion={isStreaming || !mutationsAllowed ? undefined : onSwitchMessageVersion}
        onOpenSubAgentSession={onOpenSubAgentSession}
        onInsertIllustration={onInsertIllustration}
        onReadAloud={onReadAloud}
        onGenerateInteractiveImage={isStreaming ? undefined : onGenerateInteractiveImage}
        generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
        activeSubAgentSessionKey={activeSubAgentSessionKey}
        subAgentPresentation={subAgentPresentation}
        onApprovePlan={isStreaming ? undefined : onApprovePlan}
        onContinuePlan={isStreaming ? undefined : onContinuePlan}
        onExitPlanMode={isStreaming ? undefined : onExitPlanMode}
        onResolveAsk={onResolveAsk}
        onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      />
    )
  }
  useLayoutEffect(() => {
    if (isLast && tailFollowActive) syncStreamingTailLayout?.()
  }, [isLast, item, syncStreamingTailLayout, tailFollowActive])

  // A translated active tail would visibly move after its bottom anchor is captured.
  return (
    <VirtualizedMessageRow value={item.key}>
    <motion.div
      ref={isLast ? streamingRowRef : undefined}
      data-nova-chat-item={item.kind}
      data-agent-execution-content={'processKey' in item ? item.processKey : undefined}
      data-tool-call-depth={'depth' in item ? item.depth : undefined}
      data-nova-chat-tail-row={isLast ? '' : undefined}
      data-nova-chat-row-key={item.key}
      data-nova-chat-turn-anchor={turnAnchor}
      style={'depth' in item && item.depth ? { paddingInlineStart: `${24 + Math.min(item.depth, 4) * 12}px` } : undefined}
      className={cn('min-w-0 px-6', contentClassName, isLast ? 'pb-0' : continuesTrace || ('processKey' in item && item.processKey) || item.kind === 'process' ? 'pb-2' : 'pb-4')}
      variants={listItem}
      initial={item.kind === 'attachment' || (isLast && isStreaming) ? false : 'initial'}
      animate="animate"
      transition={{ duration: 0.18, ease: novaEase }}
    >
      {item.kind === 'empty' ? (
        <div className="flex min-h-[240px] items-center justify-center">
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 text-center text-sm text-[var(--nova-text-muted)] shadow-[0_14px_34px_rgba(0,0,0,0.22)]">
            {t('chat.empty')}
          </div>
        </div>
      ) : item.kind === 'typing' ? (
        <div className="flex justify-start">
          <div className="px-1 py-2">
            <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-[var(--nova-text-muted)]" />
          </div>
        </div>
      ) : item.kind === 'activity' ? (
        <AgentActivityShimmer content={item.content} />
      ) : item.kind === 'clear' ? (
        <ContextClearDivider createdAt={item.createdAt} />
      ) : item.kind === 'process' ? (
        <AgentExecutionProcess views={item.views} running={item.running} expanded={item.expanded}
          onExpandedChange={expanded => onProcessExpandedChange?.(item.key, expanded, item.navigationAnchor)}
          timing={item.showTiming ? executionTimings.get(item.runId) : undefined} />
      ) : item.kind === 'run-actions' ? (
        <div className="flex flex-wrap items-center gap-2 px-1"><AgentRunActions projectId={projectId} runID={item.runId} /></div>
      ) : item.kind === 'attachment' ? (
        <TimelineAttachment enter={animateAttachment}>{item.content}</TimelineAttachment>
      ) : item.kind === 'legacy-message' ? (
        <MessageItem
          projectId={projectId}
          message={item.message}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onOpenSubAgentSession={item.openView && onOpenSubAgentSession ? () => onOpenSubAgentSession(item.openView as AgentMessageView) : undefined}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          onResolveAsk={item.openView && onResolveAsk ? (_message, action) => onResolveAsk(item.openView as AgentMessageView, action) : undefined}
        />
      ) : (
        renderMessageView(item.view, undefined, item.processKey && item.view.kind === 'assistant' ? 'progress' : 'message')
      )}
    </motion.div>
    </VirtualizedMessageRow>
  )
}

/** Animate only a newly finished live run, once across virtual row remounts. */
function TimelineAttachment({ enter, children }: { enter: boolean; children: ReactNode }) {
  const [revealed, setRevealed] = useVirtualizedMessageState('attachment-revealed', false)
  const [animateEntry] = useState(() => enter && !revealed)
  const reducedMotion = useReducedMotionConfig()
  const motionIntensity = useMotionIntensity()
  useLayoutEffect(() => { setRevealed(true) }, [setRevealed])
  return (
    <motion.div
      className="overflow-hidden"
      data-nova-timeline-attachment
      // Virtuoso needs a non-zero initial measurement for a new row.
      initial={animateEntry && motionIntensity !== 'off' ? { height: reducedMotion ? 'auto' : 1, opacity: 0 } : false}
      animate={{ height: 'auto', opacity: 1 }}
      transition={{ height: { duration: reducedMotion ? 0 : 0.2, ease: novaEase }, opacity: { duration: motionIntensity === 'off' ? 0 : 0.14 } }}
    >
      {children}
    </motion.div>
  )
}

export function chatListItemRunID(item: AgentChatListItem): string {
  if ('ownerRunId' in item && item.ownerRunId) return item.ownerRunId
  if (item.kind === 'activity') return item.runId || ''
  if (item.kind === 'message') return item.view.metadata.run_id || ''
  if (item.kind === 'legacy-message') return item.message.run_id || item.openView?.metadata.run_id || ''
  if (item.kind === 'process' || item.kind === 'run-actions') return item.runId
  if (item.kind === 'attachment') return item.runId
  return ''
}

export function chatListItemNavigationAnchor(item?: AgentChatListItem) {
  if (!item) return ''
  if ('navigationAnchor' in item && item.navigationAnchor) return item.navigationAnchor
  if (item.kind === 'message') return agentViewNavigationAnchor(item.view)
  if (item.kind === 'legacy-message') return item.message.navigation_turn_id || item.message.turn_id || ''
  if (item.kind === 'process') return item.navigationAnchor
  return ''
}

function ContextClearDivider({ createdAt }: { createdAt?: string }) {
  const { t } = useTranslation()
  const timeText = createdAt ? new Date(createdAt).toLocaleString() : ''

  return (
    <div className="flex items-center gap-3 py-1" role="separator" aria-label={t('chat.contextCleared')}>
      <div className="h-px flex-1 bg-[var(--nova-border)]" />
      <div className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1 text-[11px] text-[var(--nova-text-muted)]">
        {t('chat.contextClearedDetail', { time: timeText ? ` · ${timeText}` : '' })}
      </div>
      <div className="h-px flex-1 bg-[var(--nova-border)]" />
    </div>
  )
}
