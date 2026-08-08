import { lazy, memo, Suspense, useCallback, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { ProjectWritingSurface } from '@/features/writing/ProjectWritingSurface'
import { projectResourceTarget } from '@/lib/api'
import { AgentChatView } from './AgentChatView'
import type { AgentChatPageId, AgentChatPageRenderContext, AgentChatReviewRenderContext, AgentChatReviewTab } from './types'

const LoreWorkspaceTab = lazy(() => import('@/features/lore/LoreWorkspaceTab').then((module) => ({ default: module.LoreWorkspaceTab })))
const SettingPanel = lazy(() => import('@/features/interactive/components/SettingPanel').then((module) => ({ default: module.SettingPanel })))
const SkillsView = lazy(() => import('@/features/skills/SkillsView').then((module) => ({ default: module.SkillsView })))
const AgentsView = lazy(() => import('@/features/agents/AgentsView').then((module) => ({ default: module.AgentsView })))
const AutomationsView = lazy(() => import('@/features/automations/AutomationsView').then((module) => ({ default: module.AutomationsView })))
const VersionPanel = lazy(() => import('@/components/Versions/VersionPanel').then((module) => ({ default: module.VersionPanel })))
const ChangeReviewWorkspace = lazy(() => import('@/features/changes/review/ChangeReviewWorkspace').then((module) => ({ default: module.ChangeReviewWorkspace })))

interface AgentChatRouteProps {
  /** Stable identity of the foreground Writing Book, used only for outer projection refresh. */
  projectId: string
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  onTellersChange: (tellers: Teller[]) => void
  onImagePresetsChange: (presets: ImagePreset[]) => void
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
  composerSettings,
  tellers,
  imagePresets,
  autoSaveEnabled = true,
  autoSaveDelayMs = 1200,
  onTellersChange,
  onImagePresetsChange,
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
            autoSaveEnabled={autoSaveEnabled}
            autoSaveDelayMs={autoSaveDelayMs}
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
            refreshSignal={context.refreshSignal}
            onEditorFlushHandlerChange={context.onFlushHandlerChange}
          />
        )
      case 'presets':
        return (
          <SettingPanel
            projectId={projectId}
            mode="teller"
            presetUsageMode="writing"
            tellers={tellers}
            imagePresets={imagePresets}
            onTellersChange={onTellersChange}
            onImagePresetsChange={onImagePresetsChange}
          />
        )
      case 'skills':
        return <SkillsView target={projectResourceTarget(projectId)} />
      case 'agents':
        return <AgentsView target={projectResourceTarget(projectId)} />
      case 'automations':
        return <AutomationsView projectId={projectId} workspace={tabWorkspace} />
      case 'versions':
        return (
          <VersionPanel
            projectId={projectId}
            workspace={tabWorkspace}
            onWorkspaceChanged={(paths) => context.onWorkspaceChanged(paths, { impact: 'structure', origin: 'project-page' })}
          />
        )
    }
  }, [autoSaveDelayMs, autoSaveEnabled, imagePresets, onImagePresetsChange, onTellersChange, tellers])

  /** Keep each lazy page inside its own boundary so opening it never replaces live conversations. */
  const renderPage = useCallback((
    projectId: string,
    tabWorkspace: string,
    pageId: AgentChatPageId,
    context: AgentChatPageRenderContext,
  ): ReactNode => (
    <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">{t('router.loading')}</div>}>
      {pageContent(projectId, tabWorkspace, pageId, context)}
    </Suspense>
  ), [pageContent, t])

  const renderReview = useCallback((tab: AgentChatReviewTab, disabled: boolean, context: AgentChatReviewRenderContext): ReactNode => (
    <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">{t('router.loading')}</div>}>
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
      autoSaveEnabled={autoSaveEnabled}
      autoSaveDelayMs={autoSaveDelayMs}
      renderPage={renderPage}
      renderReview={renderReview}
      onFlushHandlerChange={onFlushHandlerChange}
      onWorkspaceChanged={(changedProjectId, _changedWorkspace, paths, metadata) => (
        changedProjectId === foregroundProjectId ? onWorkspaceChanged?.(paths, metadata) : undefined
      )}
    />
  )
}

export const AgentChatRoute = memo(AgentChatRouteComponent)
