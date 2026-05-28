import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import type { CSSProperties, WheelEvent } from 'react'
import { MessageItem, ToolActivityBlock } from './MessageItem'
import type { ChatMessage } from '@/lib/api'

interface MessageListProps {
  messages: ChatMessage[]
  isStreaming: boolean
  activityContent: string
  highlightDialogue?: boolean
  scrollResetKey?: string
  bottomPaddingClassName?: string
  messageStyle?: CSSProperties
}

/** 消息列表组件，支持流式内容实时展示和自动滚动 */
export function MessageList({ messages, isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', messageStyle }: MessageListProps) {
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

  return (
    <div
      ref={containerRef}
      onScroll={handleContainerScroll}
      onWheel={handleWheel}
      className={`min-h-0 flex-1 space-y-4 overflow-y-auto bg-[var(--nova-surface-2)] px-6 py-5 ${bottomPaddingClassName}`}
    >
      {messages.length === 0 && !isStreaming && (
        <div className="flex h-full items-center justify-center text-sm text-[#858b96]">
          发送消息开始对话，或输入 /help 查看可用命令
        </div>
      )}

      {messages.map((msg, i) => (
        msg.type === 'clear'
          ? <ContextClearDivider key={msg.id || msg.created_at || i} createdAt={msg.created_at} />
          : <MessageItem key={msg.id || i} message={msg} highlightDialogue={highlightDialogue} messageStyle={messageStyle} />
      ))}

      {isStreaming && (
        <>
          {activityContent && (
            <ToolActivityBlock content={activityContent} />
          )}
          {messages.length === 0 && !activityContent && (
            <div className="flex justify-start">
              <div className="px-1 py-2">
                <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-[#858b96]" />
              </div>
            </div>
          )}
        </>
      )}

      <div ref={bottomRef} />
    </div>
  )
}

/** 上下文清理分界线：清理前消息仍可阅读，但不再进入 Agent 上下文。 */
function ContextClearDivider({ createdAt }: { createdAt?: string }) {
  const timeText = createdAt ? new Date(createdAt).toLocaleString() : ''

  return (
    <div className="flex items-center gap-3 py-1" role="separator" aria-label="上下文已清理">
      <div className="h-px flex-1 bg-[#3a3d45]" />
      <div className="rounded-full border border-[#4b5563] bg-[#25262a] px-3 py-1 text-[11px] text-[#aeb4bf]">
        上下文已清理，之前消息不再参与创作Agent上下文{timeText ? ` · ${timeText}` : ''}
      </div>
      <div className="h-px flex-1 bg-[#3a3d45]" />
    </div>
  )
}
