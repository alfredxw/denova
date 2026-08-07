import type { NodeApi, NodeRendererProps, RowRendererProps } from 'react-arborist'
import {
  ChevronDown,
  ChevronRight,
  ClipboardPaste,
  Copy,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Scissors,
  Trash2,
} from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import type { ProjectFileExplorerNode } from './model'
import type { ProjectFileClipboard } from './operations'
import type { ProjectExplorerExtensions } from './types'

export interface ProjectExplorerTreeActions {
  beginCreate: (parentPath: string, type: 'file' | 'dir') => void
  rename: (node: NodeApi<ProjectFileExplorerNode>) => void
  stageClipboard: (mode: ProjectFileClipboard['mode'], paths: string[]) => void
  paste: (parentPath: string) => void
  copyPath: (path: string, relative: boolean) => void
  delete: (paths: string[]) => void
  cancelDraft: () => void
  clipboard: ProjectFileClipboard | null
}

interface ProjectExplorerRenderContextValue {
  actions: ProjectExplorerTreeActions
  extensions: ProjectExplorerExtensions
  nodesById: ReadonlyMap<string, ProjectFileExplorerNode>
  onLoadMore: (path: string) => void | Promise<void>
}

export const ProjectExplorerRenderContext = createContext<ProjectExplorerRenderContextValue | null>(null)

export function ExplorerRow({ node, attrs, innerRef, children }: RowRendererProps<ProjectFileExplorerNode>) {
  return (
    <div
      {...attrs}
      style={{ ...attrs.style, minWidth: 0 }}
      ref={innerRef}
      onFocus={(event) => event.stopPropagation()}
      onClick={node.handleClick}
      className={cn(
        'group/tree-row flex min-w-0 cursor-default items-center overflow-hidden rounded-sm outline-none',
        node.isSelected && 'bg-[var(--nova-active)] text-[var(--nova-text)]',
        !node.isSelected && 'hover:bg-[var(--nova-hover)]',
        node.isFocused && 'ring-1 ring-inset ring-[var(--nova-accent)]',
        node.isDragging && 'opacity-40',
        node.willReceiveDrop && 'bg-[var(--nova-active)] ring-1 ring-inset ring-[var(--nova-accent)]',
      )}
    >
      {children}
    </div>
  )
}

export function ExplorerNode({ node, style, dragHandle }: NodeRendererProps<ProjectFileExplorerNode>) {
  const { t } = useTranslation()
  const renderContext = useContext(ProjectExplorerRenderContext)
  if (!renderContext) throw new Error('Project file explorer render context is unavailable')
  const { actions, extensions, onLoadMore } = renderContext
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
    ? node.tree.selectedNodes.map((selected) => selected.data).filter((item) => item.type !== 'more' && !item.draft).map((item) => item.path)
    : [data.path]
  const cut = actions.clipboard?.mode === 'cut'
    && actions.clipboard.paths.some((path) => data.path === path || data.path.startsWith(`${path}/`))
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          ref={dragHandle}
          style={style}
          className={cn(
            'flex h-full min-w-0 flex-1 items-center gap-1 overflow-hidden pr-1 text-xs',
            (data.ignored || cut) && 'opacity-55',
          )}
          onPointerDown={(event) => {
            if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || node.isSelected) return
            node.select()
          }}
          onContextMenu={() => {
            if (!node.isSelected) node.select()
            node.focus()
          }}
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
          {node.isEditing
            ? <RenameInput node={node} onCancelDraft={actions.cancelDraft} />
            : <ExplorerNodeName name={data.name} type={data.type} />}
          {!data.draft ? extensions.renderNodeMeta?.(data) : null}
          {data.symlink ? <span className="shrink-0 text-[9px] text-[var(--nova-text-faint)]">↗</span> : null}
          {!data.draft ? <NodeActionDropdown node={node} actionPaths={actionPaths} actions={actions} /> : null}
        </div>
      </ContextMenuTrigger>
      {!data.draft ? (
        <ContextMenuContent>
          <NodeActions node={node} actionPaths={actionPaths} actions={actions} extensions={extensions} kind="context" />
        </ContextMenuContent>
      ) : null}
    </ContextMenu>
  )
}

function ExplorerNodeName({ name, type }: { name: string; type: 'file' | 'dir' }) {
  if (type === 'dir') return <span className="min-w-0 flex-1 truncate">{name}</span>
  const extensionIndex = name.lastIndexOf('.')
  const hasExtension = extensionIndex > 0 && extensionIndex < name.length - 1
  const stem = hasExtension ? name.slice(0, extensionIndex) : name
  const extension = hasExtension ? name.slice(extensionIndex) : ''
  return (
    <span className="flex min-w-0 flex-1" aria-label={name}>
      <span className="truncate">{stem}</span>
      {extension ? <span className="shrink-0">{extension}</span> : null}
    </span>
  )
}

function RenameInput({ node, onCancelDraft }: {
  node: NodeApi<ProjectFileExplorerNode>
  onCancelDraft: () => void
}) {
  const ref = useRef<HTMLInputElement>(null)
  const phaseRef = useRef<'idle' | 'submitting' | 'cancelled'>('idle')
  const [submitting, setSubmitting] = useState(false)
  useEffect(() => {
    const input = ref.current
    if (!input) return
    input.focus()
    const dot = node.data.draft ? -1 : node.data.name.lastIndexOf('.')
    if (dot > 0 && node.data.type === 'file') input.setSelectionRange(0, dot)
    else input.select()
  }, [node.data.draft, node.data.name, node.data.type])

  const cancel = useCallback(() => {
    if (phaseRef.current !== 'idle') return
    phaseRef.current = 'cancelled'
    node.reset()
    if (node.data.draft) onCancelDraft()
  }, [node, onCancelDraft])
  const submit = useCallback(async () => {
    if (phaseRef.current !== 'idle') return
    const name = ref.current?.value.trim() ?? ''
    if (!name || (!node.data.draft && name === node.data.name)) {
      cancel()
      return
    }
    phaseRef.current = 'submitting'
    setSubmitting(true)
    try {
      await node.submit(name)
    } catch (cause) {
      console.error('[features/project-explorer/ProjectExplorerNode.tsx] inline file operation failed', {
        path: node.data.path,
        cause,
      })
      phaseRef.current = 'idle'
      setSubmitting(false)
      ref.current?.focus()
    }
  }, [cancel, node])

  return (
    <input
      ref={ref}
      defaultValue={node.data.name}
      disabled={submitting}
      enterKeyHint="done"
      className="h-5 min-w-0 flex-1 rounded-sm border border-[var(--nova-accent)] bg-[var(--nova-bg)] px-1 text-xs outline-none"
      onClick={(event) => event.stopPropagation()}
      onBlur={() => void submit()}
      onKeyDown={(event) => {
        event.stopPropagation()
        if (event.key === 'Escape') cancel()
        if (event.key === 'Enter') {
          event.preventDefault()
          void submit()
        }
      }}
    />
  )
}

function NodeActionDropdown({ node, actionPaths, actions }: {
  node: NodeApi<ProjectFileExplorerNode>
  actionPaths: string[]
  actions: ProjectExplorerTreeActions
}) {
  const { t } = useTranslation()
  const renderContext = useContext(ProjectExplorerRenderContext)
  if (!renderContext) throw new Error('Project file explorer render context is unavailable')
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
      <DropdownMenuContent align="end" className="min-w-44">
        <NodeActions node={node} actionPaths={actionPaths} actions={actions} extensions={renderContext.extensions} kind="dropdown" />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function NodeActions({ node, actionPaths, actions, extensions, kind }: {
  node: NodeApi<ProjectFileExplorerNode>
  actionPaths: string[]
  actions: ProjectExplorerTreeActions
  extensions: ProjectExplorerExtensions
  kind: 'context' | 'dropdown'
}) {
  const { t } = useTranslation()
  const Item = kind === 'context' ? ContextMenuItem : DropdownMenuItem
  const Separator = kind === 'context' ? ContextMenuSeparator : DropdownMenuSeparator
  const path = node.data.path
  const extensionActions = extensions.getNodeActions?.({ node: node.data, paths: actionPaths }) ?? []
  return (
    <>
      {extensionActions.map((action) => (
        <Item key={action.id} disabled={action.disabled} onSelect={action.onSelect}>
          {action.icon}
          {action.label}
        </Item>
      ))}
      {extensionActions.length > 0 ? <Separator /> : null}
      {node.data.type === 'dir' ? (
        <>
          <Item onSelect={() => actions.beginCreate(path, 'file')}><FileText />{t('sidebar.createFile')}</Item>
          <Item onSelect={() => actions.beginCreate(path, 'dir')}><FolderPlus />{t('sidebar.createDir')}</Item>
          <Separator />
        </>
      ) : null}
      <Item onSelect={() => actions.stageClipboard('cut', actionPaths)}><Scissors />{t('sidebar.cut')}</Item>
      <Item disabled={node.data.symlink} onSelect={() => actions.stageClipboard('copy', actionPaths)}><Copy />{t('sidebar.copy')}</Item>
      {node.data.type === 'dir' ? (
        <Item disabled={!actions.clipboard} onSelect={() => actions.paste(path)}><ClipboardPaste />{t('sidebar.paste')}</Item>
      ) : null}
      <Separator />
      <Item onSelect={() => actions.copyPath(path, false)}><Copy />{t('sidebar.copyPath')}</Item>
      <Item onSelect={() => actions.copyPath(path, true)}><Copy />{t('sidebar.copyRelativePath')}</Item>
      <Separator />
      <Item onSelect={() => actions.rename(node)}><Pencil />{t('sidebar.rename')}</Item>
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
