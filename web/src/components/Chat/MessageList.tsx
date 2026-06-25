import { useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { MessageItem, ToolActivityBlock } from './MessageItem'
import type { ChatMessage } from '@/lib/api'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { listItem, novaEase } from '@/features/motion/motion-tokens'
import { buildSubAgentProgressMessage, isSubAgentTimelineMessage, subAgentSessionKey } from './subagent-session'

interface MessageListProps {
  messages: ChatMessage[]
  isStreaming: boolean
  activityContent: string
  highlightDialogue?: boolean
  scrollResetKey?: string
  bottomPaddingClassName?: string
  bottomPaddingPx?: number
  messageStyle?: CSSProperties
  collapseTraceBeforeAssistant?: boolean
  onEditMessage?: (message: ChatMessage) => void
  onRegenerateMessage?: (message: ChatMessage) => void
  onSwitchMessageVersion?: (message: ChatMessage, direction: -1 | 1) => void
  onOpenSubAgentSession?: (message: ChatMessage) => void
  activeSubAgentSessionKey?: string
}

/** 消息列表组件，支持流式内容实时展示和自动滚动 */
export function MessageList({ messages, isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', bottomPaddingPx, messageStyle, collapseTraceBeforeAssistant = false, onEditMessage, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, activeSubAgentSessionKey }: MessageListProps) {
  const { t } = useTranslation()
  const hasRunningContextCompaction = messages.some((message) => message.role === 'context_compaction' && message.status === 'running')
  const visibleActivityContent = hasRunningContextCompaction ? '' : activityContent
  const scrollContentKey = buildMessageListScrollKey(messages, visibleActivityContent, isStreaming, bottomPaddingPx)
  const scrollLock = useBottomScrollLock<HTMLDivElement>({
    resetKey: scrollResetKey,
    contentKey: scrollContentKey,
  })

  const renderMessage = (msg: ChatMessage, index: number) => {
    const key = msg.id || msg.created_at || index
    return (
      <motion.div
        key={key}
        layout="position"
        variants={listItem}
        initial="initial"
        animate="animate"
        transition={{ duration: 0.18, ease: novaEase }}
      >
        {msg.type === 'clear'
          ? <ContextClearDivider createdAt={msg.created_at} />
          : (
            <MessageWithHoverTime message={msg}>
              <MessageItem
                message={msg}
                highlightDialogue={highlightDialogue}
                messageStyle={messageStyle}
                onEdit={isStreaming ? undefined : onEditMessage}
                onRegenerate={isStreaming ? undefined : onRegenerateMessage}
                onSwitchVersion={isStreaming ? undefined : onSwitchMessageVersion}
                onOpenSubAgentSession={onOpenSubAgentSession}
                activeSubAgentSessionKey={activeSubAgentSessionKey}
              />
            </MessageWithHoverTime>
          )}
      </motion.div>
    )
  }

  const renderedMessages = []
  for (let index = 0; index < messages.length; index += 1) {
    const msg = messages[index]
    if (msg.role === 'token_usage') {
      continue
    }
    if (onOpenSubAgentSession && isSubAgentTimelineMessage(msg)) {
      const key = subAgentSessionKey(msg)
      const group: ChatMessage[] = []
      let nextIndex = index
      while (nextIndex < messages.length && isSubAgentTimelineMessage(messages[nextIndex]) && subAgentSessionKey(messages[nextIndex]) === key) {
        group.push(messages[nextIndex])
        nextIndex += 1
      }
      const progress = buildSubAgentProgressMessage(group)
      if (progress) {
        renderedMessages.push(renderMessage(progress, index))
        index = nextIndex - 1
        continue
      }
    }
    if (collapseTraceBeforeAssistant && isTraceMessage(msg)) {
      const traceMessages: ChatMessage[] = []
      let nextIndex = index
      while (nextIndex < messages.length && isTraceMessage(messages[nextIndex])) {
        traceMessages.push(messages[nextIndex])
        nextIndex += 1
      }
      const nextMessage = messages[nextIndex]
      if (traceMessages.length > 0 && nextMessage?.role === 'assistant' && (nextMessage.content || '').trim()) {
        renderedMessages.push(
          <motion.div
            key={`trace-${traceMessages[0].id || index}`}
            layout="position"
            variants={listItem}
            initial="initial"
            animate="animate"
            transition={{ duration: 0.18, ease: novaEase }}
          >
            <TraceGroup
              messages={traceMessages}
              highlightDialogue={highlightDialogue}
              messageStyle={messageStyle}
            />
          </motion.div>,
        )
        index = nextIndex - 1
        continue
      }
    }
    renderedMessages.push(renderMessage(msg, index))
  }

  return (
    <div
      ref={scrollLock.ref}
      onScroll={scrollLock.onScroll}
      onWheel={scrollLock.onWheel}
      onKeyDown={scrollLock.onKeyDown}
      className={`nova-chat-canvas min-h-0 flex-1 space-y-4 overflow-y-auto px-6 py-5 [overflow-anchor:none] ${bottomPaddingClassName}`}
      style={typeof bottomPaddingPx === 'number' ? { paddingBottom: bottomPaddingPx } : undefined}
    >
      {messages.length === 0 && !isStreaming && (
        <div className="flex h-full items-center justify-center">
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 text-center text-sm text-[var(--nova-text-muted)] shadow-[0_14px_34px_rgba(0,0,0,0.22)]">
            {t('chat.empty')}
          </div>
        </div>
      )}

      {renderedMessages}

      {isStreaming && (
        <>
          {visibleActivityContent && (
            <motion.div
              layout="position"
              variants={listItem}
              initial="initial"
              animate="animate"
              transition={{ duration: 0.18, ease: novaEase }}
            >
              <ToolActivityBlock content={visibleActivityContent} />
            </motion.div>
          )}
          {messages.length === 0 && !visibleActivityContent && (
            <div className="flex justify-start">
              <div className="px-1 py-2">
                <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-[var(--nova-text-muted)]" />
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function buildMessageListScrollKey(messages: ChatMessage[], activityContent: string, isStreaming: boolean, bottomPaddingPx?: number) {
  const messageKey = messages.map((message) => [
    message.id || '',
    message.type || '',
    message.role || '',
    message.status || '',
    message.streaming ? 'streaming' : '',
    (message.content || '').length,
    (message.args || '').length,
    (message.result || '').length,
  ].join(':')).join('|')
  return [
    isStreaming ? 'streaming' : 'idle',
    activityContent.length,
    typeof bottomPaddingPx === 'number' ? Math.round(bottomPaddingPx) : '',
    messageKey,
  ].join('|')
}

function MessageWithHoverTime({ message, children }: { message: ChatMessage; children: ReactNode }) {
  if (message.role !== 'user' && message.role !== 'assistant') {
    return <>{children}</>
  }

  return (
    <div className={`nova-message-with-time nova-message-with-time-${message.role}`}>
      {children}
      <MessageHoverTime message={message} />
    </div>
  )
}

function MessageHoverTime({ message }: { message: ChatMessage }) {
  const formatted = formatMessageHoverTime(message.created_at)
  if (!formatted) return null
  const align = message.role === 'user'
    ? 'nova-message-time-user'
    : 'nova-message-time-left'
  return (
    <div className={`nova-message-time ${align}`} aria-label={formatted}>
      {formatted}
    </div>
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

function isTraceMessage(message: ChatMessage) {
  return message.role === 'thinking' || message.role === 'tool_call' || message.role === 'tool_result'
}

function TraceGroup({ messages, highlightDialogue, messageStyle }: { messages: ChatMessage[]; highlightDialogue: boolean; messageStyle?: CSSProperties }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const toolCount = messages.filter((message) => message.role === 'tool_call').length
  const thinkingCount = messages.filter((message) => message.role === 'thinking').length
  const subAgentCount = messages.filter((message) => message.subagent).length
  const label = [
    thinkingCount > 0 ? t('chat.trace.thinking') : '',
    toolCount > 0 ? t('chat.trace.toolCalls', { count: toolCount }) : '',
    subAgentCount > 0 ? t('chat.subagent.label') : '',
  ].filter(Boolean).join(' · ') || t('chat.trace.execution')

  return (
    <div className="flex justify-start">
      <div className="w-full">
        <button
          type="button"
          className="flex items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
          {label}
        </button>
        {expanded && (
          <div className="space-y-2 border-l border-[var(--nova-border)] px-3 py-2">
            {messages.map((message, index) => (
              message.role === 'thinking'
                ? (
                  <div key={message.id || index} className="text-xs leading-relaxed text-[var(--nova-text-muted)] whitespace-pre-wrap">
                    {message.content}
                  </div>
                )
                : (
                  <MessageItem
                    key={message.id || index}
                    message={{ ...message, streaming: false }}
                    highlightDialogue={highlightDialogue}
                    messageStyle={messageStyle}
                  />
                )
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

/** 上下文清理分界线：清理前消息仍可阅读，但不再进入 Agent 上下文。 */
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
