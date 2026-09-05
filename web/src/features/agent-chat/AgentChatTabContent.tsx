import { lazy, Suspense, useCallback, useMemo, type ReactNode } from 'react'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { AgentChatProjectType } from './api'
import type { ReviewFeedbackBatch, ReviewFeedbackComment, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import { useDocumentReview } from '@/features/document-review/use-document-review'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { AgentChatConversationTab } from './AgentChatConversationTab'
import type { AgentChatConversationState } from './AgentChatConversationTab'
import { AgentSubAgentSessionPanel, type AgentSubAgentSessionTarget } from '@/components/Chat/AgentSubAgentSessionPanel'
import { TerminalTabView, type AgentChatTerminalStatus } from './terminal/TerminalTabView'
import type { TerminalSessionInfo } from './terminal/api'
import { tabGroup } from './tab-state'
import type {
  AgentChatDocumentReviewNavigation,
  AgentChatGroupId,
  AgentChatAgentTab,
  AgentChatPageId,
  AgentChatPageRenderContext,
  AgentChatReviewTab,
  AgentChatReviewRenderContext,
  AgentChatTab,
} from './types'
import { ToolNavigationProvider, type ToolNavigationIntent, type ToolNavigationTarget } from '@/components/Chat/tool-navigation'

const FilesTab = lazy(() => import('@/features/files/FilesTab').then((module) => ({ default: module.FilesTab })))

interface AgentChatTabContentProps {
  tab: AgentChatTab
  projectType: AgentChatProjectType
  active: boolean
  running: boolean
  conversationSyncRevision: string
  conversationState?: AgentChatConversationState
  activeSubAgentSession?: AgentSubAgentSessionTarget | null
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  autoSaveEnabled: boolean
  autoSaveDelayMs: number
  filesEditorRefreshSignal: number
  filesTreeRefreshSignal: number
  projectPageRefreshSignal: number
  renderPage: (projectId: string, workspace: string, pageId: AgentChatPageId, context: AgentChatPageRenderContext) => ReactNode
  renderReview: (tab: AgentChatReviewTab, disabled: boolean, context: AgentChatReviewRenderContext) => ReactNode
  navigationIntent: AgentChatDocumentReviewNavigation | null
  toolNavigationIntent: ToolNavigationIntent | null
  onDocumentReviewFeedbackOpen: (projectId: string, selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => void
  onOpenPage: (projectID: string, group: AgentChatGroupId, pageId: AgentChatPageId) => void
  onFlushHandlerChange: (projectID: string, tabId: string, handler: EditorFlushHandler | null) => void
  onFilesSelectedPathChange: (projectID: string, tabId: string, path: string | null) => void
  onOpenProjectFile: (projectID: string, path: string, group: AgentChatGroupId) => void
  onOpenToolTarget: (projectID: string, group: AgentChatGroupId, target: ToolNavigationTarget) => void
  onOpenChangeReview: (projectID: string, workspace: string, reviewThreadID: string, groupID: string) => void
  onWorkspaceChanged?: (
    projectId: string,
    workspace: string,
    paths: string[],
    metadata: WorkspaceChangeMetadata,
  ) => void | Promise<void>
  onRunningChange: (projectID: string, sessionId: string, running: boolean | null) => void
  onConversationStateChange: (projectID: string, tabId: string, state: AgentChatConversationState) => void
  onOpenSubAgentSession: (parentTab: AgentChatAgentTab, target: AgentSubAgentSessionTarget) => void
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
  conversationSyncRevision,
  conversationState,
  activeSubAgentSession,
  composerSettings,
  tellers,
  imagePresets,
  autoSaveEnabled,
  autoSaveDelayMs,
  filesEditorRefreshSignal,
  filesTreeRefreshSignal,
  projectPageRefreshSignal,
  renderPage,
  renderReview,
  navigationIntent,
  toolNavigationIntent,
  onDocumentReviewFeedbackOpen,
  onOpenPage,
  onFlushHandlerChange,
  onFilesSelectedPathChange,
  onOpenProjectFile,
  onOpenToolTarget,
  onOpenChangeReview,
  onWorkspaceChanged,
  onRunningChange,
  onConversationStateChange,
  onOpenSubAgentSession,
  onDraftCommitted,
  onTerminalSessionEstablished,
  onTerminalTitleChange,
  onTerminalStatusChange,
}: AgentChatTabContentProps) {
  const documentReview = useDocumentReview({
    projectId: projectType === 'book' && (tab.kind === 'agent' || tab.kind === 'page') ? tab.projectId : '',
    agentVisible: true,
    onShowAgent: noop,
  })
  const documentReviewController = useMemo(() => ({
    comments: documentReview.visibleComments,
    onCreate: documentReview.addComment,
    onUpdate: documentReview.editComment,
    onDelete: documentReview.removeComment,
  }), [documentReview.addComment, documentReview.editComment, documentReview.removeComment, documentReview.visibleComments])
  const reviewFeedback = useMemo<ReviewFeedbackBatch | null>(() => (
    documentReview.feedback ? [documentReview.feedback] : null
  ), [documentReview.feedback])
  const handleReviewFeedbackOpen = useCallback(
    (selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => {
      onDocumentReviewFeedbackOpen(tab.projectId, selection, comment)
    },
    [onDocumentReviewFeedbackOpen, tab.projectId],
  )
  const removeDocumentReviewFeedback = useCallback((_selection: ReviewFeedbackSelection, commentID: string) => {
    documentReview.removeFeedback(commentID)
  }, [documentReview.removeFeedback])
  const submitDocumentReviewFeedback = useCallback((feedback: ReviewFeedbackBatch) => {
    feedback.forEach(documentReview.submitFeedback)
  }, [documentReview.submitFeedback])
  const restoreDocumentReviewFeedback = useCallback((feedback: ReviewFeedbackBatch) => {
    feedback.forEach(documentReview.restoreFeedback)
  }, [documentReview.restoreFeedback])
  const handlePageFlushHandlerChange = useCallback(
    (handler: EditorFlushHandler | null) => {
      onFlushHandlerChange(tab.projectId, tab.id, handler)
    },
    [onFlushHandlerChange, tab.id, tab.projectId],
  )
  const openPage = useCallback(
    (pageId: AgentChatPageId) => {
      onOpenPage(tab.projectId, tabGroup(tab), pageId)
    },
    [onOpenPage, tab.group, tab.projectId],
  )
  const handleWorkspaceChanged = useCallback((
    changedWorkspace: string,
    paths: string[],
    metadata: WorkspaceChangeMetadata,
  ) => onWorkspaceChanged?.(tab.projectId, changedWorkspace, paths, metadata), [onWorkspaceChanged, tab.projectId])
  const toolNavigation = useMemo(() => ({
    workspace: tab.workspace,
    open: (target: ToolNavigationTarget) => onOpenToolTarget(tab.projectId, tabGroup(tab), target),
  }), [onOpenToolTarget, tab.group, tab.projectId, tab.workspace])
  const handleConversationStateChange = useCallback(
    (state: AgentChatConversationState) => onConversationStateChange(tab.projectId, tab.id, state),
    [onConversationStateChange, tab.id, tab.projectId],
  )
  const handleOpenSubAgentSession = useCallback(
    (target: AgentSubAgentSessionTarget) => {
      if (tab.kind === 'agent') onOpenSubAgentSession(tab, target)
    },
    [onOpenSubAgentSession, tab],
  )
  switch (tab.kind) {
    case 'agent':
      return (
        <ToolNavigationProvider value={toolNavigation}>
        <AgentChatConversationTab
          projectId={tab.projectId}
          projectType={projectType}
          workspace={tab.workspace}
          sessionId={tab.sessionId}
          syncRevision={conversationSyncRevision}
          draft={tab.draft}
          active={active}
          composerSettings={composerSettings}
          tellers={tellers}
          imagePresets={imagePresets}
          reviewFeedback={projectType === 'book' ? reviewFeedback : null}
          onReviewFeedbackOpen={projectType === 'book' ? handleReviewFeedbackOpen : undefined}
          onReviewFeedbackRemove={projectType === 'book' ? removeDocumentReviewFeedback : undefined}
          onReviewFeedbackSubmitted={projectType === 'book' ? submitDocumentReviewFeedback : undefined}
          onReviewFeedbackSubmissionFailed={projectType === 'book' ? restoreDocumentReviewFeedback : undefined}
          onOpenChangeReview={projectType === 'book' ? (threadID, groupID) => onOpenChangeReview(tab.projectId, tab.workspace, threadID, groupID) : undefined}
          onWorkspaceChanged={handleWorkspaceChanged}
          onRunningChange={onRunningChange}
          onDraftCommitted={onDraftCommitted}
          onConversationStateChange={handleConversationStateChange}
          activeSubAgentSession={activeSubAgentSession}
          onSubAgentSessionOpen={handleOpenSubAgentSession}
        />
        </ToolNavigationProvider>
      )
    case 'subagent':
      return (
        <AgentSubAgentSessionPanel
          chrome="tab"
          projectId={tab.projectId}
          messages={conversationState?.messages ?? []}
          sessionKey={tab.sessionKey}
          onResolveAsk={conversationState?.onResolveAsk}
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
    case 'files':
      return (
        <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">…</div>}>
          <FilesTab
            projectId={tab.projectId}
            workspace={tab.workspace}
            selectedPath={tab.selectedPath ?? null}
            autoSaveEnabled={autoSaveEnabled}
            autoSaveDelayMs={autoSaveDelayMs}
            editorRefreshSignal={filesEditorRefreshSignal}
            treeRefreshSignal={filesTreeRefreshSignal}
            onSelectedPathChange={(path) => onFilesSelectedPathChange(tab.projectId, tab.id, path)}
            onFlushHandlerChange={handlePageFlushHandlerChange}
            onWorkspaceChanged={handleWorkspaceChanged}
          />
        </Suspense>
      )
    case 'page':
      return (
        <ToolNavigationProvider value={toolNavigation}>
          {renderPage(tab.projectId, tab.workspace, tab.pageId, {
            projectType,
            navigationIntent,
            toolNavigationIntent,
            documentReview: documentReviewController,
            refreshSignal: projectPageRefreshSignal,
            onFlushHandlerChange: handlePageFlushHandlerChange,
            openPage,
            onWorkspaceChanged: (paths, metadata) => (
              onWorkspaceChanged?.(tab.projectId, tab.workspace, paths, metadata)
            ),
          })}
        </ToolNavigationProvider>
      )
    case 'review':
      return <>{renderReview(tab, running, {
        openFile: (path) => onOpenProjectFile(tab.projectId, path, tabGroup(tab)),
        onWorkspaceChanged: (paths) => onWorkspaceChanged?.(tab.projectId, tab.workspace, paths, {
          impact: 'structure',
          origin: 'external',
        }),
      })}</>
  }
}

function noop() {}
