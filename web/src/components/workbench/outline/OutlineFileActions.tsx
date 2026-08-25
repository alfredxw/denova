import { useCallback, useState, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react'
import { AtSign, Copy, CopyPlus, FilePlus2, FolderOpen, FolderSearch, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { DeleteConfirmDialog } from '@/components/Sidebar/DeleteConfirmDialog'
import { FileOperationDialog } from '@/components/Sidebar/FileOperationDialog'
import { FileTreeMenuShortcut } from '@/components/file-tree/FileTreeMenu'
import { writeClipboardText } from '@/components/file-tree/paths'
import { fileTreeMenuPlatformPresentation } from '@/components/file-tree/platform'
import { absoluteProjectPath, nextProjectFileDuplicatePath } from '@/features/project-explorer/operations'
import { revealProjectFile } from '@/lib/api-client/project-files'
import { workspaceFileName } from '@/lib/workspace-path'

export interface OutlineFileMenuOperations {
  projectId: string
  workspace?: string
  getExistingPaths: () => readonly string[]
  onCopyItem?: (from: string, to: string) => Promise<void>
}

interface OutlineFileActionsProps {
  path: string
  children: ReactNode
  triggerPlacement?: 'center' | 'top'
  showTrigger?: boolean
  fileOperations?: OutlineFileMenuOperations
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
  onCreateChapter?: (volumePath: string) => void
}

interface OutlineFileAction {
  label?: string
  icon?: ReactNode
  shortcut?: ReactNode
  danger?: boolean
  separator?: boolean
  onSelect?: () => void
}

const MENU_CONTENT_CLASS = 'nova-file-tree-menu-surface nova-file-tree-menu-radix'

/** File actions shared by the discoverable trigger and the native context-menu gesture. */
export function OutlineFileActions({
  path,
  children,
  triggerPlacement = 'center',
  showTrigger = true,
  fileOperations,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
  onCreateChapter,
}: OutlineFileActionsProps) {
  const { t } = useTranslation()
  const [renameOpen, setRenameOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const platformPresentation = fileTreeMenuPlatformPresentation()
  const copyPath = (relative: boolean) => {
    const value = relative || !fileOperations?.workspace
      ? path
      : absoluteProjectPath(fileOperations.workspace, path)
    void writeClipboardText(value).catch((cause) => {
      console.error('[components/workbench/outline/OutlineFileActions.tsx] copying outline file path failed', { path, relative, cause })
      toast.error(t('files.tree.copyPathFailed'))
    })
  }
  const duplicateFile = () => {
    if (!fileOperations?.onCopyItem) return
    const destination = nextProjectFileDuplicatePath(fileOperations.getExistingPaths(), path)
    void fileOperations.onCopyItem(path, destination).catch((cause) => {
      console.error('[components/workbench/outline/OutlineFileActions.tsx] duplicating outline file failed', { path, destination, cause })
      toast.error(t('files.operation.failed'), {
        description: cause instanceof Error ? cause.message : String(cause),
      })
    })
  }
  const revealInFileManager = () => {
    if (!fileOperations?.projectId) return
    void revealProjectFile(fileOperations.projectId, path).catch((cause) => {
      console.error('[components/workbench/outline/OutlineFileActions.tsx] revealing outline file in the host file manager failed', { path, cause })
      toast.error(t('files.tree.revealFailed'), {
        description: cause instanceof Error ? cause.message : String(cause),
      })
    })
  }
  const commonActions = compactActions([
    ...(fileOperations?.workspace ? [{ label: t('sidebar.copyPath'), icon: <Copy className="h-3.5 w-3.5" />, shortcut: platformPresentation.copyPath, onSelect: () => copyPath(false) }] : []),
    ...(fileOperations ? [
      { label: t('sidebar.copyRelativePath'), icon: <Copy className="h-3.5 w-3.5" />, shortcut: platformPresentation.copyRelativePath, onSelect: () => copyPath(true) },
      ...(fileOperations.onCopyItem ? [{ label: t('sidebar.duplicate'), icon: <CopyPlus className="h-3.5 w-3.5" />, onSelect: duplicateFile }] : []),
    ] : []),
    ...(onRevealFile ? [{ label: t('sidebar.revealInProjectFiles'), icon: <FolderSearch className="h-3.5 w-3.5" />, onSelect: () => { void onRevealFile(path) } }] : []),
    ...(fileOperations?.projectId ? [{ label: t(platformPresentation.revealKey), icon: <FolderOpen className="h-3.5 w-3.5" />, onSelect: revealInFileManager }] : []),
  ])
  const actions = compactActions([
    ...(onCreateChapter ? [{ label: t('planning.create.inVolume'), icon: <FilePlus2 className="h-3.5 w-3.5" />, onSelect: () => onCreateChapter(path) }] : []),
    ...(onCreateChapter ? [{ separator: true }] : []),
    ...(onReferenceFile ? [{ label: t('sidebar.referenceToChat'), icon: <AtSign className="h-3.5 w-3.5" />, onSelect: () => onReferenceFile(path) }] : []),
    ...(onReferenceFile && commonActions.length > 0 ? [{ separator: true }] : []),
    ...commonActions,
    ...(onRenameItem || onDeleteItem ? [{ separator: true }] : []),
    ...(onRenameItem ? [{ label: t('sidebar.renameFile'), icon: <Pencil className="h-3.5 w-3.5" />, onSelect: () => setRenameOpen(true) }] : []),
    ...(onDeleteItem ? [{ label: t('sidebar.delete'), icon: <Trash2 className="h-3.5 w-3.5" />, shortcut: platformPresentation.delete, danger: true, onSelect: () => setDeleteOpen(true) }] : []),
  ])

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const command = event.metaKey || event.ctrlKey
    if (command && event.altKey && event.key.toLowerCase() === 'c' && fileOperations && (event.shiftKey || fileOperations.workspace)) {
      event.preventDefault()
      event.stopPropagation()
      copyPath(event.shiftKey)
      return
    }
    if ((event.key === 'Delete' || (event.key === 'Backspace' && command)) && onDeleteItem) {
      event.preventDefault()
      event.stopPropagation()
      setDeleteOpen(true)
    }
  }

  const openContextMenuFromTrigger = useCallback((event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault()
    event.stopPropagation()
    const rect = event.currentTarget.getBoundingClientRect()
    event.currentTarget.dispatchEvent(new window.MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      clientX: rect.right,
      clientY: rect.bottom,
    }))
  }, [])

  if (actions.length === 0) return <>{children}</>

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div className="group relative min-w-0" onKeyDown={handleKeyDown}>
            {children}
            {showTrigger ? (
              <button
                type="button"
                aria-label={t('sidebar.moreActions')}
                aria-haspopup="menu"
                className={`pointer-events-none absolute right-1 z-10 flex h-6 w-6 items-center justify-center rounded text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:pointer-events-auto focus-visible:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100 max-md:pointer-events-auto max-md:opacity-100 ${triggerPlacement === 'top' ? 'top-1' : 'top-1/2 -translate-y-1/2'}`}
                onPointerDown={(event) => event.stopPropagation()}
                onClick={openContextMenuFromTrigger}
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </button>
            ) : null}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent className={MENU_CONTENT_CLASS}>
          {renderActions(actions)}
        </ContextMenuContent>
      </ContextMenu>
      {onRenameItem && renameOpen ? (
        <FileOperationDialog
          open={renameOpen}
          mode="rename"
          targetPath={path}
          defaultValue={workspaceFileName(path)}
          onOpenChange={setRenameOpen}
          onSubmit={(newName) => onRenameItem(path, newName)}
        />
      ) : null}
      {onDeleteItem && deleteOpen ? (
        <DeleteConfirmDialog
          open={deleteOpen}
          path={path}
          onOpenChange={setDeleteOpen}
          onConfirm={() => onDeleteItem(path)}
        />
      ) : null}
    </>
  )
}

function compactActions(actions: OutlineFileAction[]) {
  const compacted: OutlineFileAction[] = []
  for (const action of actions) {
    if (action.separator) {
      if (compacted.length > 0 && !compacted[compacted.length - 1].separator) compacted.push(action)
      continue
    }
    if (action.label) compacted.push(action)
  }
  if (compacted[compacted.length - 1]?.separator) compacted.pop()
  return compacted
}

function renderActions(actions: OutlineFileAction[]) {
  return actions.map((action, index) => {
    if (action.separator) {
      return <ContextMenuSeparator key={index} className="nova-file-tree-menu-separator" />
    }
    return (
      <ContextMenuItem
        key={action.label}
        className="nova-file-tree-menu-item"
        variant={action.danger ? 'destructive' : 'default'}
        onSelect={action.onSelect}
      >
        {action.icon}
        {action.label}
        {action.shortcut ? <FileTreeMenuShortcut>{action.shortcut}</FileTreeMenuShortcut> : null}
      </ContextMenuItem>
    )
  })
}
