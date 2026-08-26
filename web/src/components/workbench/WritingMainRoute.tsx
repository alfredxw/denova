import { lazy, memo } from 'react'
import type { ComponentProps, ReactNode } from 'react'
import { PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ImageFilePreview } from './ImageFilePreview'
import { WritingDocumentEditor } from '@/components/Editor/WritingDocumentEditor'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { WritingSourceEditor } from '@/components/Editor/WritingSourceEditor'
import { ChangeReviewWorkspace } from '@/features/changes/review/ChangeReviewWorkspace'
import { ProjectBinaryPreview } from '@/features/files/ProjectSourceEditor'
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import type { ChapterSummary, TextSelection, WorkspaceSummary } from '@/lib/api'
import type { ProjectFileDocument } from '@/lib/api-client/project-files'
import type { ReviewFeedbackNavigationTarget } from './use-review-feedback-navigation'
import type { Tab } from './TabController'
import { TabController } from './TabController'
import { WorkbenchRouteLayer } from './WorkbenchRouteHost'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'

const LoreWorkspaceTab = memo(lazy(() => import('@/features/lore/LoreWorkspaceTab').then((module) => ({ default: module.LoreWorkspaceTab }))))
const StableImageFilePreview = memo(ImageFilePreview)
const StableWritingDocumentEditor = memo(WritingDocumentEditor)
const StableWritingSourceEditor = memo(WritingSourceEditor)
const StableProjectBinaryPreview = memo(ProjectBinaryPreview)
const StableChangeReviewWorkspace = memo(ChangeReviewWorkspace)
const StableTabController = memo(TabController)

type ChangeReviewProps = ComponentProps<typeof ChangeReviewWorkspace>
type EditorProps = ComponentProps<typeof WritingDocumentEditor>
type LoreWorkspaceProps = ComponentProps<typeof LoreWorkspaceTab>

interface WritingMainRouteProps {
  visible: boolean
  loadingLabel: string
  projectId: string
  workspace: string
  fileTree: readonly ProjectFileExplorerNode[]
  activeReviewThreadID: string
  activeReviewRequest: ChangeReviewProps['scopeRequest']
  submittedReviewCommentIDs: ChangeReviewProps['hiddenCommentIDs']
  isStreaming: boolean
  selectedFile: string | null
  agentVisible: boolean
  tabs: Tab[]
  activeTabKey: string | null
  activeTab: Tab | null
  summary: WorkspaceSummary | null
  tabActions: ReactNode
  activeFileKind: string | null
  fileDocument: ProjectFileDocument | null
  fileContent: string
  fileRevision: string
  saveSignal: number
  editorAutoSaveEnabled: boolean
  editorAutoSaveDelayMs: number
  currentChapter?: ChapterSummary
  editorSearchIntent: EditorProps['searchIntent']
  illustrationInsertSignal: EditorProps['illustrationInsertSignal']
  documentReview: NonNullable<EditorProps['documentReview']>
  documentReviewNavigationTarget: ReviewFeedbackNavigationTarget | null
  toolNavigationIntent: ToolNavigationIntent | null
  readingTypography: EditorProps['readingTypography']
  loreEmpty: boolean
  onToggleAgent: () => void
  onCloseReview: () => void
  onOpenReviewFile: ChangeReviewProps['onOpenFile']
  onWorkspaceChanged: (paths: string[]) => void | Promise<void>
  onFeedbackCommentsChange: ChangeReviewProps['onFeedbackCommentsChange']
  onActivateTab: (tab: Tab) => void
  onCloseTab: (tab: Tab) => void
  onToggleTabPin: (tab: Tab) => void
  onMoveTab: (sourceKey: string, targetKey: string) => void
  onEditorFlushHandlerChange: (handler: EditorFlushHandler | null) => void
  onOpenLoreLibrary: () => void
  onReferenceLoreItem: LoreWorkspaceProps['onReferenceItem']
  onSelectFile: (path: string) => unknown
  onSaveCurrentFile: EditorProps['onSave']
  onQuoteSelection: (selection: TextSelection) => void
  onRevealChapter: EditorProps['onRevealChapter']
  onGenerateIllustration: EditorProps['onGenerateIllustration']
  emptyText: string
  emptyLoreTitle: string
  emptyLoreDescription: string
  emptyLoreAction: string
  onRequestWritingInit: () => void
}

export function WritingMainRoute({
  visible,
  loadingLabel,
  projectId,
  workspace,
  fileTree,
  activeReviewThreadID,
  activeReviewRequest,
  submittedReviewCommentIDs,
  isStreaming,
  selectedFile,
  agentVisible,
  tabs,
  activeTabKey,
  activeTab,
  summary,
  tabActions,
  activeFileKind,
  fileDocument,
  fileContent,
  fileRevision,
  saveSignal,
  editorAutoSaveEnabled,
  editorAutoSaveDelayMs,
  currentChapter,
  editorSearchIntent,
  illustrationInsertSignal,
  documentReview,
  documentReviewNavigationTarget,
  toolNavigationIntent,
  readingTypography,
  loreEmpty,
  onToggleAgent,
  onCloseReview,
  onOpenReviewFile,
  onWorkspaceChanged,
  onFeedbackCommentsChange,
  onActivateTab,
  onCloseTab,
  onToggleTabPin,
  onMoveTab,
  onEditorFlushHandlerChange,
  onOpenLoreLibrary,
  onReferenceLoreItem,
  onSelectFile,
  onSaveCurrentFile,
  onQuoteSelection,
  onRevealChapter,
  onGenerateIllustration,
  emptyText,
  emptyLoreTitle,
  emptyLoreDescription,
  emptyLoreAction,
  onRequestWritingInit,
}: WritingMainRouteProps) {
  const reviewVisible = Boolean(activeReviewThreadID)
  const activeDocument = fileDocument?.path === selectedFile ? fileDocument : null
  return (
    <WorkbenchRouteLayer visible={visible} loadingLabel={loadingLabel}>
      <div
        data-writing-content-layer="true"
        aria-hidden={reviewVisible}
        inert={reviewVisible}
        // Keep every tab-local overlay inside this base stacking context. Inert controls interaction,
        // but it does not stop a positioned close button from painting above the review surface.
        className={`absolute inset-0 z-0 flex min-h-0 flex-col ${reviewVisible ? 'pointer-events-none' : ''}`}
      >
        <>
          <StableTabController
            tabs={tabs}
            activeTabKey={activeTabKey}
            summary={summary}
            actions={tabActions}
            onActivateTab={onActivateTab}
            onCloseTab={onCloseTab}
            onTogglePin={onToggleTabPin}
            onMoveTab={onMoveTab}
          />
          <div className="flex min-h-0 flex-1 flex-col">
            {activeTab ? (
              activeTab.kind === 'lore' ? (
                <LoreWorkspaceTab
                  projectId={projectId}
                  documentReview={documentReview}
                  navigationIntent={documentReviewNavigationTarget?.target.kind === 'lore_item' ? documentReviewNavigationTarget : null}
                  toolNavigationIntent={toolNavigationIntent}
                  onEditorFlushHandlerChange={onEditorFlushHandlerChange}
                  onOpenLibrary={onOpenLoreLibrary}
                  onReferenceItem={onReferenceLoreItem}
                />
              ) : activeFileKind === 'image' || activeDocument?.kind === 'image' ? (
                <StableImageFilePreview projectId={projectId} path={selectedFile || activeTab.path} revision={fileRevision} />
              ) : activeDocument?.kind === 'binary' ? (
                <StableProjectBinaryPreview />
              ) : activeFileKind !== 'markdown' && activeDocument?.kind === 'text' ? (
                <StableWritingSourceEditor
                  projectId={projectId}
                  workspace={workspace}
                  fileTree={fileTree}
                  document={activeDocument}
                  onSelectFile={onSelectFile}
                  onSave={onSaveCurrentFile}
                  onQuoteSelection={onQuoteSelection}
                  saveSignal={saveSignal}
                  autoSaveEnabled={editorAutoSaveEnabled}
                  autoSaveDelayMs={editorAutoSaveDelayMs}
                  searchIntent={editorSearchIntent}
                  onFlushHandlerChange={onEditorFlushHandlerChange}
                />
              ) : (
                <StableWritingDocumentEditor
                  projectId={projectId}
                  fileName={selectedFile}
                  content={fileContent}
                  revision={fileRevision}
                  onSave={onSaveCurrentFile}
                  onQuoteSelection={onQuoteSelection}
                  saveSignal={saveSignal}
                  autoSaveEnabled={editorAutoSaveEnabled}
                  autoSaveDelayMs={editorAutoSaveDelayMs}
                  chapterSummary={currentChapter}
                  onRevealChapter={onRevealChapter}
                  searchIntent={editorSearchIntent}
                  onGenerateIllustration={onGenerateIllustration}
                  generateIllustrationDisabled={isStreaming || !currentChapter}
                  illustrationInsertSignal={illustrationInsertSignal}
                  onFlushHandlerChange={onEditorFlushHandlerChange}
                  documentReview={documentReview}
                  documentReviewNavigationIntent={documentReviewNavigationTarget?.target.kind === 'workspace_file' && documentReviewNavigationTarget.target.id === selectedFile ? documentReviewNavigationTarget : null}
                  readingTypography={readingTypography}
                />
              )
            ) : loreEmpty ? (
              <EmptyLoreGuide
                emptyText={emptyText}
                title={emptyLoreTitle}
                description={emptyLoreDescription}
                action={emptyLoreAction}
                onClick={onRequestWritingInit}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">
                {emptyText}
              </div>
            )}
          </div>
        </>
      </div>
      {activeReviewThreadID ? (
        <div data-change-review-layer="true" className="nova-change-review-layer absolute inset-0 z-10 min-h-0">
          <StableChangeReviewWorkspace
            projectId={projectId}
            threadID={activeReviewThreadID}
            scopeRequest={activeReviewRequest}
            disabled={isStreaming}
            selectedPath={selectedFile}
            agentVisible={agentVisible}
            onToggleAgent={onToggleAgent}
            onClose={onCloseReview}
            onOpenFile={onOpenReviewFile}
            onWorkspaceChanged={onWorkspaceChanged}
            onFeedbackCommentsChange={onFeedbackCommentsChange}
            hiddenCommentIDs={submittedReviewCommentIDs}
          />
        </div>
      ) : null}
    </WorkbenchRouteLayer>
  )
}

export function IdeWritingInfoActions({
  projectVisible,
  aiVisible,
  onToggleProjectVisible,
  onToggleAgent,
}: {
  projectVisible: boolean
  aiVisible: boolean
  onToggleProjectVisible: () => void
  onToggleAgent: () => void
}) {
  const { t } = useTranslation()
  const ProjectIcon = projectVisible ? PanelLeftClose : PanelLeftOpen
  const AgentIcon = aiVisible ? PanelRightClose : PanelRightOpen
  const projectLabel = projectVisible ? t('router.hideOutline') : t('router.showOutline')
  const agentLabel = aiVisible ? t('router.hideAgent') : t('router.showAgent')

  return (
    <>
      <button
        type="button"
        onClick={onToggleProjectVisible}
        aria-label={projectLabel}
        aria-pressed={projectVisible}
        className={`nova-nav-item flex h-7 w-7 items-center justify-center ${projectVisible ? 'is-active' : ''}`}
      >
        <ProjectIcon className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        onClick={onToggleAgent}
        aria-label={agentLabel}
        aria-pressed={aiVisible}
        className={`nova-nav-item flex h-7 w-7 items-center justify-center ${aiVisible ? 'is-active' : ''}`}
      >
        <AgentIcon className="h-3.5 w-3.5" />
      </button>
    </>
  )
}

function EmptyLoreGuide({
  emptyText,
  title,
  description,
  action,
  onClick,
}: {
  emptyText: string
  title: string
  description: string
  action: string
  onClick: () => void
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-md flex-col items-center gap-3 rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
        <Sparkles className="h-4 w-4 text-[var(--nova-text-muted)]" />
        <div className="space-y-1">
          <div className="text-xs text-[var(--nova-text-faint)]">{emptyText}</div>
          <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
          <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{description}</div>
        </div>
        <button
          type="button"
          className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          onClick={onClick}
        >
          {action}
        </button>
      </div>
    </div>
  )
}
