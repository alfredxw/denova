import type { GitStatusEntry } from '@pierre/trees'
import { Eye, EyeOff, FilePlus, FolderPlus, ListCollapse, Loader2, LocateFixed, MoreHorizontal, RefreshCw } from 'lucide-react'
import { memo, useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LoadingState } from '@/components/common/LoadingState'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { TooltipProvider } from '@/components/ui/tooltip'
import { revealProjectFile } from '@/lib/api-client/project-files'
import { ProjectExplorerTree, type ProjectExplorerTreeHandle } from './ProjectExplorerTree'
import type { ProjectFileExplorerNode } from './model'
import { projectFileTreeProjection } from './model'
import type { ProjectExplorerExtensions } from './types'

interface ProjectExplorerPaneProps {
  projectId: string
  nodes: readonly ProjectFileExplorerNode[]
  workspace: string
  selectedPath: string | null
  expandedPaths: readonly string[]
  gitStatus?: readonly GitStatusEntry[]
  loading: boolean
  loadingPaths: ReadonlySet<string>
  error: string | null
  onSelectFile: (path: string) => void
  onDirectoryExpand: (path: string) => void | Promise<void>
  onDirectoryExpandedChange: (path: string, expanded: boolean) => void
  onCollapseAll: () => void
  onLoadMore: (path: string) => void | Promise<void>
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRefresh: () => void | Promise<void>
  extensions?: ProjectExplorerExtensions
}

/** Virtualized project Explorer surface shared by Writing and Game layouts. */
export const ProjectExplorerPane = memo(function ProjectExplorerPane({
  projectId,
  nodes,
  workspace,
  selectedPath,
  expandedPaths,
  gitStatus = [],
  loading,
  loadingPaths,
  error,
  onSelectFile,
  onDirectoryExpand,
  onDirectoryExpandedChange,
  onCollapseAll,
  onLoadMore,
  onCreateItem,
  onDeleteItem,
  onRenameItem,
  onCopyItem,
  onMoveItem,
  onRefresh,
  extensions,
}: ProjectExplorerPaneProps) {
  const { t } = useTranslation()
  const explorerRef = useRef<ProjectExplorerTreeHandle>(null)
  const [hideWritingFilenameAffixes, setHideWritingFilenameAffixes] = useState(true)
  const incompletePaths = useMemo(() => projectFileTreeProjection(nodes).incompletePaths, [nodes])
  const actionButtonClass = 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:pointer-events-auto'
  const revealItem = useCallback((path: string) => revealProjectFile(projectId, path), [projectId])

  const revealCurrentFile = useCallback(() => {
    if (!selectedPath) return false
    return explorerRef.current?.revealPath(selectedPath) ?? false
  }, [selectedPath])

  return (
    <aside className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]">
      <TooltipProvider>
        <div
          data-slot="project-explorer-toolbar"
          className="flex h-9 shrink-0 items-center gap-1 px-2"
        >
          <TooltipIconButton label={t('sidebar.createFile')} tooltipSide="bottom" useTooltipProvider={false} className={`${actionButtonClass} ml-auto`} onClick={() => explorerRef.current?.beginCreate('file')}>
            <FilePlus />
          </TooltipIconButton>
          <TooltipIconButton label={t('sidebar.createDir')} tooltipSide="bottom" useTooltipProvider={false} className={actionButtonClass} onClick={() => explorerRef.current?.beginCreate('dir')}>
            <FolderPlus />
          </TooltipIconButton>
          <TooltipIconButton label={t('common.refresh')} tooltipSide="bottom" useTooltipProvider={false} className={actionButtonClass} onClick={() => void onRefresh()}>
            <RefreshCw className={loadingPaths.size > 0 ? 'animate-spin' : undefined} />
          </TooltipIconButton>
          <TooltipIconButton
            label={t(hideWritingFilenameAffixes ? 'files.tree.showWritingFilenameAffixes' : 'files.tree.hideWritingFilenameAffixes')}
            tooltipSide="bottom"
            useTooltipProvider={false}
            className={actionButtonClass}
            onClick={() => setHideWritingFilenameAffixes((current) => !current)}
          >
            {hideWritingFilenameAffixes ? <Eye /> : <EyeOff />}
          </TooltipIconButton>
          <TooltipIconButton
            label={t('files.tree.collapseAll')}
            tooltipSide="bottom"
            useTooltipProvider={false}
            className={actionButtonClass}
            onClick={() => {
              explorerRef.current?.collapseAll()
              onCollapseAll()
            }}
          >
            <ListCollapse />
          </TooltipIconButton>
          <TooltipIconButton
            label={t('files.tree.revealCurrent')}
            tooltipSide="bottom"
            useTooltipProvider={false}
            className={actionButtonClass}
            disabled={!selectedPath}
            onClick={() => void revealCurrentFile()}
          >
            <LocateFixed />
          </TooltipIconButton>
        </div>
      </TooltipProvider>
      {error ? (
        <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">
          {error}
        </div>
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col px-1 pb-1">
        {loading ? (
          <LoadingState label={t('files.tree.loading')} variant="panel" />
        ) : (
          <ProjectExplorerTree
            ref={explorerRef}
            nodes={nodes}
            workspace={workspace}
            selectedPath={selectedPath}
            expandedPaths={expandedPaths}
            gitStatus={gitStatus}
            onSelectFile={onSelectFile}
            onDirectoryExpand={onDirectoryExpand}
            onDirectoryExpandedChange={onDirectoryExpandedChange}
            onCreateItem={onCreateItem}
            onDeleteItem={onDeleteItem}
            onRenameItem={onRenameItem}
            onCopyItem={onCopyItem}
            onMoveItem={onMoveItem}
            onRevealItem={revealItem}
            hideWritingFilenameAffixes={hideWritingFilenameAffixes}
            extensions={extensions}
          />
        )}
        {!loading && incompletePaths.length > 0 ? (
          <button
            type="button"
            disabled={loadingPaths.has(incompletePaths[0])}
            className="mt-1 flex h-7 shrink-0 items-center justify-center gap-1 rounded text-[11px] text-[var(--nova-accent)] hover:bg-[var(--nova-hover)] disabled:opacity-50"
            onClick={() => {
              void Promise.resolve(onLoadMore(incompletePaths[0])).catch((cause) => {
                console.error('[features/project-explorer/ProjectExplorerPane.tsx] loading a project tree page failed', { path: incompletePaths[0], cause })
              })
            }}
          >
            {loadingPaths.has(incompletePaths[0]) ? <Loader2 className="size-3.5 animate-spin" /> : <MoreHorizontal className="size-3.5" />}
            {t(loadingPaths.has(incompletePaths[0]) ? 'files.tree.loadingMore' : 'files.tree.loadMore')}
          </button>
        ) : null}
      </div>
    </aside>
  )
})
