import { Children, Fragment, cloneElement, isValidElement, memo, useEffect, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Circle, CircleDot, Clock3, FileText, ListTodo, PanelRightOpen, Pencil, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChatMessage } from '@/lib/api'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { subAgentSessionKey } from './subagent-session'

interface MessageItemProps {
  message: ChatMessage
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onEdit?: (message: ChatMessage) => void
  onRegenerate?: (message: ChatMessage) => void
  onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void
  onOpenSubAgentSession?: (message: ChatMessage) => void
  activeSubAgentSessionKey?: string
  subAgentPresentation?: 'card' | 'content'
}

/** 单条消息组件，根据 role 渲染不同样式 */
export const MessageItem = memo(function MessageItem({ message, highlightDialogue = false, messageStyle, onEdit, onRegenerate, onSwitchVersion, onOpenSubAgentSession, activeSubAgentSessionKey, subAgentPresentation = 'card' }: MessageItemProps) {
  const { t } = useTranslation()
  const { role, content = '' } = message
  const canEdit = role === 'user' && Boolean(message.turn_id) && Boolean(onEdit)
  const canRegenerate = role === 'assistant' && Boolean(message.turn_id) && Boolean(onRegenerate) && !message.streaming
  const versionCount = message.turn_versions?.length || 0
  const markedVersionIndex = message.turn_versions?.findIndex((version) => version.current) ?? -1
  const versionIndex = message.turn_version_index ?? markedVersionIndex
  const canSwitchVersion = role === 'assistant' && versionCount > 1 && versionIndex >= 0 && Boolean(onSwitchVersion) && !message.streaming
  const showAssistantActions = canRegenerate || canSwitchVersion

  switch (role) {
    case 'user':
      return (
        <div className="group flex justify-end gap-2">
          {canEdit && (
            <div className="flex h-8 shrink-0 items-center gap-1 self-end opacity-80 transition-opacity group-hover:opacity-100">
              {onEdit && (
                <TooltipIconButton
                  label={t('chat.action.editTurn')}
                  className="h-7 w-7 border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                  onClick={() => onEdit(message)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </TooltipIconButton>
              )}
            </div>
          )}
          <div className="nova-user-message max-w-[88%] rounded-lg px-3.5 py-2.5 text-sm text-[var(--nova-user-message-text)] whitespace-pre-wrap" style={messageStyle}>
            {content}
          </div>
        </div>
      )

    case 'assistant':
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
      return (
        <div className="group flex justify-start">
          <div className="chat-agent-message w-full px-1 text-sm text-[var(--nova-text)]" style={messageStyle}>
            {message.streaming ? (
              <StreamingMarkdown content={content} highlightDialogue={highlightDialogue} />
            ) : (
              <MarkdownContent content={content} highlightDialogue={highlightDialogue} />
            )}
            {showAssistantActions && (
              <div className="mt-1.5 flex justify-end">
                <div className="flex items-center gap-0.5 opacity-30 transition-opacity group-hover:opacity-80 focus-within:opacity-80">
                  {canSwitchVersion && onSwitchVersion && (
                    <>
                      <TooltipIconButton
                        label={t('chat.action.prevVersion')}
                        className="h-6 w-6 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-30"
                        disabled={versionIndex <= 0}
                        onClick={() => onSwitchVersion(message, -1)}
                      >
                        <ChevronLeft className="h-3 w-3" />
                      </TooltipIconButton>
                      <span className="min-w-8 text-center font-mono text-[10px] leading-6 text-[var(--nova-text-faint)]">
                        {versionIndex + 1}/{versionCount}
                      </span>
                    </>
                  )}
                  {canRegenerate && onRegenerate && (
                    <TooltipIconButton
                      label={t('chat.action.regenerateTurn')}
                      className="h-6 w-6 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
                      onClick={() => onRegenerate(message)}
                    >
                      <RefreshCw className="h-3 w-3" />
                    </TooltipIconButton>
                  )}
                  {canSwitchVersion && onSwitchVersion && (
                    <TooltipIconButton
                      label={t('chat.action.nextVersion')}
                      className="h-6 w-6 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-30"
                      disabled={versionIndex >= versionCount - 1}
                      onClick={() => onSwitchVersion(message, 1)}
                    >
                      <ChevronRight className="h-3 w-3" />
                    </TooltipIconButton>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      )

    case 'thinking':
      return <ThinkingBlock message={message} content={content} streaming={message.streaming === true} />

    case 'tool_call':
      if ((message.name || '') === 'write_todos') {
        return <TodoListBlock message={message} />
      }
      return <ToolExecutionBlock message={message} />

    case 'tool_result':
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
        <div className={`${detailMode ? 'max-h-28' : expanded ? 'max-h-96' : 'max-h-28'} overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5`}>
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
        <div className="max-h-40 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 text-[11px] leading-relaxed text-[var(--nova-text-muted)] whitespace-pre-wrap">
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
  const displayName = isDelegationTool ? t('chat.subagent.taskLabel') : name
  const detailArgs = isDelegationTool ? formatTaskDelegationArgs(rawArgs) : args
  const hasResult = status === 'success'
  const isStreamingContent = status === 'running' && isContentTool(name) && rawArgs.length > 50
  const streamPreview = isStreamingContent ? extractStreamingContent(rawArgs) : ''
  const summary = taskSubAgent
    ? t('chat.subagent.delegating', { name: taskSubAgent })
    : buildToolArgSummary(args) || (isStreamingContent ? t('chat.tool.writing') : t('chat.tool.preparing'))
  const resultPreview = buildPreview(result, 80)
  const hasDetail = Boolean(detailArgs || result)

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
            {hasResult ? resultPreview || t('chat.tool.done') : summary}
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
          <div className="max-h-32 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-accent-green)] whitespace-pre-wrap">
            {streamPreview}
          </div>
        )}
        {expanded && !isStreamingContent && (
          <div className="grid max-h-48 gap-2 overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)]">
            {detailArgs && <pre className="whitespace-pre-wrap">{detailArgs}</pre>}
            {taskSubAgent && result && <div className="text-[var(--nova-text-muted)]">{t('chat.subagent.result')}</div>}
            {result && <pre className="whitespace-pre-wrap text-[var(--nova-accent-green)]">{result}</pre>}
          </div>
        )}
      </div>
    </div>
  )
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

/** 流式和持久化消息共用同一 Markdown 渲染器，避免刷新后段落、列表和行距重新排版。 */
function StreamingMarkdown({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  return <MarkdownContent content={content} highlightDialogue={highlightDialogue} />
}

const MarkdownContent = memo(function MarkdownContent({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={highlightDialogue ? dialogueMarkdownComponents : undefined}
    >
      {content}
    </ReactMarkdown>
  )
})

const dialogueMarkdownComponents = {
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
  const pattern = /("([^"\n]+)"|“([^”\n]+)”|「([^」\n]+)」)/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  let index = 0

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) nodes.push(text.slice(lastIndex, match.index))
    nodes.push(
      <span key={`${keyPrefix}-dialogue-${index}`} className="nova-dialogue-highlight">
        {match[0]}
      </span>,
    )
    lastIndex = pattern.lastIndex
    index += 1
  }

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
