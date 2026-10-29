import { ToolInspectorHost, type InspectableToolMessage } from './ToolInspector'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode, UIEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Virtuoso } from 'react-virtuoso'
import type { Components, ContextProp, ListItem } from 'react-virtuoso'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import {
  agentViewContent,
  agentViewToRenderMessage,
  agentViewAskInteraction,
  buildAgentMessageViews,
  shareAgentMessageViews,
  isAgentRunMetadataView,
  selectAgentExecutionTimings,
  type AgentMessageView,
  type AgentPartRef,
} from '@/lib/agent-message-view'
import { VIRTUOSO_BOTTOM_THRESHOLD, useVirtuosoBottomLock } from './useVirtuosoBottomLock'
import { ScrollToBottomButton } from './ScrollToBottomButton'
import { StableAfterContentBoundary } from './StableAfterContentBoundary'
import { AgentChatListRow, chatListItemNavigationAnchor, type AgentChatListItem } from './AgentChatListRow'
import { buildAgentChatListItems } from './agent-chat-list-items'
import { scheduleChatRowBottomAnchor, scheduleResolvedChatRowBottomAnchor } from './chat-row-bottom-anchor'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { LoadingState } from '@/components/common/LoadingState'
import { AttachmentPreviewScopeProvider } from './ComposerAttachments'
import { VirtualizedMessageState } from './VirtualizedMessageState'
import type { ChatAttachmentScope } from '@/lib/chat-attachments'

interface MessageListProps {
  projectId?: string
  attachmentScope?: ChatAttachmentScope
  messages: AgentUIMessage[]
  /** Optional projection of the parent transcript, such as one SubAgent invocation. */
  projection?: AgentMessageListProjection
  isStreaming: boolean
  /** Runtime-confirmed Run identity, available before the first model output. */
  activeRunId?: string
  /** Whether the persistently mounted chat surface currently has measurable layout geometry. */
  visible?: boolean
  /** 后端确认的真实执行态；恢复探测可保持交互忙碌，但不能展开历史执行过程。 */
  isExecutionActive?: boolean
  activityContent: string
  highlightDialogue?: boolean
  scrollResetKey?: string
  bottomPaddingClassName?: string
  bottomPaddingPx?: number
  /** Shared adaptive boundary for every timeline row and trailing content. */
  contentClassName?: string
  /** Exposes the scrollport so floating controls can share its content width. */
  onScrollerChange?: (element: HTMLElement | null) => void
  /** Display-only introduction, shown at the beginning of fully loaded history. */
  beforeContent?: ReactNode
  afterContent?: ReactNode
  /** Generic controls follow output; confirmed result panels only reveal their marked heading. */
  afterContentScroll?: 'follow' | 'reveal-heading'
  afterContentKey?: string
  hasEarlierMessages?: boolean
  isLoadingEarlierMessages?: boolean
  onLoadEarlierMessages?: () => void | Promise<void>
  /** Fetch older history when the user scrolls near the top of the window. */
  autoLoadEarlierMessages?: boolean
  onLoadExecutionDetails?: (navigationAnchor: string) => Promise<void>
  timelineAttachments?: AgentTimelineAttachment[]
  messageStyle?: CSSProperties
  /** 开启后，同一次运行的中间正文、thinking 与工具统一折叠，终态正文保持可见。 */
  collapseTraceGroups?: boolean
  /** 运行中的 trace 初始展示方式；用户手动切换后保留其选择。 */
  activeTraceDisplay?: 'expanded' | 'collapsed'
  /** 领域级变更策略；不影响展示、导航或 trace 操作。 */
  canMutateMessage?: (view: AgentMessageView) => boolean
  onEditMessage?: (view: AgentMessageView) => void
  onEditAssistantReply?: (view: AgentMessageView) => void
  /** Story-only navigation action; unlike mutations, persisted historical turns remain eligible. */
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
  turnScrollRequest?: TurnScrollRequest
  onVisibleTurnAnchorChange?: (anchorId: string) => void
}

export interface AgentMessageListProjection {
  views: AgentMessageView[]
  initialPosition: 'start' | 'end'
  subAgentPresentation: 'card' | 'content'
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

const MESSAGE_LIST_OVERSCAN = { main: 520, reverse: 260 }
const MESSAGE_LIST_INCREASE_VIEWPORT_BY = { top: 420, bottom: 900 }
const MESSAGE_LIST_COMPONENTS: Components<AgentChatListItem, MessageListVirtuosoContext> = {
  Header: MessageListHeader,
  Footer: MessageListFooter,
}

interface MessageListVirtuosoContext {
  bottomPaddingClassName: string
  bottomPaddingPx?: number
  contentClassName?: string
  beforeContent?: ReactNode
  afterContent?: ReactNode
  afterContentKey?: string
  onAfterContentInteractionStart: () => void
  onAfterContentInteraction: () => void
  onAfterContentInteractionReset: () => void
  onAfterContentLayoutStabilized: () => void
  hasEarlierMessages: boolean
  isLoadingEarlierMessages: boolean
  onLoadEarlierMessages?: () => void | Promise<void>
}

export function MessageList(props: MessageListProps) {
  return <VirtualizedMessageState key={props.scrollResetKey || 'default'}><MessageListContent {...props} /></VirtualizedMessageState>
}

function MessageListContent({ projectId, attachmentScope, messages, projection, isStreaming, activeRunId, visible = true, isExecutionActive = isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', bottomPaddingPx, contentClassName, onScrollerChange, beforeContent, afterContent, afterContentScroll = 'follow', afterContentKey, hasEarlierMessages = false, isLoadingEarlierMessages = false, onLoadEarlierMessages, autoLoadEarlierMessages = false, onLoadExecutionDetails, timelineAttachments = [], messageStyle, collapseTraceGroups = false, activeTraceDisplay = 'expanded', canMutateMessage, onEditMessage, onEditAssistantReply, onCreateBranch, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onReadAloud, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onApprovePlan, onContinuePlan, onExitPlanMode, onResolveAsk, turnScrollRequest, onVisibleTurnAnchorChange }: MessageListProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const renderedItemsRef = useRef<ListItem<AgentChatListItem>[]>([])
  const lastVisibleTurnAnchorRef = useRef('')
  const lastTurnScrollRequestIdRef = useRef<number | null>(null)
  const previousViews = useRef<AgentMessageView[]>([])
  const views = useMemo(() => {
    const next = shareAgentMessageViews(previousViews.current, projection?.views ?? buildAgentMessageViews(messages))
    previousViews.current = next
    return next
  }, [messages, projection?.views])
  const [disclosures, setDisclosures] = useState<Record<string, boolean>>({})
  const liveRunIDs = useRef(new Set<string>())
  const initialPosition = projection?.initialPosition ?? 'end'
  const subAgentPresentation = projection?.subAgentPresentation ?? 'card'
  const executionTimings = useMemo(() => selectAgentExecutionTimings(views), [views])
  // Historical tool cards can retain an interrupted running state. Only this
  // reply can replace the waiting shimmer, including turns without text yet.
  const replyStart = views.findLastIndex(view => view.kind === 'user' || view.kind === 'clear') + 1
  const replyViews = views.slice(replyStart)
  const hasActiveResponse = replyViews.some((view) =>
    view.kind !== 'user' &&
    !isAgentRunMetadataView(view) &&
    view.kind !== 'clear' &&
    (!activeRunId || !view.metadata.run_id || view.metadata.run_id === activeRunId || view.metadata.subagent) &&
    (view.streaming || view.status === 'running'),
  )
  // A committed final reply can replace streaming prose before the transport
  // closes. Do not turn that handoff back into a model-waiting indicator.
  const hasFinalResponse = replyViews.some(view => view.kind === 'assistant' &&
    !view.metadata.subagent && (!activeRunId || view.metadata.run_id === activeRunId) &&
    view.metadata.display_phase === 'final' && agentViewContent(view).trim())
  const visibleActivityContent = hasActiveResponse
    ? ''
    : activityContent || (isStreaming && !hasFinalResponse ? t('chat.activity.thinking') : '')
  const listItems = useMemo(
    () => buildAgentChatListItems({
      views,
      isStreaming,
      activeRunId,
      isExecutionActive,
      visibleActivityContent,
      collapseTraceGroups,
      groupSubAgentTimeline: Boolean(onOpenSubAgentSession),
      timelineAttachments,
      disclosures,
      activeTraceDisplay,
    }),
    [activeTraceDisplay, disclosures, activeRunId, collapseTraceGroups, isExecutionActive, isStreaming, onOpenSubAgentSession, timelineAttachments, views, visibleActivityContent],
  )
  useLayoutEffect(() => {
    if ((isStreaming || isExecutionActive) && activeRunId) liveRunIDs.current.add(activeRunId)
    // Remember the first presentation even when the row is outside the viewport.
    // Finishing a run must not reset the reader's disclosure choices.
    setDisclosures(current => {
      const missing = listItems.filter(item => item.kind === 'process' && !(item.key in current))
      if (!missing.length) return current
      const next = { ...current }
      for (const item of missing) if (item.kind === 'process') next[item.key] = item.expanded
      return next
    })
  }, [activeRunId, isExecutionActive, isStreaming, listItems])
  // Transport streaming may pause between tool/recovery phases while the turn
  // remains active and can still publish layout updates.
  // A confirmed trailing result ends prose following even if the transport is
  // still open. Its reveal must not drag the reader to the bottom of the panel.
  const revealAfterContent = Boolean(afterContent) && afterContentScroll === 'reveal-heading'
  const tailFollowActive = (isStreaming || isExecutionActive) && !revealAfterContent
  const firstItemIndex = usePrependStableFirstItemIndex(listItems, scrollResetKey)
  const initialPositionKey = scrollResetKey || 'default'
  const hasInitialContent = views.length > 0 || isStreaming || Boolean(activityContent)
  const [positionedKey, setPositionedKey] = useState('')
  const initialPositionReady = initialPosition === 'start' || !visible || !hasInitialContent || positionedKey === initialPositionKey
  const resolveMessageScroller = useCallback(
    () => containerRef.current?.querySelector<HTMLElement>('.nova-chat-canvas') || null,
    [],
  )
  const scrollLock = useVirtuosoBottomLock({
    resetKey: scrollResetKey,
    resetPosition: initialPosition,
    itemCount: listItems.length,
    autoFollowEnabled: tailFollowActive,
    revealAfterContent,
    visible,
    bottomInsetPx: bottomPaddingPx,
    resolveScroller: resolveMessageScroller,
  })
  const setScroller = useCallback((element: HTMLElement | Window | null) => {
    scrollLock.scrollerRef(element)
    onScrollerChange?.(element instanceof HTMLElement ? element : null)
  }, [onScrollerChange, scrollLock.scrollerRef])
  const latestInteractiveCardAnchor = useMemo(
    () => latestInteractiveCardBottomAnchorTarget(listItems),
    [listItems],
  )
  const lastInteractiveCardAnchorKeyRef = useRef<string | null>(null)
  const virtuosoContext = useMemo<MessageListVirtuosoContext>(
    () => ({
      bottomPaddingClassName,
      bottomPaddingPx: scrollLock.streamingSpacerPx ?? bottomPaddingPx,
      contentClassName,
      beforeContent,
      afterContent,
      afterContentKey,
      onAfterContentInteractionStart: scrollLock.beginAfterContentInteraction,
      onAfterContentInteraction: scrollLock.releaseBottomLock,
      onAfterContentInteractionReset: scrollLock.resetAfterContentInteraction,
      onAfterContentLayoutStabilized: scrollLock.restoreAfterContentScrollPosition,
      hasEarlierMessages,
      isLoadingEarlierMessages,
      onLoadEarlierMessages,
    }),
    [beforeContent, afterContent, afterContentKey, bottomPaddingClassName, bottomPaddingPx, contentClassName, hasEarlierMessages, isLoadingEarlierMessages, onLoadEarlierMessages, scrollLock.beginAfterContentInteraction, scrollLock.releaseBottomLock, scrollLock.resetAfterContentInteraction, scrollLock.restoreAfterContentScrollPosition, scrollLock.streamingSpacerPx],
  )
  const scrollButtonBottomOffset = typeof bottomPaddingPx === 'number' ? Math.max(24, bottomPaddingPx + 12) : 24
  const anchorLatestInteractiveCardBottom = useCallback((element?: HTMLElement) => {
    const row = element?.closest<HTMLElement>('[data-nova-chat-row-key]')
    const bottomInsetPx = Math.max(0, bottomPaddingPx || 0)
    const cardRowKey = row?.dataset.novaChatRowKey
    if (cardRowKey) {
      scheduleResolvedChatRowBottomAnchor(
        () => containerRef.current,
        cardRowKey,
        bottomInsetPx,
        scrollLock.scrollElementBottomIntoView,
      )
      return
    }
    const rowKey = latestInteractiveCardAnchor?.rowKey
    if (!rowKey) return
    scheduleChatRowBottomAnchor(containerRef.current, rowKey, bottomInsetPx, scrollLock.scrollElementBottomIntoView)
  }, [bottomPaddingPx, latestInteractiveCardAnchor?.rowKey, scrollLock.scrollElementBottomIntoView])

  useEffect(() => {
    const bottomInsetPx = Math.max(0, bottomPaddingPx || 0)
    const anchorKey = latestInteractiveCardAnchor ? `${latestInteractiveCardAnchor.anchorKey}:${Math.round(bottomInsetPx)}` : ''
    if (lastInteractiveCardAnchorKeyRef.current === null) {
      lastInteractiveCardAnchorKeyRef.current = anchorKey
      if (latestInteractiveCardAnchor && tailFollowActive) {
        return scheduleChatRowBottomAnchor(containerRef.current, latestInteractiveCardAnchor.rowKey, bottomInsetPx, scrollLock.scrollElementBottomIntoView)
      }
      return undefined
    }
    if (latestInteractiveCardAnchor && anchorKey !== lastInteractiveCardAnchorKeyRef.current) {
      const cancelAnchor = scheduleChatRowBottomAnchor(containerRef.current, latestInteractiveCardAnchor.rowKey, bottomInsetPx, scrollLock.scrollElementBottomIntoView)
      lastInteractiveCardAnchorKeyRef.current = anchorKey
      return cancelAnchor
    }
    lastInteractiveCardAnchorKeyRef.current = anchorKey
    return undefined
  }, [bottomPaddingPx, latestInteractiveCardAnchor, scrollLock.scrollElementBottomIntoView, tailFollowActive])

  const notifyVisibleTurnAnchor = useCallback((renderedItems: ListItem<AgentChatListItem>[], viewportStartOverride?: number) => {
    if (!onVisibleTurnAnchorChange) return
    const viewportStart = viewportStartOverride ?? resolveMessageScroller()?.scrollTop ?? 0
    for (const renderedItem of renderedItems) {
      // itemsRendered includes reverse overscan. Ignore rows ending at or above the
      // viewport so a top-aligned turn is not attributed to the preceding turn.
      if (renderedItem.offset + renderedItem.size <= viewportStart) continue
      const relativeIndex = renderedItem.index - firstItemIndex
      const item = renderedItem.data || listItems[relativeIndex]
      const anchorId = chatListItemNavigationAnchor(item)
      if (!anchorId) continue
      if (lastVisibleTurnAnchorRef.current === anchorId) return
      lastVisibleTurnAnchorRef.current = anchorId
      onVisibleTurnAnchorChange(anchorId)
      return
    }
  }, [firstItemIndex, listItems, onVisibleTurnAnchorChange, resolveMessageScroller])

  const earlierRequest = useRef(false)
  const historyScrollAnchor = useRef<{ key: string; firstKey?: string; offset: number } | null>(null)
  const captureHistoryAnchor = useCallback((scroller: HTMLElement | null) => {
    if (!scroller || (historyScrollAnchor.current && historyScrollAnchor.current.firstKey !== listItems[0]?.key)) return
    const viewport = scroller.getBoundingClientRect()
    const row = [...scroller.querySelectorAll<HTMLElement>('[data-nova-chat-row-key]')].find(row => {
      const bounds = row.getBoundingClientRect()
      return bounds.bottom > viewport.top && bounds.top < viewport.bottom
    })
    if (row) historyScrollAnchor.current = { key: row.dataset.novaChatRowKey!, firstKey: listItems[0]?.key || '', offset: (row.parentElement || row).getBoundingClientRect().top - viewport.top }
  }, [listItems])
  const handleItemsRendered = useCallback((items: ListItem<AgentChatListItem>[]) => {
    renderedItemsRef.current = items
    notifyVisibleTurnAnchor(items)
    // A scroll event can precede mounting the rows at the new viewport position.
    if (earlierRequest.current) captureHistoryAnchor(resolveMessageScroller())
  }, [captureHistoryAnchor, notifyVisibleTurnAnchor, resolveMessageScroller])

  useLayoutEffect(() => {
    if (turnScrollRequest?.anchorId && lastTurnScrollRequestIdRef.current !== turnScrollRequest.requestId) {
      const targetIndex = listItems.findIndex(item => chatListItemNavigationAnchor(item) === turnScrollRequest.anchorId)
      if (targetIndex < 0) return
      // Explicit navigation takes precedence over preserving the previous row.
      // Wait for Virtuoso to measure the prepended page and its opening header.
      historyScrollAnchor.current = null
      const frame = requestAnimationFrame(() => {
        scrollLock.scrollToIndex(targetIndex, { align: 'start', behavior: 'auto' })
        lastTurnScrollRequestIdRef.current = turnScrollRequest.requestId
      })
      return () => cancelAnimationFrame(frame)
    }
    const anchor = historyScrollAnchor.current
    if (!anchor || (anchor.firstKey !== undefined && anchor.firstKey === listItems[0]?.key)) return
    const index = listItems.findIndex(item => item.key === anchor.key)
    if (index < 0) {
      historyScrollAnchor.current = null
      return
    }
    // Virtuoso includes the current header height when positioning an index.
    // Preserve only the row's viewport offset, without counting the header twice.
    const frame = requestAnimationFrame(() => {
      scrollLock.scrollToIndex(index, { align: 'start', behavior: 'auto', offset: -anchor.offset })
      historyScrollAnchor.current = null
    })
    return () => cancelAnimationFrame(frame)
  }, [listItems, scrollLock.scrollToIndex, turnScrollRequest])
  const handleMessageScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    scrollLock.onScroll(event)
    notifyVisibleTurnAnchor(renderedItemsRef.current, event.currentTarget.scrollTop)
    const shouldLoadEarlier = autoLoadEarlierMessages && initialPositionReady && event.currentTarget.scrollTop < 240 && hasEarlierMessages && !isLoadingEarlierMessages && !earlierRequest.current && onLoadEarlierMessages
    if (shouldLoadEarlier || earlierRequest.current) captureHistoryAnchor(event.currentTarget)
    if (shouldLoadEarlier) {
      earlierRequest.current = true
      void Promise.resolve(onLoadEarlierMessages()).finally(() => { earlierRequest.current = false })
    }
  }, [autoLoadEarlierMessages, captureHistoryAnchor, hasEarlierMessages, initialPositionReady, isLoadingEarlierMessages, notifyVisibleTurnAnchor, onLoadEarlierMessages, scrollLock.onScroll])

  const onProcessExpandedChange = useCallback(async (key: string, expanded: boolean, navigationAnchor: string) => {
    scrollLock.releaseBottomLock()
    if (expanded) await onLoadExecutionDetails?.(navigationAnchor)
    const scroller = resolveMessageScroller()
    const row = scroller?.querySelector<HTMLElement>(`[data-nova-chat-row-key="${CSS.escape(key)}"]`)
    if (row && scroller) {
      historyScrollAnchor.current = { key, offset: (row.parentElement || row).getBoundingClientRect().top - scroller.getBoundingClientRect().top }
    }
    setDisclosures(current => ({ ...current, [key]: expanded }))
  }, [onLoadExecutionDetails, resolveMessageScroller, scrollLock.releaseBottomLock])

  const itemContent = useCallback((index: number, item?: AgentChatListItem) => {
    const resolvedItem = item || listItems[index - firstItemIndex]
    if (!resolvedItem) return null
    return (
      <AgentChatListRow
        projectId={projectId}
        item={resolvedItem}
        animateAttachment={resolvedItem.kind === 'attachment' && liveRunIDs.current.has(resolvedItem.runId)}
        executionTimings={executionTimings}
        contentClassName={contentClassName}
        nextItem={listItems[index - firstItemIndex + 1]}
        isStreaming={isStreaming}
        tailFollowActive={tailFollowActive}
        onProcessExpandedChange={onProcessExpandedChange}
        subAgentPresentation={subAgentPresentation}
        highlightDialogue={highlightDialogue}
        messageStyle={messageStyle}
        canMutateMessage={canMutateMessage}
        onEditMessage={onEditMessage}
        onEditAssistantReply={onEditAssistantReply}
        onCreateBranch={onCreateBranch}
        onRegenerateMessage={onRegenerateMessage}
        onSwitchMessageVersion={onSwitchMessageVersion}
        onOpenSubAgentSession={onOpenSubAgentSession}
        onInsertIllustration={onInsertIllustration}
        onReadAloud={onReadAloud}
        onGenerateInteractiveImage={onGenerateInteractiveImage}
        generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
        activeSubAgentSessionKey={activeSubAgentSessionKey}
        onApprovePlan={onApprovePlan}
        onContinuePlan={onContinuePlan}
        onExitPlanMode={onExitPlanMode}
        onResolveAsk={onResolveAsk}
        onInteractiveCardLayoutChange={anchorLatestInteractiveCardBottom}
        streamingRowRef={tailFollowActive ? scrollLock.streamingRowRef : undefined}
        syncStreamingTailLayout={tailFollowActive ? scrollLock.syncStreamingTailLayout : undefined}
      />
    )
  }, [activeSubAgentSessionKey, onProcessExpandedChange, anchorLatestInteractiveCardBottom, canMutateMessage, contentClassName, executionTimings, firstItemIndex, generatingInteractiveImageTurnId, highlightDialogue, isStreaming, listItems, messageStyle, onApprovePlan, onContinuePlan, onCreateBranch, onEditAssistantReply, onEditMessage, onExitPlanMode, onReadAloud, onGenerateInteractiveImage, onInsertIllustration, onOpenSubAgentSession, onRegenerateMessage, onResolveAsk, onSwitchMessageVersion, projectId, scrollLock.streamingRowRef, scrollLock.syncStreamingTailLayout, subAgentPresentation, tailFollowActive])

  useLayoutEffect(() => {
    if (initialPosition !== 'end' || !visible || !hasInitialContent || positionedKey === initialPositionKey) return
    const scroller = resolveMessageScroller()
    if (!scroller || scroller.clientHeight <= 0) {
      setPositionedKey(initialPositionKey)
      return
    }
    let secondFrame = 0
    const placeAtBottom = () => {
      scrollLock.virtuosoRef.current?.scrollToIndex({ index: 'LAST', align: 'end', behavior: 'auto' })
    }
    placeAtBottom()
    const firstFrame = window.requestAnimationFrame(() => {
      placeAtBottom()
      secondFrame = window.requestAnimationFrame(() => {
        placeAtBottom()
        setPositionedKey(initialPositionKey)
      })
    })
    return () => {
      window.cancelAnimationFrame(firstFrame)
      if (secondFrame) window.cancelAnimationFrame(secondFrame)
    }
  }, [hasInitialContent, initialPosition, initialPositionKey, positionedKey, resolveMessageScroller, scrollLock.virtuosoRef, visible])

  const resolveInspectedMessage = useCallback((message: InspectableToolMessage) => {
    const view = views.find(view => view.partId === message.id && view.metadata.run_id === message.run_id && view.metadata.subagent_session_id === message.subagent_session_id)
    const current = view && agentViewToRenderMessage(view)
    return current && (current.role === 'tool_call' || current.role === 'tool_result' || current.role === 'ask') ? current : undefined
  }, [views])

  return (
    <ToolInspectorHost projectId={projectId} resolveMessage={resolveInspectedMessage} restoreFocus={resolveMessageScroller}>
    <AttachmentPreviewScopeProvider projectId={projectId} scope={attachmentScope}>
      <div ref={containerRef} className="relative flex min-h-0 flex-1 flex-col">
      <Virtuoso
        key={scrollResetKey || 'default'}
        ref={scrollLock.virtuosoRef}
        scrollerRef={setScroller}
        onScroll={handleMessageScroll}
        onWheel={scrollLock.onWheel}
        onKeyDown={scrollLock.onKeyDown}
        onPointerDown={scrollLock.onPointerDown}
        onPointerMove={scrollLock.onPointerMove}
        onPointerUp={scrollLock.onPointerUp}
        onPointerCancel={scrollLock.onPointerCancel}
        atBottomStateChange={scrollLock.onAtBottomStateChange}
        atBottomThreshold={VIRTUOSO_BOTTOM_THRESHOLD}
        totalListHeightChanged={tailFollowActive ? scrollLock.syncStreamingTailLayout : scrollLock.syncIdleBottomLayout}
        initialTopMostItemIndex={initialPosition === 'end' ? { index: 'LAST', align: 'end' } : 0}
        firstItemIndex={firstItemIndex}
        data={listItems}
        context={virtuosoContext}
        components={MESSAGE_LIST_COMPONENTS}
        // Keep short conversations top-aligned. Initial restoration still scrolls
        // long history to its latest message, while the streaming tail owns active runs.
        alignToBottom={false}
        computeItemKey={(index, item) => item?.key || listItems[index - firstItemIndex]?.key || `agent-chat-item-${index}`}
        itemContent={itemContent}
        itemsRendered={handleItemsRendered}
        overscan={MESSAGE_LIST_OVERSCAN}
        increaseViewportBy={MESSAGE_LIST_INCREASE_VIEWPORT_BY}
        data-stream-active={tailFollowActive ? '' : undefined}
        className={cn(
          'nova-chat-canvas min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none] [scrollbar-gutter:stable]',
          !initialPositionReady && 'pointer-events-none opacity-0',
        )}
        aria-busy={!initialPositionReady}
        aria-hidden={!initialPositionReady || undefined}
        aria-label={t('common.messages', { count: views.length })}
      />
      {!initialPositionReady ? (
        <LoadingState
          label={t('common.loading')}
          variant="panel"
          layout="conversation"
          className="pointer-events-none absolute inset-0 min-h-0 bg-[var(--nova-bg)]"
        />
      ) : null}
      <ScrollToBottomButton
        visible={scrollLock.isAwayFromBottom}
        onClick={scrollLock.scrollToBottom}
        bottomOffsetPx={scrollButtonBottomOffset}
        rightOffsetPx={24}
      />
      </div>
    </AttachmentPreviewScopeProvider>
    </ToolInspectorHost>
  )
}

const MESSAGE_LIST_FIRST_ITEM_INDEX = 1_000_000

function usePrependStableFirstItemIndex(items: AgentChatListItem[], resetKey?: string) {
  const stateRef = useRef({
    resetKey,
    firstKey: '',
    firstItemIndex: MESSAGE_LIST_FIRST_ITEM_INDEX,
  })
  const state = stateRef.current
  const nextFirstKey = items[0]?.key || ''
  if (state.resetKey !== resetKey) {
    state.resetKey = resetKey
    state.firstKey = nextFirstKey
    state.firstItemIndex = MESSAGE_LIST_FIRST_ITEM_INDEX
    return state.firstItemIndex
  }
  if (state.firstKey && state.firstKey !== nextFirstKey) {
    const previousFirstOffset = items.findIndex((item) => item.key === state.firstKey)
    if (previousFirstOffset > 0) {
      state.firstItemIndex = Math.max(0, state.firstItemIndex - previousFirstOffset)
    } else if (previousFirstOffset < 0) {
      state.firstItemIndex = MESSAGE_LIST_FIRST_ITEM_INDEX
    }
  }
  state.firstKey = nextFirstKey
  return state.firstItemIndex
}

function MessageListHeader({ context }: ContextProp<MessageListVirtuosoContext>) {
  const { t } = useTranslation()
  if (!context.hasEarlierMessages && context.beforeContent) return (
    <div className={cn('nova-message-list-header px-3 pb-4 pt-5 sm:px-6', context.contentClassName)}>
      {context.beforeContent}
    </div>
  )
  if (!context.hasEarlierMessages) return <div aria-hidden="true" className="nova-message-list-header h-5 shrink-0" />
  return (
    <div className="nova-message-list-header flex min-h-10 shrink-0 items-center justify-center px-4 py-2">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={context.isLoadingEarlierMessages}
        onClick={() => void context.onLoadEarlierMessages?.()}
        className="h-7 text-xs text-[var(--nova-text-muted)]"
      >
        {context.isLoadingEarlierMessages ? t('chat.history.loadingEarlier') : t('chat.history.loadEarlier')}
      </Button>
    </div>
  )
}

function MessageListFooter({ context }: ContextProp<MessageListVirtuosoContext>) {
  const hasMeasuredPadding = typeof context.bottomPaddingPx === 'number'
  return (
    <>
      {context.afterContent ? (
        <StableAfterContentBoundary
          resetKey={context.afterContentKey}
          className={cn('px-3 pb-4 sm:px-6', context.contentClassName)}
          onInteractionStart={context.onAfterContentInteractionStart}
          onInteraction={context.onAfterContentInteraction}
          onInteractionReset={context.onAfterContentInteractionReset}
          onLayoutStabilized={context.onAfterContentLayoutStabilized}
        >
          {context.afterContent}
        </StableAfterContentBoundary>
      ) : null}
      <div
        aria-hidden="true"
        data-nova-chat-bottom-spacer
        className={hasMeasuredPadding ? 'shrink-0' : `shrink-0 ${context.bottomPaddingClassName}`}
        style={hasMeasuredPadding ? { height: context.bottomPaddingPx } : undefined}
      />
    </>
  )
}

function latestInteractiveCardBottomAnchorTarget(items: AgentChatListItem[]) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index]
    const views = chatListItemViews(item)
    for (let viewIndex = views.length - 1; viewIndex >= 0; viewIndex -= 1) {
      const view = views[viewIndex]
      const approval = agentViewAskInteraction(view)
      if (approval?.kind === 'tool_approval' && approval.status === 'pending') {
        return {
          anchorKey: `tool-approval:${item.key}:${approval.id}:${approval.tool_call_id}`,
          rowKey: item.key,
        }
      }
      if (view.kind !== 'proposed-plan') continue
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
  }
  return null
}

function chatListItemViews(item: AgentChatListItem): AgentMessageView[] {
  if (item.kind === 'message') return [item.view]
  if (item.kind === 'legacy-message') return item.openView ? [item.openView] : []
  if (item.kind === 'process' && !item.expanded) return item.views
  return []
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}
