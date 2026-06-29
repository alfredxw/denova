import { Children, Fragment, cloneElement, isValidElement, memo, useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Check, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Circle, CircleDot, Clock3, Copy, FileText, ImagePlus, ListTodo, Loader2, PanelRightOpen, Pencil, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { workspaceAssetURL, type ChapterIllustration, type ChatMessage, type InteractiveImage, type InteractiveImageError } from '@/lib/api'
import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'
import { isWorkspaceImagePath } from '@/lib/workspace-file-kind'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { TooltipProvider } from '@/components/ui/tooltip'
import { subAgentSessionKey } from './subagent-session'

interface MessageItemProps {
  message: ChatMessage
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onEdit?: (message: ChatMessage) => void
  onRegenerate?: (message: ChatMessage) => void
  onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void
  onOpenSubAgentSession?: (message: ChatMessage) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (message: ChatMessage) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  subAgentPresentation?: 'card' | 'content'
}

const copyFeedbackDurationMs = 1200
const messageActionTooltipDelayMs = 500
const messageActionTooltipSkipDelayMs = 300
const messageActionTooltipSideOffset = 3

/** 单条消息组件，根据 role 渲染不同样式 */
export const MessageItem = memo(function MessageItem({ message, highlightDialogue = false, messageStyle, onEdit, onRegenerate, onSwitchVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, subAgentPresentation = 'card' }: MessageItemProps) {
  const { role, content = '' } = message
  const canEdit = role === 'user' && Boolean(message.turn_id) && Boolean(onEdit)
  const canRegenerate = role === 'assistant' && Boolean(message.turn_id) && Boolean(onRegenerate) && !message.streaming
  const canGenerateInteractiveImage = role === 'assistant' && Boolean(message.turn_id) && Boolean(onGenerateInteractiveImage) && !message.streaming
  const versionCount = message.turn_versions?.length || 0
  const markedVersionIndex = message.turn_versions?.findIndex((version) => version.current) ?? -1
  const versionIndex = message.turn_version_index ?? markedVersionIndex
  const canSwitchVersion = role === 'assistant' && versionCount > 1 && versionIndex >= 0 && Boolean(onSwitchVersion) && !message.streaming

  switch (role) {
    case 'user':
      return (
        <div className="group flex justify-end gap-2">
          <div className="nova-message-body-with-meta nova-message-body-with-meta-user max-w-[88%]">
            <div className="nova-user-message rounded-lg px-3.5 py-2.5 text-sm text-[var(--nova-user-message-text)] whitespace-pre-wrap" style={messageStyle}>
              {content}
            </div>
            <MessageInlineMeta message={message} content={content} align="right" onEdit={canEdit ? onEdit : undefined} />
          </div>
        </div>
      )

    case 'assistant': {
      if (message.subagent && subAgentPresentation === 'card') {
        return (
          <SubAgentOutputWindow
            message={message}
            content={content}
            highlightDialogue={highlightDialogue}
            messageStyle={messageStyle}
            onOpen={onOpenSubAgentSession}
            active={Boolean(activeSubAgentSessionKey && activeSubAgentSessionKey === subAgentSessionKey(message))}
          />
        )
      }
      // 流式期间正文可能尚未到达，或全是被隐藏的思考内容（清洗后为空）：
      // 此时显示"正在思考"占位，避免出现一个空白气泡、像卡死无响应。
      const visibleContent = sanitizeThinkTags(content).trim()
      return (
        <div className="group flex justify-start">
          <div className="w-full">
            <div className="nova-message-body-with-meta nova-message-body-with-meta-assistant">
              <div className="chat-agent-message w-full px-1 text-sm text-[var(--nova-text)]" style={messageStyle}>
                {message.streaming && !visibleContent ? (
                  <StreamingPlaceholder />
                ) : message.streaming ? (
                  <StreamingMarkdown content={content} highlightDialogue={highlightDialogue} />
                ) : (
                  <MarkdownContent content={content} highlightDialogue={highlightDialogue} />
                )}
              </div>
              <InteractiveImageStrip message={message} />
              <MessageInlineMeta
                message={message}
                content={content}
                align="left"
                onGenerateInteractiveImage={canGenerateInteractiveImage ? onGenerateInteractiveImage : undefined}
                generatingInteractiveImage={Boolean(message.turn_id && generatingInteractiveImageTurnId === message.turn_id)}
                onRegenerate={canRegenerate ? onRegenerate : undefined}
                onSwitchVersion={canSwitchVersion ? onSwitchVersion : undefined}
                versionIndex={versionIndex}
                versionCount={versionCount}
              />
            </div>
          </div>
        </div>
      )
    }

    case 'thinking':
      return <ThinkingBlock message={message} content={content} streaming={message.streaming === true} />

    case 'tool_call':
      if ((message.name || '') === 'generate_interactive_image') {
        return <InteractiveImageBlock message={message} onRegenerate={onGenerateInteractiveImage} />
      }
      if (['generate_image', 'generate_chapter_illustration'].includes(message.name || '') && message.illustration) {
        return <ChapterIllustrationBlock message={message} onInsert={onInsertIllustration} />
      }
      if ((message.name || '') === 'write_todos') {
        return <TodoListBlock message={message} />
      }
      return <ToolExecutionBlock message={message} />

    case 'tool_result':
      if ((message.name || '') === 'generate_interactive_image' || message.interactive_image) {
        return <InteractiveImageBlock message={message} onRegenerate={onGenerateInteractiveImage} />
      }
      if (message.illustration) {
        return <ChapterIllustrationBlock message={message} onInsert={onInsertIllustration} />
      }
      return <ToolResultBlock content={content} />

    case 'context_compaction':
      return <ContextCompactionBlock message={message} />

    case 'system':
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
          <span className="rounded-full border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-1 text-xs text-[var(--nova-danger)]">
            {content}
          </span>
        </div>
      )

    default:
      return null
  }
})

function MessageInlineMeta({ message, content, align, onEdit, onGenerateInteractiveImage, generatingInteractiveImage = false, onRegenerate, onSwitchVersion, versionIndex = -1, versionCount = 0 }: { message: ChatMessage; content: string; align: 'left' | 'right'; onEdit?: (message: ChatMessage) => void; onGenerateInteractiveImage?: (message: ChatMessage) => void; generatingInteractiveImage?: boolean; onRegenerate?: (message: ChatMessage) => void; onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void; versionIndex?: number; versionCount?: number }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const formatted = formatMessageHoverTime(message.created_at)
  const canSwitchVersion = Boolean(onSwitchVersion && versionCount > 1 && versionIndex >= 0)
  const metaTooltip = {
    tooltipSide: 'top' as const,
    tooltipSideOffset: messageActionTooltipSideOffset,
    useTooltipProvider: false,
  }
  if (!formatted && !content && !onEdit && !onGenerateInteractiveImage && !onRegenerate && !canSwitchVersion) return null
  return (
    <TooltipProvider delayDuration={messageActionTooltipDelayMs} skipDelayDuration={messageActionTooltipSkipDelayMs} disableHoverableContent>
      <div className={`nova-message-meta nova-message-meta-${align}`} aria-label={formatted}>
        {formatted ? <span className="nova-message-time">{formatted}</span> : null}
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
        {onEdit && (
          <TooltipIconButton
            label={t('chat.action.editTurn')}
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
  onOpen,
  active,
}: {
  message: ChatMessage
  content: string
  highlightDialogue: boolean
  messageStyle?: CSSProperties
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
              {message.streaming ? (
                <StreamingMarkdown content={shownContent} highlightDialogue={highlightDialogue} />
              ) : (
                <MarkdownContent content={shownContent} highlightDialogue={highlightDialogue} />
              )}
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
    <span className={`mb-1 inline-flex max-w-full items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)] ${compact ? 'mb-0 shrink-0' : ''}`}>
      <span className="truncate">{label}</span>
    </span>
  )
}

/** 工具执行中的轻量状态卡片 */
export function ToolActivityBlock({ content }: { content: string }) {
  const { t } = useTranslation()
  const activity = parseActivityContent(content, t)

  return (
    <div className="flex justify-start">
      <div className="w-full rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5 text-xs shadow-[var(--nova-shadow)]">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            <Clock3 className="h-3.5 w-3.5 animate-pulse" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2 text-[var(--nova-text)]">
              <span className="font-medium">{activity.title}</span>
              {activity.toolName && (
                <code className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--nova-text-muted)]">
                  {activity.toolName}
                </code>
              )}
            </div>
            {activity.detail && <div className="mt-1 truncate text-[var(--nova-text-faint)]">{activity.detail}</div>}
          </div>
        </div>
      </div>
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
          className="max-h-40 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 text-[11px] leading-relaxed text-[var(--nova-text-muted)] whitespace-pre-wrap [overflow-anchor:none]"
        >
          {summary || (isRunning ? t('chat.contextCompaction.waiting') : t('chat.contextCompaction.empty'))}
        </div>
      </div>
    </div>
  )
}

/** 工具执行卡片，默认以单行展示运行态和结果态。 */
export function ToolExecutionBlock({ message }: { message: ChatMessage }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const info = parseToolCallContent(message.content || '')
  const name = message.name || info.name
  const rawArgs = message.args !== undefined ? message.args : info.args
  const args = formatMaybeJSON(rawArgs)
  const status = message.status || 'running'
  const result = message.result || ''
  const isDelegationTool = name === 'task'
  const taskSubAgent = isDelegationTool ? (message.subagent_type || parseTaskSubagentType(rawArgs)) : ''
  const isChapterBodyHidden = message.sse_display_notice === 'chapter_body_hidden'
  const chapterBodyHiddenPath = isChapterBodyHidden ? extractToolArgPath(rawArgs) : ''
  const chapterGeneratedChars = isChapterBodyHidden && typeof message.sse_generated_chars === 'number' ? message.sse_generated_chars : undefined
  const displayName = isDelegationTool ? t('chat.subagent.taskLabel') : name
  const detailArgs = isDelegationTool ? formatTaskDelegationArgs(rawArgs) : (isChapterBodyHidden ? '' : args)
  const hasResult = status === 'success'
  const isStreamingContent = !isChapterBodyHidden && status === 'running' && isContentTool(name) && rawArgs.length > 50
  const streamPreview = isStreamingContent ? extractStreamingContent(rawArgs) : ''
  const summary = taskSubAgent
    ? t('chat.subagent.delegating', { name: taskSubAgent })
    : buildToolArgSummary(args) || (isStreamingContent ? t('chat.tool.writing') : t('chat.tool.preparing'))
  const resultPreview = buildPreview(result, 80)
  const displaySummary = isChapterBodyHidden
    ? chapterGeneratedChars !== undefined
      ? t(hasResult ? 'chat.tool.chapterWrittenWithCount' : 'chat.tool.chapterWritingWithCount', { count: chapterGeneratedChars })
      : (hasResult ? t('chat.tool.chapterWritten') : t('chat.tool.chapterWriting'))
    : (hasResult ? resultPreview || t('chat.tool.done') : summary)
  const hasDetail = Boolean(detailArgs || result || isChapterBodyHidden)
  const streamPreviewScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: isStreamingContent,
    resetKey: `${message.id || name}:tool-stream-preview`,
    contentKey: `${status}:${rawArgs.length}:${streamPreview.length}`,
  })

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div className="flex min-h-10 min-w-0 items-center gap-2 px-3 py-2">
          <ToolStatusIcon status={status} />
          <span className="shrink-0 font-medium text-[var(--nova-text)]">{t('chat.tool.calling')}</span>
          <code className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--nova-text-muted)]">
            {displayName}
          </code>
          {taskSubAgent && (
            <span className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">
              {t('chat.subagent.delegating', { name: taskSubAgent })}
            </span>
          )}
          {message.subagent && <AgentSourceBadge message={message} compact />}
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">
            {displaySummary}
          </span>
          {hasDetail && !isStreamingContent && (
            <button
              type="button"
              className="shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[var(--nova-text-muted)] transition hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? t('chat.tool.collapse') : t('chat.tool.details')}
            </button>
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
            className="max-h-32 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-accent-green)] whitespace-pre-wrap [overflow-anchor:none]"
          >
            {streamPreview}
          </div>
        )}
        {expanded && !isStreamingContent && (
          <div className="grid max-h-48 gap-2 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)]">
            {isChapterBodyHidden && (
              <div className="grid gap-1 font-sans">
                {chapterBodyHiddenPath && (
                  <div className="min-w-0">
                    <span className="text-[var(--nova-text-faint)]">{t('chat.tool.chapterPath')}</span>
                    <code className="ml-1 break-all font-mono text-[var(--nova-text-muted)]">{chapterBodyHiddenPath}</code>
                  </div>
                )}
                {chapterGeneratedChars !== undefined && (
                  <div className="text-[var(--nova-text-faint)]">
                    {t('chat.tool.chapterGeneratedChars', { count: chapterGeneratedChars })}
                  </div>
                )}
                <div className="text-[var(--nova-text-faint)]">{t('chat.tool.chapterBodyHidden')}</div>
              </div>
            )}
            {detailArgs && <pre className="whitespace-pre-wrap">{detailArgs}</pre>}
            {taskSubAgent && result && <div className="text-[var(--nova-text-muted)]">{t('chat.subagent.result')}</div>}
            {result && <pre className="whitespace-pre-wrap text-[var(--nova-accent-green)]">{result}</pre>}
          </div>
        )}
      </div>
    </div>
  )
}

function ChapterIllustrationBlock({ message, onInsert }: { message: ChatMessage; onInsert?: (illustration: ChapterIllustration) => void }) {
  const { t } = useTranslation()
  const illustration = message.illustration
  if (!illustration) return <ToolExecutionBlock message={message} />

  const status = message.status || 'running'
  const isMarkdownChapter = isMarkdownPath(illustration.chapter_path)
  const canInsert = status === 'success' && isMarkdownChapter && Boolean(onInsert)
  const imageSrc = workspaceAssetURL(illustration.image_path)
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

function InteractiveImageBlock({ message }: { message: ChatMessage; onRegenerate?: (message: ChatMessage) => void }) {
  return (
    <div className="flex justify-start">
      <div className="w-full">
        <InteractiveImageStrip message={message} />
      </div>
    </div>
  )
}

function InteractiveImageStrip({ message }: { message: ChatMessage }) {
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
  const src = workspaceAssetURL(image.image_path)
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
  content: string
  activeForm?: string
  status: 'pending' | 'in_progress' | 'completed' | string
}

/** Agentic Loop write_todos 工具卡片：渲染为可读的待办列表，兼容流式不完整 args */
export function TodoListBlock({ message }: { message: ChatMessage }) {
  const { t } = useTranslation()
  const args = message.args || ''
  const todos = parseTodosFromArgs(args)
  const status = message.status || 'running'
  const total = todos.length
  const completed = todos.filter(t => t.status === 'completed').length
  const inProgress = todos.find(t => t.status === 'in_progress')
  const headline = inProgress?.activeForm || inProgress?.content || (status === 'success' ? t('chat.todo.updated') : t('chat.todo.updating'))

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
  const text = todo.status === 'in_progress' && todo.activeForm ? todo.activeForm : todo.content
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

/** 解析 write_todos 工具参数，对流式中可能不完整的 JSON 做容错 */
function parseTodosFromArgs(args: string): TodoItem[] {
  if (!args) return []
  const trimmed = args.trim()
  if (!trimmed) return []
  // 优先尝试完整 JSON
  try {
    const data = JSON.parse(trimmed) as { todos?: TodoItem[] }
    if (Array.isArray(data?.todos)) return data.todos
  } catch {
    // 流式中常见：args 不完整或被截断
  }
  // 回退：从 todos 数组中提取已经完整的对象
  const arrayMatch = trimmed.match(/"todos"\s*:\s*\[([\s\S]*)$/)
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

function ToolStatusIcon({ status }: { status: ChatMessage['status'] }) {
  if (status === 'success') {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-accent-green)]/45 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]">
        <CheckCircle2 className="h-3.5 w-3.5" />
      </span>
    )
  }
  if (status === 'error') {
    return <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[10px] text-[var(--nova-danger)]">!</span>
  }
  return <span className="h-5 w-5 shrink-0 animate-spin rounded-full border-2 border-[var(--nova-border)] border-t-[var(--nova-text)]" />
}

/** 工具结果卡片，默认展示摘要，避免大段结果挤占对话区 */
function ToolResultBlock({ content }: { content: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const preview = buildPreview(content, 160)
  const canExpand = content.trim().replace(/\s+/g, ' ').length > 160

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div className="flex items-start gap-3 px-3 py-2.5">
          <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-accent-green)]/35 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]">
            <CheckCircle2 className="h-3.5 w-3.5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-[var(--nova-text)]">{t('chat.tool.resultDone')}</span>
              <span className="rounded-full border border-[var(--nova-accent-green)]/35 bg-[var(--nova-accent-green)]/10 px-2 py-0.5 text-[11px] text-[var(--nova-accent-green)]">
                success
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
          <pre className="max-h-56 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)]">
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

function parseActivityContent(content: string, t: (key: string) => string) {
  const toolMatch = content.match(/^正在执行工具：([^\n]+)(?:\n([\s\S]*))?$/)
  if (toolMatch) {
    const args = formatMaybeJSON((toolMatch[2] || '').trim())
    return {
      title: t('chat.tool.runningTitle'),
      toolName: toolMatch[1].trim(),
      detail: buildToolArgSummary(args) || t('chat.tool.waitingResult'),
    }
  }

  const doneMatch = content.match(/^工具执行完成：?([\s\S]*)$/)
  if (doneMatch) {
    return {
      title: t('chat.tool.resultDone'),
      toolName: '',
      detail: buildPreview(doneMatch[1] || '', 120),
    }
  }

  return {
    title: content,
    toolName: '',
    detail: '',
  }
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
  return ['write_file', 'edit_file'].includes(name)
}

/** 从不完整的 JSON args 中提取 content/new_string 字段的流式文本 */
function extractStreamingContent(rawArgs: string): string {
  // 尝试提取 "content": "..." 或 "new_string": "..."
  const match = rawArgs.match(/"(?:content|new_string)"\s*:\s*"([\s\S]*)$/m)
  if (!match) return ''
  // 解码已有的 JSON 转义字符，末尾可能不完整
  let text = match[1]
  try {
    // 尝试解析 JSON 字符串（加上闭合引号使其合法）
    text = JSON.parse(`"${text}"`)
  } catch {
    // 不完整时做简单转义还原
    text = text.replace(/\\n/g, '\n').replace(/\\t/g, '\t').replace(/\\"/g, '"').replace(/\\\\/g, '\\')
  }
  // 只展示最后 500 字符以保持性能
  if (text.length > 500) {
    return '...' + text.slice(-500)
  }
  return text
}

/** 流式等待占位：正文尚未到达（或仅有被隐藏的思考）时显示，避免空白气泡像卡死。 */
function StreamingPlaceholder() {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2 py-1 text-sm text-[var(--nova-text-muted)]">
      <span className="flex gap-1">
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-[var(--nova-text-muted)] [animation-delay:-0.3s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-[var(--nova-text-muted)] [animation-delay:-0.15s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-[var(--nova-text-muted)]" />
      </span>
      <span>{t('chat.activity.thinking')}</span>
    </div>
  )
}

/** 流式和持久化消息共用同一 Markdown 渲染器，避免刷新后段落、列表和行距重新排版。 */
function StreamingMarkdown({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  return <MarkdownContent content={content} highlightDialogue={highlightDialogue} />
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

const MarkdownContent = memo(function MarkdownContent({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={highlightDialogue ? dialogueMarkdownComponents : markdownComponents}
    >
      {content}
    </ReactMarkdown>
  )
})

const markdownComponents: Components = {
  img: ChatMarkdownImage,
}

const dialogueMarkdownComponents: Components = {
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

function ChatMarkdownImage({ src = '', alt = '', title = '' }: { src?: string; alt?: string; title?: string }) {
  const { t } = useTranslation()
  const imageSrc = normalizeChatImageSrc(src)
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

function normalizeChatImageSrc(src: string) {
  const trimmed = src.trim()
  if (!trimmed) return ''
  if (/^(https?:|data:|blob:|\/)/i.test(trimmed)) return trimmed
  if (isWorkspaceImagePath(trimmed)) return workspaceAssetURL(trimmed)
  return trimmed
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

/** 思考过程折叠块，流式思考中自动展开，结束后自动折叠。 */
function ThinkingBlock({ message, content, streaming }: { message: ChatMessage; content: string; streaming: boolean }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(streaming)

  useEffect(() => {
    setExpanded(streaming)
  }, [streaming])

  return (
    <div className="flex justify-start">
      <div className="w-full">
        <button
          type="button"
          className="flex items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          💭 {t('chat.trace.thinking')}
          {message.subagent && <AgentSourceBadge message={message} compact />}
        </button>
        {expanded && (
          <div className="border-l border-[var(--nova-border)] px-3 py-2 text-xs text-[var(--nova-text-muted)] whitespace-pre-wrap">
            {content}
          </div>
        )}
      </div>
    </div>
  )
}
