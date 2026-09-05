import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AgentPanel,
  type AgentPanelChrome,
  type AgentPanelProps,
  type AgentPanelView,
  type WritingComposerSettingsController,
} from '@/components/Chat/AgentPanel'
import type { AgentSubAgentAskResolver, AgentSubAgentSessionTarget } from '@/components/Chat/AgentSubAgentSessionPanel'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { ReviewFeedbackBatch, ReviewFeedbackComment, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import {
  workspaceChangeImpact,
  workspaceChangePaths,
  type WorkspaceChangeMetadata,
} from '@/features/changes/types'
import { useAgentChat } from '@/hooks/useAgentChat'
import { createProjectAgentChatClient } from '@/hooks/agent-chat-client'
import { resolveAgentAskAndRefresh } from '@/lib/agent-ask'
import { agentViewAskID } from '@/lib/agent-message-view'

export interface AgentChatPendingAction {
  id: string
  message: string
  displayMessage: string
}

export interface AgentChatConversationTabProps {
  projectId: string
  projectType: 'book' | 'general' | 'agents'
  workspace: string
  sessionId: string
  /** Changes when an external owner starts or settles a turn in this durable conversation. */
  syncRevision?: string
  draft?: boolean
  active: boolean
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  reviewFeedback?: ReviewFeedbackBatch | null
  onReviewFeedbackOpen?: (selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => void
  onReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  onReviewFeedbackSubmitted?: (feedback: ReviewFeedbackBatch) => void
  onReviewFeedbackSubmissionFailed?: (feedback: ReviewFeedbackBatch) => void
  onOpenChangeReview?: (reviewThreadID: string, groupID: string) => void
  onWorkspaceChanged?: (workspace: string, paths: string[], metadata: WorkspaceChangeMetadata) => void | Promise<void>
  onRunningChange?: (projectID: string, sessionId: string, running: boolean | null) => void
  onDraftCommitted?: (message: string) => void
  pendingAction?: AgentChatPendingAction | null
  onPendingActionConsumed?: (id: string) => void
  /** Adds host-owned model context while keeping the user's message as the transcript projection. */
  messageTransform?: (message: string) => string
  onConversationStateChange?: (state: AgentChatConversationState) => void
  activeSubAgentSession?: AgentSubAgentSessionTarget | null
  onSubAgentSessionOpen?: (target: AgentSubAgentSessionTarget) => void | Promise<void>
  host?: AgentChatConversationHost
}

export interface AgentChatConversationState {
  sessionId: string
  messages: AgentPanelProps['messages']
  isStreaming: boolean
  onResolveAsk?: AgentSubAgentAskResolver
}

/** Optional controls supplied when a durable conversation is hosted in the Writing dock. */
export interface AgentChatConversationHost {
  chrome: AgentPanelChrome
  view: AgentPanelView
  onViewChange: (view: AgentPanelView) => void
  sessions: AgentPanelProps['sessions']
  sessionTransitionPending: boolean
  sessionActionsDisabled: boolean
  sessionRailVisible: boolean
  onSessionRailVisibleChange: (visible: boolean) => void
  onCreateSession: AgentPanelProps['onCreateSession']
  onSwitchSession: AgentPanelProps['onSwitchSession']
  onRenameSession: AgentPanelProps['onRenameSession']
  onDeleteSession: AgentPanelProps['onDeleteSession']
  quickPromptScope?: AgentPanelProps['quickPromptScope']
  composerDraftScope?: AgentPanelProps['composerDraftScope']
  currentChapter?: AgentPanelProps['currentChapter']
  selectedFile: AgentPanelProps['selectedFile']
  ideContext?: AgentPanelProps['ideContext']
  fileSuggestions: AgentPanelProps['fileSuggestions']
  loreReferenceLabels: AgentPanelProps['loreReferenceLabels']
  loreSuggestions: AgentPanelProps['loreSuggestions']
  onInsertIllustration?: AgentPanelProps['onInsertIllustration']
  activeSubAgentSession?: AgentSubAgentSessionTarget | null
  onSubAgentSessionOpen?: (target: AgentSubAgentSessionTarget) => void | Promise<void>
  composerContext: {
    references: AgentPanelProps['references']
    loreReferences: AgentPanelProps['loreReferences']
    styleScenes: AgentPanelProps['styleScenes']
    textSelections: AgentPanelProps['textSelections']
    onReferenceConsumed: AgentPanelProps['onReferenceRemove']
    onLoreReferenceConsumed: AgentPanelProps['onLoreReferenceRemove']
    onStyleSceneConsumed: AgentPanelProps['onStyleSceneRemove']
    onTextSelectionConsumed: AgentPanelProps['onTextSelectionRemove']
  }
}

/**
 * One independently running AgentChat conversation.
 *
 * The component stays mounted after its tab is first shown. Its hook and transport are bound
 * to one immutable project/session pair, so switching tabs never re-points a shared runtime.
 */
function AgentChatConversationTabComponent({
  projectId,
  projectType,
  workspace,
  sessionId,
  syncRevision = '',
  draft = false,
  active,
  composerSettings,
  tellers,
  imagePresets,
  reviewFeedback,
  onReviewFeedbackOpen,
  onReviewFeedbackRemove,
  onReviewFeedbackSubmitted,
  onReviewFeedbackSubmissionFailed,
  onOpenChangeReview,
  onWorkspaceChanged,
  onRunningChange,
  onDraftCommitted,
  pendingAction,
  onPendingActionConsumed,
  messageTransform,
  onConversationStateChange,
  activeSubAgentSession,
  onSubAgentSessionOpen,
  host,
}: AgentChatConversationTabProps) {
  const client = useMemo(
    () => createProjectAgentChatClient(projectId, sessionId),
    [projectId, sessionId],
  )
  const chat = useAgentChat({
    projectId,
    client,
    onAgentFileChange: () => onWorkspaceChanged?.(workspace, [], {
      impact: 'structure',
      origin: 'external',
    }),
    onWorkspaceChange: (event) => onWorkspaceChanged?.(workspace, workspaceChangePaths(event), {
      impact: workspaceChangeImpact(event),
      origin: 'external',
    }),
  })
  const initializedRef = useRef(false)
  const draftCommittedRef = useRef(false)
  const observedSyncRevisionRef = useRef(syncRevision)
  const [initialContentReady, setInitialContentReady] = useState(draft)
  const mountedRef = useRef(false)
  const submittedActionRef = useRef('')
  const consumedComposerStringsRef = useRef(new Set<string>())
  const consumedTextSelectionsRef = useRef(new Set<AgentPanelProps['textSelections'][number]>())

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const synchronize = useCallback(async (onHistoryLoaded?: () => void) => {
    await Promise.all([chat.loadSessions(), chat.loadHistory(sessionId)])
    onHistoryLoaded?.()
    await chat.resumeActiveChat(sessionId)
  }, [chat.loadHistory, chat.loadSessions, chat.resumeActiveChat, sessionId])

  useEffect(() => {
    if (draft) {
      setInitialContentReady(true)
      return
    }
    if (initializedRef.current) return
    initializedRef.current = true
    void synchronize(() => {
      if (mountedRef.current) setInitialContentReady(true)
    })
      .catch((error) => {
        console.error('[features/agent-chat/AgentChatConversationTab.tsx] initial conversation synchronization failed', {
          projectId,
          sessionId,
          error,
        })
      })
      .finally(() => {
        if (mountedRef.current) setInitialContentReady(true)
      })
  }, [draft, projectId, sessionId, synchronize])

  useEffect(() => {
    if (observedSyncRevisionRef.current === syncRevision) return
    observedSyncRevisionRef.current = syncRevision
    // A locally-owned turn already streams into this hook. Only external owners need a history
    // refresh and active-run inspection, otherwise a Project metadata poll could double-attach it.
    if (draft || !initializedRef.current || chat.isExecutionActive) return
    void synchronize()
  }, [chat.isExecutionActive, draft, syncRevision, synchronize])

  const send = useCallback(
    (message: string, options?: Parameters<typeof chat.send>[1]) => {
      const transformed = messageTransform?.(message) ?? message
      return chat.send(transformed, {
        ...options,
        displayMessage: options?.displayMessage ?? (transformed === message ? undefined : message),
        onSubmissionStart: () => {
          options?.onSubmissionStart?.()
          if (!draft || draftCommittedRef.current) return
          draftCommittedRef.current = true
          // The first request now owns the local session ID. Do not reload history when the parent
          // flips the tab out of draft state; the live useChat instance already owns that stream.
          initializedRef.current = true
          onDraftCommitted?.(message)
        },
      })
    },
    [chat.send, draft, messageTransform, onDraftCommitted],
  )

  useEffect(() => {
    if (!pendingAction || !initialContentReady || chat.isExecutionActive || submittedActionRef.current === pendingAction.id) return
    submittedActionRef.current = pendingAction.id
    void Promise.resolve(send(pendingAction.message, { displayMessage: pendingAction.displayMessage }))
      .then((accepted) => {
        if (accepted) {
          onPendingActionConsumed?.(pendingAction.id)
          return
        }
        submittedActionRef.current = ''
      })
      .catch((error) => {
        submittedActionRef.current = ''
        console.error('[features/agent-chat/AgentChatConversationTab.tsx] pending action submission failed', {
          projectId,
          sessionId,
          actionId: pendingAction.id,
          error,
        })
      })
  }, [chat.isExecutionActive, initialContentReady, onPendingActionConsumed, pendingAction, projectId, send, sessionId])

  useEffect(() => {
    onRunningChange?.(projectId, sessionId, chat.isExecutionActive)
  }, [chat.isExecutionActive, onRunningChange, projectId, sessionId])

  const resolveAsk = useCallback<AgentSubAgentAskResolver>(async (view, action) => {
    const askID = agentViewAskID(view)
    if (!askID) throw new Error('Cannot resolve an Ask without its interaction ID')
    return resolveAgentAskAndRefresh(
      action,
      {
        answer: (answers) => client.answerSessionAsk(sessionId, askID, answers),
        cancel: () => client.cancelSessionAsk(sessionId, askID),
      },
      () => chat.loadHistory(sessionId),
    )
  }, [chat.loadHistory, client, sessionId])

  useEffect(() => {
    onConversationStateChange?.({
      sessionId,
      messages: chat.messages,
      isStreaming: chat.isStreaming,
      onResolveAsk: resolveAsk,
    })
  }, [chat.isStreaming, chat.messages, onConversationStateChange, resolveAsk, sessionId])

  const hostedComposerContext = host?.composerContext
  useEffect(() => {
    if (!active || !hostedComposerContext) return
    const availableStringKeys = new Set([
      ...hostedComposerContext.references.map((value) => `file:${value}`),
      ...hostedComposerContext.loreReferences.map((value) => `lore:${value}`),
      ...hostedComposerContext.styleScenes.map((value) => `style:${value}`),
    ])
    for (const key of consumedComposerStringsRef.current) {
      if (!availableStringKeys.has(key)) consumedComposerStringsRef.current.delete(key)
    }
    for (const selection of consumedTextSelectionsRef.current) {
      if (!hostedComposerContext.textSelections.includes(selection)) consumedTextSelectionsRef.current.delete(selection)
    }
    for (const reference of hostedComposerContext.references) {
      const key = `file:${reference}`
      if (consumedComposerStringsRef.current.has(key)) continue
      consumedComposerStringsRef.current.add(key)
      chat.addReference(reference)
      hostedComposerContext.onReferenceConsumed(reference)
    }
    for (const reference of hostedComposerContext.loreReferences) {
      const key = `lore:${reference}`
      if (consumedComposerStringsRef.current.has(key)) continue
      consumedComposerStringsRef.current.add(key)
      chat.addLoreReference(reference)
      hostedComposerContext.onLoreReferenceConsumed(reference)
    }
    for (const scene of hostedComposerContext.styleScenes) {
      const key = `style:${scene}`
      if (consumedComposerStringsRef.current.has(key)) continue
      consumedComposerStringsRef.current.add(key)
      chat.addStyleScene(scene)
      hostedComposerContext.onStyleSceneConsumed(scene)
    }
    for (let index = hostedComposerContext.textSelections.length - 1; index >= 0; index -= 1) {
      const selection = hostedComposerContext.textSelections[index]
      if (consumedTextSelectionsRef.current.has(selection)) continue
      consumedTextSelectionsRef.current.add(selection)
      chat.addTextSelection(selection)
      hostedComposerContext.onTextSelectionConsumed(index)
    }
  }, [
    active,
    chat.addLoreReference,
    chat.addReference,
    chat.addStyleScene,
    chat.addTextSelection,
    hostedComposerContext,
  ])

  useEffect(
    () => () => {
      onRunningChange?.(projectId, sessionId, null)
    },
    [onRunningChange, projectId, sessionId],
  )

  return (
    <AgentPanel
      projectId={projectId}
      active={active}
      agentKind={projectType === 'book' ? 'writing' : 'general'}
      workspace={workspace}
      chrome={host?.chrome ?? 'workbench'}
      view={host?.view}
      onViewChange={host?.onViewChange}
      sessionActionsDisabled={host?.sessionActionsDisabled}
      sessionRailVisible={host?.sessionRailVisible}
      onSessionRailVisibleChange={host?.onSessionRailVisibleChange}
      initializing={!initialContentReady}
      quickPromptScope={host?.quickPromptScope}
      composerDraftScope={host?.composerDraftScope}
      composerSettings={composerSettings}
      currentChapter={host?.currentChapter}
      selectedFile={host?.selectedFile ?? null}
      tellers={tellers}
      imagePresets={imagePresets}
      reviewFeedback={reviewFeedback}
      onReviewFeedbackOpen={onReviewFeedbackOpen}
      onReviewFeedbackRemove={onReviewFeedbackRemove}
      onReviewFeedbackSubmitted={onReviewFeedbackSubmitted}
      onReviewFeedbackSubmissionFailed={onReviewFeedbackSubmissionFailed}
      messages={chat.messages}
      sessions={host?.sessions ?? chat.sessions}
      activeSessionId={chat.activeSessionId || sessionId}
      sessionDraft={draft}
      sessionTransitionPending={host?.sessionTransitionPending}
      conversationBinding={{ mode: 'agent_chat', project_id: projectId, session_id: sessionId }}
      isStreaming={chat.isStreaming}
      isExecutionActive={chat.isExecutionActive}
      runtimeProjection={chat.runtimeProjection}
      abortPending={chat.abortPending}
      commandSubmitting={chat.commandSubmitting}
      queueActionPendingCommandID={chat.queueActionPendingCommandID}
      activityContent={chat.activityContent}
      references={chat.references}
      loreReferences={chat.loreReferences}
      loreReferenceLabels={host?.loreReferenceLabels ?? {}}
      loreSuggestions={host?.loreSuggestions ?? []}
      styleScenes={chat.styleScenes}
      textSelections={chat.textSelections}
      planMode={chat.planMode}
      hasEarlierMessages={chat.hasEarlierMessages}
      isLoadingEarlierHistory={chat.isLoadingEarlierHistory}
      fileSuggestions={host?.fileSuggestions ?? []}
      onCreateSession={host?.onCreateSession ?? chat.createChatSession}
      onSwitchSession={host?.onSwitchSession ?? chat.switchChatSession}
      onRenameSession={host?.onRenameSession ?? chat.renameChatSession}
      onDeleteSession={host?.onDeleteSession ?? chat.deleteChatSession}
      onLoadEarlierHistory={chat.loadEarlierHistory}
      onRefreshHistory={chat.loadHistory}
      onAnswerAsk={client.answerSessionAsk}
      onCancelAsk={client.cancelSessionAsk}
      onRemoveContextCompaction={client.removeContextCompaction}
      onSend={send}
      onAnalyzeContext={(message, options) => chat.analyzeContext(messageTransform?.(message) ?? message, options)}
      ideContext={host?.ideContext}
      onStop={chat.stop}
      onSteerQueuedCommand={chat.steerQueuedCommand}
      onDeleteQueuedCommand={chat.deleteQueuedCommand}
      onEditQueuedCommand={chat.editQueuedCommand}
      onReferenceRemove={chat.removeReference}
      onLoreReferenceAdd={chat.addLoreReference}
      onLoreReferenceRemove={chat.removeLoreReference}
      onStyleSceneAdd={chat.addStyleScene}
      onStyleSceneRemove={chat.removeStyleScene}
      onTextSelectionRemove={chat.removeTextSelection}
      onInsertIllustration={host?.onInsertIllustration}
      onPlanModeChange={chat.setPlanMode}
      onPlanModeToggle={chat.togglePlanMode}
      onApproveProposedPlan={chat.approveProposedPlan}
      onExitPlanMode={chat.exitPlanMode}
      onOpenChangeReview={onOpenChangeReview}
      activeSubAgentSession={activeSubAgentSession ?? host?.activeSubAgentSession}
      onSubAgentSessionOpen={onSubAgentSessionOpen ?? host?.onSubAgentSessionOpen}
      onWorkspaceChanged={(paths) => onWorkspaceChanged?.(workspace, paths, {
        impact: 'structure',
        origin: 'external',
      })}
    />
  )
}

export const AgentChatConversationTab = memo(AgentChatConversationTabComponent)
