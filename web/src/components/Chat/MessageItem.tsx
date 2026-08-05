import { Children, Fragment, cloneElement, isValidElement, memo, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { Activity, AlertTriangle, Check, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Circle, CircleDot, ClipboardCheck, ClipboardList, Copy, Dice5, FileText, GitBranch, ImagePlus, ListTodo, Loader2, PanelRightOpen, Pencil, RefreshCw, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { MarkdownRenderer, type MarkdownRendererComponents } from '@/components/common/MarkdownRenderer'
import { projectFileAssetURL, type ChapterIllustration, type ChatMessage, type InteractiveImage, type InteractiveImageError } from '@/lib/api'
import type { UserMessageReference } from '@/lib/api-client/types'
import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'
import { decodeToolResultEnvelope, type ToolResultEnvelope } from '@/lib/tool-result-envelope'
import { isWorkspaceImagePath } from '@/lib/workspace-file-kind'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { TooltipProvider } from '@/components/ui/tooltip'
import { subAgentSessionKey } from './subagent-session'
import { Button } from '@/components/ui/button'
import { parsePlanQuestionSet, planDisplayContent } from '@/lib/plan-mode'
import { Message as AIMessage, MessageContent as AIMessageContent } from '@/components/ai-elements/message'
import { Reasoning, ReasoningContent, ReasoningTrigger } from '@/components/ai-elements/reasoning'
import { Plan, PlanContent, PlanHeader } from '@/components/ai-elements/plan'
import { Tool, ToolContent } from '@/components/ai-elements/tool'
import { Shimmer } from '@/components/ai-elements/shimmer'
import { StreamingContentStage } from './StreamingContentStage'
import { AskInteractionCard, type AskInteractionResolver } from './AskInteractionCard'
import { ToolApprovalCard, ToolApprovalPanel } from './ToolApprovalCard'

interface MessageItemProps {
  projectId?: string
  message: ChatMessage
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onEdit?: (message: ChatMessage) => void
  onEditAssistantReply?: (message: ChatMessage) => void
  onCreateBranch?: (message: ChatMessage) => void
  onRegenerate?: (message: ChatMessage) => void
  onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void
  onOpenSubAgentSession?: (message: ChatMessage) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (message: ChatMessage) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  subAgentPresentation?: 'card' | 'content'
  onApprovePlan?: (message: ChatMessage) => void
  onContinuePlan?: (message: ChatMessage) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  onResolveAsk?: AskInteractionResolver
}

const copyFeedbackDurationMs = 1200
const messageActionTooltipDelayMs = 500
const messageActionTooltipSkipDelayMs = 300
const messageActionTooltipSideOffset = 3
const planThinkingPreviewStaleMs = 3500

/** 单条消息组件，根据 role 渲染不同样式 */
export const MessageItem = memo(function MessageItem({ projectId = '', message, highlightDialogue = false, messageStyle, onEdit, onEditAssistantReply, onCreateBranch, onRegenerate, onSwitchVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, subAgentPresentation = 'card', onApprovePlan, onContinuePlan, onExitPlanMode, onOpenTrace, onInteractiveCardLayoutChange, onResolveAsk }: MessageItemProps) {
  const { role, content = '' } = message
  const canEdit = role === 'user' && Boolean(message.turn_id) && Boolean(onEdit)
  const canEditAssistantReply = role === 'assistant' && !message.subagent && Boolean(message.turn_id) && Boolean(onEditAssistantReply) && !message.streaming
  const canCreateBranch = role === 'assistant' && !message.subagent && Boolean(message.turn_id) && Boolean(onCreateBranch) && !message.streaming
  const canRegenerate = (role === 'assistant' || role === 'error') && Boolean(onRegenerate) && !message.streaming
  const canGenerateInteractiveImage = role === 'assistant' && Boolean(message.turn_id) && Boolean(onGenerateInteractiveImage) && !message.streaming
  const versionCount = message.turn_versions?.length || 0
  const markedVersionIndex = message.turn_versions?.findIndex((version) => version.current) ?? -1
  const versionIndex = message.turn_version_index ?? markedVersionIndex
  const canSwitchVersion = role === 'assistant' && versionCount > 1 && versionIndex >= 0 && Boolean(onSwitchVersion) && !message.streaming

  switch (role) {
    case 'user':
      return (
        <AIMessage from="user" className="max-w-none items-end">
          <div className="nova-message-body-with-meta nova-message-body-with-meta-user max-w-[88%]">
            <AIMessageContent className="nova-user-message rounded-lg bg-[var(--nova-user-message-bg-to)] px-3 py-2 text-sm leading-5 text-[var(--nova-user-message-text)] whitespace-pre-wrap group-[.is-user]:px-3 group-[.is-user]:py-2" style={messageStyle}>
              <SentMessageReferences references={message.user_references} />
              <span>{content}</span>
            </AIMessageContent>
            <MessageInlineMeta message={message} content={content} align="right" reserveSpace={Boolean(onEdit)} onEdit={canEdit ? onEdit : undefined} />
          </div>
        </AIMessage>
      )

    case 'assistant': {
      if (message.subagent && subAgentPresentation === 'card') {
        return (
          <SubAgentOutputWindow
            message={message}
            content={content}
            highlightDialogue={highlightDialogue}
            messageStyle={messageStyle}
            projectId={projectId}
            onOpen={onOpenSubAgentSession}
            active={Boolean(activeSubAgentSessionKey && activeSubAgentSessionKey === subAgentSessionKey(message))}
          />
        )
      }
      // 流式期间正文可能尚未到达，或全是被隐藏的思考内容（清洗后为空）：
      // 此时显示"正在思考"占位，避免出现一个空白气泡、像卡死无响应。
      const streamingTargetContent = message.streaming === true && message.streaming_target_content && message.streaming_target_content !== content
        ? message.streaming_target_content
        : undefined
      const visibleContent = sanitizeThinkTags(streamingTargetContent || content).trim()
      const reserveMetaSpace = message.streaming === true || Boolean(canEditAssistantReply || canCreateBranch || onGenerateInteractiveImage || onRegenerate || onSwitchVersion)
      return (
        <AIMessage from="assistant" className="max-w-none">
          <div className="w-full">
            <div className="nova-message-body-with-meta nova-message-body-with-meta-assistant">
              <AIMessageContent className="chat-agent-message block w-full gap-0 px-1 text-sm text-[var(--nova-text)]" style={messageStyle}>
                {/* 流式与完成态共用同一棵 Markdown 组件树，历史回填时只更新内容，不重新挂载正文。 */}
                {message.streaming && !visibleContent ? (
                  <StreamingPlaceholder />
                ) : (
                  <StreamingContentStage content={content} targetContent={streamingTargetContent} streaming={message.streaming === true}>
                    {(value) => <MarkdownContent content={value} highlightDialogue={highlightDialogue} projectId={projectId} />}
                  </StreamingContentStage>
                )}
              </AIMessageContent>
              <InteractiveImageStrip message={message} projectId={projectId} />
              <MessageInlineMeta
                message={message}
                content={content}
                align="left"
                reserveSpace={reserveMetaSpace}
                hideActions={message.streaming === true}
                onEdit={canEditAssistantReply ? onEditAssistantReply : undefined}
                editLabelKey="chat.action.editAssistantReply"
                onCreateBranch={canCreateBranch ? onCreateBranch : undefined}
                onGenerateInteractiveImage={canGenerateInteractiveImage ? onGenerateInteractiveImage : undefined}
                generatingInteractiveImage={Boolean(message.turn_id && generatingInteractiveImageTurnId === message.turn_id)}
                onRegenerate={canRegenerate ? onRegenerate : undefined}
                onSwitchVersion={canSwitchVersion ? onSwitchVersion : undefined}
                versionIndex={versionIndex}
                versionCount={versionCount}
              />
            </div>
          </div>
        </AIMessage>
      )
    }

    case 'thinking':
      return <ThinkingBlock message={message} content={content} streaming={message.streaming === true} />

    case 'tool_call':
      if ((message.name || '') === 'generate_interactive_image') {
        return <InteractiveImageBlock message={message} projectId={projectId} onRegenerate={onGenerateInteractiveImage} />
      }
      if (['generate_image', 'generate_chapter_illustration'].includes(message.name || '') && message.illustration) {
        return <ChapterIllustrationBlock message={message} projectId={projectId} onInsert={onInsertIllustration} />
      }
      if ((message.name || '') === 'todo') {
        return <TodoListBlock message={message} />
      }
      if ((message.name || '') === 'ask') {
        if (message.ask?.kind === 'tool_approval') return <ToolApprovalCard message={message} onResolve={onResolveAsk} />
        return <AskInteractionCard message={message} onResolve={onResolveAsk} />
      }
      return <ToolExecutionBlock message={message} onResolve={onResolveAsk} onLayoutChange={onInteractiveCardLayoutChange} />

    case 'ask':
      if (message.ask?.kind === 'tool_approval') return <ToolApprovalCard message={message} onResolve={onResolveAsk} />
      return <AskInteractionCard message={message} onResolve={onResolveAsk} />

    case 'rule_roll':
      return <RuleRollBlock message={message} />

    case 'tool_result':
      if ((message.name || '') === 'generate_interactive_image' || message.interactive_image) {
        return <InteractiveImageBlock message={message} projectId={projectId} onRegenerate={onGenerateInteractiveImage} />
      }
      if (message.illustration) {
        return <ChapterIllustrationBlock message={message} projectId={projectId} onInsert={onInsertIllustration} />
      }
      return <ToolResultBlock content={content} />

    case 'context_compaction':
      return <ContextCompactionBlock message={message} />

    case 'plan_question':
      return <LegacyPlanQuestionBlock message={message} />

    case 'proposed_plan':
      return <ProposedPlanBlock projectId={projectId} message={message} highlightDialogue={highlightDialogue} onApprove={onApprovePlan} onContinue={onContinuePlan} onExit={onExitPlanMode} onLayoutChange={onInteractiveCardLayoutChange} />

    case 'system':
      if (!content.trim()) return null
      return (
        <div className="flex justify-center">
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1 text-xs text-[var(--nova-text-muted)]">
            {content}
          </span>
        </div>
      )

    case 'error':
      return (
        <div className="flex justify-center">
          <div className="nova-message-body-with-meta max-w-full">
            <div className="inline-flex max-w-full flex-wrap items-center justify-center gap-2 rounded-full border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-1 text-xs text-[var(--nova-danger)]">
              <span className="min-w-0 truncate">{content}</span>
              <TraceLinkButton runID={message.run_id} onOpenTrace={onOpenTrace} />
            </div>
            <MessageInlineMeta message={message} content={content} align="left" onRegenerate={canRegenerate ? onRegenerate : undefined} />
          </div>
        </div>
      )

    default:
      return null
  }
})

function SentMessageReferences({ references }: { references?: UserMessageReference[] }) {
  const { t } = useTranslation()
  if (!references?.length) return null
  return (
    <div data-testid="sent-message-references" className="mb-1.5 flex max-w-full flex-col gap-1 border-b border-current/10 pb-1.5 text-[11px] leading-4">
      {references.map((reference, index) => (
        <div key={`${reference.kind}:${reference.id || reference.label}:${index}`} className="flex min-w-0 items-start gap-1.5">
          <span className="shrink-0 rounded bg-black/10 px-1 py-0.5 text-[10px] opacity-75 dark:bg-white/10">
            {t(`chat.reference.${reference.kind}`)}
          </span>
          <span className="min-w-0 break-words">
            <span className="font-medium">{reference.label}{formatReferenceLines(reference)}</span>
            {reference.detail ? <span className="ml-1 opacity-75">— {reference.detail}</span> : null}
          </span>
        </div>
      ))}
    </div>
  )
}

function formatReferenceLines(reference: UserMessageReference): string {
  if (reference.start_line === undefined) return ''
  if (reference.end_line !== undefined && reference.end_line !== reference.start_line) return `:L${reference.start_line}-L${reference.end_line}`
  return `:L${reference.start_line}`
}

function TraceLinkButton({ runID, onOpenTrace }: { runID?: string; onOpenTrace?: (runID: string) => void }) {
  const { t } = useTranslation()
  if (!runID || !onOpenTrace) return null
  return (
    <button
      type="button"
      onClick={() => onOpenTrace(runID)}
      className="nova-nav-item inline-flex h-6 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] px-1.5 text-[10px] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
    >
      <Activity className="h-3 w-3" />
      {t('chat.tracePanel.viewTrace')}
    </button>
  )
}

function RuleRollBlock({ message }: { message: ChatMessage }) {
  const { t } = useTranslation()
  const roll = message.rule_roll
  if (!roll) return null
  const rolls = roll.rolls?.length ? roll.rolls.join(', ') : '-'
  const kept = Number.isFinite(roll.kept_roll) ? roll.kept_roll : undefined
  const bonus = Number.isFinite(roll.bonus_total) ? roll.bonus_total : undefined
  const total = Number.isFinite(roll.total) ? roll.total : undefined
  const target = Number.isFinite(roll.target) ? roll.target : undefined
  const cost = roll.cost || roll.stakes || ''
  const stateChanges = roll.state_changes || []
  return (
    <div className="flex justify-start">
      <div className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs shadow-[var(--nova-shadow)]">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Dice5 className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 truncate font-semibold text-[var(--nova-text)]">{roll.label || t('snapshot.ruleRoll.title')}</span>
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{roll.difficulty || t('snapshot.noRecord')}</span>
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{[roll.dice, roll.roll_mode].filter(Boolean).join(' ') || t('snapshot.noRecord')}</span>
          {roll.outcome ? <span className={`ml-auto shrink-0 font-semibold ${ruleRollOutcomeClass(roll.outcome)}`}>{roll.outcome}</span> : null}
        </div>
        <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-[var(--nova-text-muted)]">
          <span>{t('snapshot.field.rolls')}: {rolls}</span>
          {kept !== undefined ? <span>{t('snapshot.field.kept_roll')}: {formatRuleRollNumber(kept)}</span> : null}
          {bonus !== undefined ? <span>{t('snapshot.field.bonus_total')}: {formatSignedRuleRollNumber(bonus)}</span> : null}
          {total !== undefined || target !== undefined ? <span>{t('snapshot.ruleRoll.totalTarget', { total: total !== undefined ? formatRuleRollNumber(total) : '-', target: target !== undefined ? formatRuleRollNumber(target) : '-' })}</span> : null}
          {Number.isFinite(roll.base_target) ? <span>{t('snapshot.field.base_target')}: {formatRuleRollNumber(roll.base_target || 0)}</span> : null}
        </div>
        {roll.result ? <div className="mt-1.5 text-[var(--nova-text)]">{roll.result}</div> : null}
        {cost ? <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('snapshot.ruleRoll.cost')}: {cost}</div> : null}
        {stateChanges.length ? (
          <div className="mt-1 flex flex-wrap gap-1">
            {stateChanges.map((change, index) => (
              <span key={`${change.actor_id}:${change.field_id}:${index}`} className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">
                {change.actor_id} / {change.field_id} {formatSignedRuleRollNumber(change.change)}
              </span>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function ruleRollOutcomeClass(outcome: string) {
  if (outcome.includes('success')) return 'text-[var(--nova-success)]'
  if (outcome.includes('failure')) return 'text-[var(--nova-danger)]'
  return 'text-[var(--nova-text-muted)]'
}

function formatRuleRollNumber(value: number) {
  if (!Number.isFinite(value)) return '-'
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

function formatSignedRuleRollNumber(value: number) {
  if (!Number.isFinite(value)) return '-'
  const formatted = formatRuleRollNumber(value)
  return value > 0 ? `+${formatted}` : formatted
}

function MessageInlineMeta({ message, content, align, reserveSpace = false, hideActions = false, onEdit, editLabelKey = 'chat.action.editTurn', onCreateBranch, onGenerateInteractiveImage, generatingInteractiveImage = false, onRegenerate, onSwitchVersion, versionIndex = -1, versionCount = 0 }: { message: ChatMessage; content: string; align: 'left' | 'right'; reserveSpace?: boolean; hideActions?: boolean; onEdit?: (message: ChatMessage) => void; editLabelKey?: 'chat.action.editTurn' | 'chat.action.editAssistantReply'; onCreateBranch?: (message: ChatMessage) => void; onGenerateInteractiveImage?: (message: ChatMessage) => void; generatingInteractiveImage?: boolean; onRegenerate?: (message: ChatMessage) => void; onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void; versionIndex?: number; versionCount?: number }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const formatted = formatMessageHoverTime(message.created_at)
  const canSwitchVersion = Boolean(onSwitchVersion && versionCount > 1 && versionIndex >= 0)
  const hasMessageAction = !hideActions && Boolean(onEdit || onCreateBranch || onGenerateInteractiveImage || onRegenerate || canSwitchVersion)
  const showCopyAction = !hideActions && Boolean(content.trim())
  const metaTooltip = {
    tooltipSide: 'top' as const,
    tooltipSideOffset: messageActionTooltipSideOffset,
    useTooltipProvider: false,
  }
  if (!formatted && !showCopyAction && !hasMessageAction) {
    if (!reserveSpace) return null
    return (
      <div className={`nova-message-meta nova-message-meta-${align} nova-message-meta-spacer`} aria-hidden="true">
        <span />
      </div>
    )
  }
  return (
    <TooltipProvider delayDuration={messageActionTooltipDelayMs} skipDelayDuration={messageActionTooltipSkipDelayMs} disableHoverableContent>
      <div className={`nova-message-meta nova-message-meta-${align}`} aria-label={formatted}>
        {formatted ? <span className="nova-message-time">{formatted}</span> : null}
        {showCopyAction && (
          <TooltipIconButton
            label={copied ? t('chat.action.copyMessageDone') : t('chat.action.copyMessage')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              setCopied(true)
              window.setTimeout(() => setCopied(false), copyFeedbackDurationMs)
              void copyText(content)
            }}
          >
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          </TooltipIconButton>
        )}
        {onEdit && (
          <TooltipIconButton
            label={t(editLabelKey)}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              onEdit(message)
            }}
          >
            <Pencil className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {onCreateBranch && (
          <TooltipIconButton
            label={t('chat.action.createBranch')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              onCreateBranch(message)
            }}
          >
            <GitBranch className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {onGenerateInteractiveImage && (
          <TooltipIconButton
            label={message.interactive_images?.length || message.interactive_image ? t('chat.interactiveImage.regenerate') : t('chat.action.generateInteractiveImage')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-45"
            disabled={generatingInteractiveImage}
            onClick={(event) => {
              event.stopPropagation()
              onGenerateInteractiveImage(message)
            }}
          >
            {generatingInteractiveImage ? <Loader2 className="h-3 w-3 animate-spin" /> : <ImagePlus className="h-3 w-3" />}
          </TooltipIconButton>
        )}
        {onRegenerate && (
          <TooltipIconButton
            label={t('chat.action.regenerateTurn')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              onRegenerate(message)
            }}
          >
            <RefreshCw className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {canSwitchVersion && onSwitchVersion && (
          <>
            <TooltipIconButton
              label={t('chat.action.prevVersion')}
              {...metaTooltip}
              className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-30"
              disabled={versionIndex <= 0}
              onClick={(event) => {
                event.stopPropagation()
                onSwitchVersion(message, -1)
              }}
            >
              <ChevronLeft className="h-3 w-3" />
            </TooltipIconButton>
            <span className="min-w-7 text-center font-mono text-[10px] leading-5 text-[var(--nova-text-faint)]">
              {versionIndex + 1}/{versionCount}
            </span>
            <TooltipIconButton
              label={t('chat.action.nextVersion')}
              {...metaTooltip}
              className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-30"
              disabled={versionIndex >= versionCount - 1}
              onClick={(event) => {
                event.stopPropagation()
                onSwitchVersion(message, 1)
              }}
            >
              <ChevronRight className="h-3 w-3" />
            </TooltipIconButton>
          </>
        )}
      </div>
    </TooltipProvider>
  )
}

function formatMessageHoverTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const time = `${padTime(date.getHours())}:${padTime(date.getMinutes())}`
  const now = new Date()
  const sameDay = date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()
  if (sameDay) return time
  return `${date.getFullYear()}-${padTime(date.getMonth() + 1)}-${padTime(date.getDate())} ${time}`
}

function padTime(value: number) {
  return value.toString().padStart(2, '0')
}

async function copyText(content: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(content)
      return true
    } catch {
      // Fall through to the legacy path for embedded/local browser surfaces.
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = content
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

function SubAgentOutputWindow({
  message,
  content,
  highlightDialogue,
  messageStyle,
  projectId,
  onOpen,
  active,
}: {
  message: ChatMessage
  content: string
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  projectId: string
  onOpen?: (message: ChatMessage) => void
  active?: boolean
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const name = message.agent_name || message.subagent_type || t('chat.subagent.label')
  const preview = buildMarkdownPreview(content, 220)
  const hasContent = Boolean(content.trim())
  const statusLabel = message.streaming ? t('chat.subagent.status.streaming') : t('chat.subagent.status.done')
  const detailMode = Boolean(onOpen)
  const actionLabel = detailMode ? t('chat.subagent.openSession') : (expanded ? t('chat.subagent.collapse') : t('chat.subagent.expand'))
  const shownContent = detailMode || !expanded ? preview : content
  const shownTargetContent = message.streaming_target_content && shownContent === content ? message.streaming_target_content : undefined
  const contentScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: message.streaming === true,
    resetKey: `${message.id || message.created_at || name}:subagent-output`,
    contentKey: `${message.streaming ? 'streaming' : 'idle'}:${detailMode ? 'detail' : 'inline'}:${expanded ? 'expanded' : 'collapsed'}:${shownContent.length}`,
  })

  return (
    <div className="flex justify-start">
      <div className={`w-full overflow-hidden rounded-lg border bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)] ${active ? 'border-[var(--nova-accent)] ring-1 ring-[var(--nova-accent)]/40' : 'border-[var(--nova-border)]'}`}>
        <button
          type="button"
          className="flex min-h-10 w-full min-w-0 items-center gap-2 px-3 py-2 text-left"
          onClick={() => {
            if (onOpen) {
              onOpen(message)
              return
            }
            setExpanded(!expanded)
          }}
          aria-expanded={expanded}
          aria-label={t('chat.subagent.outputFrom', { name })}
        >
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            {detailMode ? <PanelRightOpen className="h-3.5 w-3.5" /> : expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium text-[var(--nova-text)]">{t('chat.subagent.outputFrom', { name })}</span>
            <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{statusLabel}</span>
          </span>
          <span className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">
            {actionLabel}
          </span>
        </button>
        <div
          ref={contentScrollLock.ref}
          onScroll={contentScrollLock.onScroll}
          onWheel={contentScrollLock.onWheel}
          onKeyDown={contentScrollLock.onKeyDown}
          data-nova-scroll-lock="subagent-output"
          className={`${detailMode ? 'max-h-28' : expanded ? 'max-h-96' : 'max-h-28'} overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 [overflow-anchor:none]`}
        >
          {hasContent ? (
            <div className="chat-agent-message text-sm text-[var(--nova-text)]" style={messageStyle}>
              <StreamingContentStage content={shownContent} targetContent={shownTargetContent} streaming={message.streaming === true}>
                {(value) => <MarkdownContent content={value} highlightDialogue={highlightDialogue} projectId={projectId} />}
              </StreamingContentStage>
            </div>
          ) : (
            <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.subagent.empty')}</div>
          )}
        </div>
      </div>
    </div>
  )
}

function AgentSourceBadge({ message, compact = false }: { message: ChatMessage; compact?: boolean }) {
  const { t } = useTranslation()
  const name = message.agent_name || message.subagent_type || t('chat.subagent.label')
  const label = compact ? name : t('chat.subagent.outputFrom', { name })
  return (
    <span className={`mb-1 inline-flex max-w-full items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)] ${compact ? 'mb-0 min-w-0' : ''}`}>
      <span className="truncate">{label}</span>
    </span>
  )
}

/** Agent 运行中、尚无具体消息时的轻量活动提示。 */
export function AgentActivityShimmer({ content }: { content: string }) {
  return (
    <div className="flex justify-start px-1 py-1" role="status" aria-live="polite">
      <Shimmer as="span" className="text-sm font-medium">{content}</Shimmer>
    </div>
  )
}

function ContextCompactionBlock({ message }: { message: ChatMessage }) {
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

/** Keeps previously persisted plan-question cards readable after interactive
 * clarification moved to the durable Ask tool. Legacy cards are intentionally
 * display-only: there is no live waiter that could safely accept an answer. */
function LegacyPlanQuestionBlock({ message }: { message: ChatMessage }) {
  const { t } = useTranslation()
  const questionSet = parsePlanQuestionSet(message.content || '')
  const fallback = planDisplayContent(message.content || '')

  if (!questionSet) {
    return (
      <PlanShell icon={<ClipboardList className="h-3.5 w-3.5" />} title={t('chat.plan.questionTitle')} badge={t('chat.plan.legacyBadge')}>
        <div className="space-y-2 text-xs leading-relaxed text-[var(--nova-text-muted)]">
          <p>{t('chat.plan.legacyDescription')}</p>
          {fallback && <div className="min-w-0 max-w-full max-h-72 overflow-x-hidden overflow-y-auto whitespace-pre-wrap [overflow-wrap:anywhere]">{fallback}</div>}
        </div>
      </PlanShell>
    )
  }

  return (
    <PlanShell icon={<ClipboardList className="h-3.5 w-3.5" />} title={t('chat.plan.questionTitle')} badge={t('chat.plan.legacyBadge')}>
      <div className="max-h-[min(620px,calc(100vh-220px))] space-y-3 overflow-y-auto overscroll-contain pr-1 [scrollbar-gutter:stable]">
        <p className="text-xs leading-5 text-[var(--nova-text-muted)]">{t('chat.plan.legacyDescription')}</p>
        {questionSet.questions.map((question) => (
          <section key={question.id} className="rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2.5">
            <div className="flex min-w-0 items-start gap-2">
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium leading-5 text-[var(--nova-text)]">{question.question}</div>
                {question.description && <div className="mt-1 text-xs leading-5 text-[var(--nova-text-muted)]">{question.description}</div>}
              </div>
              <span className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
                {question.type === 'multi' ? t('chat.plan.multiChoice') : t('chat.plan.singleChoice')}
              </span>
            </div>
            <div className="mt-2 space-y-1.5">
              {question.options.map((option) => (
                <div key={option.id} className="rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[var(--nova-text-muted)]">
                  <span className="flex flex-wrap items-center gap-1.5 text-xs font-medium text-[var(--nova-text)]">
                    {option.label}
                    {option.recommended && <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('chat.plan.recommended')}</span>}
                  </span>
                  {option.description && <span className="mt-0.5 block text-[11px] leading-4 text-[var(--nova-text-faint)]">{option.description}</span>}
                </div>
              ))}
            </div>
          </section>
        ))}
      </div>
    </PlanShell>
  )
}

function ProposedPlanBlock({ projectId, message, highlightDialogue, onApprove, onContinue, onExit, onLayoutChange }: { projectId: string; message: ChatMessage; highlightDialogue?: boolean; onApprove?: (message: ChatMessage) => void; onContinue?: (message: ChatMessage) => void; onExit?: () => void; onLayoutChange?: () => void }) {
  const { t } = useTranslation()
  const display = planDisplayContent(message.content || '')
  const [localAction, setLocalAction] = useState<ChatMessage['plan_action']>(message.plan_action)
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
          <MarkdownContent content={display} highlightDialogue={highlightDialogue === true} projectId={projectId} />
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

function planActionStatusText(t: ReturnType<typeof useTranslation>['t'], action: ChatMessage['plan_action']) {
  switch (action) {
    case 'approved':
      return t('chat.plan.approvedStatus')
    case 'continue':
      return t('chat.plan.continueStatus')
    case 'exited':
      return t('chat.plan.exitedStatus')
    case 'answered':
      return t('chat.plan.answerSubmittedStatus')
    default:
      return ''
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

/** 工具执行卡片，默认以单行展示运行态和结果态。 */
function ToolExecutionBlock({ message, onResolve, onLayoutChange }: { message: ChatMessage; onResolve?: AskInteractionResolver; onLayoutChange?: (element: HTMLElement) => void }) {
  const { t } = useTranslation()
  const approvalInteraction = message.ask?.kind === 'tool_approval' ? message.ask : undefined
  const approvalPending = approvalInteraction?.status === 'pending'
  const [expanded, setExpanded] = useState(() => approvalPending)
  const info = parseToolCallContent(message.content || '')
  const name = message.name || info.name
  const rawArgs = message.args !== undefined ? message.args : info.args
  const args = formatMaybeJSON(rawArgs)
  const status = message.status || 'running'
  const result = message.result || ''
  const isDelegationTool = name === 'task'
  const taskSubAgent = isDelegationTool ? (message.subagent_type || parseTaskSubagentType(rawArgs)) : ''
  const isChapterBodyHidden = message.sse_display_notice === 'chapter_body_hidden'
  const isDirectorPlanHidden = isChapterBodyHidden && message.agent_kind === 'interactive_director'
  const chapterBodyHiddenPath = isChapterBodyHidden ? extractToolArgPath(rawArgs) : ''
  const chapterGeneratedChars = isChapterBodyHidden && typeof message.sse_generated_chars === 'number' ? message.sse_generated_chars : undefined
  const displayName = isDelegationTool ? t('chat.subagent.taskLabel') : name
  const detailArgs = isDelegationTool ? formatTaskDelegationArgs(rawArgs) : (isChapterBodyHidden ? '' : args)
  const hasResult = status === 'success'
  useLayoutEffect(() => {
    if (approvalPending) setExpanded(true)
  }, [approvalPending, approvalInteraction?.id])
  const isStreamingContent = !approvalInteraction && !isChapterBodyHidden && status === 'running' && isContentTool(name) && rawArgs.length > 50
  const streamPreview = isStreamingContent ? extractStreamingContent(rawArgs) : ''
  // 内容工具运行中但不展示流式预览时（流式为 off / 参数较短），展示"正在写入文件"的 Loading 文案
  const isContentToolLoading = !isChapterBodyHidden && !isStreamingContent && status === 'running' && isContentTool(name)
  const contentToolChars = isContentToolLoading && typeof message.sse_generated_chars === 'number' ? message.sse_generated_chars : undefined
  const summary = taskSubAgent
    ? t('chat.subagent.delegating', { name: taskSubAgent })
    : buildToolArgSummary(args) || (isStreamingContent ? t('chat.tool.writing') : t('chat.tool.preparing'))
  const resultBody = stripToolResultMetadata(result)
  const resultEnvelope = decodeToolResultEnvelope(resultBody)
  const resultSeverity = status === 'error' ? 'error' : resultEnvelope?.severity || 'success'
  const showReadableOutcome = resultSeverity !== 'success'
  const resultPreview = resultEnvelope
    ? buildToolResultEnvelopeSummary(t, resultEnvelope)
    : buildPreview(resultBody, 80)
  const detailResult = resultEnvelope ? formatMaybeJSON(resultBody) : result
  const displaySummary = isChapterBodyHidden
    ? chapterGeneratedChars !== undefined
      ? t(isDirectorPlanHidden ? (hasResult ? 'chat.tool.fileWrittenWithCount' : 'chat.tool.fileWritingWithCount') : (hasResult ? 'chat.tool.chapterWrittenWithCount' : 'chat.tool.chapterWritingWithCount'), { count: chapterGeneratedChars })
      : (isDirectorPlanHidden ? (hasResult ? t('chat.tool.fileWritten') : t('chat.tool.fileWriting')) : (hasResult ? t('chat.tool.chapterWritten') : t('chat.tool.chapterWriting')))
    : (hasResult
      ? resultPreview || t('chat.tool.done')
      : status === 'error'
        ? buildPreview(resultBody, 160) || t('chat.tool.failed')
      : isContentToolLoading
        ? (contentToolChars !== undefined ? t('chat.tool.fileWritingWithCount', { count: contentToolChars }) : t('chat.tool.fileWriting'))
        : summary)
  const headerSummary = approvalPending ? t('agentApproval.approval.waiting') : displaySummary
  const hasDetail = Boolean(approvalInteraction || detailArgs || result || isChapterBodyHidden)
  const streamPreviewScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: isStreamingContent,
    resetKey: `${message.id || name}:tool-stream-preview`,
    contentKey: `${status}:${rawArgs.length}:${streamPreview.length}`,
  })

  return (
    <div className="flex justify-start">
      <Tool open={expanded} onOpenChange={setExpanded} className="mb-0 w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div
          data-nova-tool-header
          className={`grid min-h-10 min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 px-3 py-2 ${showReadableOutcome ? 'gap-y-1' : ''}`}
        >
          <ToolStatusIcon status={resultSeverity === 'error' ? 'error' : status} warning={resultSeverity === 'warning'} />
          <div className="flex min-w-0 items-center gap-2 overflow-hidden">
            <span className="shrink-0 font-medium text-[var(--nova-text)]">{t('chat.tool.calling')}</span>
            <code
              className="min-w-0 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--nova-text-muted)]"
              title={displayName}
            >
              {displayName}
            </code>
            {taskSubAgent && (
              <span
                className="min-w-0 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]"
                title={t('chat.subagent.delegating', { name: taskSubAgent })}
              >
                {t('chat.subagent.delegating', { name: taskSubAgent })}
              </span>
            )}
            {message.subagent && <AgentSourceBadge message={message} compact />}
            {approvalPending && (
              <span className="shrink-0 rounded-full border border-amber-500/25 bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-300">
                {t('agentApproval.approval.waiting')}
              </span>
            )}
            {!showReadableOutcome && (
              <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]" title={headerSummary}>
                {headerSummary}
              </span>
            )}
          </div>
          {hasDetail && !isStreamingContent && (
            <button
              type="button"
              className="col-start-3 row-start-1 shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[var(--nova-text-muted)] transition hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? t('chat.tool.collapse') : t('chat.tool.details')}
            </button>
          )}
          {showReadableOutcome && (
            <span className={`col-start-2 col-end-4 whitespace-normal pt-1 leading-4 ${resultSeverity === 'warning' ? 'text-[var(--nova-warning)]' : 'text-[var(--nova-danger)]'}`}>
              {displaySummary}
            </span>
          )}
        </div>
        {/* 流式写入时展示实时内容预览 */}
        {isStreamingContent && streamPreview && (
          <div
            ref={streamPreviewScrollLock.ref}
            onScroll={streamPreviewScrollLock.onScroll}
            onWheel={streamPreviewScrollLock.onWheel}
            onKeyDown={streamPreviewScrollLock.onKeyDown}
            data-nova-scroll-lock="tool-stream-preview"
            className="min-w-0 max-w-full max-h-32 overflow-x-hidden overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-accent-green)] whitespace-pre-wrap [overflow-anchor:none] [overflow-wrap:anywhere]"
          >
            {streamPreview}
          </div>
        )}
        {!isStreamingContent && (
          <ToolContent className={`grid min-w-0 max-w-full gap-2 overflow-x-hidden overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)] ${approvalInteraction ? 'max-h-80' : 'max-h-48'}`}>
            {approvalInteraction && <ToolApprovalPanel message={message} onResolve={onResolve} embedded onLayoutChange={onLayoutChange} />}
            {isChapterBodyHidden && (
              <div className="grid gap-1 font-sans">
                {chapterBodyHiddenPath && (
                  <div className="min-w-0">
                    <span className="text-[var(--nova-text-faint)]">{t(isDirectorPlanHidden ? 'chat.tool.filePath' : 'chat.tool.chapterPath')}</span>
                    <code className="ml-1 break-all font-mono text-[var(--nova-text-muted)]">{chapterBodyHiddenPath}</code>
                  </div>
                )}
                {chapterGeneratedChars !== undefined && (
                  <div className="text-[var(--nova-text-faint)]">
                    {t(isDirectorPlanHidden ? 'chat.tool.fileGeneratedChars' : 'chat.tool.chapterGeneratedChars', { count: chapterGeneratedChars })}
                  </div>
                )}
                <div className="text-[var(--nova-text-faint)]">{t(isDirectorPlanHidden ? 'chat.tool.fileBodyHidden' : 'chat.tool.chapterBodyHidden')}</div>
              </div>
            )}
            {detailArgs && !approvalInteraction?.approval?.command && <pre className="m-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]">{detailArgs}</pre>}
            {taskSubAgent && result && <div className="text-[var(--nova-text-muted)]">{t('chat.subagent.result')}</div>}
            {result && <pre className={`m-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere] ${resultSeverity === 'error' ? 'text-[var(--nova-danger)]' : resultSeverity === 'warning' ? 'text-[var(--nova-warning)]' : 'text-[var(--nova-accent-green)]'}`}>{detailResult}</pre>}
          </ToolContent>
        )}
      </Tool>
    </div>
  )
}

function ChapterIllustrationBlock({ projectId, message, onInsert }: { projectId: string; message: ChatMessage; onInsert?: (illustration: ChapterIllustration) => void }) {
  const { t } = useTranslation()
  const illustration = message.illustration
  if (!illustration) return <ToolExecutionBlock message={message} />

  const status = message.status || 'running'
  const isMarkdownChapter = isMarkdownPath(illustration.chapter_path)
  const canInsert = status === 'success' && isMarkdownChapter && Boolean(onInsert)
  const imageSrc = chatAssetURL(projectId, illustration.image_path)
  const imageTitle = illustration.alt_text || t('chat.illustration.previewAlt')

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <ImagePreviewDialog src={imageSrc} title={imageTitle} alt={imageTitle} path={illustration.image_path}>
          <button
            type="button"
            className="group relative block w-full overflow-hidden bg-black/90 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]"
            aria-label={t('chat.illustration.openPreview')}
          >
            <img
              src={imageSrc}
              alt={imageTitle}
              className="max-h-80 w-full object-contain"
              loading="lazy"
            />
            <span className="absolute left-3 top-3 inline-flex items-center gap-1.5 rounded-md border border-white/15 bg-black/45 px-2 py-1 text-[11px] font-medium text-white opacity-90 backdrop-blur">
              <ToolStatusIcon status={status} />
              {t('chat.illustration.title')}
            </span>
          </button>
        </ImagePreviewDialog>
        <div className="flex min-w-0 flex-col gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 sm:flex-row sm:items-center">
          <code className="min-w-0 flex-1 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 font-mono text-[10px] text-[var(--nova-text-muted)]" title={illustration.image_path}>
            {illustration.image_path}
          </code>
          <div className="flex min-w-0 items-center justify-end gap-2">
            {!isMarkdownChapter && (
              <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{t('chat.illustration.markdownOnly')}</span>
            )}
            <button
              type="button"
              disabled={!canInsert}
              onClick={() => illustration && onInsert?.(illustration)}
              className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] font-medium text-[var(--nova-text-muted)] transition hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-45"
            >
              <ImagePlus className="h-3.5 w-3.5" />
              {status === 'running' ? t('chat.illustration.generating') : t('chat.illustration.insert')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function InteractiveImageBlock({ projectId, message }: { projectId: string; message: ChatMessage; onRegenerate?: (message: ChatMessage) => void }) {
  return (
    <div className="flex justify-start">
      <div className="w-full">
        <InteractiveImageStrip message={message} projectId={projectId} />
      </div>
    </div>
  )
}

function InteractiveImageStrip({ message, projectId }: { message: ChatMessage; projectId: string }) {
  const { t } = useTranslation()
  const images = interactiveImagesFromMessage(message)
  const error = message.interactive_image_error || readInteractiveImageErrorFromMessage(message)
  const status = message.interactive_image_status || message.status
  const [index, setIndex] = useState(Math.max(0, images.length - 1))
  const previousImageCountRef = useRef(images.length)

  useEffect(() => {
    const previousLength = previousImageCountRef.current
    previousImageCountRef.current = images.length
    setIndex((current) => {
      if (images.length > previousLength) return images.length - 1
      return Math.min(Math.max(0, images.length - 1), Math.max(0, current))
    })
  }, [images.length])

  if (images.length === 0) {
    if (status === 'running') {
      return (
        <div className="mt-3 flex items-center gap-2 px-1 text-xs text-[var(--nova-text-faint)]">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          <span>{t('chat.interactiveImage.generating')}</span>
        </div>
      )
    }
    if (error) {
      return (
        <div className="mt-3 rounded-md border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">
          {error.message || t('chat.interactiveImage.failed')}
        </div>
      )
    }
    return null
  }

  const safeIndex = Math.min(index, images.length - 1)
  const image = images[safeIndex]
  const title = image.alt_text || t('chat.interactiveImage.previewAlt')
  const src = chatAssetURL(projectId, image.image_path)
  const canSwitch = images.length > 1

  return (
    <div className="mt-3 max-w-full">
      <ImagePreviewDialog src={src} title={title} alt={title} path={image.image_path}>
        <div
          role="button"
          tabIndex={0}
          className="group relative block w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-black/90 text-left shadow-[var(--nova-shadow)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]"
          aria-label={t('chat.interactiveImage.openPreview')}
        >
          <img
            src={src}
            alt={title}
            className="max-h-[440px] w-full object-contain"
            loading="lazy"
          />
          {canSwitch && (
            <span className="absolute bottom-2 right-2 inline-flex items-center gap-1 rounded-md border border-white/10 bg-black/35 px-1 py-0.5 text-[10px] text-white/70 opacity-45 backdrop-blur transition group-hover:opacity-90">
              <button
                type="button"
                aria-label={t('chat.interactiveImage.prevVersion')}
                className={`flex h-5 w-5 items-center justify-center rounded border border-transparent ${safeIndex <= 0 ? 'opacity-30' : 'hover:border-white/15 hover:bg-white/10'}`}
                disabled={safeIndex <= 0}
                onClick={(event) => {
                  event.preventDefault()
                  event.stopPropagation()
                  setIndex((current) => Math.max(0, current - 1))
                }}
              >
                <ChevronLeft className="h-3 w-3" />
              </button>
              <span className="min-w-7 text-center font-mono leading-5">{safeIndex + 1}/{images.length}</span>
              <button
                type="button"
                aria-label={t('chat.interactiveImage.nextVersion')}
                className={`flex h-5 w-5 items-center justify-center rounded border border-transparent ${safeIndex >= images.length - 1 ? 'opacity-30' : 'hover:border-white/15 hover:bg-white/10'}`}
                disabled={safeIndex >= images.length - 1}
                onClick={(event) => {
                  event.preventDefault()
                  event.stopPropagation()
                  setIndex((current) => Math.min(images.length - 1, current + 1))
                }}
              >
                <ChevronRight className="h-3 w-3" />
              </button>
            </span>
          )}
        </div>
      </ImagePreviewDialog>
    </div>
  )
}

function interactiveImagesFromMessage(message: ChatMessage): InteractiveImage[] {
  if (message.interactive_images?.length) return message.interactive_images.filter((image) => Boolean(image.image_path))
  const image = message.interactive_image?.image_path ? message.interactive_image : readInteractiveImageFromMessage(message)
  return image?.image_path ? [image] : []
}

function readInteractiveImageFromMessage(message: ChatMessage): InteractiveImage | undefined {
  if (message.interactive_image?.image_path) return message.interactive_image
  const data = parseMessageResult(message.result)
  if (isInteractiveImage(data)) return data
  return undefined
}

function readInteractiveImageErrorFromMessage(message: ChatMessage): InteractiveImageError | undefined {
  if (message.interactive_image_error) return message.interactive_image_error
  const data = parseMessageResult(message.result)
  if (isInteractiveImageError(data)) return data
  return undefined
}

function parseMessageResult(result?: string): unknown {
  if (!result) return null
  try {
    return JSON.parse(result)
  } catch {
    return null
  }
}

function isInteractiveImage(value: unknown): value is InteractiveImage {
  if (!value || typeof value !== 'object') return false
  const data = value as Record<string, unknown>
  return data.schema === 'interactive_image.v1' && typeof data.image_path === 'string' && Boolean(data.image_path)
}

function isInteractiveImageError(value: unknown): value is InteractiveImageError {
  if (!value || typeof value !== 'object') return false
  const data = value as Record<string, unknown>
  return data.schema === 'interactive_image_error.v1'
}

function isMarkdownPath(path?: string) {
  return /\.(md|markdown)$/i.test(path || '')
}

interface TodoItem {
  step: string
  status: 'pending' | 'in_progress' | 'completed' | string
}

/** todo 工具卡片：运行时容错解析流式参数，完成后以结构化结果为真源。 */
function TodoListBlock({ message }: { message: ChatMessage }) {
  const { t } = useTranslation()
  const args = message.args || ''
  const status = message.status || 'running'
  const resultPlan = status === 'success' ? parseTodoPlanResult(stripToolResultMetadata(message.result || '')) : null
  const todos = resultPlan ?? parseTodoPlanFromArgs(args)
  const total = todos.length
  const completed = todos.filter(t => t.status === 'completed').length
  const inProgress = todos.find(t => t.status === 'in_progress')
  const headline = inProgress?.step || (status === 'success' ? t('chat.todo.updated') : t('chat.todo.updating'))

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div className="flex min-h-10 min-w-0 items-center gap-2 px-3 py-2">
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            <ListTodo className="h-3.5 w-3.5" />
          </span>
          <span className="shrink-0 font-medium text-[var(--nova-text)]">{t('chat.todo.list')}</span>
          {total > 0 && (
            <span className="shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--nova-text-faint)]">
              {completed}/{total}
            </span>
          )}
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{headline}</span>
        </div>
        {todos.length > 0 && (
          <ul className="grid gap-1 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5">
            {todos.map((todo, index) => (
              <TodoListItem key={index} todo={todo} />
            ))}
          </ul>
        )}
        {todos.length === 0 && (
          <div className="border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 text-[var(--nova-text-faint)]">
            {status === 'running' ? t('chat.todo.parsing') : t('chat.todo.empty')}
          </div>
        )}
      </div>
    </div>
  )
}

function TodoListItem({ todo }: { todo: TodoItem }) {
  const text = todo.step
  if (todo.status === 'completed') {
    return (
      <li className="flex items-start gap-2 rounded-md px-2 py-1.5 leading-5">
        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-green)]" />
        <span className="text-[var(--nova-text-faint)] line-through">{text}</span>
      </li>
    )
  }
  if (todo.status === 'in_progress') {
    return (
      <li className="flex items-start gap-2 rounded-md border border-[var(--nova-border)] bg-[var(--nova-hover)] px-2 py-1.5 leading-5">
        <CircleDot className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-pulse text-[var(--nova-text)]" />
        <span className="text-[var(--nova-text)]">{text}</span>
      </li>
    )
  }
  return (
    <li className="flex items-start gap-2 rounded-md px-2 py-1.5 leading-5">
      <Circle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
      <span className="text-[var(--nova-text-muted)]">{text}</span>
    </li>
  )
}

function parseTodoPlanResult(result: string): TodoItem[] | null {
  if (!result) return null
  try {
    const data = JSON.parse(result) as { schema?: string; plan?: TodoItem[] }
    if (data.schema === 'todo.plan.v1' && Array.isArray(data.plan)) return data.plan
  } catch {
    // 非结构化或不完整结果不作为成功状态真源。
  }
  return null
}

/** 解析 todo 参数，对流式中可能不完整的 JSON 做容错。 */
function parseTodoPlanFromArgs(args: string): TodoItem[] {
  if (!args) return []
  const trimmed = args.trim()
  if (!trimmed) return []
  // 优先尝试完整 JSON
  try {
    const data = JSON.parse(trimmed) as { plan?: TodoItem[] }
    if (Array.isArray(data?.plan)) return data.plan
  } catch {
    // 流式中常见：args 不完整或被截断
  }
  // 回退：从 plan 数组中提取已经完整的对象
  const arrayMatch = trimmed.match(/"plan"\s*:\s*\[([\s\S]*)$/)
  if (!arrayMatch) return []
  const body = arrayMatch[1]
  const items: TodoItem[] = []
  let depth = 0
  let start = -1
  let inString = false
  let escape = false
  for (let i = 0; i < body.length; i++) {
    const ch = body[i]
    if (escape) { escape = false; continue }
    if (ch === '\\') { escape = true; continue }
    if (ch === '"') { inString = !inString; continue }
    if (inString) continue
    if (ch === '{') {
      if (depth === 0) start = i
      depth++
    } else if (ch === '}') {
      depth--
      if (depth === 0 && start >= 0) {
        const piece = body.slice(start, i + 1)
        try {
          items.push(JSON.parse(piece) as TodoItem)
        } catch {
          // 单个对象解析失败时跳过
        }
        start = -1
      }
    }
  }
  return items
}

function ToolStatusIcon({ status, warning = false }: { status: ChatMessage['status']; warning?: boolean }) {
  if (status === 'error') {
    return <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[10px] text-[var(--nova-danger)]">!</span>
  }
  if (warning) {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-warning)]/40 bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]">
        <AlertTriangle className="h-3.5 w-3.5" />
      </span>
    )
  }
  if (status === 'success') {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-accent-green)]/45 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]">
        <CheckCircle2 className="h-3.5 w-3.5" />
      </span>
    )
  }
  return <span className="h-5 w-5 shrink-0 animate-spin rounded-full border-2 border-[var(--nova-border)] border-t-[var(--nova-text)]" />
}

/** 工具结果卡片，默认展示摘要，避免大段结果挤占对话区 */
function ToolResultBlock({ content }: { content: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const envelope = decodeToolResultEnvelope(stripToolResultMetadata(content))
  const severity = envelope?.severity || 'success'
  const preview = envelope ? buildToolResultEnvelopeSummary(t, envelope) : buildPreview(content, 160)
  const canExpand = content.trim().replace(/\s+/g, ' ').length > 160
  const isProcessExitWarning = severity === 'warning'
    && envelope?.schema === 'process.result.v1'
    && envelope.status === 'failed'
  const tone = severity === 'error'
    ? 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'
    : severity === 'warning'
      ? 'border-[var(--nova-warning)]/40 bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]'
      : 'border-[var(--nova-accent-green)]/35 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]'

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div className="flex items-start gap-3 px-3 py-2.5">
          <span className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border ${tone}`}>
            {severity === 'error'
              ? <span className="text-xs font-semibold">!</span>
              : severity === 'warning'
                ? <AlertTriangle className="h-3.5 w-3.5" />
                : <CheckCircle2 className="h-3.5 w-3.5" />}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-[var(--nova-text)]">
                {t(severity === 'error'
                  ? 'chat.tool.resultFailed'
                  : isProcessExitWarning
                    ? 'chat.tool.resultAttention'
                    : severity === 'warning'
                      ? 'chat.tool.resultPartial'
                      : 'chat.tool.resultDone')}
              </span>
              <span className={`rounded-full border px-2 py-0.5 text-[11px] ${tone}`}>
                {severity}
              </span>
            </div>
            <div className="mt-1 flex min-w-0 items-center gap-2 text-[var(--nova-text-faint)]">
              <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{preview || t('chat.tool.noReturn')}</span>
              {canExpand && (
                <button
                  type="button"
                  className="shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[var(--nova-text-muted)] transition hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                  onClick={() => setExpanded(!expanded)}
                >
                  {expanded ? t('chat.tool.collapse') : t('chat.tool.expand')}
                </button>
              )}
            </div>
          </div>
        </div>
        {expanded && (
          <pre className="m-0 min-w-0 max-w-full max-h-56 overflow-x-hidden overflow-y-auto whitespace-pre-wrap border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">
            {content}
          </pre>
        )}
      </div>
    </div>
  )
}

function parseToolCallContent(content: string) {
  const [rawName = 'unknown_tool', ...rest] = content.split('\n')
  const name = rawName.trim() || 'unknown_tool'
  const args = formatMaybeJSON(rest.join('\n').trim())

  return {
    name,
    args,
    summary: buildToolArgSummary(args),
  }
}

function parseTaskSubagentType(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    return typeof data.subagent_type === 'string' ? data.subagent_type : ''
  } catch {
    const match = args.match(/"subagent_type"\s*:\s*"([^"]+)"/)
    return match?.[1] || ''
  }
}

function formatTaskDelegationArgs(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    delete data.subagent_type
    return Object.keys(data).length > 0 ? formatMaybeJSON(JSON.stringify(data)) : ''
  } catch {
    return formatMaybeJSON(args.replace(/"subagent_type"\s*:\s*"[^"]+"\s*,?\s*/g, '').replace(/,\s*}/g, '}'))
  }
}

function stripToolResultMetadata(result: string) {
  for (const separator of ['\n\n[Denova tool result metadata]', '\n[Denova tool result metadata]']) {
    const markerIndex = result.lastIndexOf(separator)
    if (markerIndex >= 0) return result.slice(0, markerIndex).trimEnd()
  }
  return result
}

function buildToolResultEnvelopeSummary(t: ReturnType<typeof useTranslation>['t'], result: ToolResultEnvelope) {
  const isWebAccess = result.schema === 'web_fetch.v1' || result.schema === 'web_search.v1'
  const webStatusKey = isWebAccess ? {
    blocked: 'chat.tool.webAccess.blocked',
    no_results: 'chat.tool.webAccess.noResults',
    providers_unavailable: 'chat.tool.webAccess.providersUnavailable',
  }[result.status] : undefined
  const webRetryKey = isWebAccess && result.retryStrategy ? {
    change_query: 'chat.tool.webAccess.changeQuery',
    use_alternate_source: 'chat.tool.webAccess.useAlternateSource',
    wait_or_reconfigure: 'chat.tool.webAccess.waitOrReconfigure',
    wait_or_use_alternate_source: 'chat.tool.webAccess.waitOrUseAlternateSource',
  }[result.retryStrategy] : undefined

  let headline = webStatusKey ? t(webStatusKey) : ''
  if (!headline && result.schema === 'process.result.v1') {
    if (result.status === 'failed') {
      headline = result.exitCode === undefined
        ? t('chat.tool.result.commandExitedNonZero')
        : t('chat.tool.result.commandExitedWithCode', { code: result.exitCode })
    } else if (result.status === 'timed_out') {
      headline = t('chat.tool.result.timedOut')
    } else if (result.status === 'cancelled') {
      headline = t('chat.tool.result.cancelled')
    }
  }
  const isContinuablePage = result.status === 'partial'
    && result.continuation !== undefined
    && (result.schema === 'resource.read.v1' || result.schema === 'workspace.search.v1')
  if (!headline && isContinuablePage) {
    headline = t('chat.tool.result.pageReady')
  } else if (!headline && (result.status === 'partial' || result.truncated)) {
    headline = t(result.status === 'partial' ? 'chat.tool.result.partial' : 'chat.tool.result.truncated')
  }
  if (!headline && result.severity !== 'success') headline = result.status

  const continuation = result.continuation
    ? t(result.continuation.kind === 'offset' ? 'chat.tool.result.moreOffset' : 'chat.tool.result.moreCursor', {
        value: result.continuation.kind === 'cursor' ? buildPreview(result.continuation.value, 72) : result.continuation.value,
      })
    : ''
  // Web recovery strategies already have localized, actionable labels. Avoid
  // duplicating the provider's often bilingual suggested_action beside them.
  // Continuations already state the next page, while a process non-zero exit
  // may be the intended diagnostic result rather than a command to "fix".
  const recovery = !isWebAccess
    && !result.continuation
    && !(result.schema === 'process.result.v1' && result.status === 'failed')
    && result.recovery
    ? result.recovery
    : ''
  return [headline, webRetryKey ? t(webRetryKey) : '', continuation, recovery].filter(Boolean).join(' · ')
}

function formatMaybeJSON(value: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function buildToolArgSummary(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const path = data.file_path || data.path || data.cwd || data.command
    if (typeof path === 'string' && path) return path
  } catch {
    // 非 JSON 参数使用通用预览。
  }
  return buildPreview(args, 120)
}

function extractToolArgPath(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const path = data.file_path || data.path
    return typeof path === 'string' ? path : ''
  } catch {
    const match = args.match(/"(?:file_path|path)"\s*:\s*"([^"]+)"/)
    return match?.[1] || ''
  }
}

function buildPreview(content: string, maxLength: number) {
  const normalized = content.trim().replace(/\s+/g, ' ')
  if (normalized.length <= maxLength) return normalized
  return `${normalized.slice(0, maxLength)}...`
}

function buildMarkdownPreview(content: string, maxLength: number) {
  const trimmed = content.trim()
  const chars = Array.from(trimmed)
  if (chars.length <= maxLength) return trimmed
  return `${chars.slice(0, maxLength).join('').trimEnd()}\n\n...`
}

/** 判断是否为会产生大量内容参数的工具（适合流式预览） */
function isContentTool(name: string): boolean {
  return ['write', 'edit'].includes(name)
}

/** Extract generated text from complete or incrementally streamed tool arguments. */
function extractStreamingContent(rawArgs: string): string {
  try {
    const parsed = JSON.parse(rawArgs) as Record<string, unknown>
    if (typeof parsed.content === 'string') return parsed.content
    if (Array.isArray(parsed.edits)) {
      const replacements = parsed.edits.flatMap((entry) => {
        if (!entry || typeof entry !== 'object') return []
        const value = (entry as Record<string, unknown>).new_string
        return typeof value === 'string' ? [value] : []
      })
      if (replacements.some((value) => value.length > 0)) return replacements.join('\n\n')
    }
    if (typeof parsed.new_string === 'string') return parsed.new_string
  } catch {
    // The accumulated stream can be incomplete; scan its valid string tokens.
  }

  const content = extractStreamingJSONStringValues(rawArgs, 'content')
  if (content.length > 0) return content[0]
  const replacements = extractStreamingJSONStringValues(rawArgs, 'new_string')
  return replacements.some((value) => value.length > 0) ? replacements.join('\n\n') : ''
}

type StreamingJSONStringToken = {
  value: string
  end: number
  complete: boolean
}

function extractStreamingJSONStringValues(rawArgs: string, targetKey: string): string[] {
  const values: string[] = []
  let offset = 0
  while (offset < rawArgs.length) {
    if (rawArgs[offset] !== '"') {
      offset += 1
      continue
    }
    const key = readStreamingJSONString(rawArgs, offset)
    if (!key.complete) break
    offset = key.end
    let cursor = skipJSONWhitespace(rawArgs, offset)
    if (key.value !== targetKey || rawArgs[cursor] !== ':') continue
    cursor = skipJSONWhitespace(rawArgs, cursor + 1)
    if (rawArgs[cursor] !== '"') {
      offset = cursor
      continue
    }
    const value = readStreamingJSONString(rawArgs, cursor)
    values.push(value.value)
    offset = value.end
    if (!value.complete) break
  }
  return values
}

function readStreamingJSONString(source: string, start: number): StreamingJSONStringToken {
  let escaped = false
  for (let index = start + 1; index < source.length; index += 1) {
    const char = source[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '"') {
      return {
        value: decodeStreamingJSONString(source.slice(start + 1, index)),
        end: index + 1,
        complete: true,
      }
    }
  }
  return {
    value: decodeStreamingJSONString(source.slice(start + 1)),
    end: source.length,
    complete: false,
  }
}

function decodeStreamingJSONString(rawValue: string): string {
  try {
    return JSON.parse(`"${rawValue}"`) as string
  } catch {
    let decoded = ''
    for (let index = 0; index < rawValue.length; index += 1) {
      const char = rawValue[index]
      if (char !== '\\') {
        decoded += char
        continue
      }
      const escaped = rawValue[index + 1]
      if (escaped === undefined) break
      index += 1
      const simpleEscape = ({
        '"': '"',
        '\\': '\\',
        '/': '/',
        b: '\b',
        f: '\f',
        n: '\n',
        r: '\r',
        t: '\t',
      } as Record<string, string>)[escaped]
      if (simpleEscape !== undefined) {
        decoded += simpleEscape
        continue
      }
      if (escaped === 'u') {
        const hex = rawValue.slice(index + 1, index + 5)
        if (!/^[0-9a-fA-F]{4}$/.test(hex)) break
        decoded += String.fromCharCode(Number.parseInt(hex, 16))
        index += 4
        continue
      }
      decoded += escaped
    }
    return decoded
  }
}

function skipJSONWhitespace(source: string, start: number): number {
  let offset = start
  while (offset < source.length && /\s/.test(source[offset])) offset += 1
  return offset
}

/** 流式等待占位：正文尚未到达（或仅有被隐藏的思考）时显示，避免空白气泡像卡死。 */
function StreamingPlaceholder() {
  const { t } = useTranslation()
  return (
    <div className="py-1" role="status" aria-live="polite">
      <Shimmer as="span" className="text-sm font-medium">{t('chat.activity.thinking')}</Shimmer>
    </div>
  )
}

function sanitizeThinkTags(text: string): string {
  let result = text
  // 部分 provider 返回的内部特殊 token 与文本形式的工具调用残留（兜底历史数据；新对话已由后端解析执行）
  result = result.replace(/\]<\]minimax\[>\[/g, '')
  result = result.replace(/<tool_call>[\s\S]*?<\/tool_call>/gi, '')
  result = result.replace(/<invoke\s+name="[^"]*"[\s\S]*?<\/invoke>/gi, '')
  // 配对或未闭合的 <think>...</think>
  result = result.replace(/<think>[\s\S]*?(?:<\/think>|$)/gi, '')
  // 无 <think> 开始标签、仅以 </think> 收尾的思考前言：删除开头直到首个 </think>
  const close = result.search(/<\s*\/\s*think\s*>/i)
  if (close >= 0) {
    result = result.slice(close).replace(/<\s*\/\s*think\s*>/i, '')
  }
  // 清理任何残留 think 标签
  return result.replace(/<\/?\s*think\s*>/gi, '')
}

const MarkdownContent = memo(function MarkdownContent({ content, highlightDialogue, projectId }: { content: string; highlightDialogue: boolean; projectId: string }) {
  const components = useMemo<MarkdownRendererComponents>(() => ({
    ...(highlightDialogue ? dialogueMarkdownComponents : markdownComponents),
    img: (props) => <ChatMarkdownImage {...props} projectId={projectId} />,
  }), [highlightDialogue, projectId])
  return (
    <MarkdownRenderer content={content} components={components} />
  )
})

const markdownComponents: MarkdownRendererComponents = {
  a: ChatMarkdownLink,
  img: ChatMarkdownImage,
}

const dialogueMarkdownComponents: MarkdownRendererComponents = {
  ...markdownComponents,
  p: ({ children }: { children?: ReactNode }) => <p>{highlightDialogueNodes(children)}</p>,
  li: ({ children }: { children?: ReactNode }) => <li>{highlightDialogueNodes(children)}</li>,
  h1: ({ children }: { children?: ReactNode }) => <h1>{highlightDialogueNodes(children)}</h1>,
  h2: ({ children }: { children?: ReactNode }) => <h2>{highlightDialogueNodes(children)}</h2>,
  h3: ({ children }: { children?: ReactNode }) => <h3>{highlightDialogueNodes(children)}</h3>,
  h4: ({ children }: { children?: ReactNode }) => <h4>{highlightDialogueNodes(children)}</h4>,
  h5: ({ children }: { children?: ReactNode }) => <h5>{highlightDialogueNodes(children)}</h5>,
  h6: ({ children }: { children?: ReactNode }) => <h6>{highlightDialogueNodes(children)}</h6>,
  blockquote: ({ children }: { children?: ReactNode }) => <blockquote>{highlightDialogueNodes(children)}</blockquote>,
}

function ChatMarkdownLink({ href, title, children }: { href?: string; title?: string; children?: ReactNode }) {
  const external = /^https?:\/\//i.test(href?.trim() || '')
  return (
    <a
      href={href}
      title={title}
      {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
    >
      {children}
    </a>
  )
}

function ChatMarkdownImage({ src = '', alt = '', title = '', projectId = '' }: { src?: string; alt?: string; title?: string; projectId?: string }) {
  const { t } = useTranslation()
  const imageSrc = normalizeChatImageSrc(src, projectId)
  if (!imageSrc) return null
  const imageTitle = alt || title || t('chat.image.previewTitle')
  const imagePath = shouldShowImagePath(src) ? src : undefined

  return (
    <ImagePreviewDialog src={imageSrc} title={imageTitle} alt={alt || imageTitle} path={imagePath}>
      <button type="button" className="nova-chat-image-button" aria-label={t('chat.image.openPreview')}>
        <img src={imageSrc} alt={alt || imageTitle} title={title || undefined} loading="lazy" />
      </button>
    </ImagePreviewDialog>
  )
}

function normalizeChatImageSrc(src: string, projectId: string) {
  const trimmed = src.trim()
  if (!trimmed) return ''
  if (/^(https?:|data:|blob:|\/)/i.test(trimmed)) return trimmed
  if (isWorkspaceImagePath(trimmed)) return chatAssetURL(projectId, trimmed)
  return trimmed
}

function chatAssetURL(projectId: string, path: string) {
  return projectId ? projectFileAssetURL(projectId, path) : ''
}

function shouldShowImagePath(src: string) {
  const trimmed = src.trim()
  return Boolean(trimmed && !/^(data:|blob:)/i.test(trimmed))
}

function highlightDialogueNodes(children: ReactNode): ReactNode {
  return Children.map(children, (child, index) => {
    if (typeof child === 'string') return highlightDialogueText(child, true, `md-${index}`)
    if (!isValidElement(child)) return child
    const props = child.props as { children?: ReactNode }
    if (props.children === undefined) return child
    return cloneElement(child, undefined, highlightDialogueNodes(props.children))
  })
}

function highlightDialogueText(text: string, enabled: boolean, keyPrefix: string): ReactNode {
  if (!enabled || !text) return text
  const nodes: ReactNode[] = []
  const ranges = findDialogueHighlightRanges(text)
  let lastIndex = 0

  ranges.forEach((range, index) => {
    if (range.from > lastIndex) nodes.push(text.slice(lastIndex, range.from))
    nodes.push(
      <span key={`${keyPrefix}-dialogue-${index}`} className="nova-dialogue-highlight">
        {text.slice(range.from, range.to)}
      </span>,
    )
    lastIndex = range.to
  })

  if (lastIndex < text.length) nodes.push(text.slice(lastIndex))
  if (nodes.length === 0) return text
  return <Fragment>{nodes}</Fragment>
}

/** 思考过程折叠块：流式时展开，历史消息从首帧开始保持折叠。 */
function ThinkingBlock({ message, content, streaming }: { message: ChatMessage; content: string; streaming: boolean }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(streaming)

  useLayoutEffect(() => {
    setExpanded(streaming)
  }, [streaming])

  // 流式思考但内容尚未到达时，复用 Shimmer 显示“思考中...”，避免空白折叠块像卡死。
  const showActivityShimmer = streaming && !content.trim()

  return (
    <div className="flex justify-start">
      <div className="w-full">
        <Reasoning isStreaming={streaming} open={expanded} onOpenChange={setExpanded} className="mb-0">
          <ReasoningTrigger className="flex items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
            {expanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
            {showActivityShimmer ? (
              <Shimmer as="span" className="text-xs font-medium">{t('chat.activity.thinking')}</Shimmer>
            ) : (
              <span>{t('chat.trace.thinking')}</span>
            )}
            {message.subagent && <AgentSourceBadge message={message} compact />}
          </ReasoningTrigger>
          <ReasoningContent className="mt-0 border-l border-[var(--nova-border)] px-3 py-2 text-xs text-[var(--nova-text-muted)] whitespace-pre-wrap">
            <StreamingContentStage content={content} targetContent={streaming ? message.streaming_target_content : undefined} streaming={streaming}>
              {(value) => value}
            </StreamingContentStage>
          </ReasoningContent>
        </Reasoning>
      </div>
    </div>
  )
}
