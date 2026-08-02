import type { NodeApi, NodeRendererProps, RowRendererProps, TreeApi } from 'react-arborist'
import { Tree } from 'react-arborist'
import {
  ChevronDown,
  ChevronRight,
  Copy,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  Loader2,
  MoreHorizontal,
  MoveRight,
  Pencil,
  Plus,
  Trash2,
} from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { DeleteConfirmDialog } from '@/components/Sidebar/DeleteConfirmDialog'
import { FileOperationDialog, type FileOperationMode } from '@/components/Sidebar/FileOperationDialog'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import type { ProjectFileExplorerNode } from './project-file-explorer-model'

interface ProjectFileTreeProps {
  nodes: readonly ProjectFileExplorerNode[]
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
}

interface OperationState {
  mode: FileOperationMode
  targetPath: string
  defaultValue: string
}

interface TreeActions {
  create: (parentPath: string, type: 'file' | 'dir') => void
  rename: (node: NodeApi<ProjectFileExplorerNode>) => void
  copy: (path: string) => void
  move: (path: string) => void
  delete: (paths: string[]) => void
}

interface ExplorerRenderContextValue {
  actions: TreeActions
  onLoadMore: (path: string) => void | Promise<void>
}

const ExplorerRenderContext = createContext<ExplorerRenderContextValue | null>(null)

export function ProjectFileTree({
  nodes,
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
}: ProjectFileTreeProps) {
  const { t } = useTranslation()
  const hostRef = useRef<HTMLDivElement>(null)
  const size = useElementSize(hostRef)
  const [operation, setOperation] = useState<OperationState | null>(null)
  const [deletePaths, setDeletePaths] = useState<string[]>([])
  const initialOpenState = useMemo(
    () => Object.fromEntries(expandedPaths.map((path) => [path, true])),
    [expandedPaths],
  )

  const actions = useMemo<TreeActions>(() => ({
    create: (parentPath, type) => setOperation({
      mode: type === 'dir' ? 'create-dir' : 'create-file',
      targetPath: parentPath,
      defaultValue: parentPath ? `${parentPath}/` : '',
    }),
    rename: (node) => void node.edit(),
    copy: (path) => setOperation({ mode: 'copy', targetPath: path, defaultValue: copyDefaultPath(path) }),
    move: (path) => setOperation({ mode: 'move', targetPath: path, defaultValue: path }),
    delete: setDeletePaths,
  }), [])
  const renderContext = useMemo(() => ({ actions, onLoadMore }), [actions, onLoadMore])

  const handleToggle = useCallback((id: string) => {
    const node = treeRef.current?.get(id)
    if (!node || node.data.type !== 'dir') return
    const expanded = treeRef.current?.isOpen(id) === true
    onDirectoryExpandedChange(node.data.path, expanded)
    if (expanded && !node.data.loaded) {
      void Promise.resolve(onDirectoryExpand(node.data.path)).catch((cause) => {
        console.error('[features/files/ProjectFileTree.tsx] expanding directory failed', {
          path: node.data.path,
          cause,
        })
      })
    }
  }, [onDirectoryExpand, onDirectoryExpandedChange, treeRef])

  const handleActivate = useCallback((node: NodeApi<ProjectFileExplorerNode>) => {
    if (node.data.type === 'file') onSelectFile(node.data.path)
    if (node.data.type === 'dir') node.toggle()
    if (node.data.type === 'more') {
      void Promise.resolve(onLoadMore(node.data.path)).catch((cause) => {
        console.error('[features/files/ProjectFileTree.tsx] loading directory page failed', {
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
    const sources = removeNestedPaths(dragNodes
      .map((node) => node.data)
      .filter((node) => node.type !== 'more')
      .map((node) => node.path))
    for (const source of sources) {
      const destination = joinPath(parent, baseName(source))
      if (source !== destination) await onMoveItem(source, destination)
    }
  }, [onMoveItem])
  const handleRename = useCallback(async ({ node, name }: {
    node: NodeApi<ProjectFileExplorerNode>
    name: string
  }) => onRenameItem(node.data.path, name), [onRenameItem])

  const submitOperation = useCallback(async (value: string) => {
    if (!operation) return
    switch (operation.mode) {
      case 'create-file':
        await onCreateItem(value, 'file')
        return
      case 'create-dir':
        await onCreateItem(value, 'dir')
        return
      case 'rename':
        await onRenameItem(operation.targetPath, value)
        return
      case 'copy':
        await onCopyItem(operation.targetPath, value)
        return
      case 'move':
        await onMoveItem(operation.targetPath, value)
    }
  }, [onCopyItem, onCreateItem, onMoveItem, onRenameItem, operation])

  return (
    <div ref={hostRef} className="h-full min-h-0 min-w-0 overflow-hidden">
      <ExplorerRenderContext.Provider value={renderContext}>
        <Tree<ProjectFileExplorerNode>
          ref={treeRef}
          data={nodes}
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
      </ExplorerRenderContext.Provider>
      <FileOperationDialog
        open={operation !== null}
        mode={operation?.mode ?? 'copy'}
        targetPath={operation?.targetPath ?? ''}
        defaultValue={operation?.defaultValue ?? ''}
        onOpenChange={(open) => {
          if (!open) setOperation(null)
        }}
        onSubmit={submitOperation}
      />
      <DeleteConfirmDialog
        open={deletePaths.length > 0}
        path={deletePaths}
        recovery="none"
        onOpenChange={(open) => {
          if (!open) setDeletePaths([])
        }}
        onConfirm={async () => {
          for (const path of removeNestedPaths(deletePaths)) await onDeleteItem(path)
          setDeletePaths([])
        }}
      />
    </div>
  )
}

function ExplorerRow({ node, attrs, innerRef, children }: RowRendererProps<ProjectFileExplorerNode>) {
  return (
    <div
      {...attrs}
      ref={innerRef}
      onFocus={(event) => event.stopPropagation()}
      onClick={node.handleClick}
      className={cn(
        'group/tree-row flex cursor-default items-center rounded-sm outline-none',
        node.isSelected && 'bg-[var(--nova-active)] text-[var(--nova-text)]',
        !node.isSelected && 'hover:bg-[var(--nova-hover)]',
        node.isFocused && 'ring-1 ring-inset ring-[var(--nova-accent)]',
      )}
    >
      {children}
    </div>
  )
}

function ExplorerNode({ node, style, dragHandle }: NodeRendererProps<ProjectFileExplorerNode>) {
  const { t } = useTranslation()
  const renderContext = useContext(ExplorerRenderContext)
  if (!renderContext) throw new Error('Project file explorer render context is unavailable')
  const { actions, onLoadMore } = renderContext
  const data = node.data
  if (data.type === 'more') {
    return (
      <button
        type="button"
        style={style}
        className="flex h-full min-w-0 flex-1 items-center gap-1.5 pr-2 text-[11px] text-[var(--nova-accent)] hover:underline"
        onClick={(event) => {
          event.stopPropagation()
          void onLoadMore(data.path)
        }}
      >
        {data.loading ? <Loader2 className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />}
        {t(data.loading ? 'files.tree.loadingMore' : 'files.tree.loadMore')}
      </button>
    )
  }

  const actionPaths = node.isSelected
    ? node.tree.selectedNodes.map((selected) => selected.data).filter((item) => item.type !== 'more').map((item) => item.path)
    : [data.path]
  const menu = (
    <NodeActions
      node={node}
      actionPaths={actionPaths}
      actions={actions}
      kind="context"
    />
  )
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          ref={dragHandle}
          style={style}
          className={cn(
            'flex h-full min-w-0 flex-1 items-center gap-1 pr-1 text-xs',
            data.ignored && 'opacity-55',
          )}
          title={data.path}
        >
          {data.type === 'dir' ? (
            <button
              type="button"
              className="flex size-5 shrink-0 items-center justify-center rounded text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              aria-label={t(node.isOpen ? 'files.tree.collapseDirectory' : 'files.tree.expandDirectory', { name: data.name })}
              onClick={(event) => {
                event.stopPropagation()
                if (event.altKey) toggleLoadedBranch(node, !node.isOpen)
                else node.toggle()
              }}
            >
              {data.loading ? <Loader2 className="size-3.5 animate-spin" /> : node.isOpen ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
            </button>
          ) : <span className="size-5 shrink-0" />}
          {data.type === 'dir'
            ? node.isOpen ? <FolderOpen className="size-4 shrink-0 text-[var(--nova-tree-icon)]" /> : <Folder className="size-4 shrink-0 text-[var(--nova-tree-icon)]" />
            : <FileText className="size-4 shrink-0 text-[var(--nova-tree-icon)]" />}
          {node.isEditing ? <RenameInput node={node} /> : <span className="min-w-0 flex-1 truncate">{data.name}</span>}
          {data.symlink ? <span className="shrink-0 text-[9px] text-[var(--nova-text-faint)]">↗</span> : null}
          <NodeActionDropdown node={node} actionPaths={actionPaths} actions={actions} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>{menu}</ContextMenuContent>
    </ContextMenu>
  )
}

function RenameInput({ node }: { node: NodeApi<ProjectFileExplorerNode> }) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])
  return (
    <input
      ref={ref}
      defaultValue={node.data.name}
      className="h-5 min-w-0 flex-1 rounded-sm border border-[var(--nova-accent)] bg-[var(--nova-bg)] px-1 text-xs outline-none"
      onClick={(event) => event.stopPropagation()}
      onBlur={() => node.reset()}
      onKeyDown={(event) => {
        event.stopPropagation()
        if (event.key === 'Escape') node.reset()
        if (event.key === 'Enter') {
          const name = event.currentTarget.value.trim()
          if (name && name !== node.data.name) void node.submit(name)
          else node.reset()
        }
      }}
    />
  )
}

function NodeActionDropdown({ node, actionPaths, actions }: {
  node: NodeApi<ProjectFileExplorerNode>
  actionPaths: string[]
  actions: TreeActions
}) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="flex size-5 shrink-0 items-center justify-center rounded text-[var(--nova-text-faint)] opacity-0 hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] group-hover/tree-row:opacity-100 data-[state=open]:opacity-100 focus-visible:opacity-100"
          onClick={(event) => event.stopPropagation()}
          aria-label={t('files.tree.moreActions')}
        >
          <MoreHorizontal className="size-3.5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-40">
        <NodeActions node={node} actionPaths={actionPaths} actions={actions} kind="dropdown" />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function NodeActions({ node, actionPaths, actions, kind }: {
  node: NodeApi<ProjectFileExplorerNode>
  actionPaths: string[]
  actions: TreeActions
  kind: 'context' | 'dropdown'
}) {
  const { t } = useTranslation()
  const Item = kind === 'context' ? ContextMenuItem : DropdownMenuItem
  const Separator = kind === 'context' ? ContextMenuSeparator : DropdownMenuSeparator
  const path = node.data.path
  return (
    <>
      {node.data.type === 'dir' ? (
        <>
          <Item onSelect={() => actions.create(path, 'file')}><FileText />{t('sidebar.createFile')}</Item>
          <Item onSelect={() => actions.create(path, 'dir')}><FolderPlus />{t('sidebar.createDir')}</Item>
          <Separator />
        </>
      ) : null}
      <Item onSelect={() => actions.rename(node)}><Pencil />{t('sidebar.rename')}</Item>
      <Item disabled={node.data.symlink} onSelect={() => actions.copy(path)}><Copy />{t('sidebar.copy')}</Item>
      <Item onSelect={() => actions.move(path)}><MoveRight />{t('sidebar.move')}</Item>
      <Separator />
      <Item variant="destructive" onSelect={() => actions.delete(actionPaths)}><Trash2 />{t('sidebar.delete')}</Item>
    </>
  )
}

function toggleLoadedBranch(node: NodeApi<ProjectFileExplorerNode>, open: boolean) {
  const visit = (current: NodeApi<ProjectFileExplorerNode>) => {
    if (!current.isInternal) return
    if (open) current.tree.open(current.id, false)
    else current.tree.close(current.id, false)
    if (open && !current.data.loaded) return
    current.children?.forEach(visit)
  }
  visit(node)
  node.tree.redrawList(node.rowIndex ?? 0)
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

function removeNestedPaths(paths: readonly string[]) {
  const sorted = [...new Set(paths)].sort((left, right) => left.length - right.length)
  return sorted.filter((path, index) => !sorted.slice(0, index).some((parent) => path.startsWith(`${parent}/`)))
}

function baseName(path: string) {
  return path.slice(path.lastIndexOf('/') + 1)
}

function joinPath(parent: string, name: string) {
  return parent ? `${parent}/${name}` : name
}

function copyDefaultPath(path: string) {
  const parent = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : ''
  const name = baseName(path)
  const dot = name.lastIndexOf('.')
  const copyName = dot > 0 ? `${name.slice(0, dot)}-copy${name.slice(dot)}` : `${name}-copy`
  return joinPath(parent, copyName)
}

function disableProjectFileEdit(node: ProjectFileExplorerNode) {
  return node.type !== 'file' && node.type !== 'dir'
}

function disableProjectFileDrag(node: ProjectFileExplorerNode) {
  return node.type === 'more'
}

function disableProjectFileDrop({ parentNode }: { parentNode: NodeApi<ProjectFileExplorerNode> }) {
  return !parentNode.isRoot && (parentNode.data.type !== 'dir' || parentNode.data.symlink)
}
