import { useState, type ReactNode } from 'react'
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
  'min-w-[180px] rounded-lg border-[#303238] bg-[#202124]/95 p-1 text-[#d7dbe2] shadow-[0_12px_32px_rgba(0,0,0,0.45)] backdrop-blur'
const MENU_ITEM_CLASS =
  'cursor-pointer rounded-md px-2 py-1.5 text-xs text-[#c5c9d1] transition-colors focus:bg-[#4a4d54]/25 focus:text-[#f0f2f5] data-[highlighted]:bg-[#4a4d54]/25 data-[highlighted]:text-[#f0f2f5] [&_svg]:text-[#858b96] focus:[&_svg]:text-[#c5c9d1] data-[highlighted]:[&_svg]:text-[#c5c9d1]'
const MENU_DANGER_CLASS =
  'text-red-300 focus:bg-red-500/15 focus:text-red-200 data-[highlighted]:bg-red-500/15 data-[highlighted]:text-red-200 [&_svg]:text-red-300'
const MENU_SEPARATOR_CLASS = 'mx-1 my-1 h-px bg-[#303238]'

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

  // 弹窗操作（仅复制 / 移动）
  const [operation, setOperation] = useState<{
    open: boolean
    mode: FileOperationMode
    targetPath: string
    defaultValue: string
  }>({ open: false, mode: 'copy', targetPath: '', defaultValue: '' })
  const [deleteTarget, setDeleteTarget] = useState('')

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
        onSelectFile={onSelectFile}
        onReferenceFile={onReferenceFile}
        onStartInlineEdit={startInlineEdit}
        inlineEdit={inlineEdit}
        onInlineConfirm={confirmInlineEdit}
        onInlineCancel={() => setInlineEdit(null)}
        onOpenOperation={(mode, targetPath, defaultValue) =>
          setOperation({ open: true, mode, targetPath, defaultValue })
        }
        onDeleteTarget={setDeleteTarget}
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
        open={deleteTarget !== ''}
        path={deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget('')
        }}
        onConfirm={async () => {
          if (deleteTarget) {
            await onDeleteItem?.(deleteTarget)
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
  onSelectFile: (path: string) => void
  onReferenceFile?: (path: string) => void
  onStartInlineEdit: (type: InlineEditState['type'], parentPath: string, defaultValue: string, renamePath?: string) => void
  inlineEdit: InlineEditState | null
  onInlineConfirm: (value: string) => void
  onInlineCancel: () => void
  onOpenOperation: (mode: FileOperationMode, targetPath: string, defaultValue: string) => void
  onDeleteTarget: (path: string) => void
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
            ? <Folder className="h-4 w-4 shrink-0 text-[#a8adb7]" />
            : <FileText className="h-4 w-4 shrink-0 text-[#858b96]" />
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
  onSelectFile: (path: string) => void
  onReferenceFile?: (path: string) => void
  onStartInlineEdit: (type: InlineEditState['type'], parentPath: string, defaultValue: string, renamePath?: string) => void
  inlineEdit: InlineEditState | null
  onInlineConfirm: (value: string) => void
  onInlineCancel: () => void
  onOpenOperation: (mode: FileOperationMode, targetPath: string, defaultValue: string) => void
  onDeleteTarget: (path: string) => void
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
  onSelectFile,
  onReferenceFile,
  onStartInlineEdit,
  inlineEdit,
  onInlineConfirm,
  onInlineCancel,
  onOpenOperation,
  onDeleteTarget,
  chapterStats,
}: FileTreeNodeProps) {
  const [expanded, setExpanded] = useState(DEFAULT_EXPANDED.has(node.name))
  const isDir = node.type === 'dir'
  const isSelected = selectedFile === path
  const parentPath = getParentPath(path)
  const defaultCopyPath = `${path}-copy`
  const chapter = chapterStats[path]

  // 是否正在重命名此节点
  const isRenaming = inlineEdit?.type === 'rename' && inlineEdit.renamePath === path

  const createTargetDir = isDir ? path : parentPath

  const actions: TreeAction[] = [
    ...(!isDir
      ? [
          {
            label: '引用到 Chat',
            icon: <AtSign className="h-3.5 w-3.5" />,
            onSelect: () => onReferenceFile?.(path),
          },
          { separator: true },
        ]
      : []),
    {
      label: '新建文件',
      icon: <FilePlus className="h-3.5 w-3.5" />,
      onSelect: () => {
        if (isDir) setExpanded(true)
        onStartInlineEdit('create-file', createTargetDir, '')
      },
    },
    {
      label: '新建目录',
      icon: <FolderPlus className="h-3.5 w-3.5" />,
      onSelect: () => {
        if (isDir) setExpanded(true)
        onStartInlineEdit('create-dir', createTargetDir, '')
      },
    },
    { separator: true },
    {
      label: '重命名',
      icon: <Pencil className="h-3.5 w-3.5" />,
      onSelect: () => onStartInlineEdit('rename', parentPath, node.name, path),
    },
    {
      label: '复制',
      icon: <Copy className="h-3.5 w-3.5" />,
      onSelect: () => onOpenOperation('copy', path, defaultCopyPath),
    },
    {
      label: '移动',
      icon: <MoveRight className="h-3.5 w-3.5" />,
      onSelect: () => onOpenOperation('move', path, path),
    },
    { separator: true },
    {
      label: '删除',
      icon: <Trash2 className="h-3.5 w-3.5" />,
      danger: true,
      onSelect: () => onDeleteTarget(path),
    },
  ]

  if (isDir) {
    return (
      <li>
        <ContextMenu>
          <ContextMenuTrigger asChild>
            <div className="group flex w-full items-center rounded text-[#aeb4bf] hover:bg-[#2a2c31]">
              <button
                type="button"
                className="flex min-w-0 flex-1 items-center gap-1 px-2 py-1 text-left"
                onClick={() => setExpanded(!expanded)}
              >
                {expanded ? (
                  <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[#6f7682]" />
                ) : (
                  <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[#6f7682]" />
                )}
                {expanded ? (
                  <FolderOpen className="h-4 w-4 shrink-0 text-[#a8adb7]" />
                ) : (
                  <Folder className="h-4 w-4 shrink-0 text-[#a8adb7]" />
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
              onSelectFile={onSelectFile}
              onReferenceFile={onReferenceFile}
              onStartInlineEdit={onStartInlineEdit}
              inlineEdit={inlineEdit}
              onInlineConfirm={onInlineConfirm}
              onInlineCancel={onInlineCancel}
              onOpenOperation={onOpenOperation}
              onDeleteTarget={onDeleteTarget}
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
              isSelected
                ? 'bg-[#4a4d54]/25 text-[#f0f2f5]'
                : 'text-[#aeb4bf] hover:bg-[#2a2c31]'
            }`}
          >
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-1 px-2 py-1 text-left"
              onClick={() => !isRenaming && onSelectFile(path)}
            >
              <span className="w-3.5 shrink-0" />
              <FileText className="h-4 w-4 shrink-0 text-[#858b96]" />
              {isRenaming ? (
                <InlineInput
                  defaultValue={inlineEdit!.defaultValue}
                  isRename
                  onConfirm={onInlineConfirm}
                  onCancel={onInlineCancel}
                />
              ) : (
                <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
                  <span className="truncate">{chapter?.display_title || node.name}</span>
                  {chapter && (
                    <span className="flex shrink-0 items-center gap-1 text-[10px] text-[#7f8794]">
                      <span>{formatCompactWords(chapter.words)}</span>
                      <span className="rounded border border-[#3a4658] bg-[#1c2430] px-1 text-[#c5c9d1]">{chapter.status}</span>
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
          className="mr-1 hidden rounded p-0.5 text-[#858b96] hover:bg-[#3a3d44] group-hover:block"
          onClick={(e) => e.stopPropagation()}
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className={MENU_CONTENT_CLASS} align="start">
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

function joinPath(parent: string, name: string) {
  return parent ? `${parent}/${name}` : name
}

function sortFileNodesForDisplay(nodes: FileNode[]) {
  return [...nodes].sort((a, b) => {
    if (a.type !== b.type) {
      return a.type === 'dir' ? -1 : 1
    }
    return a.name.localeCompare(b.name, 'zh-Hans-CN')
  })
}

function formatCompactWords(words: number) {
  if (words >= 10000) return `${(words / 10000).toFixed(1)}w`
  if (words >= 1000) return `${(words / 1000).toFixed(1)}k`
  return String(words)
}
