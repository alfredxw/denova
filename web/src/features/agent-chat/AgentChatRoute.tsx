import { BookOpen } from 'lucide-react'
import { lazy, memo, Suspense, useCallback, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { EmptyState } from '@/components/common/EmptyState'
import type {
  ReviewFeedbackBatch,
  ReviewFeedbackSelection,
} from '@/features/changes/agent/ReviewFeedbackTray'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { FileNode } from '@/hooks/useWorkspace'
import type { WorkspaceSummary } from '@/lib/api'
import { AgentChatReader, type AgentChatSaveFile } from './AgentChatReader'
import { AgentChatView } from './AgentChatView'
import type { AgentChatPageId, AgentChatPageRenderContext, AgentChatReviewTab } from './types'

const LoreWorkspaceTab = lazy(() => import('@/features/lore/LoreWorkspaceTab').then((module) => ({ default: module.LoreWorkspaceTab })))
const SettingPanel = lazy(() => import('@/features/interactive/components/SettingPanel').then((module) => ({ default: module.SettingPanel })))
const SkillsView = lazy(() => import('@/features/skills/SkillsView').then((module) => ({ default: module.SkillsView })))
const AgentsView = lazy(() => import('@/features/agents/AgentsView').then((module) => ({ default: module.AgentsView })))
const AutomationsView = lazy(() => import('@/features/automations/AutomationsView').then((module) => ({ default: module.AutomationsView })))
const ChangeReviewWorkspace = lazy(() => import('@/features/changes/review/ChangeReviewWorkspace').then((module) => ({ default: module.ChangeReviewWorkspace })))

interface AgentChatRouteProps {
  /** Foreground Writing book. Mutable resource pages are deliberately fenced to this workspace. */
  workspace: string
  composerSettings: WritingComposerSettingsController
  tree: FileNode[]
  summary: WorkspaceSummary | null
  selectedFile: string | null
  tellers: Teller[]
  imagePresets: ImagePreset[]
  documentReview: DocumentReviewController
  documentReviewFeedback?: ReviewFeedbackSelection | null
  onDocumentReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  onDocumentReviewFeedbackSubmitted?: (feedback: ReviewFeedbackBatch) => void
  onDocumentReviewFeedbackSubmissionFailed?: (feedback: ReviewFeedbackBatch) => void
  onTellersChange: (tellers: Teller[]) => void
  onImagePresetsChange: (presets: ImagePreset[]) => void
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  /** The writing workbench's save handler, so a manuscript tab edits through the same path. */
  onSaveFile?: AgentChatSaveFile
  /** Explicitly activates a background project before exposing its mutable resource pages. */
  onActivateWorkspace: (workspace: string) => Promise<boolean>
  /** Registers every mounted project-page draft with the workbench navigation guard. */
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  /** Opens a file from a diff. The navigation result is ignored; the review tab stays open. */
  onOpenFile?: (path: string) => boolean | void | Promise<boolean | void>
}

/**
 * Composes AgentChat project pages from the same focused workspaces used by Writing.
 * AgentChatView remains responsible only for tabs, sessions, feedback routing and layout.
 */
function AgentChatRouteComponent({
  workspace,
  composerSettings,
  tree,
  summary,
  selectedFile,
  tellers,
  imagePresets,
  documentReview,
  documentReviewFeedback,
  onDocumentReviewFeedbackRemove,
  onDocumentReviewFeedbackSubmitted,
  onDocumentReviewFeedbackSubmissionFailed,
  onTellersChange,
  onImagePresetsChange,
  onSetChapterConfirmed,
  onSaveFile,
  onActivateWorkspace,
  onFlushHandlerChange,
  onOpenFile,
}: AgentChatRouteProps) {
  const { t } = useTranslation()

  const pageContent = useCallback((
    tabWorkspace: string,
    pageId: AgentChatPageId,
    context: AgentChatPageRenderContext,
  ): ReactNode => {
    const foregroundPage = tabWorkspace === workspace
    switch (pageId) {
      case 'reader':
        if (!foregroundPage) {
          return <WorkspaceActivationGate workspace={tabWorkspace} activate={context.activateWorkspace} />
        }
        return (
          <AgentChatReader
            key={tabWorkspace}
            workspace={tabWorkspace}
            tree={tree}
            summary={summary}
            initialPath={selectedFile}
            onSaveFile={onSaveFile}
            documentReview={documentReview}
            navigationIntent={context.navigationIntent?.target.kind === 'workspace_file' ? context.navigationIntent : null}
            onOpenLoreTab={() => {
              context.openPage('lore')
            }}
            onSetChapterConfirmed={onSetChapterConfirmed}
            onFlushHandlerChange={context.onFlushHandlerChange}
          />
        )
      case 'lore':
        if (!foregroundPage) {
          return <WorkspaceActivationGate workspace={tabWorkspace} activate={context.activateWorkspace} />
        }
        return (
          <LoreWorkspaceTab
            workspace={tabWorkspace}
            documentReview={documentReview}
            navigationIntent={context.navigationIntent?.target.kind === 'lore_item' ? context.navigationIntent : null}
            onEditorFlushHandlerChange={context.onFlushHandlerChange}
          />
        )
      case 'presets':
        return (
          <SettingPanel
            mode="teller"
            workspace={tabWorkspace}
            presetUsageMode="writing"
            tellers={tellers}
            imagePresets={imagePresets}
            onTellersChange={onTellersChange}
            onImagePresetsChange={onImagePresetsChange}
          />
        )
      case 'skills':
        return <SkillsView workspace={tabWorkspace} />
      case 'agents':
        return <AgentsView />
      case 'automations':
        return <AutomationsView workspace={tabWorkspace} />
    }
  }, [documentReview, imagePresets, onImagePresetsChange, onSaveFile, onSetChapterConfirmed, onTellersChange, selectedFile, summary, tellers, tree, workspace])

  /** Keep each lazy page inside its own boundary so opening it never replaces live conversations. */
  const renderPage = useCallback((
    tabWorkspace: string,
    pageId: AgentChatPageId,
    context: AgentChatPageRenderContext,
  ): ReactNode => (
    <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">{t('router.loading')}</div>}>
      {pageContent(tabWorkspace, pageId, context)}
    </Suspense>
  ), [pageContent, t])

  const renderReview = useCallback((tab: AgentChatReviewTab, close: () => void, disabled: boolean): ReactNode => (
    <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">{t('router.loading')}</div>}>
      <ChangeReviewWorkspace
        workspace={tab.workspace}
        threadID={tab.threadID}
        scopeRequest={tab.groupID ? { id: 0, threadID: tab.threadID, groupID: tab.groupID } : null}
        disabled={disabled}
        selectedPath={tab.workspace === workspace ? selectedFile : null}
        onClose={close}
        onOpenFile={tab.workspace === workspace && onOpenFile ? async (path) => { await onOpenFile(path) } : undefined}
      />
    </Suspense>
  ), [onOpenFile, selectedFile, t, workspace])

  return (
    <AgentChatView
      composerSettings={composerSettings}
      tellers={tellers}
      imagePresets={imagePresets}
      renderPage={renderPage}
      renderReview={renderReview}
      documentReviewWorkspace={workspace}
      documentReviewFeedback={documentReviewFeedback}
      onDocumentReviewFeedbackRemove={onDocumentReviewFeedbackRemove}
      onDocumentReviewFeedbackSubmitted={onDocumentReviewFeedbackSubmitted}
      onDocumentReviewFeedbackSubmissionFailed={onDocumentReviewFeedbackSubmissionFailed}
      onActivateWorkspace={onActivateWorkspace}
      onFlushHandlerChange={onFlushHandlerChange}
    />
  )
}

function WorkspaceActivationGate({ workspace, activate }: { workspace: string; activate: () => Promise<boolean> }) {
  const { t } = useTranslation()
  const [activating, setActivating] = useState(false)
  const projectName = workspace.split(/[\\/]/).filter(Boolean).at(-1) || workspace
  const activateProject = () => {
    if (activating) return
    setActivating(true)
    void activate()
      .catch((error) => {
        console.error('[features/agent-chat/AgentChatRoute.tsx] activating project workspace failed', { workspace, error })
      })
      .finally(() => setActivating(false))
  }
  return (
    <EmptyState
      variant="page"
      icon={BookOpen}
      title={t('agentChat.resource.inactiveTitle', { name: projectName })}
      description={t('agentChat.resource.inactiveDescription')}
      action={{
        label: t(activating ? 'agentChat.resource.activating' : 'agentChat.resource.activate'),
        onClick: activateProject,
      }}
    />
  )
}

export const AgentChatRoute = memo(AgentChatRouteComponent)
