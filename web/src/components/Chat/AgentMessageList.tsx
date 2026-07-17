import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Virtuoso } from 'react-virtuoso'
import type { Components, ContextProp, ListItem, ListRange } from 'react-virtuoso'
import type { ChapterIllustration, ChatMessage } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import {
  agentSubAgentSessionKey,
  agentViewToRenderMessage,
  agentViewContent,
  agentViewNavigationAnchor,
  agentViewStableKey,
  buildAgentMessageViews,
  isAgentSubAgentTimelineView,
  isAgentTraceView,
  type AgentMessageView,
  type AgentPartRef,
} from '@/lib/agent-message-view'
import { listItem, novaEase } from '@/features/motion/motion-tokens'
import { buildSubAgentProgressMessage } from './subagent-session'
import { VIRTUOSO_BOTTOM_THRESHOLD, useVirtuosoBottomLock, type ScrollElementBottomIntoViewOptions } from './useVirtuosoBottomLock'
import { ScrollToBottomButton } from './ScrollToBottomButton'
import { AgentMessageItem } from './AgentMessageItem'
import { AgentActivityShimmer, MessageItem } from './MessageItem'

interface MessageListProps {
  messages: AgentUIMessage[]
  isStreaming: boolean
  activityContent: string
  highlightDialogue?: boolean
  scrollResetKey?: string
  bottomPaddingClassName?: string
  bottomPaddingPx?: number
  afterContent?: ReactNode
  afterContentKey?: string
  timelineAttachments?: AgentTimelineAttachment[]
  messageStyle?: CSSProperties
  collapseTraceBeforeAssistant?: boolean
  onEditMessage?: (view: AgentMessageView) => void
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  onSubmitPlanQuestion?: (ref: AgentPartRef, content: string, preview: string) => void
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  turnScrollRequest?: TurnScrollRequest
  onVisibleTurnAnchorChange?: (anchorId: string) => void
}

/** Durable UI attached to the last visible row of one Agent run. */
export interface AgentTimelineAttachment {
  id: string
  runId: string
  content: ReactNode
}

export interface TurnScrollRequest {
  anchorId: string
  requestId: number
}

type AgentChatListItem =
  | { kind: 'empty'; key: string }
  | { kind: 'typing'; key: string }
  | { kind: 'activity'; key: string; content: string }
  | { kind: 'clear'; key: string; createdAt?: string }
  | { kind: 'message'; key: string; view: AgentMessageView; sourceIndex: number }
  | { kind: 'legacy-message'; key: string; message: ChatMessage; sourceIndex: number; openView?: AgentMessageView }
  | { kind: 'trace'; key: string; views: AgentMessageView[]; activeStreamingTrace: boolean }
  | { kind: 'attachment'; key: string; runId: string; content: ReactNode }

const MESSAGE_LIST_OVERSCAN = { main: 520, reverse: 260 }
const MESSAGE_LIST_INCREASE_VIEWPORT_BY = { top: 420, bottom: 900 }
const MESSAGE_LIST_COMPONENTS: Components<AgentChatListItem, MessageListVirtuosoContext> = {
  Header: MessageListHeader,
  Footer: MessageListFooter,
}

interface MessageListVirtuosoContext {
  bottomPaddingClassName: string
  bottomPaddingPx?: number
  afterContent?: ReactNode
}

export function MessageList({ messages, isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', bottomPaddingPx, afterContent, afterContentKey, timelineAttachments = [], messageStyle, collapseTraceBeforeAssistant = false, onEditMessage, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onSubmitPlanQuestion, onApprovePlan, onContinuePlan, onExitPlanMode, onOpenTrace, turnScrollRequest, onVisibleTurnAnchorChange }: MessageListProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const lastVisibleTurnAnchorRef = useRef('')
  const lastTurnScrollRequestIdRef = useRef<number | null>(null)
  const views = useMemo(() => buildAgentMessageViews(messages), [messages])
  const hasRunningContextCompaction = views.some((view) => view.kind === 'context-compaction' && view.status === 'running')
  const hasActiveTrace = views.some((view) => isAgentTraceView(view) && (view.streaming || view.status === 'running'))
  // 真实 thinking / tool 行已经承担进度展示；保留额外 activity 行会在 trace 增高时
  // 先被文档流推走、再被底部锁定拉回，产生持续抖动。
  const visibleActivityContent = hasRunningContextCompaction || hasActiveTrace ? '' : activityContent
  const listItems = useMemo(
    () => buildAgentChatListItems({
      views,
      isStreaming,
      visibleActivityContent,
      collapseTraceBeforeAssistant,
      groupSubAgentTimeline: Boolean(onOpenSubAgentSession),
      timelineAttachments,
    }),
    [collapseTraceBeforeAssistant, isStreaming, onOpenSubAgentSession, timelineAttachments, views, visibleActivityContent],
  )
  const scrollContentKey = useMemo(
    () => buildMessageListScrollKey(listItems, bottomPaddingPx, afterContent ? afterContentKey || 'after-content' : ''),
    [afterContent, afterContentKey, bottomPaddingPx, listItems],
  )
  const resolveMessageScroller = useCallback(
    () => containerRef.current?.querySelector<HTMLElement>('.nova-chat-canvas') || null,
    [],
  )
  const scrollLock = useVirtuosoBottomLock({
    resetKey: scrollResetKey,
    contentKey: scrollContentKey,
    itemCount: listItems.length,
    resolveScroller: resolveMessageScroller,
  })
  const latestPlanCardAnchor = useMemo(
    () => latestPlanCardBottomAnchorTarget(listItems),
    [listItems],
  )
  const lastPlanCardAnchorKeyRef = useRef<string | null>(null)
  const virtuosoContext = useMemo<MessageListVirtuosoContext>(
    () => ({ bottomPaddingClassName, bottomPaddingPx, afterContent }),
    [afterContent, bottomPaddingClassName, bottomPaddingPx],
  )
  const scrollButtonBottomOffset = typeof bottomPaddingPx === 'number' ? Math.max(24, bottomPaddingPx + 12) : 24
  const anchorLatestPlanCardBottom = useCallback(() => {
    if (!latestPlanCardAnchor) return
    const bottomInsetPx = Math.max(0, bottomPaddingPx || 0)
    scheduleChatRowBottomAnchor(containerRef.current, latestPlanCardAnchor.rowKey, bottomInsetPx, scrollLock.scrollElementBottomIntoView, { observeResize: false })
  }, [bottomPaddingPx, latestPlanCardAnchor, scrollLock.scrollElementBottomIntoView])

  useEffect(() => {
    const bottomInsetPx = Math.max(0, bottomPaddingPx || 0)
    const anchorKey = latestPlanCardAnchor ? `${latestPlanCardAnchor.anchorKey}:${Math.round(bottomInsetPx)}` : ''
    if (lastPlanCardAnchorKeyRef.current === null) {
      lastPlanCardAnchorKeyRef.current = anchorKey
      if (latestPlanCardAnchor && isStreaming) {
        return scheduleChatRowBottomAnchor(containerRef.current, latestPlanCardAnchor.rowKey, bottomInsetPx, scrollLock.scrollElementBottomIntoView)
      }
      return undefined
    }
    if (latestPlanCardAnchor && anchorKey !== lastPlanCardAnchorKeyRef.current) {
      const cancelAnchor = scheduleChatRowBottomAnchor(containerRef.current, latestPlanCardAnchor.rowKey, bottomInsetPx, scrollLock.scrollElementBottomIntoView)
      lastPlanCardAnchorKeyRef.current = anchorKey
      return cancelAnchor
    }
    lastPlanCardAnchorKeyRef.current = anchorKey
    return undefined
  }, [bottomPaddingPx, isStreaming, latestPlanCardAnchor, scrollLock.scrollElementBottomIntoView])

  useEffect(() => {
    if (!turnScrollRequest?.anchorId) return
    if (lastTurnScrollRequestIdRef.current === turnScrollRequest.requestId) return
    lastTurnScrollRequestIdRef.current = turnScrollRequest.requestId
    const targetIndex = listItems.findIndex((item) => chatListItemNavigationAnchor(item) === turnScrollRequest.anchorId)
    if (targetIndex < 0) return
    scrollLock.scrollToIndex(targetIndex, { align: 'start', behavior: 'smooth' })
  }, [listItems, scrollLock, turnScrollRequest])

  const notifyVisibleTurnAnchor = useCallback((startIndex: number, endIndex: number) => {
    if (!onVisibleTurnAnchorChange) return
    for (let index = Math.max(0, startIndex); index <= Math.min(listItems.length - 1, endIndex); index += 1) {
      const anchorId = chatListItemNavigationAnchor(listItems[index])
      if (!anchorId) continue
      if (lastVisibleTurnAnchorRef.current === anchorId) return
      lastVisibleTurnAnchorRef.current = anchorId
      onVisibleTurnAnchorChange(anchorId)
      return
    }
  }, [listItems, onVisibleTurnAnchorChange])

  const handleRangeChanged = useCallback((range: ListRange) => {
    notifyVisibleTurnAnchor(range.startIndex, range.endIndex)
  }, [notifyVisibleTurnAnchor])

  const handleItemsRendered = useCallback((items: ListItem<AgentChatListItem>[]) => {
    const firstIndex = items[0]?.index
    const lastIndex = items[items.length - 1]?.index
    if (firstIndex === undefined || lastIndex === undefined) return
    notifyVisibleTurnAnchor(firstIndex, lastIndex)
  }, [notifyVisibleTurnAnchor])

  const itemContent = useCallback((index: number, item?: AgentChatListItem) => {
    const resolvedItem = item || listItems[index]
    if (!resolvedItem) return null
    return (
      <AgentChatListRow
        item={resolvedItem}
        isLast={index === listItems.length - 1}
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
        onSubmitPlanQuestion={onSubmitPlanQuestion}
        onApprovePlan={onApprovePlan}
        onContinuePlan={onContinuePlan}
        onExitPlanMode={onExitPlanMode}
        onOpenTrace={onOpenTrace}
        onPlanCardLayoutChange={anchorLatestPlanCardBottom}
      />
    )
  }, [activeSubAgentSessionKey, anchorLatestPlanCardBottom, generatingInteractiveImageTurnId, highlightDialogue, isStreaming, listItems, messageStyle, onApprovePlan, onContinuePlan, onEditMessage, onExitPlanMode, onGenerateInteractiveImage, onInsertIllustration, onOpenSubAgentSession, onOpenTrace, onRegenerateMessage, onSubmitPlanQuestion, onSwitchMessageVersion])

  return (
    <div ref={containerRef} className="relative flex min-h-0 flex-1 flex-col">
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
        computeItemKey={(index, item) => item?.key || listItems[index]?.key || `agent-chat-item-${index}`}
        itemContent={itemContent}
        rangeChanged={handleRangeChanged}
        itemsRendered={handleItemsRendered}
        overscan={MESSAGE_LIST_OVERSCAN}
        increaseViewportBy={MESSAGE_LIST_INCREASE_VIEWPORT_BY}
        className="nova-chat-canvas min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none]"
        aria-label={t('common.messages', { count: messages.length })}
      />
      <ScrollToBottomButton
        visible={scrollLock.isAwayFromBottom}
        onClick={scrollLock.scrollToBottom}
        bottomOffsetPx={scrollButtonBottomOffset}
        rightOffsetPx={24}
      />
    </div>
  )
}

function MessageListHeader() {
  return <div aria-hidden="true" className="h-5 shrink-0" />
}

function MessageListFooter({ context }: ContextProp<MessageListVirtuosoContext>) {
  const hasMeasuredPadding = typeof context.bottomPaddingPx === 'number'
  return (
    <>
      {context.afterContent ? <div data-nova-chat-after-content className="px-3 pb-4 sm:px-6">{context.afterContent}</div> : null}
      <div
        aria-hidden="true"
        data-nova-chat-bottom-spacer
        className={hasMeasuredPadding ? 'shrink-0' : `shrink-0 ${context.bottomPaddingClassName}`}
        style={hasMeasuredPadding ? { height: context.bottomPaddingPx } : undefined}
      />
    </>
  )
}

function AgentChatListRow({ item, isLast, isStreaming, highlightDialogue, messageStyle, onEditMessage, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onSubmitPlanQuestion, onApprovePlan, onContinuePlan, onExitPlanMode, onOpenTrace, onPlanCardLayoutChange }: {
  item: AgentChatListItem
  isLast: boolean
  isStreaming: boolean
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  onEditMessage?: (view: AgentMessageView) => void
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  onSubmitPlanQuestion?: (ref: AgentPartRef, content: string, preview: string) => void
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  onPlanCardLayoutChange?: () => void
}) {
  const { t } = useTranslation()
  const turnAnchor = chatListItemNavigationAnchor(item)

  return (
    <motion.div
      data-nova-chat-item={item.kind}
      data-nova-chat-row-key={item.key}
      data-nova-chat-turn-anchor={turnAnchor}
      className={`min-w-0 px-6 ${isLast ? 'pb-0' : 'pb-4'}`}
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
        <AgentActivityShimmer content={item.content} />
      ) : item.kind === 'clear' ? (
        <ContextClearDivider createdAt={item.createdAt} />
      ) : item.kind === 'trace' ? (
        <TraceGroup
          views={item.views}
          activeStreamingTrace={item.activeStreamingTrace}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onInsertIllustration={onInsertIllustration}
          onGenerateInteractiveImage={onGenerateInteractiveImage}
          onOpenTrace={onOpenTrace}
        />
      ) : item.kind === 'attachment' ? (
        item.content
      ) : item.kind === 'legacy-message' ? (
        <MessageItem
          message={item.message}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onOpenSubAgentSession={item.openView && onOpenSubAgentSession ? () => onOpenSubAgentSession(item.openView as AgentMessageView) : undefined}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          onOpenTrace={onOpenTrace}
        />
      ) : (
        <AgentMessageItem
          view={item.view}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onEditMessage={isStreaming ? undefined : onEditMessage}
          onRegenerateMessage={isStreaming ? undefined : onRegenerateMessage}
          onSwitchMessageVersion={isStreaming ? undefined : onSwitchMessageVersion}
          onOpenSubAgentSession={onOpenSubAgentSession}
          onInsertIllustration={onInsertIllustration}
          onGenerateInteractiveImage={isStreaming ? undefined : onGenerateInteractiveImage}
          generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          onSubmitPlanQuestion={isStreaming ? undefined : onSubmitPlanQuestion}
          onApprovePlan={isStreaming ? undefined : onApprovePlan}
          onContinuePlan={isStreaming ? undefined : onContinuePlan}
          onExitPlanMode={isStreaming ? undefined : onExitPlanMode}
          onOpenTrace={onOpenTrace}
          onPlanCardLayoutChange={onPlanCardLayoutChange}
        />
      )}
    </motion.div>
  )
}

function buildAgentChatListItems({ views, isStreaming, visibleActivityContent, collapseTraceBeforeAssistant, groupSubAgentTimeline, timelineAttachments }: { views: AgentMessageView[]; isStreaming: boolean; visibleActivityContent: string; collapseTraceBeforeAssistant: boolean; groupSubAgentTimeline: boolean; timelineAttachments: AgentTimelineAttachment[] }): AgentChatListItem[] {
  const items: AgentChatListItem[] = []
  if (views.length === 0 && !isStreaming) {
    items.push({ kind: 'empty', key: 'empty' })
    return items
  }

  for (let index = 0; index < views.length; index += 1) {
    const view = views[index]
    if (view.kind === 'token-usage') continue
    if (groupSubAgentTimeline && isAgentSubAgentTimelineView(view)) {
      const key = agentSubAgentSessionKey(view)
      const group: AgentMessageView[] = []
      let nextIndex = index
      while (nextIndex < views.length && isAgentSubAgentTimelineView(views[nextIndex]) && agentSubAgentSessionKey(views[nextIndex]) === key) {
        group.push(views[nextIndex])
        nextIndex += 1
      }
      const progress = buildSubAgentProgressMessage(group.map(item => agentViewToRenderMessage(item)).filter((item): item is ChatMessage => Boolean(item)))
      if (progress) {
        items.push({ kind: 'legacy-message', key: `subagent-${key || index}`, message: progress, sourceIndex: index, openView: group[0] })
        index = nextIndex - 1
        continue
      }
    }
    if (collapseTraceBeforeAssistant && isAgentTraceView(view)) {
      const traceViews: AgentMessageView[] = []
      let nextIndex = index
      while (nextIndex < views.length && isAgentTraceView(views[nextIndex])) {
        traceViews.push(views[nextIndex])
        nextIndex += 1
      }
      const nextView = views[nextIndex]
      const hasAssistantAfterTrace = nextView?.kind === 'assistant' && agentViewContent(nextView).trim()
      const activeStreamingTrace = isActiveStreamingTrace(views, nextIndex, isStreaming)
      if (traceViews.length > 0 && (hasAssistantAfterTrace || activeStreamingTrace)) {
        items.push({ kind: 'trace', key: `trace-${traceViews[0].partId || index}`, views: traceViews, activeStreamingTrace })
        index = nextIndex - 1
        continue
      }
    }
    if (view.kind === 'clear') {
      items.push({ kind: 'clear', key: agentMessageItemKey(view, index), createdAt: readString(view.data.created_at) || view.metadata.created_at })
      continue
    }
    items.push({ kind: 'message', key: agentMessageItemKey(view, index), view, sourceIndex: index })
  }

  for (const attachment of timelineAttachments) {
    const runId = attachment.runId.trim()
    if (!runId) continue
    let insertAt = -1
    for (let index = items.length - 1; index >= 0; index -= 1) {
      if (chatListItemRunID(items[index]) === runId) {
        insertAt = index + 1
        break
      }
    }
    if (insertAt < 0) continue
    while (insertAt < items.length && items[insertAt]?.kind === 'attachment') insertAt += 1
    items.splice(insertAt, 0, {
      kind: 'attachment',
      key: `attachment-${attachment.id}`,
      runId,
      content: attachment.content,
    })
  }

  if (isStreaming) {
    if (visibleActivityContent) {
      items.push({ kind: 'activity', key: `activity-${visibleActivityContent.length}`, content: visibleActivityContent })
    } else if (views.length === 0) {
      items.push({ kind: 'typing', key: 'typing' })
    }
  }

  return items
}

function buildMessageListScrollKey(items: AgentChatListItem[], bottomPaddingPx?: number, afterContentKey = '') {
  const itemKey = items.map((item) => {
    if (item.kind === 'message') {
      const view = item.view
      return [
        item.key,
        view.kind,
        view.status || '',
        view.streaming ? 'streaming' : '',
        agentViewContent(view).length,
        readString(view.data.plan_action),
        readString(view.data.thinking_preview).length,
        stringifyLength(view.input),
        stringifyLength(view.output),
        readImagePath(view.data),
        readString(view.metadata.navigation_turn_id),
      ].join(':')
    }
    if (item.kind === 'legacy-message') {
      const message = item.message
      return `${item.key}:${message.role || ''}:${message.status || ''}:${(message.streaming_target_content || message.content || '').length}:${(message.result || '').length}`
    }
    if (item.kind === 'trace') {
      return `${item.key}:${item.activeStreamingTrace ? 'active' : 'idle'}:${item.views.length}:${item.views.map((view) => `${view.partId}:${view.kind}:${view.status || ''}:${agentViewContent(view).length}:${stringifyLength(view.output)}`).join(',')}`
    }
    if (item.kind === 'activity') return `${item.key}:${item.content.length}`
    if (item.kind === 'attachment') return `${item.key}:${item.runId}`
    return item.key
  }).join('|')
  return [
    typeof bottomPaddingPx === 'number' ? Math.round(bottomPaddingPx) : '',
    afterContentKey,
    itemKey,
  ].join('|')
}

function chatListItemRunID(item: AgentChatListItem): string {
  if (item.kind === 'message') return item.view.metadata.run_id || ''
  if (item.kind === 'legacy-message') return item.message.run_id || item.openView?.metadata.run_id || ''
  if (item.kind === 'trace') {
    for (let index = item.views.length - 1; index >= 0; index -= 1) {
      const runID = item.views[index]?.metadata.run_id
      if (runID) return runID
    }
  }
  if (item.kind === 'attachment') return item.runId
  return ''
}

function agentMessageItemKey(view: AgentMessageView, index: number) {
  const prefix = view.kind === 'clear' ? 'clear' : 'message'
  const stableKey = agentViewStableKey(view)
  if (stableKey) return `${prefix}-${stableKey}`
  if (view.metadata.created_at) return `${prefix}-${view.metadata.created_at}-${index}`
  return `${prefix}-${index}`
}

function latestPlanCardBottomAnchorTarget(items: AgentChatListItem[]) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index]
    if (item.kind !== 'message') continue
    const view = item.view
    if (view.kind !== 'plan-question' && view.kind !== 'proposed-plan') continue
    const content = agentViewContent(view)
    const stableKey = view.partId || view.messageId || view.metadata.created_at || `${content.slice(0, 64)}:${content.length}`
    const dynamicKey = view.streaming || view.status === 'running'
      ? `${stableKey}:${view.status || ''}:${content.length}:${readString(view.data.thinking_preview).length}`
      : stableKey
    return {
      anchorKey: `${view.kind}:${item.key}:${dynamicKey}`,
      rowKey: item.key,
    }
  }
  return null
}

function chatListItemNavigationAnchor(item?: AgentChatListItem) {
  if (!item) return ''
  if (item.kind === 'message') return agentViewNavigationAnchor(item.view)
  if (item.kind === 'legacy-message') return item.message.navigation_turn_id || item.message.turn_id || ''
  return ''
}

function isActiveStreamingTrace(views: AgentMessageView[], afterTraceIndex: number, isStreaming: boolean) {
  if (!isStreaming) return false
  for (let index = afterTraceIndex; index < views.length; index += 1) {
    const view = views[index]
    if (view.kind === 'token-usage') continue
    if (view.kind === 'user') return false
    if (view.kind === 'assistant' && agentViewContent(view).trim()) {
      return view.streaming
    }
  }
  return true
}

function TraceGroup({ views, activeStreamingTrace, highlightDialogue, messageStyle, onInsertIllustration, onGenerateInteractiveImage, onOpenTrace }: { views: AgentMessageView[]; activeStreamingTrace: boolean; highlightDialogue: boolean; messageStyle?: CSSProperties; onInsertIllustration?: (illustration: ChapterIllustration) => void; onGenerateInteractiveImage?: (view: AgentMessageView) => void; onOpenTrace?: (runID: string) => void }) {
  const { t } = useTranslation()
  const active = activeStreamingTrace || views.some((view) => view.streaming || view.status === 'running')
  const [expanded, setExpanded] = useState(active)
  const userToggledRef = useRef(false)
  const toolCount = views.filter((view) => view.kind === 'tool').length
  const thinkingCount = views.filter((view) => view.kind === 'reasoning').length
  const subAgentCount = views.filter((view) => view.metadata.subagent).length
  const label = [
    thinkingCount > 0 ? t('chat.trace.thinking') : '',
    toolCount > 0 ? t('chat.trace.toolCalls', { count: toolCount }) : '',
    subAgentCount > 0 ? t('chat.subagent.label') : '',
  ].filter(Boolean).join(' · ') || t('chat.trace.execution')

  useEffect(() => {
    if (active) {
      userToggledRef.current = false
      setExpanded(true)
      return
    }
    if (!userToggledRef.current) setExpanded(false)
  }, [active])

  return (
    <div className="flex justify-start">
      <div className="w-full">
        <button
          type="button"
          className="flex items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          onClick={() => {
            userToggledRef.current = true
            setExpanded(!expanded)
          }}
        >
          {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
          {label}
        </button>
        {expanded && (
          <div className="space-y-2 border-l border-[var(--nova-border)] px-3 py-2">
            {views.map((view, index) => (
              view.kind === 'reasoning'
                ? (
                  <div key={view.partId || index} className="text-xs leading-relaxed text-[var(--nova-text-muted)] whitespace-pre-wrap">
                    {agentViewContent(view)}
                  </div>
                )
                : (
                  <AgentMessageItem
                    key={view.partId || index}
                    view={view}
                    highlightDialogue={highlightDialogue}
                    messageStyle={messageStyle}
                    onInsertIllustration={onInsertIllustration}
                    onGenerateInteractiveImage={onGenerateInteractiveImage}
                    onOpenTrace={onOpenTrace}
                  />
                )
            ))}
          </div>
        )}
      </div>
    </div>
  )
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

function findChatRowElement(container: HTMLElement | null, rowKey: string) {
  if (!container) return null
  const rows = container.querySelectorAll<HTMLElement>('[data-nova-chat-row-key]')
  for (const row of rows) {
    if (row.dataset.novaChatRowKey === rowKey) return row
  }
  return null
}

function scheduleChatRowBottomAnchor(container: HTMLElement | null, rowKey: string, bottomInsetPx: number, anchor: (element: HTMLElement, options?: ScrollElementBottomIntoViewOptions) => void, options: { observeResize?: boolean } = {}) {
  let cancelled = false
  let retryFrame: number | null = null
  let anchorFrame: number | null = null
  let followupFrame: number | null = null
  let timer: number | null = null
  let resizeObserver: ResizeObserver | null = null
  let attempt = 0
  const anchorRow = (row: HTMLElement) => {
    anchor(row, {
      bottomInsetPx,
      lockAfterScroll: true,
      visibleBottomPx: resolveChatVisibleBottomPx(container, bottomInsetPx),
    })
  }
  const scheduleAnchor = (row: HTMLElement) => {
    if (anchorFrame !== null) cancelAnimationFrame(anchorFrame)
    if (followupFrame !== null) cancelAnimationFrame(followupFrame)
    if (timer !== null) window.clearTimeout(timer)
    anchorFrame = requestAnimationFrame(() => {
      anchorFrame = null
      if (cancelled) return
      anchorRow(row)
      followupFrame = requestAnimationFrame(() => {
        followupFrame = null
        if (!cancelled) anchorRow(row)
      })
      timer = window.setTimeout(() => {
        timer = null
        if (!cancelled) anchorRow(row)
      }, 80)
    })
  }
  const attachAnchor = (row: HTMLElement) => {
    scheduleAnchor(row)
    if (options.observeResize === false || typeof ResizeObserver === 'undefined') return
    resizeObserver = new ResizeObserver(() => scheduleAnchor(row))
    resizeObserver.observe(row)
  }
  const tryAnchor = () => {
    if (cancelled) return
    const row = findChatRowElement(container, rowKey)
    if (row) {
      attachAnchor(row)
      return
    }
    attempt += 1
    if (attempt < 4) {
      retryFrame = requestAnimationFrame(tryAnchor)
      return
    }
    timer = window.setTimeout(() => {
      timer = null
      if (!cancelled) {
        const fallbackRow = findChatRowElement(container, rowKey)
        if (fallbackRow) attachAnchor(fallbackRow)
      }
    }, 80)
  }
  tryAnchor()
  return () => {
    cancelled = true
    if (retryFrame !== null) cancelAnimationFrame(retryFrame)
    if (anchorFrame !== null) cancelAnimationFrame(anchorFrame)
    if (followupFrame !== null) cancelAnimationFrame(followupFrame)
    if (timer !== null) window.clearTimeout(timer)
    resizeObserver?.disconnect()
  }
}

function resolveChatVisibleBottomPx(container: HTMLElement | null, bottomInsetPx: number) {
  const scroller = container?.querySelector<HTMLElement>('.nova-chat-canvas') || null
  if (!scroller) return undefined
  const scrollerRect = scroller.getBoundingClientRect()
  const composerTop = findChatComposerTop(container, scrollerRect)
  if (composerTop !== null) return composerTop
  return scrollerRect.bottom - Math.max(0, bottomInsetPx)
}

function findChatComposerTop(container: HTMLElement | null, scrollerRect: DOMRect) {
  const parent = container?.parentElement
  if (!parent) return null
  const composers = parent.querySelectorAll<HTMLElement>('.nova-chat-input-area .nova-agent-composer')
  let visibleTop: number | null = null
  for (const composer of composers) {
    if (container?.contains(composer)) continue
    const rect = composer.getBoundingClientRect()
    if (
      rect.width <= 0
      || rect.height <= 0
      || !Number.isFinite(rect.top)
      || rect.top <= scrollerRect.top
      || rect.top > scrollerRect.bottom
    ) {
      continue
    }
    visibleTop = visibleTop === null ? rect.top : Math.max(visibleTop, rect.top)
  }
  return visibleTop
}

function stringifyLength(value: unknown) {
  if (value === undefined || value === null) return 0
  if (typeof value === 'string') return value.length
  try {
    return JSON.stringify(value).length
  } catch {
    return String(value).length
  }
}

function readImagePath(data: Record<string, unknown>) {
  const interactiveImage = objectData(data.interactive_image)
  const illustration = objectData(data.illustration)
  return readString(interactiveImage.image_path) || readString(illustration.image_path)
}

function objectData(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}
