import { lazy, memo, Suspense, useCallback, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { ReadingTypographySettings } from '@/components/Editor/EditorSettingsPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { ProjectWritingSurface } from '@/features/writing/ProjectWritingSurface'
import { ChangeReviewWorkspace } from '@/features/changes/review/ChangeReviewWorkspace'
import { AgentChatView } from './AgentChatView'
import type { AgentChatPageId, AgentChatPageRenderContext, AgentChatReviewRenderContext, AgentChatReviewTab } from './types'
import { LoadingState } from '@/components/common/LoadingState'

const LoreWorkspaceTab = lazy(() => import('@/features/lore/LoreWorkspaceTab').then((module) => ({ default: module.LoreWorkspaceTab })))
const VersionPanel = lazy(() => import('@/components/Versions/VersionPanel').then((module) => ({ default: module.VersionPanel })))

interface AgentChatRouteProps {
  /** Stable identity of the foreground Writing Book, used only for outer projection refresh. */
  projectId: string
  novaDir: string
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  readingTypography: ReadingTypographySettings
  onBeforeCreateBook: () => Promise<boolean>
  onBookCreated: (workspace: string) => void | Promise<void>
  onBooksChange: () => void | Promise<void>
  /** Registers every mounted project-page draft with the workbench navigation guard. */
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged?: (paths: string[], metadata: WorkspaceChangeMetadata) => void | Promise<void>
}

/**
 * Composes AgentChat project pages from the same focused workspaces used by Writing.
 * AgentChatView remains responsible only for tabs, sessions, feedback routing and layout.
 */
function AgentChatRouteComponent({
  projectId: foregroundProjectId,
  novaDir,
  composerSettings,
  tellers,
  imagePresets,
  autoSaveEnabled = true,
  autoSaveDelayMs = 1200,
  readingTypography,
  onBeforeCreateBook,
  onBookCreated,
  onBooksChange,
  onFlushHandlerChange,
  onWorkspaceChanged,
}: AgentChatRouteProps) {
  const { t } = useTranslation()

  const pageContent = useCallback((
    projectId: string,
    tabWorkspace: string,
    pageId: AgentChatPageId,
    context: AgentChatPageRenderContext,
  ): ReactNode => {
    switch (pageId) {
      case 'reader':
        return (
          <ProjectWritingSurface
            key={projectId}
            projectId={projectId}
            workspace={tabWorkspace}
            autoSaveEnabled={autoSaveEnabled}
            autoSaveDelayMs={autoSaveDelayMs}
            readingTypography={readingTypography}
            documentReview={context.documentReview}
            navigationIntent={context.navigationIntent?.target.kind === 'workspace_file' ? context.navigationIntent : null}
            refreshSignal={context.refreshSignal}
            onOpenLoreTab={() => {
              context.openPage('lore')
            }}
            onFlushHandlerChange={context.onFlushHandlerChange}
            onWorkspaceChanged={context.onWorkspaceChanged}
          />
        )
      case 'lore':
        return (
          <LoreWorkspaceTab
            projectId={projectId}
            documentReview={context.documentReview}
            navigationIntent={context.navigationIntent?.target.kind === 'lore_item' ? context.navigationIntent : null}
            toolNavigationIntent={context.toolNavigationIntent}
            refreshSignal={context.refreshSignal}
            onEditorFlushHandlerChange={context.onFlushHandlerChange}
          />
        )
      case 'versions':
        return (
          <VersionPanel
            projectId={projectId}
            workspace={tabWorkspace}
            refreshSignal={context.refreshSignal}
            onWorkspaceChanged={(paths) => context.onWorkspaceChanged(paths, { impact: 'structure', origin: 'project-page' })}
          />
        )
    }
  }, [autoSaveDelayMs, autoSaveEnabled, readingTypography])

  /** Keep each lazy page inside its own boundary so opening it never replaces live conversations. */
  const renderPage = useCallback((
    projectId: string,
    tabWorkspace: string,
    pageId: AgentChatPageId,
    context: AgentChatPageRenderContext,
  ): ReactNode => (
    <Suspense fallback={<LoadingState label={t('router.loading')} className="h-full min-h-0" />}>
      {pageContent(projectId, tabWorkspace, pageId, context)}
    </Suspense>
  ), [pageContent, t])

  const renderReview = useCallback((tab: AgentChatReviewTab, disabled: boolean, context: AgentChatReviewRenderContext): ReactNode => (
    <Suspense fallback={<LoadingState label={t('router.loading')} className="h-full min-h-0" />}>
      <ChangeReviewWorkspace
        projectId={tab.projectId}
        threadID={tab.threadID}
        scopeRequest={tab.groupID ? { id: 0, threadID: tab.threadID, groupID: tab.groupID } : null}
        disabled={disabled}
        selectedPath={null}
        onOpenFile={(path) => context.openFile(path)}
        onWorkspaceChanged={context.onWorkspaceChanged}
      />
    </Suspense>
  ), [t])

  return (
    <AgentChatView
      composerSettings={composerSettings}
      tellers={tellers}
      imagePresets={imagePresets}
      novaDir={novaDir}
      autoSaveEnabled={autoSaveEnabled}
      autoSaveDelayMs={autoSaveDelayMs}
      renderPage={renderPage}
      renderReview={renderReview}
      onFlushHandlerChange={onFlushHandlerChange}
      onBeforeCreateBook={onBeforeCreateBook}
      onBookCreated={onBookCreated}
      onBooksChange={onBooksChange}
      onWorkspaceChanged={(changedProjectId, _changedWorkspace, paths, metadata) => (
        changedProjectId === foregroundProjectId ? onWorkspaceChanged?.(paths, metadata) : undefined
      )}
    />
  )
}

export const AgentChatRoute = memo(AgentChatRouteComponent)
