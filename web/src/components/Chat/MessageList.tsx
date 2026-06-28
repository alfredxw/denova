import { useCallback, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Virtuoso } from 'react-virtuoso'
import type { Components, ContextProp } from 'react-virtuoso'
import { MessageItem, ToolActivityBlock } from './MessageItem'
import type { ChapterIllustration, ChatMessage } from '@/lib/api'
import { listItem, novaEase } from '@/features/motion/motion-tokens'
import { buildSubAgentProgressMessage, isSubAgentTimelineMessage, subAgentSessionKey } from './subagent-session'
import { VIRTUOSO_BOTTOM_THRESHOLD, useVirtuosoBottomLock } from './useVirtuosoBottomLock'

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
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (message: ChatMessage) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
}

type ChatListItem =
  | { kind: 'empty'; key: string }
  | { kind: 'typing'; key: string }
  | { kind: 'activity'; key: string; content: string }
  | { kind: 'clear'; key: string; createdAt?: string }
  | { kind: 'message'; key: string; message: ChatMessage; sourceIndex: number }
  | { kind: 'trace'; key: string; messages: ChatMessage[] }

const MESSAGE_LIST_OVERSCAN = { main: 520, reverse: 260 }
const MESSAGE_LIST_INCREASE_VIEWPORT_BY = { top: 420, bottom: 900 }
const MESSAGE_LIST_COMPONENTS: Components<ChatListItem, MessageListVirtuosoContext> = {
  Header: MessageListHeader,
  Footer: MessageListFooter,
}

interface MessageListVirtuosoContext {
  bottomPaddingClassName: string
  bottomPaddingPx?: number
}

/** 消息列表组件，支持流式内容实时展示和自动滚动 */
export function MessageList({ messages, isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', bottomPaddingPx, messageStyle, collapseTraceBeforeAssistant = false, onEditMessage, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey }: MessageListProps) {
  const { t } = useTranslation()
  const hasRunningContextCompaction = messages.some((message) => message.role === 'context_compaction' && message.status === 'running')
  const visibleActivityContent = hasRunningContextCompaction ? '' : activityContent
  const listItems = useMemo(
    () => buildChatListItems({
      messages,
      isStreaming,
      visibleActivityContent,
      collapseTraceBeforeAssistant,
      groupSubAgentTimeline: Boolean(onOpenSubAgentSession),
    }),
    [collapseTraceBeforeAssistant, isStreaming, messages, onOpenSubAgentSession, visibleActivityContent],
  )
  const scrollContentKey = useMemo(
    () => buildMessageListScrollKey(listItems, isStreaming, bottomPaddingPx),
    [bottomPaddingPx, isStreaming, listItems],
  )
  const scrollLock = useVirtuosoBottomLock({
    resetKey: scrollResetKey,
    contentKey: scrollContentKey,
    itemCount: listItems.length,
  })
  const virtuosoContext = useMemo<MessageListVirtuosoContext>(
    () => ({ bottomPaddingClassName, bottomPaddingPx }),
    [bottomPaddingClassName, bottomPaddingPx],
  )

  const itemContent = useCallback((index: number, item?: ChatListItem) => {
    const resolvedItem = item || listItems[index]
    if (!resolvedItem) return null
    return (
      <ChatListRow
        item={resolvedItem}
        isStreaming={isStreaming}
        highlightDialogue={highlightDialogue}
        messageStyle={messageStyle}
        onEditMessage={onEditMessage}
        onRegenerateMessage={onRegenerateMessage}
        onSwitchMessageVersion={onSwitchMessageVersion}
        onOpenSubAgentSession={onOpenSubAgentSession}
        onInsertIllustration={onInsertIllustration}
        onGenerateInteractiveImage={onGenerateInteractiveImage}
        generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
        activeSubAgentSessionKey={activeSubAgentSessionKey}
      />
    )
  }, [activeSubAgentSessionKey, generatingInteractiveImageTurnId, highlightDialogue, isStreaming, listItems, messageStyle, onEditMessage, onGenerateInteractiveImage, onInsertIllustration, onOpenSubAgentSession, onRegenerateMessage, onSwitchMessageVersion])

  return (
    <Virtuoso
      ref={scrollLock.virtuosoRef}
      scrollerRef={scrollLock.scrollerRef}
      onScroll={scrollLock.onScroll}
      onWheel={scrollLock.onWheel}
      onKeyDown={scrollLock.onKeyDown}
      atBottomStateChange={scrollLock.onAtBottomStateChange}
      atBottomThreshold={VIRTUOSO_BOTTOM_THRESHOLD}
      followOutput={scrollLock.followOutput}
      initialItemCount={Math.min(listItems.length, 40)}
      data={listItems}
      context={virtuosoContext}
      components={MESSAGE_LIST_COMPONENTS}
      computeItemKey={(index, item) => item?.key || listItems[index]?.key || `chat-item-${index}`}
      itemContent={itemContent}
      overscan={MESSAGE_LIST_OVERSCAN}
      increaseViewportBy={MESSAGE_LIST_INCREASE_VIEWPORT_BY}
      className="nova-chat-canvas min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none]"
      aria-label={t('common.messages', { count: messages.length })}
    />
  )
}

function MessageListHeader() {
  return <div aria-hidden="true" className="h-5 shrink-0" />
}

function MessageListFooter({ context }: ContextProp<MessageListVirtuosoContext>) {
  const hasMeasuredPadding = typeof context.bottomPaddingPx === 'number'
  return (
    <div
      aria-hidden="true"
      data-nova-chat-bottom-spacer
      className={hasMeasuredPadding ? 'shrink-0' : `shrink-0 ${context.bottomPaddingClassName}`}
      style={hasMeasuredPadding ? { height: context.bottomPaddingPx } : undefined}
    />
  )
}

function ChatListRow({ item, isStreaming, highlightDialogue, messageStyle, onEditMessage, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey }: {
  item: ChatListItem
  isStreaming: boolean
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  onEditMessage?: (message: ChatMessage) => void
  onRegenerateMessage?: (message: ChatMessage) => void
  onSwitchMessageVersion?: (message: ChatMessage, direction: -1 | 1) => void
  onOpenSubAgentSession?: (message: ChatMessage) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (message: ChatMessage) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
}) {
  const { t } = useTranslation()

  return (
    <motion.div
      data-nova-chat-item={item.kind}
      className="min-w-0 px-6 pb-4 last:pb-0"
      variants={listItem}
      initial="initial"
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
        <ToolActivityBlock content={item.content} />
      ) : item.kind === 'clear' ? (
        <ContextClearDivider createdAt={item.createdAt} />
      ) : item.kind === 'trace' ? (
        <TraceGroup
          messages={item.messages}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onInsertIllustration={onInsertIllustration}
          onGenerateInteractiveImage={onGenerateInteractiveImage}
        />
      ) : (
        <MessageItem
          message={item.message}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onEdit={isStreaming ? undefined : onEditMessage}
          onRegenerate={isStreaming ? undefined : onRegenerateMessage}
          onSwitchVersion={isStreaming ? undefined : onSwitchMessageVersion}
          onOpenSubAgentSession={onOpenSubAgentSession}
          onInsertIllustration={onInsertIllustration}
          onGenerateInteractiveImage={isStreaming ? undefined : onGenerateInteractiveImage}
          generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
        />
      )}
    </motion.div>
  )
}

function buildChatListItems({ messages, isStreaming, visibleActivityContent, collapseTraceBeforeAssistant, groupSubAgentTimeline }: { messages: ChatMessage[]; isStreaming: boolean; visibleActivityContent: string; collapseTraceBeforeAssistant: boolean; groupSubAgentTimeline: boolean }): ChatListItem[] {
  const items: ChatListItem[] = []
  if (messages.length === 0 && !isStreaming) {
    items.push({ kind: 'empty', key: 'empty' })
    return items
  }

  for (let index = 0; index < messages.length; index += 1) {
    const msg = messages[index]
    if (msg.role === 'token_usage') {
      continue
    }
    if (groupSubAgentTimeline && isSubAgentTimelineMessage(msg)) {
      const key = subAgentSessionKey(msg)
      const group: ChatMessage[] = []
      let nextIndex = index
      while (nextIndex < messages.length && isSubAgentTimelineMessage(messages[nextIndex]) && subAgentSessionKey(messages[nextIndex]) === key) {
        group.push(messages[nextIndex])
        nextIndex += 1
      }
      const progress = buildSubAgentProgressMessage(group)
      if (progress) {
        items.push({ kind: 'message', key: messageItemKey(progress, index), message: progress, sourceIndex: index })
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
        items.push({ kind: 'trace', key: `trace-${traceMessages[0].id || index}`, messages: traceMessages })
        index = nextIndex - 1
        continue
      }
    }
    if (msg.type === 'clear') {
      items.push({ kind: 'clear', key: messageItemKey(msg, index), createdAt: msg.created_at })
      continue
    }
    items.push({ kind: 'message', key: messageItemKey(msg, index), message: msg, sourceIndex: index })
  }

  if (isStreaming) {
    if (visibleActivityContent) {
      items.push({ kind: 'activity', key: `activity-${visibleActivityContent.length}`, content: visibleActivityContent })
    } else if (messages.length === 0) {
      items.push({ kind: 'typing', key: 'typing' })
    }
  }

  return items
}

function buildMessageListScrollKey(items: ChatListItem[], isStreaming: boolean, bottomPaddingPx?: number) {
  const itemKey = items.map((item) => {
    if (item.kind === 'message') {
      const message = item.message
      return [
        item.key,
        message.type || '',
        message.role || '',
        message.status || '',
        message.streaming ? 'streaming' : '',
        (message.content || '').length,
        (message.args || '').length,
        (message.result || '').length,
        message.illustration?.image_path || '',
        message.interactive_image?.image_path || '',
        message.interactive_images?.map((image) => image.image_path).join(',') || '',
        message.interactive_image_status || '',
      ].join(':')
    }
    if (item.kind === 'trace') {
      return `${item.key}:${item.messages.length}:${item.messages.map((message) => `${message.id || ''}:${message.status || ''}:${(message.content || '').length}:${(message.result || '').length}`).join(',')}`
    }
    if (item.kind === 'activity') return `${item.key}:${item.content.length}`
    return item.key
  }).join('|')
  return [
    isStreaming ? 'streaming' : 'idle',
    typeof bottomPaddingPx === 'number' ? Math.round(bottomPaddingPx) : '',
    itemKey,
  ].join('|')
}

function messageItemKey(message: ChatMessage, index: number) {
  return `${message.type === 'clear' ? 'clear' : 'message'}-${message.id || message.created_at || index}`
}

function isTraceMessage(message: ChatMessage) {
  if (message.name === 'generate_interactive_image' || message.interactive_image) return false
  return message.role === 'thinking' || message.role === 'tool_call' || message.role === 'tool_result'
}

function TraceGroup({ messages, highlightDialogue, messageStyle, onInsertIllustration, onGenerateInteractiveImage }: { messages: ChatMessage[]; highlightDialogue: boolean; messageStyle?: CSSProperties; onInsertIllustration?: (illustration: ChapterIllustration) => void; onGenerateInteractiveImage?: (message: ChatMessage) => void }) {
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
                    onInsertIllustration={onInsertIllustration}
                    onGenerateInteractiveImage={onGenerateInteractiveImage}
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
