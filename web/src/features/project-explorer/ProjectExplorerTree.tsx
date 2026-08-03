import type { NodeApi, TreeApi } from 'react-arborist'
import { Tree } from 'react-arborist'
import { ClipboardPaste, FileText, FolderPlus } from 'lucide-react'
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { DeleteConfirmDialog } from '@/components/Sidebar/DeleteConfirmDialog'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import type { ProjectFileExplorerNode } from './model'
import {
  ExplorerNode,
  ExplorerRow,
  ProjectExplorerRenderContext,
  type ProjectExplorerTreeActions,
} from './ProjectExplorerNode'
import {
  absoluteProjectPath,
  buildProjectFilePastePlan,
  insertProjectFileDraft,
  joinProjectPath,
  projectBaseName,
  projectParentPath,
  PROJECT_FILE_DRAFT_PREFIX,
  removeNestedProjectPaths,
  type ProjectFileClipboard,
  type ProjectFileDraft,
} from './operations'
import type { ProjectExplorerExtensions } from './types'

interface ProjectExplorerTreeProps {
  nodes: readonly ProjectFileExplorerNode[]
  workspace: string
  selectedPath: string | null
  expandedPaths: readonly string[]
  onSelectFile: (path: string) => void
  onDirectoryExpand: (path: string) => void | Promise<void>
  onDirectoryExpandedChange: (path: string, expanded: boolean) => void
  onLoadMore: (path: string) => void | Promise<void>
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  treeRef: RefObject<TreeApi<ProjectFileExplorerNode> | null>
  extensions?: ProjectExplorerExtensions
}

export interface ProjectExplorerTreeHandle {
  beginCreate: (type: 'file' | 'dir') => void
}

/** Virtualized Explorer behavior shared by Writing and Game project files. */
export const ProjectExplorerTree = forwardRef<ProjectExplorerTreeHandle, ProjectExplorerTreeProps>(function ProjectExplorerTree({
  nodes,
  workspace,
  selectedPath,
  expandedPaths,
  onSelectFile,
  onDirectoryExpand,
  onDirectoryExpandedChange,
  onLoadMore,
  onCreateItem,
  onDeleteItem,
  onRenameItem,
  onCopyItem,
  onMoveItem,
  treeRef,
  extensions = {},
}, ref) {
  const { t } = useTranslation()
  const hostRef = useRef<HTMLDivElement>(null)
  const size = useElementSize(hostRef)
  const draftSequenceRef = useRef(0)
  const [draft, setDraft] = useState<ProjectFileDraft | null>(null)
  const [clipboard, setClipboard] = useState<ProjectFileClipboard | null>(null)
  const [deletePaths, setDeletePaths] = useState<string[]>([])
  const renderedNodes = useMemo(() => insertProjectFileDraft(nodes, draft), [draft, nodes])
  const initialOpenState = useMemo(
    () => Object.fromEntries(expandedPaths.map((path) => [path, true])),
    [expandedPaths],
  )

  const beginCreate = useCallback((type: 'file' | 'dir', explicitParent?: string) => {
    const tree = treeRef.current
    const parentPath = explicitParent ?? insertionDirectory(tree, selectedPath)
    if (parentPath) tree?.open(parentPath)
    if (tree?.editingId) tree.reset()
    draftSequenceRef.current += 1
    setDraft({
      id: `${PROJECT_FILE_DRAFT_PREFIX}${draftSequenceRef.current}`,
      parentPath,
      type,
      index: 0,
    })
  }, [selectedPath, treeRef])

  useImperativeHandle(ref, () => ({
    beginCreate: (type) => beginCreate(type),
  }), [beginCreate])

  useEffect(() => {
    if (!draft) return
    const tree = treeRef.current
    const node = tree?.get(draft.id)
    if (!tree || !node || tree.editingId === draft.id) return
    tree.openParents(draft.id)
    tree.focus(node)
    void node.edit()
  }, [draft, renderedNodes, treeRef])

  const handleToggle = useCallback((id: string) => {
    const node = treeRef.current?.get(id)
    if (!node || node.data.type !== 'dir') return
    const expanded = treeRef.current?.isOpen(id) === true
    onDirectoryExpandedChange(node.data.path, expanded)
    if (expanded && !node.data.loaded) {
      void Promise.resolve(onDirectoryExpand(node.data.path)).catch((cause) => {
        console.error('[features/project-explorer/ProjectExplorerTree.tsx] expanding directory failed', {
          path: node.data.path,
          cause,
        })
      })
    }
  }, [onDirectoryExpand, onDirectoryExpandedChange, treeRef])

  const handleActivate = useCallback((node: NodeApi<ProjectFileExplorerNode>) => {
    if (node.data.draft) return
    if (node.data.type === 'file') onSelectFile(node.data.path)
    if (node.data.type === 'dir') node.toggle()
    if (node.data.type === 'more') {
      void Promise.resolve(onLoadMore(node.data.path)).catch((cause) => {
        console.error('[features/project-explorer/ProjectExplorerTree.tsx] loading directory page failed', {
          path: node.data.path,
          cause,
        })
      })
    }
  }, [onLoadMore, onSelectFile])

  const handleMove = useCallback(async ({
    dragNodes,
    parentId,
  }: {
    dragNodes: NodeApi<ProjectFileExplorerNode>[]
    parentId: string | null
  }) => {
    const parent = parentId ?? ''
    const sources = removeNestedProjectPaths(dragNodes
      .map((node) => node.data)
      .filter((node) => node.type !== 'more' && !node.draft)
      .map((node) => node.path))
    for (const source of sources) {
      const destination = joinProjectPath(parent, projectBaseName(source))
      if (source !== destination) await onMoveItem(source, destination)
    }
  }, [onMoveItem])

  const handleRename = useCallback(async ({ node, name }: {
    node: NodeApi<ProjectFileExplorerNode>
    name: string
  }) => {
    if (node.data.draft && draft) {
      await onCreateItem(joinProjectPath(draft.parentPath, name), draft.type)
      setDraft(null)
      return
    }
    await onRenameItem(node.data.path, name)
  }, [draft, onCreateItem, onRenameItem])

  const stageClipboard = useCallback((mode: ProjectFileClipboard['mode'], paths: string[]) => {
    const actionable = removeNestedProjectPaths(paths)
    if (actionable.length > 0) setClipboard({ mode, paths: actionable })
  }, [])

  const pasteInto = useCallback(async (targetDirectory: string) => {
    if (!clipboard) return
    const transfers = buildProjectFilePastePlan(nodes, clipboard, targetDirectory)
    for (const transfer of transfers) {
      if (clipboard.mode === 'copy') await onCopyItem(transfer.source, transfer.destination)
      else await onMoveItem(transfer.source, transfer.destination)
    }
    if (clipboard.mode === 'cut' && transfers.length > 0) setClipboard(null)
  }, [clipboard, nodes, onCopyItem, onMoveItem])

  const copyPath = useCallback((path: string, relative: boolean) => {
    const value = relative ? path : absoluteProjectPath(workspace, path)
    void writeClipboardText(value).catch((cause) => {
      console.error('[features/project-explorer/ProjectExplorerTree.tsx] copying project file path failed', {
        path,
        relative,
        cause,
      })
      toast.error(t('files.tree.copyPathFailed'))
    })
  }, [t, workspace])

  const actions = useMemo<ProjectExplorerTreeActions>(() => ({
    beginCreate: (parentPath, type) => beginCreate(type, parentPath),
    rename: (node) => void node.edit(),
    stageClipboard,
    paste: (parentPath) => {
      void pasteInto(parentPath).catch((cause) => {
        console.error('[features/project-explorer/ProjectExplorerTree.tsx] pasting project files failed', {
          parentPath,
          cause,
        })
      })
    },
    copyPath,
    delete: setDeletePaths,
    cancelDraft: () => setDraft(null),
    clipboard,
  }), [beginCreate, clipboard, copyPath, pasteInto, stageClipboard])
  const renderContext = useMemo(() => ({ actions, extensions, onLoadMore }), [actions, extensions, onLoadMore])

  const handleKeyDownCapture = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return
    const tree = treeRef.current
    if (!tree) return
    const focused = actionableFocusedNode(tree)
    const paths = actionableSelection(tree)
    const command = event.metaKey || event.ctrlKey

    if (event.key === 'F2' && focused) {
      consumeKeyboardEvent(event)
      void focused.edit()
      return
    }
    if (event.key === 'Enter' && focused) {
      consumeKeyboardEvent(event)
      focused.select()
      if (focused.data.type === 'file') onSelectFile(focused.data.path)
      else focused.toggle()
      return
    }
    if ((event.key === 'Delete' || (event.key === 'Backspace' && command)) && paths.length > 0) {
      consumeKeyboardEvent(event)
      setDeletePaths(paths)
      return
    }
    if (command && event.key.toLowerCase() === 'c' && paths.length > 0) {
      consumeKeyboardEvent(event)
      stageClipboard('copy', paths)
      return
    }
    if (command && event.key.toLowerCase() === 'x' && paths.length > 0) {
      consumeKeyboardEvent(event)
      stageClipboard('cut', paths)
      return
    }
    if (command && event.key.toLowerCase() === 'v' && clipboard) {
      consumeKeyboardEvent(event)
      void pasteInto(insertionDirectory(tree, selectedPath)).catch((cause) => {
        console.error('[features/project-explorer/ProjectExplorerTree.tsx] keyboard paste failed', { cause })
      })
    }
  }, [clipboard, onSelectFile, pasteInto, selectedPath, stageClipboard, treeRef])

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div
            ref={hostRef}
            className="relative h-full min-h-0 min-w-0 overflow-hidden"
            onKeyDownCapture={handleKeyDownCapture}
          >
            <ProjectExplorerRenderContext.Provider value={renderContext}>
              <Tree<ProjectFileExplorerNode>
                ref={treeRef}
                data={renderedNodes}
                idAccessor="id"
                childrenAccessor="children"
                width={Math.max(size.width, 1)}
                height={Math.max(size.height, 1)}
                rowHeight={26}
                indent={14}
                overscanCount={12}
                openByDefault={false}
                selection={selectedPath ?? undefined}
                initialOpenState={initialOpenState}
                aria-label={t('files.tree.title')}
                disableEdit={disableProjectFileEdit}
                disableDrag={disableProjectFileDrag}
                disableDrop={disableProjectFileDrop}
                onActivate={handleActivate}
                onToggle={handleToggle}
                onRename={handleRename}
                onMove={handleMove}
                renderRow={ExplorerRow}
              >
                {ExplorerNode}
              </Tree>
            </ProjectExplorerRenderContext.Provider>
            {renderedNodes.length === 0 ? (
              <div className="pointer-events-none absolute inset-x-3 top-8 text-center text-xs text-[var(--nova-text-faint)]">
                {t('files.tree.empty')}
              </div>
            ) : null}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem onSelect={() => beginCreate('file', '')}><FileText />{t('sidebar.createFile')}</ContextMenuItem>
          <ContextMenuItem onSelect={() => beginCreate('dir', '')}><FolderPlus />{t('sidebar.createDir')}</ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem disabled={!clipboard} onSelect={() => actions.paste('')}><ClipboardPaste />{t('sidebar.paste')}</ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
      <DeleteConfirmDialog
        open={deletePaths.length > 0}
        path={deletePaths}
        recovery={extensions.deleteRecovery ?? 'none'}
        onOpenChange={(open) => {
          if (!open) setDeletePaths([])
        }}
        onConfirm={async () => {
          for (const path of removeNestedProjectPaths(deletePaths)) await onDeleteItem(path)
          setDeletePaths([])
        }}
      />
    </>
  )
})

function insertionDirectory(
  tree: TreeApi<ProjectFileExplorerNode> | null,
  selectedPath: string | null,
): string {
  const node = tree?.focusedNode ?? (selectedPath ? tree?.get(selectedPath) : null)
  if (!node || node.data.type === 'more' || node.data.draft) return ''
  return node.data.type === 'dir' ? node.data.path : projectParentPath(node.data.path)
}

function actionableFocusedNode(tree: TreeApi<ProjectFileExplorerNode>) {
  const node = tree.focusedNode
  return node && node.data.type !== 'more' && !node.data.draft ? node : null
}

function actionableSelection(tree: TreeApi<ProjectFileExplorerNode>): string[] {
  const selected = tree.selectedNodes
    .map((node) => node.data)
    .filter((node) => node.type !== 'more' && !node.draft)
    .map((node) => node.path)
  if (selected.length > 0) return removeNestedProjectPaths(selected)
  const focused = actionableFocusedNode(tree)
  return focused ? [focused.data.path] : []
}

function consumeKeyboardEvent(event: KeyboardEvent) {
  event.preventDefault()
  event.stopPropagation()
}

async function writeClipboardText(value: string) {
  if (!navigator.clipboard?.writeText) throw new Error('Clipboard API is unavailable')
  await navigator.clipboard.writeText(value)
}

function useElementSize(ref: RefObject<HTMLElement | null>) {
  const [size, setSize] = useState({ width: 280, height: 400 })
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const update = () => {
      const bounds = element.getBoundingClientRect()
      const next = { width: Math.round(bounds.width), height: Math.round(bounds.height) }
      setSize((current) => current.width === next.width && current.height === next.height ? current : next)
    }
    update()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])
  return size
}

function disableProjectFileEdit(node: ProjectFileExplorerNode) {
  return node.type !== 'file' && node.type !== 'dir'
}

function disableProjectFileDrag(node: ProjectFileExplorerNode) {
  return node.type === 'more' || node.draft === true
}

function disableProjectFileDrop({ parentNode }: { parentNode: NodeApi<ProjectFileExplorerNode> }) {
  return !parentNode.isRoot && (parentNode.data.type !== 'dir' || parentNode.data.symlink || parentNode.data.draft === true)
}
