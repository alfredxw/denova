import type { TreeApi } from 'react-arborist'
import { FilePlus, FolderPlus, ListCollapse, Loader2, LocateFixed, RefreshCw } from 'lucide-react'
import { memo, useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ProjectExplorerTree, type ProjectExplorerTreeHandle } from './ProjectExplorerTree'
import type { ProjectFileExplorerNode } from './model'
import type { ProjectExplorerExtensions } from './types'

interface ProjectExplorerPaneProps {
  nodes: readonly ProjectFileExplorerNode[]
  workspace: string
  selectedPath: string | null
  expandedPaths: readonly string[]
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
  const treeRef = useRef<TreeApi<ProjectFileExplorerNode>>(null)
  const explorerRef = useRef<ProjectExplorerTreeHandle>(null)
  const revealedPathRef = useRef<string | null>(null)
  const actionButtonClass = 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

  const revealCurrentFile = useCallback(() => {
    if (!selectedPath) return false
    const tree = treeRef.current
    if (!tree || !containsPath(nodes, selectedPath)) return false
    // A collapsed descendant is intentionally absent from react-arborist's
    // visible-node index. scrollTo accepts the stable ID and opens its parents
    // before rebuilding that index, after which the node can receive focus.
    void Promise.resolve(tree.scrollTo(selectedPath, 'smart')).then(() => {
      const node = tree.get(selectedPath)
      if (node) tree.focus(node, { scroll: false })
    })
    return true
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
      <div className="flex h-10 shrink-0 items-center gap-1 border-b border-[var(--nova-border)] px-2">
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
            treeRef.current?.closeAll()
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
      <div className="min-h-0 flex-1 p-1">
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-8 text-xs text-[var(--nova-text-faint)]">
            <Loader2 className="size-3.5 animate-spin" />
            {t('files.tree.loading')}
          </div>
        ) : (
          <ProjectExplorerTree
            ref={explorerRef}
            treeRef={treeRef}
            nodes={nodes}
            workspace={workspace}
            selectedPath={selectedPath}
            expandedPaths={expandedPaths}
            onSelectFile={onSelectFile}
            onDirectoryExpand={onDirectoryExpand}
            onDirectoryExpandedChange={onDirectoryExpandedChange}
            onLoadMore={onLoadMore}
            onCreateItem={onCreateItem}
            onDeleteItem={onDeleteItem}
            onRenameItem={onRenameItem}
            onCopyItem={onCopyItem}
            onMoveItem={onMoveItem}
            extensions={extensions}
          />
        )}
      </div>
    </aside>
  )
})

function containsPath(nodes: readonly ProjectFileExplorerNode[], path: string): boolean {
  return nodes.some((node) => node.path === path || (node.children ? containsPath(node.children, path) : false))
}
