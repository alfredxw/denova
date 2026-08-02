import type { TreeApi } from 'react-arborist'
import { Eye, EyeOff, FilePlus, FolderPlus, ListCollapse, Loader2, LocateFixed, RefreshCw } from 'lucide-react'
import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileOperationDialog, type FileOperationMode } from '@/components/Sidebar/FileOperationDialog'
import { Button } from '@/components/ui/button'
import { ProjectFileTree } from './ProjectFileTree'
import type { ProjectFileExplorerNode } from './project-file-explorer-model'

interface ProjectFilesSidebarProps {
  nodes: readonly ProjectFileExplorerNode[]
  selectedPath: string | null
  expandedPaths: readonly string[]
  loading: boolean
  loadingPaths: ReadonlySet<string>
  error: string | null
  showIgnored: boolean
  onShowIgnoredChange: (show: boolean) => void
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
}

/** Right-hand, virtualized project explorer for both Writing and Game projects. */
export const ProjectFilesSidebar = memo(function ProjectFilesSidebar({
  nodes,
  selectedPath,
  expandedPaths,
  loading,
  loadingPaths,
  error,
  showIgnored,
  onShowIgnoredChange,
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
}: ProjectFilesSidebarProps) {
  const { t } = useTranslation()
  const treeRef = useRef<TreeApi<ProjectFileExplorerNode>>(null)
  const revealedPathRef = useRef<string | null>(null)
  const [createMode, setCreateMode] = useState<FileOperationMode | null>(null)
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
        <span className="min-w-0 flex-1 truncate px-1 text-xs font-medium">{t('files.tree.title')}</span>
        <Button type="button" variant="ghost" size="icon-xs" className={actionButtonClass} onClick={() => setCreateMode('create-file')} aria-label={t('sidebar.createFile')} title={t('sidebar.createFile')}>
          <FilePlus />
        </Button>
        <Button type="button" variant="ghost" size="icon-xs" className={actionButtonClass} onClick={() => setCreateMode('create-dir')} aria-label={t('sidebar.createDir')} title={t('sidebar.createDir')}>
          <FolderPlus />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className={actionButtonClass}
          disabled={!selectedPath}
          onClick={() => void revealCurrentFile()}
          aria-label={t('files.tree.revealCurrent')}
          title={t('files.tree.revealCurrent')}
        >
          <LocateFixed />
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
          title={`${t('files.tree.collapseAll')} · ${t('files.tree.recursiveFoldHint')}`}
        >
          <ListCollapse />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className={actionButtonClass}
          onClick={() => onShowIgnoredChange(!showIgnored)}
          aria-pressed={showIgnored}
          aria-label={t(showIgnored ? 'files.tree.hideIgnored' : 'files.tree.showIgnored')}
          title={`${t(showIgnored ? 'files.tree.hideIgnored' : 'files.tree.showIgnored')} · ${t('files.tree.generatedHint')}`}
        >
          {showIgnored ? <EyeOff /> : <Eye />}
        </Button>
        <Button type="button" variant="ghost" size="icon-xs" className={actionButtonClass} onClick={() => void onRefresh()} aria-label={t('common.refresh')} title={t('common.refresh')}>
          <RefreshCw className={loadingPaths.size > 0 ? 'animate-spin' : undefined} />
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
        ) : nodes.length === 0 ? (
          <div className="px-3 py-8 text-center text-xs text-[var(--nova-text-faint)]">{t('files.tree.empty')}</div>
        ) : (
          <ProjectFileTree
            treeRef={treeRef}
            nodes={nodes}
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
          />
        )}
      </div>
      <FileOperationDialog
        open={createMode !== null}
        mode={createMode ?? 'create-file'}
        targetPath=""
        defaultValue=""
        onOpenChange={(open) => {
          if (!open) setCreateMode(null)
        }}
        onSubmit={(path) => onCreateItem(path, createMode === 'create-dir' ? 'dir' : 'file')}
      />
    </aside>
  )
})

function containsPath(nodes: readonly ProjectFileExplorerNode[], path: string): boolean {
  return nodes.some((node) => node.path === path || (node.children ? containsPath(node.children, path) : false))
}
