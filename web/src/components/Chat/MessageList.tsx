import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, WheelEvent } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { MessageItem, ToolActivityBlock } from './MessageItem'
import type { ChatMessage } from '@/lib/api'
import { listItem, novaEase } from '@/features/motion/motion-tokens'

interface MessageListProps {
  messages: ChatMessage[]
  isStreaming: boolean
  activityContent: string
  highlightDialogue?: boolean
  scrollResetKey?: string
  bottomPaddingClassName?: string
  messageStyle?: CSSProperties
  collapseTraceBeforeAssistant?: boolean
  onEditMessage?: (message: ChatMessage) => void
  onRegenerateMessage?: (message: ChatMessage) => void
  onSwitchMessageVersion?: (message: ChatMessage, direction: -1 | 1) => void
}

/** 消息列表组件，支持流式内容实时展示和自动滚动 */
export function MessageList({ messages, isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', messageStyle, collapseTraceBeforeAssistant = false, onEditMessage, onRegenerateMessage, onSwitchMessageVersion }: MessageListProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const shouldAutoScrollRef = useRef(true)
  const mountedRef = useRef(false)
  const hasRenderedContentRef = useRef(false)
  const scrollRafRef = useRef<number | null>(null)
  const resetScrollRafRef = useRef<number[]>([])
  const resetScrollTimerRef = useRef<number | null>(null)
  const bottomThreshold = 8

  const isNearBottom = useCallback(() => {
    const el = containerRef.current
    if (!el) return true
    return el.scrollHeight - el.scrollTop - el.clientHeight <= bottomThreshold
  }, [])

  const forceScrollToBottom = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [])

  const forceScrollToBottomIfAllowed = useCallback(() => {
    if (!shouldAutoScrollRef.current) return
    forceScrollToBottom()
  }, [forceScrollToBottom])

  const cancelResetScroll = useCallback(() => {
    for (const id of resetScrollRafRef.current) {
      cancelAnimationFrame(id)
    }
    resetScrollRafRef.current = []
    if (resetScrollTimerRef.current !== null) {
      window.clearTimeout(resetScrollTimerRef.current)
      resetScrollTimerRef.current = null
    }
  }, [])

  const scheduleForceScrollToBottom = useCallback(() => {
    cancelResetScroll()
    forceScrollToBottom()
    resetScrollRafRef.current.push(requestAnimationFrame(() => {
      forceScrollToBottomIfAllowed()
      resetScrollRafRef.current.push(requestAnimationFrame(forceScrollToBottomIfAllowed))
    }))
    resetScrollTimerRef.current = window.setTimeout(forceScrollToBottomIfAllowed, 80)
  }, [cancelResetScroll, forceScrollToBottom, forceScrollToBottomIfAllowed])

  const cancelPendingAutoScroll = useCallback(() => {
    if (scrollRafRef.current !== null) {
      cancelAnimationFrame(scrollRafRef.current)
      scrollRafRef.current = null
    }
    cancelResetScroll()
  }, [cancelResetScroll])

  useLayoutEffect(() => {
    hasRenderedContentRef.current = false
    shouldAutoScrollRef.current = true
    scheduleForceScrollToBottom()
    return cancelResetScroll
  }, [cancelResetScroll, scheduleForceScrollToBottom, scrollResetKey])

  // 自动滚动到底部（仅在用户未上滑时）
  useLayoutEffect(() => {
    const hasContent = messages.length > 0 || activityContent.length > 0 || isStreaming
    const shouldJumpToBottom = hasContent && !hasRenderedContentRef.current
    if (shouldJumpToBottom) {
      shouldAutoScrollRef.current = true
      scheduleForceScrollToBottom()
      hasRenderedContentRef.current = true
      mountedRef.current = true
      return
    }

    if (shouldAutoScrollRef.current) {
      if (scrollRafRef.current !== null) {
        cancelAnimationFrame(scrollRafRef.current)
      }
      scrollRafRef.current = requestAnimationFrame(() => {
        scrollRafRef.current = null
        if (!shouldAutoScrollRef.current) return
        bottomRef.current?.scrollIntoView({ behavior: isStreaming ? 'instant' : (mountedRef.current ? 'smooth' : 'instant') })
        if (hasContent) {
          hasRenderedContentRef.current = true
        }
      })
    }
    mountedRef.current = true
  }, [messages, activityContent, isStreaming, scheduleForceScrollToBottom])

  /** 主列表：用户上滑时暂停自动滚动，回到底部后恢复。 */
  const handleContainerScroll = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    shouldAutoScrollRef.current = isNearBottom()
    if (!shouldAutoScrollRef.current) cancelPendingAutoScroll()
  }, [cancelPendingAutoScroll, isNearBottom])

  const handleWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (event.deltaY < 0) {
      shouldAutoScrollRef.current = false
      cancelPendingAutoScroll()
    }
  }, [cancelPendingAutoScroll])

  useEffect(() => () => {
    if (scrollRafRef.current !== null) {
      cancelAnimationFrame(scrollRafRef.current)
    }
    cancelResetScroll()
  }, [])

  // 新一轮对话开始时重置跟随状态
  useEffect(() => {
    if (isStreaming) {
      // 检查当前是否在底部附近，如果是则确保跟随
      if (isNearBottom()) {
        shouldAutoScrollRef.current = true
      }
    }
  }, [isNearBottom, isStreaming])

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
          : <MessageItem message={msg} highlightDialogue={highlightDialogue} messageStyle={messageStyle} onEdit={isStreaming ? undefined : onEditMessage} onRegenerate={isStreaming ? undefined : onRegenerateMessage} onSwitchVersion={isStreaming ? undefined : onSwitchMessageVersion} />}
      </motion.div>
    )
  }

  const renderedMessages = []
  for (let index = 0; index < messages.length; index += 1) {
    const msg = messages[index]
    if (msg.role === 'token_usage') {
      continue
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
      ref={containerRef}
      onScroll={handleContainerScroll}
      onWheel={handleWheel}
      className={`nova-chat-canvas min-h-0 flex-1 space-y-4 overflow-y-auto px-6 py-5 ${bottomPaddingClassName}`}
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
          {activityContent && (
            <motion.div
              layout="position"
              variants={listItem}
              initial="initial"
              animate="animate"
              transition={{ duration: 0.18, ease: novaEase }}
            >
              <ToolActivityBlock content={activityContent} />
            </motion.div>
          )}
          {messages.length === 0 && !activityContent && (
            <div className="flex justify-start">
              <div className="px-1 py-2">
                <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-[var(--nova-text-muted)]" />
              </div>
            </div>
          )}
        </>
      )}

      <div ref={bottomRef} />
    </div>
  )
}

function isTraceMessage(message: ChatMessage) {
  return message.role === 'thinking' || message.role === 'tool_call' || message.role === 'tool_result'
}

function TraceGroup({ messages, highlightDialogue, messageStyle }: { messages: ChatMessage[]; highlightDialogue: boolean; messageStyle?: CSSProperties }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const toolCount = messages.filter((message) => message.role === 'tool_call').length
  const thinkingCount = messages.filter((message) => message.role === 'thinking').length
  const label = [
    thinkingCount > 0 ? t('chat.trace.thinking') : '',
    toolCount > 0 ? t('chat.trace.toolCalls', { count: toolCount }) : '',
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
