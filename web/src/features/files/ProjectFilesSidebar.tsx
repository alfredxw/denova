import { Eye, EyeOff, FilePlus, FolderPlus, Loader2, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileTree } from '@/components/Sidebar/FileTree'
import { FileOperationDialog, type FileOperationMode } from '@/components/Sidebar/FileOperationDialog'
import { Button } from '@/components/ui/button'
import type { FileNode } from '@/hooks/useWorkspace'

interface ProjectFilesSidebarProps {
  nodes: FileNode[]
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
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRefresh: () => void | Promise<void>
}

/** Right-hand project tree. FileTree remains the single reusable interaction implementation. */
export function ProjectFilesSidebar({
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
  onCreateItem,
  onDeleteItem,
  onRenameItem,
  onCopyItem,
  onMoveItem,
  onRefresh,
}: ProjectFilesSidebarProps) {
  const { t } = useTranslation()
  const [createMode, setCreateMode] = useState<FileOperationMode | null>(null)
  const actionButtonClass = 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

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
          onClick={() => onShowIgnoredChange(!showIgnored)}
          aria-pressed={showIgnored}
          aria-label={t(showIgnored ? 'files.tree.hideIgnored' : 'files.tree.showIgnored')}
          title={`${t(showIgnored ? 'files.tree.hideIgnored' : 'files.tree.showIgnored')} · ${t('files.tree.generatedHint')}`}
        >
          {showIgnored ? <EyeOff /> : <Eye />}
        </Button>
        <Button type="button" variant="ghost" size="icon-xs" className={actionButtonClass} onClick={() => void onRefresh()} aria-label={t('common.refresh')} title={t('common.refresh')}>
          <RefreshCw className={loading ? 'animate-spin' : undefined} />
        </Button>
      </div>
      {error ? (
        <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">
          {error}
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-8 text-xs text-[var(--nova-text-faint)]">
            <Loader2 className="size-3.5 animate-spin" />
            {t('files.tree.loading')}
          </div>
        ) : nodes.length === 0 ? (
          <div className="px-3 py-8 text-center text-xs text-[var(--nova-text-faint)]">{t('files.tree.empty')}</div>
        ) : (
          <FileTree
            nodes={nodes}
            selectedFile={selectedPath}
            onSelectFile={onSelectFile}
            defaultExpandedPaths={expandedPaths}
            onDirectoryExpand={onDirectoryExpand}
            onDirectoryExpandedChange={onDirectoryExpandedChange}
            isDirectoryLoading={(path) => loadingPaths.has(path)}
            deleteRecovery="none"
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
}
