import { useMemo, useState, type DragEvent, type MouseEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDown,
  ChevronRight,
  Copy,
  FilePlus,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  MoreHorizontal,
  MoveRight,
  Pencil,
  Trash2,
  AtSign,
} from 'lucide-react'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ChapterSummary } from '@/lib/api'
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
import { FileOperationDialog, type FileOperationMode } from './FileOperationDialog'
import { DeleteConfirmDialog } from './DeleteConfirmDialog'
import { InlineInput } from './InlineInput'

interface FileTreeProps {
  nodes: FileNode[]
  basePath?: string
  selectedFile: string | null
  onSelectFile: (path: string) => void
  onReferenceFile?: (path: string) => void
  onCreateItem?: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onCopyItem?: (from: string, to: string) => Promise<void>
  onMoveItem?: (from: string, to: string) => Promise<void>
  chapterStats?: Record<string, ChapterSummary>
}

/** 内联编辑状态 */
interface InlineEditState {
  /** 正在编辑的父目录路径 */
  parentPath: string
  /** 编辑类型 */
  type: 'create-file' | 'create-dir' | 'rename'
  /** 正在重命名的完整路径（仅 rename 时有值） */
  renamePath?: string
  /** 默认值 */
  defaultValue: string
}

/** 默认展开的目录名 */
const DEFAULT_EXPANDED = new Set(['setting', 'chapters'])
const MENU_CONTENT_CLASS =
  'min-w-[180px] rounded-lg border-[var(--nova-border)] bg-[var(--nova-menu-bg)] p-1 text-[var(--nova-text)] shadow-[0_12px_32px_rgba(0,0,0,0.18)] backdrop-blur'
const MENU_ITEM_CLASS =
  'cursor-pointer rounded-md px-2 py-1.5 text-xs text-[var(--nova-text-muted)] transition-colors focus:bg-[var(--nova-menu-item-hover-bg)] focus:text-[var(--nova-text)] data-[highlighted]:bg-[var(--nova-menu-item-hover-bg)] data-[highlighted]:text-[var(--nova-text)] [&_svg]:text-[var(--nova-tree-icon)] focus:[&_svg]:text-[var(--nova-text)] data-[highlighted]:[&_svg]:text-[var(--nova-text)]'
const MENU_DANGER_CLASS =
  'text-[var(--nova-danger)] focus:bg-[var(--nova-danger-bg)] focus:text-[var(--nova-danger)] data-[highlighted]:bg-[var(--nova-danger-bg)] data-[highlighted]:text-[var(--nova-danger)] [&_svg]:text-[var(--nova-danger)]'
const MENU_SEPARATOR_CLASS = 'mx-1 my-1 h-px bg-[var(--nova-border)]'

/** 递归渲染目录树组件 */
export function FileTree({
  nodes,
  basePath = '',
  selectedFile,
  onSelectFile,
  onReferenceFile,
  onCreateItem,
  onDeleteItem,
  onRenameItem,
  onCopyItem,
  onMoveItem,
  chapterStats = {},
}: FileTreeProps) {
  // 内联编辑状态（新建 / 重命名）
  const [inlineEdit, setInlineEdit] = useState<InlineEditState | null>(null)
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(() => new Set())
  const [lastSelectedPath, setLastSelectedPath] = useState('')
  const [dragPaths, setDragPaths] = useState<string[]>([])
  const [dragOverPath, setDragOverPath] = useState('')
  const orderedPaths = useMemo(() => collectFileNodePaths(nodes, basePath), [nodes, basePath])

  // 弹窗操作（仅复制 / 移动）
  const [operation, setOperation] = useState<{
    open: boolean
    mode: FileOperationMode
    targetPath: string
    defaultValue: string
    paths: string[]
    batch: boolean
  }>({ open: false, mode: 'copy', targetPath: '', defaultValue: '', paths: [], batch: false })
  const [deleteTarget, setDeleteTarget] = useState<string | string[] | null>(null)

  const selectedPathList = useMemo(() => Array.from(selectedPaths), [selectedPaths])

  const clearSelection = () => setSelectedPaths(new Set())

  const updateSelection = (path: string, event?: MouseEvent) => {
    setSelectedPaths(prev => {
      const next = new Set(prev)
      if (event?.shiftKey && lastSelectedPath) {
        const start = orderedPaths.indexOf(lastSelectedPath)
        const end = orderedPaths.indexOf(path)
        if (start >= 0 && end >= 0) {
          const [from, to] = start < end ? [start, end] : [end, start]
          orderedPaths.slice(from, to + 1).forEach(item => next.add(item))
          return next
        }
      }
      if (event?.metaKey || event?.ctrlKey || event?.shiftKey) {
        if (next.has(path)) next.delete(path)
        else next.add(path)
        return next
      }
      next.clear()
      next.add(path)
      return next
    })
    setLastSelectedPath(path)
  }

  const selectForContextMenu = (path: string) => {
    if (selectedPaths.has(path)) return
    setSelectedPaths(new Set([path]))
    setLastSelectedPath(path)
  }

  const pathsForAction = (path?: string) => {
    if (!path) return selectedPathList
    return selectedPaths.has(path) ? selectedPathList : [path]
  }

  const runBatchOperation = async (mode: 'copy' | 'move', sources: string[], targetDir: string) => {
    const cleanTargetDir = targetDir.replace(/\/+$/, '')
    for (const source of sources) {
      if (!cleanTargetDir || cleanTargetDir === source || cleanTargetDir.startsWith(`${source}/`)) {
        continue
      }
      const target = joinPath(cleanTargetDir, getBaseName(source))
      if (mode === 'copy') await onCopyItem?.(source, target)
      else await onMoveItem?.(source, target)
    }
    if (mode === 'move') clearSelection()
  }

  const handleDropToDir = async (targetDir: string, event: DragEvent) => {
    event.preventDefault()
    event.stopPropagation()
    const paths = dragPaths.length > 0 ? dragPaths : selectedPathList
    const mode = event.altKey ? 'copy' : 'move'
    setDragOverPath('')
    setDragPaths([])
    if (paths.length === 0) return
    try {
      await runBatchOperation(mode, paths, targetDir)
    } catch (e) {
      console.warn(`${mode === 'copy' ? '复制' : '移动'}文件失败`, e)
    }
  }

  /** 开始内联编辑 */
  const startInlineEdit = (type: InlineEditState['type'], parentPath: string, defaultValue: string, renamePath?: string) => {
    setInlineEdit({ parentPath, type, defaultValue, renamePath })
  }

  /** 内联编辑确认 */
  const confirmInlineEdit = async (value: string) => {
    if (!inlineEdit) return
    try {
      switch (inlineEdit.type) {
        case 'create-file': {
          const fullPath = joinPath(inlineEdit.parentPath, value)
          await onCreateItem?.(fullPath, 'file')
          break
        }
        case 'create-dir': {
          const fullPath = joinPath(inlineEdit.parentPath, value)
          await onCreateItem?.(fullPath, 'dir')
          break
        }
        case 'rename':
          if (inlineEdit.renamePath) {
            await onRenameItem?.(inlineEdit.renamePath, value)
          }
          break
      }
    } catch {
      // 忽略错误，由后端 toast 提示
    }
    setInlineEdit(null)
  }

  /** 弹窗操作提交（复制 / 移动） */
  const submitOperation = async (value: string) => {
    if (operation.batch) {
      await runBatchOperation(operation.mode === 'copy' ? 'copy' : 'move', operation.paths, value)
      return
    }
    switch (operation.mode) {
      case 'copy':
        await onCopyItem?.(operation.targetPath, value)
        break
      case 'move':
        await onMoveItem?.(operation.targetPath, value)
        break
    }
  }

  return (
    <>
      <FileTreeList
        nodes={nodes}
        basePath={basePath}
        selectedFile={selectedFile}
        selectedPaths={selectedPaths}
        dragPaths={dragPaths}
        dragOverPath={dragOverPath}
        onSelectFile={onSelectFile}
        onSelectPath={updateSelection}
        onContextSelectPath={selectForContextMenu}
        onReferenceFile={onReferenceFile}
        onStartInlineEdit={startInlineEdit}
        inlineEdit={inlineEdit}
        onInlineConfirm={confirmInlineEdit}
        onInlineCancel={() => setInlineEdit(null)}
        onOpenOperation={(mode, targetPath, defaultValue, paths = [targetPath], batch = false) =>
          setOperation({ open: true, mode, targetPath, defaultValue, paths, batch })
        }
        onDeleteTarget={setDeleteTarget}
        onDragStartPaths={(paths) => {
          setDragPaths(paths)
          setSelectedPaths(new Set(paths))
        }}
        onDragEnd={() => {
          setDragPaths([])
          setDragOverPath('')
        }}
        onDragOverPath={setDragOverPath}
        onDropToDir={handleDropToDir}
        getActionPaths={pathsForAction}
        chapterStats={chapterStats}
      />
      <FileOperationDialog
        open={operation.open}
        mode={operation.mode}
        targetPath={operation.targetPath}
        defaultValue={operation.defaultValue}
        onOpenChange={(open) => setOperation(prev => ({ ...prev, open }))}
        onSubmit={submitOperation}
      />
      <DeleteConfirmDialog
        open={deleteTarget !== null}
        path={deleteTarget || ''}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        onConfirm={async () => {
          if (deleteTarget) {
            const targets = Array.isArray(deleteTarget) ? deleteTarget : [deleteTarget]
            for (const target of targets) {
              await onDeleteItem?.(target)
            }
            clearSelection()
          }
        }}
      />
    </>
  )
}

interface FileTreeListProps {
  nodes: FileNode[]
  basePath: string
  selectedFile: string | null
  selectedPaths: Set<string>
  dragPaths: string[]
  dragOverPath: string
  onSelectFile: (path: string) => void
  onSelectPath: (path: string, event?: MouseEvent) => void
  onContextSelectPath: (path: string) => void
  onReferenceFile?: (path: string) => void
  onStartInlineEdit: (type: InlineEditState['type'], parentPath: string, defaultValue: string, renamePath?: string) => void
  inlineEdit: InlineEditState | null
  onInlineConfirm: (value: string) => void
  onInlineCancel: () => void
  onOpenOperation: (mode: FileOperationMode, targetPath: string, defaultValue: string, paths?: string[], batch?: boolean) => void
  onDeleteTarget: (path: string | string[]) => void
  onDragStartPaths: (paths: string[]) => void
  onDragEnd: () => void
  onDragOverPath: (path: string) => void
  onDropToDir: (targetDir: string, event: DragEvent) => Promise<void>
  getActionPaths: (path?: string) => string[]
  chapterStats: Record<string, ChapterSummary>
}

/** 递归树列表 */
function FileTreeList(props: FileTreeListProps) {
  const sortedNodes = sortFileNodesForDisplay(props.nodes)
  const { inlineEdit, basePath, onInlineConfirm, onInlineCancel } = props

  // 判断当前层级是否有内联新建的 placeholder
  const showInlineCreate = inlineEdit &&
    (inlineEdit.type === 'create-file' || inlineEdit.type === 'create-dir') &&
    inlineEdit.parentPath === basePath

  return (
    <ul className="select-none text-xs">
      {showInlineCreate && (
        <li className="flex items-center gap-1 px-2 py-0.5">
          <span className="w-3.5 shrink-0" />
          {inlineEdit!.type === 'create-dir'
            ? <Folder className="h-4 w-4 shrink-0 text-[var(--nova-tree-folder)]" />
            : <FileText className="h-4 w-4 shrink-0 text-[var(--nova-tree-icon)]" />
          }
          <InlineInput
            defaultValue={inlineEdit!.defaultValue}
            onConfirm={onInlineConfirm}
            onCancel={onInlineCancel}
          />
        </li>
      )}
      {sortedNodes.map((node) => {
        const fullPath = basePath ? `${basePath}/${node.name}` : node.name
        return <FileTreeNode key={fullPath} node={node} path={fullPath} {...props} />
      })}
    </ul>
  )
}

interface FileTreeNodeProps {
  node: FileNode
  path: string
  basePath: string
  nodes: FileNode[]
  selectedFile: string | null
  selectedPaths: Set<string>
  dragPaths: string[]
  dragOverPath: string
  onSelectFile: (path: string) => void
  onSelectPath: (path: string, event?: MouseEvent) => void
  onContextSelectPath: (path: string) => void
  onReferenceFile?: (path: string) => void
  onStartInlineEdit: (type: InlineEditState['type'], parentPath: string, defaultValue: string, renamePath?: string) => void
  inlineEdit: InlineEditState | null
  onInlineConfirm: (value: string) => void
  onInlineCancel: () => void
  onOpenOperation: (mode: FileOperationMode, targetPath: string, defaultValue: string, paths?: string[], batch?: boolean) => void
  onDeleteTarget: (path: string | string[]) => void
  onDragStartPaths: (paths: string[]) => void
  onDragEnd: () => void
  onDragOverPath: (path: string) => void
  onDropToDir: (targetDir: string, event: DragEvent) => Promise<void>
  getActionPaths: (path?: string) => string[]
  chapterStats: Record<string, ChapterSummary>
}

interface TreeAction {
  label?: string
  icon?: ReactNode
  danger?: boolean
  separator?: boolean
  onSelect?: () => void
}

/** 单个目录树节点 */
function FileTreeNode({
  node,
  path,
  selectedFile,
  selectedPaths,
  dragPaths,
  dragOverPath,
  onSelectFile,
  onSelectPath,
  onContextSelectPath,
  onReferenceFile,
  onStartInlineEdit,
  inlineEdit,
  onInlineConfirm,
  onInlineCancel,
  onOpenOperation,
  onDeleteTarget,
  onDragStartPaths,
  onDragEnd,
  onDragOverPath,
  onDropToDir,
  getActionPaths,
  chapterStats,
}: FileTreeNodeProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(DEFAULT_EXPANDED.has(node.name))
  const isDir = node.type === 'dir'
  const isSelected = selectedFile === path
  const isMultiSelected = selectedPaths.has(path)
  const isDragOver = isDir && dragOverPath === path && dragPaths.some(source => source !== path && !path.startsWith(`${source}/`))
  const parentPath = getParentPath(path)
  const defaultCopyPath = `${path}-copy`
  const chapter = chapterStats[path]
  const actionPaths = getActionPaths(path)
  const isBatchAction = actionPaths.length > 1

  // 是否正在重命名此节点
  const isRenaming = inlineEdit?.type === 'rename' && inlineEdit.renamePath === path

  const createTargetDir = isDir ? path : parentPath
  const startDrag = (event: DragEvent) => {
    if (isRenaming) return
    const paths = selectedPaths.has(path) ? Array.from(selectedPaths) : [path]
    event.dataTransfer.effectAllowed = 'copyMove'
    event.dataTransfer.setData('text/plain', paths.join('\n'))
    onDragStartPaths(paths)
  }

  const actions: TreeAction[] = [
    ...(!isDir && !isBatchAction
      ? [
          {
            label: t('sidebar.referenceToChat'),
            icon: <AtSign className="h-3.5 w-3.5" />,
            onSelect: () => onReferenceFile?.(path),
          },
          { separator: true },
        ]
      : []),
    {
      label: t('sidebar.createFile'),
      icon: <FilePlus className="h-3.5 w-3.5" />,
      onSelect: () => {
        if (isDir) setExpanded(true)
        onStartInlineEdit('create-file', createTargetDir, '')
      },
    },
    {
      label: t('sidebar.createDir'),
      icon: <FolderPlus className="h-3.5 w-3.5" />,
      onSelect: () => {
        if (isDir) setExpanded(true)
        onStartInlineEdit('create-dir', createTargetDir, '')
      },
    },
    { separator: true },
    {
      label: isBatchAction ? undefined : t('sidebar.rename'),
      icon: <Pencil className="h-3.5 w-3.5" />,
      onSelect: () => onStartInlineEdit('rename', parentPath, node.name, path),
    },
    {
      label: isBatchAction ? t('sidebar.copySelected', { count: actionPaths.length }) : t('sidebar.copy'),
      icon: <Copy className="h-3.5 w-3.5" />,
      onSelect: () => onOpenOperation('copy', isBatchAction ? t('common.items', { count: actionPaths.length }) : path, isBatchAction ? parentPath : defaultCopyPath, actionPaths, isBatchAction),
    },
    {
      label: isBatchAction ? t('sidebar.moveSelected', { count: actionPaths.length }) : t('sidebar.move'),
      icon: <MoveRight className="h-3.5 w-3.5" />,
      onSelect: () => onOpenOperation('move', isBatchAction ? t('common.items', { count: actionPaths.length }) : path, isBatchAction ? parentPath : path, actionPaths, isBatchAction),
    },
    { separator: true },
    {
      label: isBatchAction ? t('sidebar.deleteSelected', { count: actionPaths.length }) : t('sidebar.delete'),
      icon: <Trash2 className="h-3.5 w-3.5" />,
      danger: true,
      onSelect: () => onDeleteTarget(isBatchAction ? actionPaths : path),
    },
  ].filter(action => action.separator || action.label) as TreeAction[]

  if (isDir) {
    return (
      <li>
        <ContextMenu>
          <ContextMenuTrigger asChild>
            <div
              className={`group flex w-full items-center rounded ${
                isMultiSelected
                  ? 'bg-[var(--nova-selection-bg)] text-[var(--nova-text)]'
                  : isDragOver
                    ? 'bg-[var(--nova-drop-bg)] text-[var(--nova-text)]'
                    : 'text-[var(--nova-tree-text)] hover:bg-[var(--nova-hover)]'
              }`}
              draggable={!isRenaming}
              onContextMenu={() => onContextSelectPath(path)}
              onDragStart={startDrag}
              onDragEnd={onDragEnd}
              onDragOver={(event) => {
                if (!isDir) return
                event.preventDefault()
                event.dataTransfer.dropEffect = event.altKey ? 'copy' : 'move'
                onDragOverPath(path)
              }}
              onDragLeave={() => {
                if (dragOverPath === path) onDragOverPath('')
              }}
              onDrop={(event) => void onDropToDir(path, event)}
            >
              <button
                type="button"
                className="flex min-w-0 flex-1 items-center gap-1 px-2 py-1 text-left max-md:min-h-[36px] max-md:py-1.5"
                onClick={(event) => {
                  if (event.metaKey || event.ctrlKey || event.shiftKey) {
                    onSelectPath(path, event)
                    return
                  }
                  onSelectPath(path)
                  setExpanded(!expanded)
                }}
              >
                {expanded ? (
                  <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-tree-chevron)]" />
                ) : (
                  <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-tree-chevron)]" />
                )}
                {expanded ? (
                  <FolderOpen className="h-4 w-4 shrink-0 text-[var(--nova-tree-folder)]" />
                ) : (
                  <Folder className="h-4 w-4 shrink-0 text-[var(--nova-tree-folder)]" />
                )}
                {isRenaming ? (
                  <InlineInput
                    defaultValue={inlineEdit!.defaultValue}
                    isRename
                    onConfirm={onInlineConfirm}
                    onCancel={onInlineCancel}
                  />
                ) : (
                  <span className="truncate">{node.name}</span>
                )}
              </button>
              {!isRenaming && <NodeDropdown actions={actions} />}
            </div>
          </ContextMenuTrigger>
          <ContextMenuContent className={MENU_CONTENT_CLASS}>
            {renderActionMenu(actions, 'context')}
          </ContextMenuContent>
        </ContextMenu>
        {expanded && (
          <div className="ml-3">
            <FileTreeList
              nodes={node.children ?? []}
              basePath={path}
              selectedFile={selectedFile}
              selectedPaths={selectedPaths}
              dragPaths={dragPaths}
              dragOverPath={dragOverPath}
              onSelectFile={onSelectFile}
              onSelectPath={onSelectPath}
              onContextSelectPath={onContextSelectPath}
              onReferenceFile={onReferenceFile}
              onStartInlineEdit={onStartInlineEdit}
              inlineEdit={inlineEdit}
              onInlineConfirm={onInlineConfirm}
              onInlineCancel={onInlineCancel}
              onOpenOperation={onOpenOperation}
              onDeleteTarget={onDeleteTarget}
              onDragStartPaths={onDragStartPaths}
              onDragEnd={onDragEnd}
              onDragOverPath={onDragOverPath}
              onDropToDir={onDropToDir}
              getActionPaths={getActionPaths}
              chapterStats={chapterStats}
            />
          </div>
        )}
      </li>
    )
  }

  return (
    <li>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div
            className={`group flex w-full items-center rounded ${
              isSelected || isMultiSelected
                ? 'bg-[var(--nova-selection-bg)] text-[var(--nova-text)]'
                : 'text-[var(--nova-tree-text)] hover:bg-[var(--nova-hover)]'
            }`}
            draggable={!isRenaming}
            onContextMenu={() => onContextSelectPath(path)}
            onDragStart={startDrag}
            onDragEnd={onDragEnd}
          >
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-1 px-2 py-1 text-left"
              onClick={(event) => {
                if (isRenaming) return
                if (event.metaKey || event.ctrlKey || event.shiftKey) {
                  onSelectPath(path, event)
                  return
                }
                onSelectPath(path)
                onSelectFile(path)
              }}
            >
              <span className="w-3.5 shrink-0" />
              <FileText className="h-4 w-4 shrink-0 text-[var(--nova-tree-icon)]" />
              {isRenaming ? (
                <InlineInput
                  defaultValue={inlineEdit!.defaultValue}
                  isRename
                  onConfirm={onInlineConfirm}
                  onCancel={onInlineCancel}
                />
              ) : (
                <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
                  <span className="truncate">{node.name}</span>
                  {chapter && (
                    <span className="flex shrink-0 items-center gap-1 text-[10px] text-[var(--nova-text-faint)]">
                      <span>{formatCompactWords(chapter.words)}</span>
                      <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1 text-[var(--nova-text-muted)]">{chapter.status}</span>
                    </span>
                  )}
                </span>
              )}
            </button>
            {!isRenaming && <NodeDropdown actions={actions} />}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent className={MENU_CONTENT_CLASS}>
          {renderActionMenu(actions, 'context')}
        </ContextMenuContent>
      </ContextMenu>
    </li>
  )
}

function NodeDropdown({ actions }: { actions: TreeAction[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="mr-1 hidden rounded p-0.5 text-[var(--nova-tree-icon)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] group-hover:block max-md:block max-md:p-1.5"
          onClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
          }}
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className={MENU_CONTENT_CLASS} align="end" sideOffset={6}>
        {renderActionMenu(actions, 'dropdown')}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function renderActionMenu(actions: TreeAction[], type: 'context' | 'dropdown') {
  return actions.map((action, index) => {
    if (action.separator) {
      return type === 'context'
        ? <ContextMenuSeparator key={index} className={MENU_SEPARATOR_CLASS} />
        : <DropdownMenuSeparator key={index} className={MENU_SEPARATOR_CLASS} />
    }
    const className = `${MENU_ITEM_CLASS} ${action.danger ? MENU_DANGER_CLASS : ''}`
    if (type === 'context') {
      return (
        <ContextMenuItem key={index} className={className} onSelect={action.onSelect}>
          {action.icon}
          {action.label}
        </ContextMenuItem>
      )
    }
    return (
      <DropdownMenuItem key={index} className={className} onSelect={action.onSelect}>
        {action.icon}
        {action.label}
      </DropdownMenuItem>
    )
  })
}

function getParentPath(path: string) {
  const idx = path.lastIndexOf('/')
  return idx >= 0 ? path.slice(0, idx) : ''
}

function getBaseName(path: string) {
  const idx = path.lastIndexOf('/')
  return idx >= 0 ? path.slice(idx + 1) : path
}

function joinPath(parent: string, name: string) {
  return parent ? `${parent}/${name}` : name
}

function collectFileNodePaths(nodes: FileNode[], basePath = ''): string[] {
  const paths: string[] = []
  for (const node of sortFileNodesForDisplay(nodes)) {
    const path = basePath ? `${basePath}/${node.name}` : node.name
    paths.push(path)
    if (node.type === 'dir' && node.children?.length) {
      paths.push(...collectFileNodePaths(node.children, path))
    }
  }
  return paths
}

function sortFileNodesForDisplay(nodes: FileNode[]) {
  return [...nodes].sort((a, b) => {
    if (a.type !== b.type) {
      return a.type === 'dir' ? -1 : 1
    }
    return compareFileNodeNames(a.name, b.name)
  })
}

function compareFileNodeNames(left: string, right: string) {
  const chapterCompare = compareChapterLikeNames(left, right)
  if (chapterCompare !== 0) {
    return chapterCompare
  }
  return left.localeCompare(right, 'zh-Hans-CN')
}

function compareChapterLikeNames(left: string, right: string) {
  const leftKey = chapterSortKey(left)
  const rightKey = chapterSortKey(right)
  if (leftKey.ok && rightKey.ok) {
    return leftKey.order - rightKey.order
  }
  if (leftKey.ok) return -1
  if (rightKey.ok) return 1
  return 0
}

function stripHiddenSortPrefix(baseName: string) {
  return baseName
    .replace(/^ch\d{5}[-_ ]+/i, '')
    .replace(/^v\d{5}[-_ ]+/i, '')
}

function chapterSortKey(name: string) {
  const baseName = name.replace(/\.[^.]+$/, '')
  const hiddenMatch = /^(?:ch|v)(\d{5})[-_ ]+/i.exec(baseName)
  if (hiddenMatch) {
    return { ok: true, order: Number.parseInt(hiddenMatch[1], 10) }
  }
  const visibleBaseName = stripHiddenSortPrefix(baseName)
  if (/^(序章|序幕|楔子|引子|前言|正文)(?:[-_ 、.．].*)?$/.test(visibleBaseName)) {
    return { ok: true, order: 0 }
  }

  const chapterMatch =
    /^ch(\d+)[-_ ]*/i.exec(visibleBaseName) ||
    /^第([0-9零〇一二三四五六七八九十百千万两]+)[章节回集卷部][-_ 、.．]*/.exec(visibleBaseName) ||
    /^(?:chapter|ch)[-_ ]*([0-9ivxlcdm]+)[-_ .:：]*/i.exec(visibleBaseName) ||
    /^(\d{1,6})[-_ 、.．]+/.exec(visibleBaseName)
  if (!chapterMatch) {
    return { ok: false, order: 0 }
  }
  const order = parseChapterOrdinal(chapterMatch[1])
  return { ok: order > 0, order }
}

function parseChapterOrdinal(value: string) {
  if (/^\d+$/.test(value)) {
    return Number.parseInt(value, 10)
  }
  const roman = parseRomanNumeral(value)
  if (roman > 0) {
    return roman
  }
  return parseChineseNumber(value)
}

function parseRomanNumeral(value: string) {
  const values: Record<string, number> = { I: 1, V: 5, X: 10, L: 50, C: 100, D: 500, M: 1000 }
  let total = 0
  let prev = 0
  for (const ch of value.toUpperCase().split('').reverse()) {
    const current = values[ch] || 0
    if (current === 0) return 0
    if (current < prev) {
      total -= current
    } else {
      total += current
      prev = current
    }
  }
  return total
}

function parseChineseNumber(value: string) {
  const digits: Record<string, number> = { 零: 0, 〇: 0, 一: 1, 二: 2, 两: 2, 三: 3, 四: 4, 五: 5, 六: 6, 七: 7, 八: 8, 九: 9 }
  const units: Record<string, number> = { 十: 10, 百: 100, 千: 1000, 万: 10000 }
  let total = 0
  let section = 0
  let number = 0
  let seen = false

  for (const ch of value) {
    if (Object.prototype.hasOwnProperty.call(digits, ch)) {
      number = digits[ch]
      seen = true
      continue
    }
    const unit = units[ch]
    if (!unit) {
      return 0
    }
    seen = true
    if (unit === 10000) {
      if (number !== 0) section += number
      if (section === 0) section = 1
      total += section * unit
      section = 0
      number = 0
      continue
    }
    if (number === 0) number = 1
    section += number * unit
    number = 0
  }
  if (!seen) {
    return 0
  }
  return total + section + number
}

function formatCompactWords(words: number) {
  if (words >= 10000) return `${(words / 10000).toFixed(1)}w`
  if (words >= 1000) return `${(words / 1000).toFixed(1)}k`
  return String(words)
}
