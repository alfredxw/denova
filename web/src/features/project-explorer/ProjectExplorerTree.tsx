import {
  preparePresortedFileTreeInput,
  type FileTree,
  type ContextMenuItem,
  type ContextMenuOpenContext,
  type FileTreeDirectoryHandle,
  type FileTreeDropResult,
  type FileTreeRenameEvent,
  type GitStatusEntry,
} from '@pierre/trees'
import {
  ClipboardPaste,
  Copy,
  CopyPlus,
  Eye,
  FilePlus2,
  FolderSearch,
  FolderPlus,
  Pencil,
  Scissors,
  Trash2,
} from 'lucide-react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import { forwardRef, useCallback, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { DeleteConfirmDialog } from '@/components/Sidebar/DeleteConfirmDialog'
import { FileTreeMenu, FileTreeMenuItem, FileTreeMenuSeparator, FileTreeMenuShortcut } from '@/components/file-tree/FileTreeMenu'
import { NovaFileTree } from '@/components/file-tree/NovaFileTree'
import { applicationFileTreePath, canonicalFileTreePath, writeClipboardText } from '@/components/file-tree/paths'
import type { ProjectFileExplorerNode } from './model'
import { projectFileTreeProjection } from './model'
import {
  absoluteProjectPath,
  buildProjectFileDuplicatePlan,
  buildProjectFilePastePlan,
  joinProjectPath,
  projectBaseName,
  projectParentPath,
  removeNestedProjectPaths,
  type ProjectFileClipboard,
} from './operations'
import type { ProjectExplorerExtensions } from './types'

interface ProjectExplorerTreeProps {
  nodes: readonly ProjectFileExplorerNode[]
  workspace: string
  selectedPath: string | null
  expandedPaths: readonly string[]
  gitStatus?: readonly GitStatusEntry[]
  onSelectFile: (path: string) => void
  onDirectoryExpand: (path: string) => void | Promise<void>
  onDirectoryExpandedChange: (path: string, expanded: boolean) => void
  onScrollOffsetChange?: (offset: number) => void
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRevealItem: (path: string) => Promise<void>
  extensions?: ProjectExplorerExtensions
}

export interface ProjectExplorerTreeHandle {
  beginCreate: (type: 'file' | 'dir') => void
  collapseAll: () => void
  revealPath: (path: string) => boolean
}

interface PendingDraft {
  path: string
  type: 'file' | 'dir'
}

/** Editable Pierre tree shared by Writing and Game project-file surfaces. */
export const ProjectExplorerTree = forwardRef<ProjectExplorerTreeHandle, ProjectExplorerTreeProps>(function ProjectExplorerTree({
  nodes,
  workspace,
  selectedPath,
  expandedPaths,
  gitStatus = [],
  onSelectFile,
  onDirectoryExpand,
  onDirectoryExpandedChange,
  onScrollOffsetChange,
  onCreateItem,
  onDeleteItem,
  onRenameItem,
  onCopyItem,
  onMoveItem,
  onRevealItem,
  extensions = {},
}, ref) {
  const { t } = useTranslation()
  const treeRef = useRef<FileTree | null>(null)
  const authoritativeRef = useRef({ nodes, expandedPaths })
  authoritativeRef.current = { nodes, expandedPaths }
  const pendingDraftRef = useRef<PendingDraft | null>(null)
  const busyRef = useRef(false)
  const [busy, setBusy] = useState(false)
  const [clipboard, setClipboard] = useState<ProjectFileClipboard | null>(null)
  const [deletePaths, setDeletePaths] = useState<string[]>([])
  const projection = useMemo(() => projectFileTreeProjection(nodes), [nodes])
  const platformPresentation = useMemo(projectFileTreePlatformPresentation, [])
  const canonicalExpandedPaths = useMemo(() => expandedPaths.map((path) => canonicalFileTreePath(path, true)), [expandedPaths])
  const canonicalSelectedPaths = useMemo(() => selectedPath ? [selectedPath] : [], [selectedPath])
  const mergedGitStatus = useMemo(() => {
    const statuses = new Map(gitStatus.map((entry) => [entry.path, entry]))
    for (const node of projection.nodesByPath.values()) {
      if (node.ignored) statuses.set(canonicalFileTreePath(node.path, node.type === 'dir'), {
        path: canonicalFileTreePath(node.path, node.type === 'dir'),
        status: 'ignored',
      })
    }
    return [...statuses.values()]
  }, [gitStatus, projection.nodesByPath])

  const resetToAuthoritative = useCallback(() => {
    const tree = treeRef.current
    if (!tree) return
    const current = authoritativeRef.current
    const next = projectFileTreeProjection(current.nodes)
    tree.resetPaths({
      preparedInput: preparePresortedFileTreeInput(next.paths),
      initialExpandedPaths: current.expandedPaths.map((path) => canonicalFileTreePath(path, true)),
    })
  }, [])

  const selectPaths = useCallback((paths: readonly string[]) => {
    const tree = treeRef.current
    if (!tree) return
    const currentProjection = projectFileTreeProjection(authoritativeRef.current.nodes)
    const desired = paths.map((path) => canonicalFileTreePath(path, currentProjection.nodesByPath.get(path)?.type === 'dir'))
    for (const path of tree.getSelectedPaths()) tree.getItem(path)?.deselect()
    for (const path of desired) tree.getItem(path)?.select()
    const last = desired.at(-1)
    if (last) tree.scrollToPath(last, { focus: true, offset: 'nearest' })
  }, [])

  const runStructureOperation = useCallback(async (operation: string, mutate: () => Promise<void>, nextSelection?: readonly string[]) => {
    if (busyRef.current) return
    busyRef.current = true
    setBusy(true)
    try {
      await mutate()
      if (nextSelection) selectPaths(nextSelection)
    } catch (cause) {
      console.error('[features/project-explorer/ProjectExplorerTree.tsx] project tree operation failed', { operation, cause })
      resetToAuthoritative()
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }, [resetToAuthoritative, selectPaths])

  const beginCreate = useCallback((type: 'file' | 'dir', explicitParent?: string) => {
    if (busyRef.current) return
    const tree = treeRef.current
    if (!tree) return
    const focused = applicationFileTreePath(tree.getFocusedPath() ?? '')
    const focusedNode = projection.nodesByPath.get(focused)
    const parent = explicitParent ?? (focusedNode?.type === 'dir' ? focused : projectParentPath(focused))
    const baseName = t(type === 'dir' ? 'files.tree.newDirectoryName' : 'files.tree.newFileName')
    let placeholder = canonicalFileTreePath(joinProjectPath(parent, baseName), type === 'dir')
    for (let suffix = 2; tree.getItem(placeholder); suffix += 1) {
      placeholder = canonicalFileTreePath(joinProjectPath(parent, `${baseName} ${suffix}`), type === 'dir')
    }
    pendingDraftRef.current = { path: applicationFileTreePath(placeholder), type }
    tree.add(placeholder)
    if (!tree.startRenaming(placeholder, { removeIfCanceled: true })) {
      pendingDraftRef.current = null
      tree.remove(placeholder, type === 'dir' ? { recursive: true } : undefined)
    }
  }, [projection.nodesByPath, t])

  const collapseAll = useCallback(() => {
    const tree = treeRef.current
    if (!tree) return
    const rows = tree.getVisibleRows(0, Math.max(0, tree.getVisibleCount() - 1))
    for (const row of [...rows].reverse()) {
      const item = tree.getItem(row.path)
      if (item?.isDirectory()) {
        const directory = item as FileTreeDirectoryHandle
        if (directory.isExpanded()) directory.collapse()
      }
    }
  }, [])

  const revealPath = useCallback((path: string) => {
    const tree = treeRef.current
    if (!tree || !projection.nodesByPath.has(path)) return false
    const components = path.split('/')
    for (let index = 1; index < components.length; index += 1) {
      const item = tree.getItem(canonicalFileTreePath(components.slice(0, index).join('/'), true))
      if (item?.isDirectory()) (item as FileTreeDirectoryHandle).expand()
    }
    tree.scrollToPath(path, { focus: true, offset: 'center' })
    return true
  }, [projection.nodesByPath])

  useImperativeHandle(ref, () => ({ beginCreate, collapseAll, revealPath }), [beginCreate, collapseAll, revealPath])

  const handleRename = useCallback((event: FileTreeRenameEvent) => {
    const source = applicationFileTreePath(event.sourcePath)
    const destination = applicationFileTreePath(event.destinationPath)
    const draft = pendingDraftRef.current
    if (draft?.path === source) {
      pendingDraftRef.current = null
      void runStructureOperation('create', () => onCreateItem(destination, draft.type), [destination])
      return
    }
    void runStructureOperation('rename', () => onRenameItem(source, projectBaseName(destination)), [destination])
  }, [onCreateItem, onRenameItem, runStructureOperation])

  const handleDrop = useCallback((event: FileTreeDropResult) => {
    const parent = applicationFileTreePath(event.target.directoryPath ?? '')
    const sources = removeNestedProjectPaths(event.draggedPaths.map(applicationFileTreePath))
    const moves = sources.flatMap((source) => {
      const destination = joinProjectPath(parent, projectBaseName(source))
      return source === destination ? [] : [{ source, destination }]
    })
    if (moves.length === 0) return
    void runStructureOperation('move', async () => {
      const failures: unknown[] = []
      for (const move of moves) {
        try {
          await onMoveItem(move.source, move.destination)
        } catch (cause) {
          failures.push(cause)
        }
      }
      if (failures.length > 0) throw failures[0]
    }, moves.map((move) => move.destination))
  }, [onMoveItem, runStructureOperation])

  const stageClipboard = useCallback((mode: ProjectFileClipboard['mode'], paths: readonly string[]) => {
    const actionable = removeNestedProjectPaths(paths)
    if (actionable.length > 0) setClipboard({ mode, paths: actionable })
  }, [])

  const pasteInto = useCallback((targetDirectory: string) => {
    if (!clipboard) return
    const transfers = buildProjectFilePastePlan(nodes, clipboard, targetDirectory)
    if (transfers.length === 0) return
    void runStructureOperation('paste', async () => {
      for (const transfer of transfers) {
        if (clipboard.mode === 'copy') await onCopyItem(transfer.source, transfer.destination)
        else await onMoveItem(transfer.source, transfer.destination)
      }
      if (clipboard.mode === 'cut') setClipboard(null)
    }, transfers.map((transfer) => transfer.destination))
  }, [clipboard, nodes, onCopyItem, onMoveItem, runStructureOperation])

  const duplicatePaths = useCallback((paths: readonly string[]) => {
    const transfers = buildProjectFileDuplicatePlan(nodes, paths)
    if (transfers.length === 0) return
    void runStructureOperation('duplicate', async () => {
      const failures: unknown[] = []
      for (const transfer of transfers) {
        try {
          await onCopyItem(transfer.source, transfer.destination)
        } catch (cause) {
          failures.push(cause)
        }
      }
      if (failures.length > 0) throw failures[0]
    }, transfers.map((transfer) => transfer.destination))
  }, [nodes, onCopyItem, runStructureOperation])

  const copyPath = useCallback((path: string, relative: boolean) => {
    const value = relative ? path : absoluteProjectPath(workspace, path)
    void writeClipboardText(value).catch((cause) => {
      console.error('[features/project-explorer/ProjectExplorerTree.tsx] copying project file path failed', { path, relative, cause })
      toast.error(t('files.tree.copyPathFailed'))
    })
  }, [t, workspace])

  const revealItem = useCallback((path: string) => {
    void onRevealItem(path).catch((cause) => {
      console.error('[features/project-explorer/ProjectExplorerTree.tsx] revealing project file in the host file manager failed', { path, cause })
      toast.error(t('files.tree.revealFailed'), {
        description: cause instanceof Error ? cause.message : String(cause),
      })
    })
  }, [onRevealItem, t])

  const renderContextMenu = useCallback((item: ContextMenuItem, context: ContextMenuOpenContext) => {
    const path = applicationFileTreePath(item.path)
    const node = projection.nodesByPath.get(path)
    if (!node) return null
    const tree = treeRef.current
    const selected = tree?.getSelectedPaths() ?? []
    const actionPaths = selected.includes(item.path)
      ? removeNestedProjectPaths(selected.map(applicationFileTreePath))
      : [path]
    const containsSymlink = actionPaths.some((actionPath) => projection.nodesByPath.get(actionPath)?.symlink)
    const createParent = node.type === 'dir' ? path : projectParentPath(path)
    const extensionActions = extensions.getNodeActions?.({ node, paths: actionPaths }) ?? []
    const closeThen = (action: () => void, restoreFocus = true) => {
      context.close({ restoreFocus })
      action()
    }
    return (
      <FileTreeMenu anchorRect={context.anchorRect}>
        {extensionActions.map((action) => (
          <FileTreeMenuItem key={action.id} disabled={action.disabled || busy} onClick={() => closeThen(action.onSelect)}>
            {action.icon}{action.label}
          </FileTreeMenuItem>
        ))}
        {extensionActions.length > 0 ? <FileTreeMenuSeparator /> : null}
        <FileTreeMenuItem disabled={busy || (node.type === 'dir' && node.symlink)} onClick={() => closeThen(() => beginCreate('file', createParent), false)}><FilePlus2 />{t('sidebar.createFile')}</FileTreeMenuItem>
        <FileTreeMenuItem disabled={busy || (node.type === 'dir' && node.symlink)} onClick={() => closeThen(() => beginCreate('dir', createParent), false)}><FolderPlus />{t('sidebar.createDir')}</FileTreeMenuItem>
        <FileTreeMenuSeparator />
        <FileTreeMenuItem disabled={busy} onClick={() => closeThen(() => stageClipboard('cut', actionPaths))}><Scissors />{t('sidebar.cut')}</FileTreeMenuItem>
        <FileTreeMenuItem disabled={busy || containsSymlink} onClick={() => closeThen(() => stageClipboard('copy', actionPaths))}><Copy />{t('sidebar.copy')}<FileTreeMenuShortcut>{platformPresentation.copy}</FileTreeMenuShortcut></FileTreeMenuItem>
        {node.type === 'dir' ? (
          <FileTreeMenuItem disabled={busy || !clipboard} onClick={() => closeThen(() => pasteInto(path))}><ClipboardPaste />{t('sidebar.paste')}</FileTreeMenuItem>
        ) : null}
        <FileTreeMenuSeparator />
        <FileTreeMenuItem onClick={() => closeThen(() => copyPath(path, false))}><Copy />{t('sidebar.copyPath')}<FileTreeMenuShortcut>{platformPresentation.copyPath}</FileTreeMenuShortcut></FileTreeMenuItem>
        <FileTreeMenuItem onClick={() => closeThen(() => copyPath(path, true))}><Copy />{t('sidebar.copyRelativePath')}<FileTreeMenuShortcut>{platformPresentation.copyRelativePath}</FileTreeMenuShortcut></FileTreeMenuItem>
        <FileTreeMenuItem disabled={busy || containsSymlink} onClick={() => closeThen(() => duplicatePaths(actionPaths))}><CopyPlus />{t('sidebar.duplicate')}</FileTreeMenuItem>
        {node.type === 'file' ? (
          <FileTreeMenuItem onClick={() => closeThen(() => onSelectFile(path))}><Eye />{t('sidebar.viewFile')}</FileTreeMenuItem>
        ) : null}
        <FileTreeMenuItem onClick={() => closeThen(() => revealItem(path))}><FolderSearch />{t(platformPresentation.revealKey)}</FileTreeMenuItem>
        <FileTreeMenuSeparator />
        <FileTreeMenuItem disabled={busy} onClick={() => closeThen(() => { tree?.startRenaming(item.path) }, false)}><Pencil />{t('sidebar.rename')}</FileTreeMenuItem>
        <FileTreeMenuItem variant="destructive" disabled={busy} onClick={() => closeThen(() => setDeletePaths(actionPaths))}><Trash2 />{t('sidebar.delete')}<FileTreeMenuShortcut>{platformPresentation.delete}</FileTreeMenuShortcut></FileTreeMenuItem>
      </FileTreeMenu>
    )
  }, [beginCreate, busy, clipboard, copyPath, duplicatePaths, extensions, onSelectFile, pasteInto, platformPresentation, projection.nodesByPath, revealItem, stageClipboard, t])

  const handleSelectionChange = useCallback((canonicalPaths: readonly string[]) => {
    const path = applicationFileTreePath(canonicalPaths.at(-1) ?? '')
    if (projection.nodesByPath.get(path)?.type === 'file') onSelectFile(path)
  }, [onSelectFile, projection.nodesByPath])

  const handleKeyDownCapture = useCallback((event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.nativeEvent.composedPath().some((target) => target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement)) return
    const tree = treeRef.current
    if (!tree) return
    const focused = tree.getFocusedItem()
    const selected = actionablePaths(tree)
    const command = event.metaKey || event.ctrlKey
    if (command && event.altKey && event.key.toLowerCase() === 'c' && focused) {
      consumeKeyboardEvent(event)
      copyPath(applicationFileTreePath(focused.getPath()), event.shiftKey)
      return
    }
    if (event.key === 'Enter' && focused) {
      consumeKeyboardEvent(event)
      const path = applicationFileTreePath(focused.getPath())
      if (focused.isDirectory()) (focused as FileTreeDirectoryHandle).toggle()
      else onSelectFile(path)
      return
    }
    if ((event.key === 'Delete' || (event.key === 'Backspace' && command)) && selected.length > 0) {
      consumeKeyboardEvent(event)
      setDeletePaths(selected)
      return
    }
    if (command && !event.altKey && event.key.toLowerCase() === 'c' && selected.length > 0) {
      consumeKeyboardEvent(event)
      stageClipboard('copy', selected)
      return
    }
    if (command && event.key.toLowerCase() === 'x' && selected.length > 0) {
      consumeKeyboardEvent(event)
      stageClipboard('cut', selected)
      return
    }
    if (command && event.key.toLowerCase() === 'v' && clipboard) {
      consumeKeyboardEvent(event)
      const focusedPath = applicationFileTreePath(focused?.getPath() ?? '')
      pasteInto(focused?.isDirectory() ? focusedPath : projectParentPath(focusedPath))
    }
  }, [clipboard, copyPath, onSelectFile, pasteInto, stageClipboard])

  return (
    <>
      <div
        className="relative flex h-full min-h-0 min-w-0 flex-1"
        onPointerDownCapture={(event) => {
          if (event.nativeEvent.composedPath().some((target) => target instanceof HTMLElement && target.hasAttribute('data-item-path'))) return
          for (const path of treeRef.current?.getSelectedPaths() ?? []) treeRef.current?.getItem(path)?.deselect()
        }}
      >
        <NovaFileTree
          ref={treeRef}
          paths={projection.paths}
          presorted
          ariaLabel={t('files.tree.title')}
          searchLabel={t('files.tree.search')}
          initialExpandedPaths={canonicalExpandedPaths}
          selectedPaths={canonicalSelectedPaths}
          gitStatus={mergedGitStatus}
          gitStatusPresentation="color-only"
          dragAndDrop={{
            canDrag: (paths) => !busyRef.current && paths.every((path) => projection.nodesByPath.has(applicationFileTreePath(path))),
            canDrop: ({ draggedPaths, target }) => {
              if (busyRef.current) return false
              const parent = applicationFileTreePath(target.directoryPath ?? '')
              const parentNode = projection.nodesByPath.get(parent)
              if (parentNode?.symlink) return false
              const sources = draggedPaths.map(applicationFileTreePath)
              if (sources.some((source) => parent === source || parent.startsWith(`${source}/`))) return false
              return sources.some((source) => projectParentPath(source) !== parent)
            },
            onDropComplete: handleDrop,
            onDropError: (error, drop) => {
              console.error('[features/project-explorer/ProjectExplorerTree.tsx] Pierre rejected a project file drop', { error, drop })
              resetToAuthoritative()
            },
          }}
          renaming={{
            canRename: ({ path }) => !busyRef.current && (projection.nodesByPath.has(applicationFileTreePath(path)) || pendingDraftRef.current?.path === applicationFileTreePath(path)),
            onRename: handleRename,
            onError: (error) => {
              console.error('[features/project-explorer/ProjectExplorerTree.tsx] Pierre rejected a project file rename', { error })
            },
          }}
          renderRowDecoration={({ item }) => (
            projection.nodesByPath.get(applicationFileTreePath(item.path))?.symlink ? { text: '↗' } : null
          )}
          renderContextMenu={renderContextMenu}
          contextMenuTriggerMode="right-click"
          onSelectionChange={handleSelectionChange}
          onDirectoryExpandedChange={(path, expanded) => onDirectoryExpandedChange(applicationFileTreePath(path), expanded)}
          onDirectoryExpand={(path) => onDirectoryExpand(applicationFileTreePath(path))}
          onScrollOffsetChange={onScrollOffsetChange}
          onKeyDownCapture={handleKeyDownCapture}
        />
        {projection.paths.length === 0 ? (
          <div className="pointer-events-none absolute inset-x-3 top-10 text-center text-xs text-[var(--nova-text-faint)]">
            {t('files.tree.empty')}
          </div>
        ) : null}
      </div>
      <DeleteConfirmDialog
        open={deletePaths.length > 0}
        path={deletePaths}
        recovery={extensions.deleteRecovery ?? 'none'}
        onOpenChange={(open) => {
          if (!open) setDeletePaths([])
        }}
        onConfirm={async () => {
          const nextSelection = selectionAfterDeletion(treeRef.current, deletePaths)
          for (const path of removeNestedProjectPaths(deletePaths)) await onDeleteItem(path)
          setDeletePaths([])
          selectPaths(nextSelection)
        }}
      />
    </>
  )
})

function actionablePaths(tree: FileTree): string[] {
  const selected = tree.getSelectedPaths().map(applicationFileTreePath)
  if (selected.length > 0) return removeNestedProjectPaths(selected)
  const focused = applicationFileTreePath(tree.getFocusedPath() ?? '')
  return focused ? [focused] : []
}

function selectionAfterDeletion(tree: FileTree | null, paths: readonly string[]): string[] {
  if (!tree) return []
  const removed = removeNestedProjectPaths(paths)
  const rows = tree.getVisibleRows(0, Math.max(0, tree.getVisibleCount() - 1))
  const indices = removed.map((path) => rows.findIndex((row) => applicationFileTreePath(row.path) === path)).filter((index) => index >= 0)
  if (indices.length === 0) return []
  const first = Math.min(...indices)
  const candidates = rows.filter((row) => {
    const path = applicationFileTreePath(row.path)
    return !removed.some((root) => path === root || path.startsWith(`${root}/`))
  })
  const next = candidates.find((row) => row.index > first)
  const previous = candidates.findLast((row) => row.index < first)
  const fallback = next ?? previous
  return fallback ? [applicationFileTreePath(fallback.path)] : []
}

function consumeKeyboardEvent(event: ReactKeyboardEvent) {
  event.preventDefault()
  event.stopPropagation()
}

function projectFileTreePlatformPresentation() {
  const platform = typeof navigator === 'undefined' ? '' : navigator.platform
  if (/Mac|iPhone|iPad/.test(platform)) {
    return { copy: '⌘C', copyPath: '⌘⌥C', copyRelativePath: '⌘⌥⇧C', delete: '⌘⌫', revealKey: 'sidebar.revealInFinder' as const }
  }
  if (/Win/.test(platform)) {
    return { copy: 'Ctrl+C', copyPath: 'Ctrl+Alt+C', copyRelativePath: 'Ctrl+Alt+Shift+C', delete: 'Delete', revealKey: 'sidebar.revealInFileExplorer' as const }
  }
  return { copy: 'Ctrl+C', copyPath: 'Ctrl+Alt+C', copyRelativePath: 'Ctrl+Alt+Shift+C', delete: 'Delete', revealKey: 'sidebar.revealInFileManager' as const }
}
