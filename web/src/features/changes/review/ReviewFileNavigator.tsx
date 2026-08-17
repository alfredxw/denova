import type { NodeApi, NodeRendererProps, TreeApi } from 'react-arborist'
import {
  AlertTriangle,
  Check,
  ChevronDown,
  ChevronRight,
  FileText,
  Folder,
  FolderOpen,
  Minus,
  PanelRightClose,
  Pencil,
  Plus,
  Search,
} from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { ProjectFileTreeNodeName, ProjectFileTreeRow } from '@/features/project-explorer/ProjectFileTreeChrome'
import { ProjectFileTreeView } from '@/features/project-explorer/ProjectFileTreeView'
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import { cn } from '@/lib/utils'
import type { ReviewThreadFile } from '../types'

interface ReviewFileNavigatorProps {
  files: ReviewThreadFile[]
  selectedPath: string
  onSelect: (path: string) => void
  onCollapse: () => void
}

type ReviewFileKind = 'added' | 'modified' | 'deleted'

const ReviewFileTreeContext = createContext<ReadonlyMap<string, ReviewThreadFile> | null>(null)

/** Compact read-only projection of the shared Project file tree for changed files only. */
export function ReviewFileNavigator({ files, selectedPath, onSelect, onCollapse }: ReviewFileNavigatorProps) {
  const { t } = useTranslation()
  const treeRef = useRef<TreeApi<ProjectFileExplorerNode> | null>(null)
  const [filter, setFilter] = useState('')
  const normalizedFilter = filter.trim().toLocaleLowerCase()
  const visibleFiles = useMemo(() => normalizedFilter
    ? files.filter((file) => file.path.toLocaleLowerCase().includes(normalizedFilter))
    : files, [files, normalizedFilter])
  const treeNodes = useMemo(() => buildReviewFileTree(visibleFiles), [visibleFiles])
  const fileIndex = useMemo(() => new Map(files.map((file) => [file.path, file])), [files])
  const directoryPaths = useMemo(() => collectDirectoryPaths(treeNodes), [treeNodes])
  const [expandedPaths, setExpandedPaths] = useState<ReadonlySet<string>>(() => new Set(directoryPaths))

  useEffect(() => {
    if (normalizedFilter) return
    setExpandedPaths((current) => {
      const next = new Set(directoryPaths)
      return next.size === current.size && [...next].every((path) => current.has(path)) ? current : next
    })
  }, [directoryPaths, normalizedFilter])

  const handleActivate = useCallback((node: NodeApi<ProjectFileExplorerNode>) => {
    if (node.data.type === 'dir') node.toggle()
    if (node.data.type === 'file') onSelect(node.data.path)
  }, [onSelect])

  const handleToggle = useCallback((id: string) => {
    const expanded = treeRef.current?.isOpen(id) === true
    setExpandedPaths((current) => {
      const next = new Set(current)
      if (expanded) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])

  return (
    <aside data-review-file-navigator className="nova-review-file-navigator h-full min-h-0 w-full border-l border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="nova-review-file-navigator-wide h-full min-h-0 flex-col">
        <div className="flex h-9 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-2.5">
          <span className="min-w-0 flex-1 truncate text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--nova-text-muted)]">
            {t('changes.changedFiles')}
          </span>
          <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[9px] tabular-nums text-[var(--nova-text-faint)]">
            {files.length}
          </span>
          <button
            type="button"
            onClick={onCollapse}
            aria-label={t('changes.hideFileNavigator')}
            className="nova-nav-item flex size-6 items-center justify-center"
          >
            <PanelRightClose className="size-3.5" />
          </button>
        </div>

        <label className="relative mx-2 my-1.5 block shrink-0">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
          <Input
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            aria-label={t('changes.filterFiles')}
            placeholder={t('changes.filterFiles')}
            className="h-7 rounded-md border-[var(--nova-border)] bg-[var(--nova-bg)] pl-7 pr-2 text-[11px] shadow-none"
          />
        </label>

        <div className="min-h-0 flex-1 px-1 pb-1.5">
          <ProjectFileTreeView
            key={normalizedFilter || 'all-files'}
            nodes={treeNodes}
            treeRef={treeRef}
            selectedPath={visibleFiles.some((file) => file.path === selectedPath) ? selectedPath : null}
            expandedPaths={normalizedFilter ? directoryPaths : [...expandedPaths]}
            ariaLabel={t('changes.fileNavigator')}
            rowHeight={24}
            indent={12}
            disableEdit
            disableDrag
            disableDrop
            onActivate={handleActivate}
            onToggle={handleToggle}
            renderNode={ReviewFileTreeNode}
            renderRow={ProjectFileTreeRow}
            renderTree={(tree) => (
              <ReviewFileTreeContext.Provider value={fileIndex}>
                {tree}
              </ReviewFileTreeContext.Provider>
            )}
            overlay={treeNodes.length === 0 ? (
              <div className="pointer-events-none absolute inset-x-3 top-8 text-center text-[11px] text-[var(--nova-text-faint)]">
                {t('changes.noMatchingFiles')}
              </div>
            ) : null}
          />
        </div>
      </div>

      <div className="nova-review-file-navigator-compact min-w-0 items-center gap-1 p-1.5">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" size="sm" variant="ghost" className="min-w-0 flex-1 justify-between font-normal">
              <span className="flex min-w-0 items-center gap-2">
                <span className="truncate">{t('changes.changedFiles')}</span>
                <span className="text-[var(--nova-text-faint)]">{files.length}</span>
              </span>
              <ChevronDown data-icon="inline-end" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-[min(60vh,28rem)] w-[min(28rem,calc(100vw-1.5rem))]">
            <DropdownMenuGroup>
              {files.map((file) => (
                <DropdownMenuCheckboxItem
                  key={file.path}
                  checked={file.path === selectedPath}
                  onSelect={() => onSelect(file.path)}
                  aria-label={t('changes.jumpToFile', { path: file.path })}
                  className="min-w-0 py-1.5"
                >
                  <ReviewFileItem file={file} fullPath />
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          onClick={onCollapse}
          aria-label={t('changes.hideFileNavigator')}
        >
          <PanelRightClose />
        </Button>
      </div>
    </aside>
  )
}

function ReviewFileTreeNode({ node, style }: NodeRendererProps<ProjectFileExplorerNode>) {
  const { t } = useTranslation()
  const files = useContext(ReviewFileTreeContext)
  if (!files) throw new Error('Review file tree context is unavailable')
  const data = node.data
  const file = files.get(data.path)

  if (data.type === 'dir') {
    return (
      <div style={style} className="flex h-full min-w-0 flex-1 items-center gap-1 pr-1 text-[11px] text-[var(--nova-text-muted)]">
        <span className="flex size-4 shrink-0 items-center justify-center text-[var(--nova-text-faint)]">
          {node.isOpen ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
        </span>
        {node.isOpen
          ? <FolderOpen className="size-3.5 shrink-0 text-[var(--nova-tree-icon)]" />
          : <Folder className="size-3.5 shrink-0 text-[var(--nova-tree-icon)]" />}
        <ProjectFileTreeNodeName name={data.name} type="dir" />
      </div>
    )
  }

  if (!file) return null
  return (
    <div
      style={style}
      aria-label={t('changes.jumpToFile', { path: file.path })}
      className="nova-review-tree-file flex h-full min-w-0 flex-1 items-center gap-1 pr-1 text-[11px]"
    >
      <span className="size-4 shrink-0" />
      <FileText className="size-3.5 shrink-0 text-[var(--nova-tree-icon)]" />
      <ProjectFileTreeNodeName name={data.name} type="file" />
      <ReviewFileStatus file={file} />
    </div>
  )
}

function ReviewFileItem({ file, fullPath = false }: { file: ReviewThreadFile; fullPath?: boolean }) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-1.5">
      <FileText className="size-3.5 shrink-0 text-[var(--nova-tree-icon)]" />
      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--nova-text)]">
        {fullPath ? file.path : file.path.split('/').at(-1)}
      </span>
      <ReviewFileStatus file={file} />
    </span>
  )
}

function ReviewFileStatus({ file }: { file: ReviewThreadFile }) {
  const { t } = useTranslation()
  const kind = reviewFileKind(file)
  const conflicted = file.continuity !== 'continuous' || file.apply_state === 'conflicted'
  const Icon = kind === 'added' ? Plus : kind === 'deleted' ? Minus : Pencil
  return (
    <span className="ml-auto flex shrink-0 items-center gap-1">
      {conflicted ? <AlertTriangle className="size-3 text-[var(--nova-warning)]" aria-label={t('changes.applyState.conflicted')} /> : null}
      {file.review_status === 'accepted' ? <Check className="size-3 text-[var(--nova-success)]" aria-label={t('changes.status.accepted')} /> : null}
      <span
        data-review-file-status={kind}
        aria-label={t(`changes.fileStatus.${kind}`)}
        title={t(`changes.fileStatus.${kind}`)}
        className={cn(
          'flex size-4 items-center justify-center rounded-[4px] border',
          kind === 'added' && 'border-[var(--nova-success)]/55 text-[var(--nova-success)]',
          kind === 'modified' && 'border-[var(--nova-warning)]/55 text-[var(--nova-warning)]',
          kind === 'deleted' && 'border-[var(--nova-danger-border)] text-[var(--nova-danger)]',
        )}
      >
        <Icon className="size-2.5" strokeWidth={2.4} />
      </span>
    </span>
  )
}

export function reviewFileKind(file: Pick<ReviewThreadFile, 'before_exists' | 'after_exists'>): ReviewFileKind {
  if (file.after_exists === false) return 'deleted'
  if (file.before_exists === false) return 'added'
  return 'modified'
}

export function buildReviewFileTree(files: readonly ReviewThreadFile[]): ProjectFileExplorerNode[] {
  const root: MutableReviewTreeNode = { path: '', name: '', type: 'dir', children: new Map() }
  for (const file of files) {
    const parts = file.path.split('/').filter(Boolean)
    let parent = root
    parts.forEach((name, index) => {
      const path = parts.slice(0, index + 1).join('/')
      const type = index === parts.length - 1 ? 'file' : 'dir'
      let child = parent.children.get(name)
      if (!child) {
        child = { path, name, type, children: new Map() }
        parent.children.set(name, child)
      }
      parent = child
    })
  }
  return materializeReviewTree(root)
}

interface MutableReviewTreeNode {
  path: string
  name: string
  type: 'file' | 'dir'
  children: Map<string, MutableReviewTreeNode>
}

function materializeReviewTree(parent: MutableReviewTreeNode): ProjectFileExplorerNode[] {
  return [...parent.children.values()]
    .sort((left, right) => left.type === right.type
      ? left.name.localeCompare(right.name)
      : left.type === 'dir' ? -1 : 1)
    .map((node) => ({
      id: node.path,
      path: node.path,
      name: node.name,
      type: node.type,
      ignored: false,
      symlink: false,
      loaded: true,
      loading: false,
      children: node.type === 'dir' ? materializeReviewTree(node) : undefined,
    }))
}

function collectDirectoryPaths(nodes: readonly ProjectFileExplorerNode[]): string[] {
  const paths: string[] = []
  const visit = (items: readonly ProjectFileExplorerNode[]) => {
    for (const node of items) {
      if (node.type !== 'dir') continue
      paths.push(node.path)
      if (node.children) visit(node.children)
    }
  }
  visit(nodes)
  return paths
}
