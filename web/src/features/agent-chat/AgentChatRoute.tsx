import { lazy, memo, Suspense, useCallback, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { ChapterSummary } from '@/lib/api'
import { AgentChatReader, type AgentChatSaveFile } from './AgentChatReader'
import { AgentChatView } from './AgentChatView'
import type { AgentChatPageId, AgentChatReviewTab } from './types'

const SettingPanel = lazy(() => import('@/features/interactive/components/SettingPanel').then((module) => ({ default: module.SettingPanel })))
const SkillsView = lazy(() => import('@/features/skills/SkillsView').then((module) => ({ default: module.SkillsView })))
const AgentsView = lazy(() => import('@/features/agents/AgentsView').then((module) => ({ default: module.AgentsView })))
const AutomationsView = lazy(() => import('@/features/automations/AutomationsView').then((module) => ({ default: module.AutomationsView })))
const ChangeReviewWorkspace = lazy(() => import('@/features/changes/review/ChangeReviewWorkspace').then((module) => ({ default: module.ChangeReviewWorkspace })))

interface AgentChatRouteProps {
  /** Foreground Writing book. It is only used when an embedded page targets that same book. */
  workspace: string
  composerSettings: WritingComposerSettingsController
  chapters: ChapterSummary[]
  selectedFile: string | null
  tellers: Teller[]
  imagePresets: ImagePreset[]
  onTellersChange: (tellers: Teller[]) => void
  onImagePresetsChange: (presets: ImagePreset[]) => void
  /** The writing workbench's save handler, so a chapter tab edits through the same path. */
  onSaveFile?: AgentChatSaveFile
  /** Opens a file from a diff. The navigation result is ignored; the review tab stays open. */
  onOpenFile?: (path: string) => boolean | void | Promise<boolean | void>
}

/**
 * Hosts the project pages that AgentChat can open in a tab and hands the tabbed
 * workbench everything it needs. Page composition lives here so the workbench itself
 * stays about tabs, sessions and layout.
 */
function AgentChatRouteComponent({
  workspace,
  composerSettings,
  chapters,
  selectedFile,
  tellers,
  imagePresets,
  onTellersChange,
  onImagePresetsChange,
  onSaveFile,
  onOpenFile,
}: AgentChatRouteProps) {
  const { t } = useTranslation()

  const pageContent = useCallback((tabWorkspace: string, pageId: AgentChatPageId): ReactNode => {
    const foregroundPage = tabWorkspace === workspace
    switch (pageId) {
      case 'reader':
        return (
          <AgentChatReader
            workspace={tabWorkspace}
            chapters={foregroundPage ? chapters : []}
            initialPath={foregroundPage ? selectedFile : null}
            onSaveFile={foregroundPage ? onSaveFile : undefined}
          />
        )
      case 'lore':
        return <SettingPanel mode="lore" workspace={tabWorkspace} />
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
  }, [chapters, imagePresets, onImagePresetsChange, onSaveFile, onTellersChange, selectedFile, tellers, workspace])

  /**
   * Each page suspends inside its own boundary. One boundary around the workbench would swap
   * the conversation tree, the tab strips and every open terminal for the fallback — and reset
   * their scroll — for as long as a newly opened page's chunk takes to load.
   */
  const renderPage = useCallback((tabWorkspace: string, pageId: AgentChatPageId): ReactNode => (
    <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">{t('router.loading')}</div>}>
      {pageContent(tabWorkspace, pageId)}
    </Suspense>
  ), [pageContent, t])

  /**
   * The same review surface the writing workbench uses, hosted by a tab. Its close button closes
   * the tab, so a review is dismissed the same way whichever side of the split it sits on.
   */
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
    />
  )
}

export const AgentChatRoute = memo(AgentChatRouteComponent)
