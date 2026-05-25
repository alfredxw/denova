import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { MessageItem, ToolActivityBlock } from './MessageItem'
import type { ChatMessage } from '@/lib/api'

interface MessageListProps {
  messages: ChatMessage[]
  isStreaming: boolean
  activityContent: string
}

/** 消息列表组件，支持流式内容实时展示和自动滚动 */
export function MessageList({ messages, isStreaming, activityContent }: MessageListProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const shouldAutoScrollRef = useRef(true)
  const mountedRef = useRef(false)
  const hasRenderedContentRef = useRef(false)
  const scrollRafRef = useRef<number | null>(null)

  /** 主列表：用户上滑时暂停自动滚动，回到底部后恢复。 */
  const handleContainerScroll = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    shouldAutoScrollRef.current = distanceToBottom < 50
  }, [])

  // 自动滚动到底部（仅在用户未上滑时）
  useLayoutEffect(() => {
    if (shouldAutoScrollRef.current) {
      if (scrollRafRef.current !== null) {
        cancelAnimationFrame(scrollRafRef.current)
      }
      const hasContent = messages.length > 0 || activityContent.length > 0 || isStreaming
      const shouldJumpToBottom = hasContent && !hasRenderedContentRef.current
      scrollRafRef.current = requestAnimationFrame(() => {
        scrollRafRef.current = null
        const el = containerRef.current
        if (shouldJumpToBottom && el) {
          el.scrollTop = el.scrollHeight
        } else {
          bottomRef.current?.scrollIntoView({ behavior: isStreaming ? 'instant' : (mountedRef.current ? 'smooth' : 'instant') })
        }
        if (hasContent) {
          hasRenderedContentRef.current = true
        }
      })
    }
    mountedRef.current = true
  }, [messages, activityContent, isStreaming])

  useEffect(() => () => {
    if (scrollRafRef.current !== null) {
      cancelAnimationFrame(scrollRafRef.current)
    }
  }, [])

  // 新一轮对话开始时重置跟随状态
  useEffect(() => {
    if (isStreaming) {
      // 检查当前是否在底部附近，如果是则确保跟随
      const el = containerRef.current
      if (el) {
        const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight
        if (distanceToBottom < 50) {
          shouldAutoScrollRef.current = true
        }
      }
    }
  }, [isStreaming])

  return (
    <div
      ref={containerRef}
      onScroll={handleContainerScroll}
      className="flex-1 overflow-y-auto space-y-3 bg-[#202124] px-4 py-4"
    >
      {messages.length === 0 && !isStreaming && (
        <div className="flex h-full items-center justify-center text-sm text-[#858b96]">
          发送消息开始对话，或输入 /help 查看可用命令
        </div>
      )}

      {messages.map((msg, i) => (
        msg.type === 'clear'
          ? <ContextClearDivider key={msg.id || msg.created_at || i} createdAt={msg.created_at} />
          : <MessageItem key={msg.id || i} message={msg} />
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
