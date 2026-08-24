import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { LoadingState } from '@/components/common/LoadingState'
import { SearchPanel } from '@/components/Sidebar/SearchPanel'
import { WritingProjectExplorer } from '@/features/project-explorer/WritingProjectExplorer'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ChapterSummary, DocumentPreview, WorkspaceSearchResult } from '@/lib/api'
import { ChapterOutline, type OutlineRevealRequest } from './outline/ChapterOutline'

const StableSearchPanel = memo(SearchPanel)
const StableChapterOutline = memo(ChapterOutline)
const StableWritingProjectExplorer = memo(WritingProjectExplorer)

interface WritingSidebarProps {
  sidebarView: 'outline' | 'files' | 'search'
  loading: boolean
  projectId: string
  workspace: string
  tree: FileNode[]
  chapters: ChapterSummary[]
  summaryAvailable: boolean
  ideas?: DocumentPreview
  outline?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  loreTabActive: boolean
  revealRequest: OutlineRevealRequest | null
  projectExplorerRefreshSignal: number
  onSetSidebarView: (view: 'outline' | 'files' | 'search') => void
  onSelectOutlineFile: (path: string) => void
  onOpenLoreTab: () => Promise<boolean>
  onReferenceFile: (path: string) => void
  onRevealFile: (path: string) => void | Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRequestBookSettingCreate: (item: { path: string; title: string }) => void
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onSelectSearchResult: (result: WorkspaceSearchResult, query: string) => void
  onBeforeReplace: () => Promise<boolean>
  onExternalContentChange: (paths: string[]) => void | Promise<void>
  onSelectFile: (path: string) => boolean | void | Promise<boolean | void>
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRefreshWorkspace: () => void | Promise<void>
}

export const WritingSidebar = memo(function WritingSidebar({
  sidebarView,
  loading,
  projectId,
  workspace,
  tree,
  chapters,
  summaryAvailable,
  ideas,
  outline,
  chapterPlans,
  selectedFile,
  loreTabActive,
  revealRequest,
  projectExplorerRefreshSignal,
  onSetSidebarView,
  onSelectOutlineFile,
  onOpenLoreTab,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
  onRequestBookSettingCreate,
  onSetChapterConfirmed,
  onSelectSearchResult,
  onBeforeReplace,
  onExternalContentChange,
  onSelectFile,
  onCreateItem,
  onCopyItem,
  onMoveItem,
  onRefreshWorkspace,
}: WritingSidebarProps) {
  const { t } = useTranslation()
  const showLoading = Boolean(workspace) && (
    sidebarView === 'files'
      ? !projectId
      : sidebarView === 'outline' && loading && tree.length === 0 && !summaryAvailable
  )

  return (
    <section className="nova-sidebar flex h-full flex-col border-r">
      <div data-slot="writing-sidebar-view-switcher" className="px-3 pt-2">
        <div className="grid grid-cols-3 gap-1">
          {(['outline', 'files', 'search'] as const).map((view) => (
            <button
              key={view}
              type="button"
              onClick={() => onSetSidebarView(view)}
              className={`nova-nav-item h-7 min-w-0 truncate whitespace-nowrap px-1 text-[11px] ${sidebarView === view ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`}
            >
              {t(`router.${view}`)}
            </button>
          ))}
        </div>
      </div>
      <div className="min-h-0 flex-1 text-xs">
        {showLoading ? (
          <LoadingState label={t('router.loading')} variant="panel" className="h-full min-h-0" />
        ) : sidebarView === 'outline' ? (
          <StableChapterOutline
            projectId={projectId}
            workspace={workspace}
            tree={tree}
            chapters={chapters}
            ideas={ideas}
            outline={outline}
            chapterPlans={chapterPlans}
            selectedFile={selectedFile}
            loreTabActive={loreTabActive}
            revealRequest={revealRequest}
            onSelectFile={onSelectOutlineFile}
            onOpenLoreTab={onOpenLoreTab}
            onReferenceFile={onReferenceFile}
            onRevealFile={onRevealFile}
            onRenameItem={onRenameItem}
            onDeleteItem={onDeleteItem}
            onCopyItem={onCopyItem}
            onCreateItem={onCreateItem}
            onRequestBookSettingCreate={onRequestBookSettingCreate}
            onSetChapterConfirmed={onSetChapterConfirmed}
          />
        ) : (
          <div className="h-full min-h-0">
            {sidebarView === 'search' ? (
              <div className="h-full overflow-y-auto p-2">
                <StableSearchPanel
                  projectId={projectId}
                  onSelectResult={onSelectSearchResult}
                  onBeforeReplace={onBeforeReplace}
                  onWorkspaceChanged={onExternalContentChange}
                />
              </div>
            ) : !projectId ? (
              <LoadingState label={t('router.loading')} variant="panel" className="h-full min-h-0" />
            ) : (
              <StableWritingProjectExplorer
                key={projectId}
                projectId={projectId}
                workspace={workspace}
                selectedPath={selectedFile}
                structureRefreshSignal={projectExplorerRefreshSignal}
                onSelectFile={onSelectFile}
                onReferenceFile={onReferenceFile}
                onCreateItem={onCreateItem}
                onDeleteItem={onDeleteItem}
                onRenameItem={onRenameItem}
                onCopyItem={onCopyItem}
                onMoveItem={onMoveItem}
                onRefreshWorkspace={onRefreshWorkspace}
              />
            )}
          </div>
        )}
      </div>
    </section>
  )
})
