import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AgentPanel, type WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { ReviewFeedbackBatch, ReviewFeedbackComment, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import {
  workspaceChangeImpact,
  workspaceChangePaths,
  type WorkspaceChangeMetadata,
} from '@/features/changes/types'
import { useAgentChat } from '@/hooks/useAgentChat'
import { createProjectAgentChatClient } from '@/hooks/agent-chat-client'

interface AgentChatConversationTabProps {
  projectId: string
  projectType: 'book' | 'general'
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
}: AgentChatConversationTabProps) {
  const client = useMemo(() => createProjectAgentChatClient(projectId, sessionId), [projectId, sessionId])
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
    (message: string, options?: Parameters<typeof chat.send>[1]) =>
      chat.send(message, {
        ...options,
        onSubmissionStart: () => {
          options?.onSubmissionStart?.()
          if (!draft || draftCommittedRef.current) return
          draftCommittedRef.current = true
          // The first request now owns the local session ID. Do not reload history when the parent
          // flips the tab out of draft state; the live useChat instance already owns that stream.
          initializedRef.current = true
          onDraftCommitted?.(message)
        },
      }),
    [chat.send, draft, onDraftCommitted],
  )

  useEffect(() => {
    onRunningChange?.(projectId, sessionId, chat.isExecutionActive)
  }, [chat.isExecutionActive, onRunningChange, projectId, sessionId])

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
      agentKind={projectType === 'general' ? 'general' : 'writing'}
      workspace={workspace}
      chrome="workbench"
      initializing={!initialContentReady}
      composerSettings={composerSettings}
      selectedFile={null}
      tellers={tellers}
      imagePresets={imagePresets}
      reviewFeedback={reviewFeedback}
      onReviewFeedbackOpen={onReviewFeedbackOpen}
      onReviewFeedbackRemove={onReviewFeedbackRemove}
      onReviewFeedbackSubmitted={onReviewFeedbackSubmitted}
      onReviewFeedbackSubmissionFailed={onReviewFeedbackSubmissionFailed}
      messages={chat.messages}
      sessions={chat.sessions}
      activeSessionId={chat.activeSessionId || sessionId}
      sessionDraft={draft}
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
      loreReferenceLabels={{}}
      loreSuggestions={[]}
      styleScenes={chat.styleScenes}
      textSelections={chat.textSelections}
      planMode={chat.planMode}
      hasEarlierMessages={chat.hasEarlierMessages}
      isLoadingEarlierHistory={chat.isLoadingEarlierHistory}
      fileSuggestions={[]}
      onCreateSession={chat.createChatSession}
      onSwitchSession={chat.switchChatSession}
      onRenameSession={chat.renameChatSession}
      onDeleteSession={chat.deleteChatSession}
      onLoadEarlierHistory={chat.loadEarlierHistory}
      onRefreshHistory={chat.loadHistory}
      onAnswerAsk={client.answerSessionAsk}
      onCancelAsk={client.cancelSessionAsk}
      onRemoveContextCompaction={client.removeContextCompaction}
      onSend={send}
      onAnalyzeContext={chat.analyzeContext}
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
      onPlanModeChange={chat.setPlanMode}
      onPlanModeToggle={chat.togglePlanMode}
      onApproveProposedPlan={chat.approveProposedPlan}
      onExitPlanMode={chat.exitPlanMode}
      onOpenChangeReview={onOpenChangeReview}
      onWorkspaceChanged={(paths) => onWorkspaceChanged?.(workspace, paths, {
        impact: 'structure',
        origin: 'external',
      })}
      onClose={() => {}}
    />
  )
}

export const AgentChatConversationTab = memo(AgentChatConversationTabComponent)
