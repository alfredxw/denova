import { useCallback, useMemo, type ReactNode } from 'react'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { AgentChatProjectType } from './api'
import type { ReviewFeedbackBatch, ReviewFeedbackComment, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { AgentChatConversationTab } from './AgentChatConversationTab'
import { TerminalTabView, type AgentChatTerminalStatus } from './terminal/TerminalTabView'
import type { TerminalSessionInfo } from './terminal/api'
import { tabGroup } from './tab-state'
import type {
  AgentChatDocumentReviewNavigation,
  AgentChatGroupId,
  AgentChatPageId,
  AgentChatPageRenderContext,
  AgentChatReviewTab,
  AgentChatTab,
} from './types'

interface AgentChatTabContentProps {
  tab: AgentChatTab
  projectType: AgentChatProjectType
  active: boolean
  running: boolean
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  renderPage: (workspace: string, pageId: AgentChatPageId, context: AgentChatPageRenderContext) => ReactNode
  renderReview: (tab: AgentChatReviewTab, disabled: boolean) => ReactNode
  navigationIntent: AgentChatDocumentReviewNavigation | null
  documentReviewFeedback?: ReviewFeedbackSelection | null
  onDocumentReviewFeedbackOpen: (workspace: string, selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => void
  onDocumentReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  onDocumentReviewFeedbackSubmitted?: (feedback: ReviewFeedbackBatch) => void
  onDocumentReviewFeedbackSubmissionFailed?: (feedback: ReviewFeedbackBatch) => void
  onOpenPage: (projectID: string, group: AgentChatGroupId, pageId: AgentChatPageId) => void
  onActivateWorkspace: (workspace: string) => Promise<boolean>
  onPageFlushHandlerChange: (projectID: string, tabId: string, handler: EditorFlushHandler | null) => void
  onOpenChangeReview: (projectID: string, workspace: string, reviewThreadID: string, groupID: string) => void
  onWorkspaceChanged?: (workspace: string, paths: string[]) => void | Promise<void>
  onRunningChange: (projectID: string, sessionId: string, running: boolean | null) => void
  onDraftCommitted: (message: string) => void
  onTerminalSessionEstablished: (tabId: string, session: TerminalSessionInfo) => boolean
  onTerminalTitleChange: (tabId: string, title: string) => void
  onTerminalStatusChange: (tabId: string, status: AgentChatTerminalStatus | null) => void
}

/** Maps a tab record onto its independent, persistently mounted runtime surface. */
export function AgentChatTabContent({
  tab,
  projectType,
  active,
  running,
  composerSettings,
  tellers,
  imagePresets,
  renderPage,
  renderReview,
  navigationIntent,
  documentReviewFeedback,
  onDocumentReviewFeedbackOpen,
  onDocumentReviewFeedbackRemove,
  onDocumentReviewFeedbackSubmitted,
  onDocumentReviewFeedbackSubmissionFailed,
  onOpenPage,
  onActivateWorkspace,
  onPageFlushHandlerChange,
  onOpenChangeReview,
  onWorkspaceChanged,
  onRunningChange,
  onDraftCommitted,
  onTerminalSessionEstablished,
  onTerminalTitleChange,
  onTerminalStatusChange,
}: AgentChatTabContentProps) {
  const reviewFeedback = useMemo<ReviewFeedbackBatch | null>(() => (documentReviewFeedback ? [documentReviewFeedback] : null), [documentReviewFeedback])
  const handleReviewFeedbackOpen = useCallback(
    (selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => {
      onDocumentReviewFeedbackOpen(tab.workspace, selection, comment)
    },
    [onDocumentReviewFeedbackOpen, tab.workspace],
  )
  const handlePageFlushHandlerChange = useCallback(
    (handler: EditorFlushHandler | null) => {
      onPageFlushHandlerChange(tab.projectId, tab.id, handler)
    },
    [onPageFlushHandlerChange, tab.id, tab.projectId],
  )
  const openPage = useCallback(
    (pageId: AgentChatPageId) => {
      onOpenPage(tab.projectId, tabGroup(tab), pageId)
    },
    [onOpenPage, tab.group, tab.projectId],
  )
  const activateWorkspace = useCallback(() => onActivateWorkspace(tab.workspace), [onActivateWorkspace, tab.workspace])

  switch (tab.kind) {
    case 'agent':
      return (
        <AgentChatConversationTab
          projectId={tab.projectId}
          projectType={projectType}
          workspace={tab.workspace}
          sessionId={tab.sessionId}
          draft={tab.draft}
          active={active}
          composerSettings={composerSettings}
          tellers={tellers}
          imagePresets={imagePresets}
          reviewFeedback={projectType === 'book' ? reviewFeedback : null}
          onReviewFeedbackOpen={projectType === 'book' ? handleReviewFeedbackOpen : undefined}
          onReviewFeedbackRemove={projectType === 'book' ? onDocumentReviewFeedbackRemove : undefined}
          onReviewFeedbackSubmitted={projectType === 'book' ? onDocumentReviewFeedbackSubmitted : undefined}
          onReviewFeedbackSubmissionFailed={projectType === 'book' ? onDocumentReviewFeedbackSubmissionFailed : undefined}
          onOpenChangeReview={projectType === 'book' ? (threadID, groupID) => onOpenChangeReview(tab.projectId, tab.workspace, threadID, groupID) : undefined}
          onWorkspaceChanged={onWorkspaceChanged}
          onRunningChange={onRunningChange}
          onDraftCommitted={onDraftCommitted}
        />
      )
    case 'terminal':
      return (
        <TerminalTabView
          tab={tab}
          active={active}
          onSessionEstablished={onTerminalSessionEstablished}
          onTitleChange={onTerminalTitleChange}
          onStatusChange={onTerminalStatusChange}
        />
      )
    case 'page':
      return (
        <>
          {renderPage(tab.workspace, tab.pageId, {
            navigationIntent,
            onFlushHandlerChange: handlePageFlushHandlerChange,
            openPage,
            activateWorkspace,
          })}
        </>
      )
    case 'review':
      return <>{renderReview(tab, running)}</>
  }
}
