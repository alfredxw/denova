import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode, RefCallback, UIEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { motion } from 'motion/react'
import { Virtuoso } from 'react-virtuoso'
import type { Components, ContextProp, ListItem } from 'react-virtuoso'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration, ChatMessage } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import {
  agentViewToRenderMessage,
  agentViewContent,
  agentViewAskInteraction,
  agentViewNavigationAnchor,
  agentViewStableKey,
  buildAgentSubAgentTimelineGroups,
  buildAgentMessageViews,
  isAgentRunMetadataView,
  isAgentTraceView,
  selectAgentExecutionTimings,
  type AgentExecutionTiming,
  type AgentMessageView,
  type AgentPartRef,
} from '@/lib/agent-message-view'
import { listItem, novaEase, timelineAttachment } from '@/features/motion/motion-tokens'
import { buildSubAgentProgressMessage } from './subagent-session'
import { VIRTUOSO_BOTTOM_THRESHOLD, useVirtuosoBottomLock } from './useVirtuosoBottomLock'
import { ScrollToBottomButton } from './ScrollToBottomButton'
import { StableAfterContentBoundary } from './StableAfterContentBoundary'
import { AgentMessageItem } from './AgentMessageItem'
import { AgentActivityShimmer, MessageItem, type AssistantMessagePresentation } from './MessageItem'
import { AgentExecutionProcess } from './AgentExecutionProcess'
import { buildAgentRunPresentation, type AgentRunPresentationSection } from './agent-run-presentation'
import { scheduleChatRowBottomAnchor, scheduleResolvedChatRowBottomAnchor } from './chat-row-bottom-anchor'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { LoadingState } from '@/components/common/LoadingState'

interface MessageListProps {
  projectId?: string
  messages: AgentUIMessage[]
  /** Optional projection of the parent transcript, such as one SubAgent invocation. */
  projection?: AgentMessageListProjection
  isStreaming: boolean
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
  afterContent?: ReactNode
  afterContentKey?: string
  hasEarlierMessages?: boolean
  isLoadingEarlierMessages?: boolean
  onLoadEarlierMessages?: () => void | Promise<void>
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
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
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

type AgentChatListItem =
  | { kind: 'empty'; key: string }
  | { kind: 'typing'; key: string }
  | { kind: 'activity'; key: string; content: string }
  | { kind: 'clear'; key: string; createdAt?: string }
  | { kind: 'message'; key: string; view: AgentMessageView; sourceIndex: number }
  | { kind: 'legacy-message'; key: string; message: ChatMessage; sourceIndex: number; openView?: AgentMessageView }
  | { kind: 'trace'; key: string; views: AgentMessageView[]; activeStreamingTrace: boolean }
  | { kind: 'run'; key: string; runId: string; sections: AgentRunPresentationSection[]; sourceIndex: number }
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
  contentClassName?: string
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

export function MessageList({ projectId, messages, projection, isStreaming, visible = true, isExecutionActive = isStreaming, activityContent, highlightDialogue = false, scrollResetKey, bottomPaddingClassName = '', bottomPaddingPx, contentClassName, afterContent, afterContentKey, hasEarlierMessages = false, isLoadingEarlierMessages = false, onLoadEarlierMessages, timelineAttachments = [], messageStyle, collapseTraceGroups = false, activeTraceDisplay = 'expanded', canMutateMessage, onEditMessage, onEditAssistantReply, onCreateBranch, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onApprovePlan, onContinuePlan, onExitPlanMode, onOpenTrace, onResolveAsk, turnScrollRequest, onVisibleTurnAnchorChange }: MessageListProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const renderedItemsRef = useRef<ListItem<AgentChatListItem>[]>([])
  const lastVisibleTurnAnchorRef = useRef('')
  const lastTurnScrollRequestIdRef = useRef<number | null>(null)
  const views = useMemo(() => projection?.views ?? buildAgentMessageViews(messages), [messages, projection?.views])
  const initialPosition = projection?.initialPosition ?? 'end'
  const subAgentPresentation = projection?.subAgentPresentation ?? 'card'
  const executionTimings = useMemo(() => selectAgentExecutionTimings(views), [views])
  const hasActiveResponse = views.some((view) =>
    view.kind !== 'user' &&
    !isAgentRunMetadataView(view) &&
    view.kind !== 'clear' &&
    (view.streaming || view.status === 'running'),
  )
  // 真实 thinking / tool / 正文行已经承担进度展示；额外 activity 行会重复展示，
  // 并在内容增高时被底部锁定反复拉动。没有真实输出时则统一使用 Shimmer 填充等待态。
  const visibleActivityContent = hasActiveResponse
    ? ''
    : activityContent || (isStreaming ? t('chat.activity.thinking') : '')
  const listItems = useMemo(
    () => buildAgentChatListItems({
      views,
      isStreaming,
      isExecutionActive,
      visibleActivityContent,
      collapseTraceGroups,
      groupSubAgentTimeline: Boolean(onOpenSubAgentSession),
      timelineAttachments,
    }),
    [collapseTraceGroups, isExecutionActive, isStreaming, onOpenSubAgentSession, timelineAttachments, views, visibleActivityContent],
  )
  // Transport streaming may pause between tool/recovery phases while the turn
  // remains active and can still publish layout updates.
  const tailFollowActive = isStreaming || isExecutionActive
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
    visible,
    bottomInsetPx: bottomPaddingPx,
    resolveScroller: resolveMessageScroller,
  })
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
    [afterContent, afterContentKey, bottomPaddingClassName, bottomPaddingPx, contentClassName, hasEarlierMessages, isLoadingEarlierMessages, onLoadEarlierMessages, scrollLock.beginAfterContentInteraction, scrollLock.releaseBottomLock, scrollLock.resetAfterContentInteraction, scrollLock.restoreAfterContentScrollPosition, scrollLock.streamingSpacerPx],
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
  }, [bottomPaddingPx, latestInteractiveCardAnchor, scrollLock.scrollElementBottomIntoView])

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

  useEffect(() => {
    if (!turnScrollRequest?.anchorId) return
    if (lastTurnScrollRequestIdRef.current === turnScrollRequest.requestId) return
    lastTurnScrollRequestIdRef.current = turnScrollRequest.requestId
    const targetIndex = listItems.findIndex((item) => chatListItemNavigationAnchor(item) === turnScrollRequest.anchorId)
    if (targetIndex < 0) return
    scrollLock.scrollToIndex(targetIndex, { align: 'start', behavior: 'auto' })
  }, [listItems, scrollLock, turnScrollRequest])

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

  const handleItemsRendered = useCallback((items: ListItem<AgentChatListItem>[]) => {
    renderedItemsRef.current = items
    notifyVisibleTurnAnchor(items)
  }, [notifyVisibleTurnAnchor])

  const handleMessageScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    scrollLock.onScroll(event)
    notifyVisibleTurnAnchor(renderedItemsRef.current, event.currentTarget.scrollTop)
  }, [notifyVisibleTurnAnchor, scrollLock.onScroll])

  const itemContent = useCallback((index: number, item?: AgentChatListItem) => {
    const resolvedItem = item || listItems[index - firstItemIndex]
    if (!resolvedItem) return null
    return (
      <AgentChatListRow
        projectId={projectId}
        item={resolvedItem}
        executionTimings={executionTimings}
        contentClassName={contentClassName}
        isLast={index === firstItemIndex + listItems.length - 1}
        isStreaming={isStreaming}
        tailFollowActive={tailFollowActive}
        activeTraceDisplay={activeTraceDisplay}
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
        onGenerateInteractiveImage={onGenerateInteractiveImage}
        generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
        activeSubAgentSessionKey={activeSubAgentSessionKey}
        onApprovePlan={onApprovePlan}
        onContinuePlan={onContinuePlan}
        onExitPlanMode={onExitPlanMode}
        onOpenTrace={onOpenTrace}
        onResolveAsk={onResolveAsk}
        onInteractiveCardLayoutChange={anchorLatestInteractiveCardBottom}
        streamingRowRef={tailFollowActive ? scrollLock.streamingRowRef : undefined}
        syncStreamingTailLayout={tailFollowActive ? scrollLock.syncStreamingTailLayout : undefined}
      />
    )
  }, [activeSubAgentSessionKey, activeTraceDisplay, anchorLatestInteractiveCardBottom, canMutateMessage, contentClassName, executionTimings, firstItemIndex, generatingInteractiveImageTurnId, highlightDialogue, isStreaming, listItems, messageStyle, onApprovePlan, onContinuePlan, onCreateBranch, onEditAssistantReply, onEditMessage, onExitPlanMode, onGenerateInteractiveImage, onInsertIllustration, onOpenSubAgentSession, onOpenTrace, onRegenerateMessage, onResolveAsk, onSwitchMessageVersion, projectId, scrollLock.streamingRowRef, scrollLock.syncStreamingTailLayout, subAgentPresentation, tailFollowActive])

  useLayoutEffect(() => {
    if (initialPosition !== 'end' || !visible || !hasInitialContent || positionedKey === initialPositionKey) return
    const scroller = resolveMessageScroller()
    if (!scroller || scroller.clientHeight <= 0) {
      setPositionedKey(initialPositionKey)
      return
    }
    let secondFrame = 0
    const placeAtBottom = () => {
      scroller.scrollTop = Math.max(0, scroller.scrollHeight - scroller.clientHeight)
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
  }, [hasInitialContent, initialPosition, initialPositionKey, positionedKey, resolveMessageScroller, visible])

  return (
    <div ref={containerRef} className="relative flex min-h-0 flex-1 flex-col">
      <Virtuoso
        key={scrollResetKey || 'default'}
        ref={scrollLock.virtuosoRef}
        scrollerRef={scrollLock.scrollerRef}
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
        initialItemCount={Math.min(listItems.length, 40)}
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
          'nova-chat-canvas min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none]',
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
  if (!context.hasEarlierMessages) return <div aria-hidden="true" className="h-5 shrink-0" />
  return (
    <div className="flex min-h-10 shrink-0 items-center justify-center px-4 py-2">
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

function AgentChatListRow({ projectId, item, executionTimings, isLast, isStreaming, tailFollowActive, activeTraceDisplay, subAgentPresentation, highlightDialogue, messageStyle, contentClassName, canMutateMessage, onEditMessage, onEditAssistantReply, onCreateBranch, onRegenerateMessage, onSwitchMessageVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, onApprovePlan, onContinuePlan, onExitPlanMode, onOpenTrace, onResolveAsk, onInteractiveCardLayoutChange, streamingRowRef, syncStreamingTailLayout }: {
  projectId?: string
  item: AgentChatListItem
  executionTimings: ReadonlyMap<string, AgentExecutionTiming>
  isLast: boolean
  isStreaming: boolean
  tailFollowActive: boolean
  activeTraceDisplay: 'expanded' | 'collapsed'
  subAgentPresentation: 'card' | 'content'
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  contentClassName?: string
  canMutateMessage?: (view: AgentMessageView) => boolean
  onEditMessage?: (view: AgentMessageView) => void
  onEditAssistantReply?: (view: AgentMessageView) => void
  onCreateBranch?: (view: AgentMessageView) => void
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  onResolveAsk?: (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => Promise<AgentAskResolution>
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  streamingRowRef?: RefCallback<HTMLDivElement>
  syncStreamingTailLayout?: () => void
}) {
  const { t } = useTranslation()
  const turnAnchor = chatListItemNavigationAnchor(item)
  const renderMessageView = (view: AgentMessageView, key?: string, assistantPresentation: AssistantMessagePresentation = 'message') => {
    const mutationsAllowed = canMutateMessage?.(view) !== false
    return (
      <AgentMessageItem
        projectId={projectId}
        key={key}
        view={view}
        assistantPresentation={assistantPresentation}
        highlightDialogue={highlightDialogue}
        messageStyle={messageStyle}
        onEditMessage={isStreaming || !mutationsAllowed ? undefined : onEditMessage}
        onEditAssistantReply={isStreaming || !mutationsAllowed ? undefined : onEditAssistantReply}
        onCreateBranch={isStreaming ? undefined : onCreateBranch}
        onRegenerateMessage={isStreaming || !mutationsAllowed ? undefined : onRegenerateMessage}
        onSwitchMessageVersion={isStreaming || !mutationsAllowed ? undefined : onSwitchMessageVersion}
        onOpenSubAgentSession={onOpenSubAgentSession}
        onInsertIllustration={onInsertIllustration}
        onGenerateInteractiveImage={isStreaming || !mutationsAllowed ? undefined : onGenerateInteractiveImage}
        generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
        activeSubAgentSessionKey={activeSubAgentSessionKey}
        subAgentPresentation={subAgentPresentation}
        onApprovePlan={isStreaming ? undefined : onApprovePlan}
        onContinuePlan={isStreaming ? undefined : onContinuePlan}
        onExitPlanMode={isStreaming ? undefined : onExitPlanMode}
        onOpenTrace={onOpenTrace}
        onResolveAsk={onResolveAsk}
        onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      />
    )
  }
  const renderExecutionProcess = (key: string, views: AgentMessageView[], active: boolean, timing?: AgentExecutionTiming) => views.length > 0 ? (
    <AgentExecutionProcess
      projectId={projectId}
      key={key}
      views={views}
      active={active}
      activeSubAgentSessionKey={activeSubAgentSessionKey}
      activeTraceDisplay={activeTraceDisplay}
      highlightDialogue={highlightDialogue}
      messageStyle={messageStyle}
      onInsertIllustration={onInsertIllustration}
      onGenerateInteractiveImage={onGenerateInteractiveImage}
      onOpenSubAgentSession={onOpenSubAgentSession}
      onOpenTrace={onOpenTrace}
      onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      onResolveAsk={onResolveAsk}
      timing={timing}
    />
  ) : null
  const timedProcessKey = item.kind === 'run'
    ? (item.sections.find(section => section.kind === 'process' && section.active)?.key ||
      item.sections.find(section => section.kind === 'process')?.key)
    : undefined
  useLayoutEffect(() => {
    if (isLast && tailFollowActive) syncStreamingTailLayout?.()
  }, [isLast, item, syncStreamingTailLayout, tailFollowActive])

  // A translated active tail would visibly move after its bottom anchor is captured.
  return (
    <motion.div
      ref={isLast ? streamingRowRef : undefined}
      data-nova-chat-item={item.kind}
      data-nova-chat-tail-row
      data-nova-chat-row-key={item.key}
      data-nova-chat-turn-anchor={turnAnchor}
      className={cn('min-w-0 px-6', contentClassName, isLast ? 'pb-0' : 'pb-4')}
      variants={item.kind === 'attachment' ? timelineAttachment : listItem}
      initial={isLast && isStreaming ? false : 'initial'}
      animate="animate"
      transition={{ duration: item.kind === 'attachment' ? 0.14 : 0.18, ease: novaEase }}
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
        <AgentExecutionProcess
          projectId={projectId}
          views={item.views}
          active={item.activeStreamingTrace}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          activeTraceDisplay={activeTraceDisplay}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onInsertIllustration={onInsertIllustration}
          onGenerateInteractiveImage={onGenerateInteractiveImage}
          onOpenSubAgentSession={onOpenSubAgentSession}
          onOpenTrace={onOpenTrace}
          onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
          onResolveAsk={onResolveAsk}
          timing={executionTimings.get(chatListItemRunID(item))}
        />
      ) : item.kind === 'run' ? (
        <div className="space-y-2">
          {item.sections.map(section => section.kind === 'process'
            ? renderExecutionProcess(
                section.key,
                section.views,
                section.active,
                section.key === timedProcessKey ? executionTimings.get(item.runId) : undefined,
              )
            : renderMessageView(section.view, section.key))}
        </div>
      ) : item.kind === 'attachment' ? (
        item.content
      ) : item.kind === 'legacy-message' ? (
        <MessageItem
          projectId={projectId}
          message={item.message}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onOpenSubAgentSession={item.openView && onOpenSubAgentSession ? () => onOpenSubAgentSession(item.openView as AgentMessageView) : undefined}
          activeSubAgentSessionKey={activeSubAgentSessionKey}
          onOpenTrace={onOpenTrace}
          onResolveAsk={item.openView && onResolveAsk ? (_message, action) => onResolveAsk(item.openView as AgentMessageView, action) : undefined}
        />
      ) : (
        renderMessageView(item.view)
      )}
    </motion.div>
  )
}

function buildAgentChatListItems({ views, isStreaming, isExecutionActive, visibleActivityContent, collapseTraceGroups, groupSubAgentTimeline, timelineAttachments }: { views: AgentMessageView[]; isStreaming: boolean; isExecutionActive: boolean; visibleActivityContent: string; collapseTraceGroups: boolean; groupSubAgentTimeline: boolean; timelineAttachments: AgentTimelineAttachment[] }): AgentChatListItem[] {
  const items: AgentChatListItem[] = []
  if (!isStreaming && views.every(isAgentRunMetadataView)) {
    items.push({ kind: 'empty', key: 'empty' })
    return items
  }
  const subAgentGroupsByStart = new Map(
    (groupSubAgentTimeline ? buildAgentSubAgentTimelineGroups(views) : []).map(group => [group.startIndex, group]),
  )

  for (let index = 0; index < views.length; index += 1) {
    const view = views[index]
    if (isAgentRunMetadataView(view)) continue
    const subAgentGroup = subAgentGroupsByStart.get(index)
    if (subAgentGroup) {
      const pendingApprovalView = subAgentGroup.views.find(item => agentViewAskInteraction(item)?.status === 'pending')
      if (pendingApprovalView) {
        const approvalMessage = agentViewToRenderMessage(pendingApprovalView)
        if (approvalMessage) {
          items.push({
            kind: 'legacy-message', key: `subagent-approval-${subAgentGroup.key || index}`,
            message: approvalMessage, sourceIndex: index, openView: pendingApprovalView,
          })
          index = subAgentGroup.nextIndex - 1
          continue
        }
      }
      const progress = buildSubAgentProgressMessage(subAgentGroup.views.map(item => agentViewToRenderMessage(item)).filter((item): item is ChatMessage => Boolean(item)))
      if (progress) {
        items.push({ kind: 'legacy-message', key: `subagent-${subAgentGroup.key || index}`, message: progress, sourceIndex: index, openView: subAgentGroup.views[0] })
        index = subAgentGroup.nextIndex - 1
        continue
      }
    }
    if (collapseTraceGroups) {
      const run = buildAgentRunPresentation(views, index, isExecutionActive)
      if (run) {
        items.push({
          kind: 'run',
          key: run.key,
          runId: run.runID,
          sections: run.sections,
          sourceIndex: index,
        })
        index = run.nextIndex - 1
        continue
      }
    }
    if (collapseTraceGroups && isAgentTraceView(view)) {
      // 连续的 thinking/工具调用统一折成一个分组，不要求后面紧跟正文：
      // 游戏模式正文之后（提交结果、重试循环）和回合末尾的 trace 也归组折叠。
      const traceViews: AgentMessageView[] = []
      let nextIndex = index
      while (nextIndex < views.length && isAgentTraceView(views[nextIndex])) {
        traceViews.push(views[nextIndex])
        nextIndex += 1
      }
      const activeStreamingTrace = isActiveStreamingTrace(views, nextIndex, isExecutionActive)
      items.push({ kind: 'trace', key: `trace-${agentViewStableKey(traceViews[0]) || index}`, views: traceViews, activeStreamingTrace })
      index = nextIndex - 1
      continue
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

function chatListItemRunID(item: AgentChatListItem): string {
  if (item.kind === 'message') return item.view.metadata.run_id || ''
  if (item.kind === 'legacy-message') return item.message.run_id || item.openView?.metadata.run_id || ''
  if (item.kind === 'run') return item.runId
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
  if (item.kind === 'trace') return item.views
  if (item.kind === 'run') {
    return item.sections.flatMap(section => section.kind === 'process' ? section.views : [section.view])
  }
  return []
}

function chatListItemNavigationAnchor(item?: AgentChatListItem) {
  if (!item) return ''
  if (item.kind === 'message') return agentViewNavigationAnchor(item.view)
  if (item.kind === 'legacy-message') return item.message.navigation_turn_id || item.message.turn_id || ''
  if (item.kind === 'run') {
    for (let sectionIndex = item.sections.length - 1; sectionIndex >= 0; sectionIndex -= 1) {
      const section = item.sections[sectionIndex]
      const views = section.kind === 'process' ? section.views : [section.view]
      for (let viewIndex = views.length - 1; viewIndex >= 0; viewIndex -= 1) {
        const anchor = agentViewNavigationAnchor(views[viewIndex])
        if (anchor) return anchor
      }
    }
  }
  return ''
}

function isActiveStreamingTrace(views: AgentMessageView[], afterTraceIndex: number, isStreaming: boolean) {
  if (!isStreaming) return false
  for (let index = afterTraceIndex; index < views.length; index += 1) {
    const view = views[index]
    if (isAgentRunMetadataView(view)) continue
    if (view.kind === 'user') return false
    if (view.kind === 'assistant' && agentViewContent(view).trim()) {
      // A prose row is the semantic boundary after the preceding trace. The
      // prose may still be streaming, but its thinking/tools disclosure is no
      // longer the active tail and must match the completed presentation.
      return false
    }
  }
  return true
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

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}
