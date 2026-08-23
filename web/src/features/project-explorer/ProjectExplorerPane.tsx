import type { GitStatusEntry } from '@pierre/trees'
import { FilePlus, FolderPlus, ListCollapse, Loader2, LocateFixed, MoreHorizontal, RefreshCw } from 'lucide-react'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/common/LoadingState'
import { ProjectExplorerTree, type ProjectExplorerTreeHandle } from './ProjectExplorerTree'
import type { ProjectFileExplorerNode } from './model'
import { projectFileTreeProjection } from './model'
import type { ProjectExplorerExtensions } from './types'

interface ProjectExplorerPaneProps {
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
  const revealedPathRef = useRef<string | null>(null)
  const [treeScrolled, setTreeScrolled] = useState(false)
  const incompletePaths = useMemo(() => projectFileTreeProjection(nodes).incompletePaths, [nodes])
  const actionButtonClass = 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

  const revealCurrentFile = useCallback(() => {
    if (!selectedPath) return false
    return explorerRef.current?.revealPath(selectedPath) ?? false
  }, [nodes, selectedPath])

  useEffect(() => {
    if (!selectedPath) {
      revealedPathRef.current = null
      return
    }
    if (revealedPathRef.current !== selectedPath && revealCurrentFile()) {
      revealedPathRef.current = selectedPath
    }
  }, [nodes, revealCurrentFile, selectedPath])

  return (
    <aside className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-surface)] text-[var(--nova-text)]">
      <div
        data-slot="project-explorer-toolbar"
        className={`flex h-9 shrink-0 items-center gap-1 border-b px-2 ${
          treeScrolled ? 'border-[var(--nova-border)]' : 'border-transparent'
        }`}
      >
        <Button type="button" variant="ghost" size="icon-xs" className={`${actionButtonClass} ml-auto`} onClick={() => explorerRef.current?.beginCreate('file')} aria-label={t('sidebar.createFile')}>
          <FilePlus />
        </Button>
        <Button type="button" variant="ghost" size="icon-xs" className={actionButtonClass} onClick={() => explorerRef.current?.beginCreate('dir')} aria-label={t('sidebar.createDir')}>
          <FolderPlus />
        </Button>
        <Button type="button" variant="ghost" size="icon-xs" className={actionButtonClass} onClick={() => void onRefresh()} aria-label={t('common.refresh')}>
          <RefreshCw className={loadingPaths.size > 0 ? 'animate-spin' : undefined} />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className={actionButtonClass}
          onClick={() => {
            explorerRef.current?.collapseAll()
            onCollapseAll()
          }}
          aria-label={t('files.tree.collapseAll')}
        >
          <ListCollapse />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className={actionButtonClass}
          disabled={!selectedPath}
          onClick={() => void revealCurrentFile()}
          aria-label={t('files.tree.revealCurrent')}
        >
          <LocateFixed />
        </Button>
      </div>
      {error ? (
        <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">
          {error}
        </div>
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col p-1">
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
            onScrollOffsetChange={(scrollOffset) => setTreeScrolled(scrollOffset > 0)}
            onCreateItem={onCreateItem}
            onDeleteItem={onDeleteItem}
            onRenameItem={onRenameItem}
            onCopyItem={onCopyItem}
            onMoveItem={onMoveItem}
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
