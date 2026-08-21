import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen, MoreHorizontal } from 'lucide-react'
import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ProjectFileTreeNodeName } from '@/features/project-explorer/ProjectFileTreeChrome'
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import { cn } from '@/lib/utils'

interface ProjectFileBreadcrumbProps {
  workspace: string
  nodes: readonly ProjectFileExplorerNode[]
  selectedPath: string | null
  loading: boolean
  onSelectFile: (path: string) => void | Promise<void>
  onDirectoryExpand: (path: string) => void | Promise<void>
  onLoadMore: (path: string) => void | Promise<void>
}

interface BreadcrumbSegment {
  key: string
  label: string
  parentPath: string | null
}

/** Compact path navigation with per-segment file browsing. */
export function ProjectFileBreadcrumb({
  workspace,
  nodes,
  selectedPath,
  loading,
  onSelectFile,
  onDirectoryExpand,
  onLoadMore,
}: ProjectFileBreadcrumbProps) {
  const { t } = useTranslation()
  const [openSegment, setOpenSegment] = useState<string | null>(null)
  const breadcrumbRef = useRef<HTMLElement>(null)
  const segments = useMemo<BreadcrumbSegment[]>(() => {
    const normalizedWorkspace = workspace.replace(/[\\/]+$/, '')
    const workspaceName = normalizedWorkspace.split(/[\\/]/).at(-1) || t('files.title')
    const pathParts = selectedPath?.split('/').filter(Boolean) ?? []
    return [
      { key: 'workspace', label: workspaceName, parentPath: null },
      ...pathParts.map((label, index) => ({
        key: pathParts.slice(0, index + 1).join('/'),
        label,
        parentPath: pathParts.slice(0, index).join('/'),
      })),
    ]
  }, [selectedPath, t, workspace])
  const selectedDirectories = useMemo(() => {
    const parts = selectedPath?.split('/').filter(Boolean) ?? []
    return parts.slice(0, -1).map((_, index) => parts.slice(0, index + 1).join('/'))
  }, [selectedPath])
  const directories = useMemo(() => {
    const index = new Map<string, ProjectFileExplorerNode>()
    const visit = (items: readonly ProjectFileExplorerNode[]) => {
      for (const item of items) {
        if (item.type === 'dir') index.set(item.path, item)
        if (item.children) visit(item.children)
      }
    }
    visit(nodes)
    return index
  }, [nodes])

  useEffect(() => {
    const breadcrumb = breadcrumbRef.current
    if (!breadcrumb) return
    const revealCurrentSegment = () => {
      breadcrumb.scrollLeft = breadcrumb.scrollWidth
    }
    revealCurrentSegment()
    const observer = new ResizeObserver(revealCurrentSegment)
    observer.observe(breadcrumb)
    return () => observer.disconnect()
  }, [selectedPath])

  return (
    <Breadcrumb
      ref={breadcrumbRef}
      aria-label={t('files.breadcrumb.label')}
      title={selectedPath ? `${workspace.replace(/[\\/]+$/, '')}/${selectedPath}` : workspace}
      className="nova-file-breadcrumb min-w-0 flex-1 overflow-x-auto"
    >
      <BreadcrumbList className="min-w-max flex-nowrap gap-0 text-xs">
        {segments.map((segment, index) => {
          const isCurrent = index === segments.length - 1
          const menuNodes = !segment.parentPath ? nodes : directories.get(segment.parentPath)?.children ?? []
          let defaultExpandedPath: string | null = null
          if (segment.key === 'workspace') {
            defaultExpandedPath = selectedDirectories[0] ?? null
          } else if (selectedDirectories.includes(segment.key)) {
            defaultExpandedPath = segment.key
          }
          const focusedChildName = defaultExpandedPath && selectedPath?.startsWith(`${defaultExpandedPath}/`)
            ? selectedPath.slice(defaultExpandedPath.length + 1).split('/')[0]
            : null
          const focusedPath = focusedChildName
            ? `${defaultExpandedPath}/${focusedChildName}`
            : selectedPath
          return (
            <Fragment key={segment.key}>
              {index > 0 ? (
                <BreadcrumbSeparator className="mx-0.5 shrink-0" />
              ) : null}
              <BreadcrumbItem className="min-w-0 shrink-0 gap-0">
                <Popover
                  open={openSegment === segment.key}
                  onOpenChange={(open) => setOpenSegment(open ? segment.key : null)}
                >
                  {isCurrent ? (
                    <PopoverTrigger asChild>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-current="page"
                        aria-label={t('files.breadcrumb.browse', { name: segment.label })}
                        className="min-w-0 justify-start px-1.5"
                      >
                        <BreadcrumbPage
                          role="presentation"
                          aria-current={undefined}
                          aria-disabled={undefined}
                          className="whitespace-nowrap font-medium"
                        >
                          {segment.label}
                        </BreadcrumbPage>
                      </Button>
                    </PopoverTrigger>
                  ) : (
                    <PopoverTrigger asChild>
                      <BreadcrumbLink asChild>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          aria-label={t('files.breadcrumb.browse', { name: segment.label })}
                          className="max-w-48 min-w-0 justify-start px-1.5"
                        >
                          <span className="truncate">{segment.label}</span>
                        </Button>
                      </BreadcrumbLink>
                    </PopoverTrigger>
                  )}
                  <PopoverContent
                    align="start"
                    sideOffset={6}
                    className="nova-panel w-[min(32rem,calc(100vw-1rem))] gap-0 border border-[var(--nova-border)] bg-[var(--nova-surface)] p-1.5 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
                  >
                    <div className="max-h-[min(60dvh,30rem)] overflow-y-auto overscroll-contain">
                      {loading && nodes.length === 0 ? (
                        <div className="px-3 py-5 text-center text-xs text-[var(--nova-text-faint)]">
                          {t('files.tree.loading')}
                        </div>
                      ) : menuNodes.length > 0 ? (
                        <ProjectBreadcrumbTree
                          nodes={menuNodes}
                          selectedPath={selectedPath}
                          defaultExpandedPath={defaultExpandedPath}
                          focusedPath={focusedPath}
                          onSelectFile={(path) => {
                            setOpenSegment(null)
                            return onSelectFile(path)
                          }}
                          onDirectoryExpand={onDirectoryExpand}
                          onLoadMore={onLoadMore}
                        />
                      ) : (
                        <div className="px-3 py-5 text-center text-xs text-[var(--nova-text-faint)]">
                          {t('files.breadcrumb.empty')}
                        </div>
                      )}
                    </div>
                  </PopoverContent>
                </Popover>
              </BreadcrumbItem>
            </Fragment>
          )
        })}
      </BreadcrumbList>
    </Breadcrumb>
  )
}

function ProjectBreadcrumbTree({
  nodes,
  selectedPath,
  defaultExpandedPath,
  focusedPath,
  onSelectFile,
  onDirectoryExpand,
  onLoadMore,
}: {
  nodes: readonly ProjectFileExplorerNode[]
  selectedPath: string | null
  defaultExpandedPath: string | null
  focusedPath: string | null
  onSelectFile: (path: string) => void | Promise<void>
  onDirectoryExpand: (path: string) => void | Promise<void>
  onLoadMore: (path: string) => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [expandedPaths, setExpandedPaths] = useState<ReadonlySet<string>>(() => (
    new Set(defaultExpandedPath ? [defaultExpandedPath] : [])
  ))
  const focusedNodeRef = useRef<HTMLLIElement | null>(null)

  useLayoutEffect(() => {
    const focusedNode = focusedNodeRef.current
    if (!focusedNode || typeof focusedNode.scrollIntoView !== 'function') return
    focusedNode.scrollIntoView({ block: 'center', inline: 'nearest' })
  }, [focusedPath, nodes])

  return (
    <ul role="tree" aria-label={t('files.breadcrumb.browser')} className="space-y-0.5">
      {nodes.map((node) => (
        <ProjectBreadcrumbTreeNode
          key={node.id}
          node={node}
          selectedPath={selectedPath}
          focusedPath={focusedPath}
          focusedNodeRef={focusedNodeRef}
          pinned={node.path === defaultExpandedPath}
          expandedPaths={expandedPaths}
          onExpandedPathsChange={setExpandedPaths}
          onSelectFile={onSelectFile}
          onDirectoryExpand={onDirectoryExpand}
          onLoadMore={onLoadMore}
        />
      ))}
    </ul>
  )
}

function ProjectBreadcrumbTreeNode({
  node,
  selectedPath,
  focusedPath,
  focusedNodeRef,
  pinned,
  expandedPaths,
  onExpandedPathsChange,
  onSelectFile,
  onDirectoryExpand,
  onLoadMore,
}: {
  node: ProjectFileExplorerNode
  selectedPath: string | null
  focusedPath: string | null
  focusedNodeRef: RefObject<HTMLLIElement | null>
  pinned: boolean
  expandedPaths: ReadonlySet<string>
  onExpandedPathsChange: (paths: ReadonlySet<string>) => void
  onSelectFile: (path: string) => void | Promise<void>
  onDirectoryExpand: (path: string) => void | Promise<void>
  onLoadMore: (path: string) => void | Promise<void>
}) {
  const { t } = useTranslation()
  if (node.type === 'more') {
    return (
      <li
        ref={node.path === focusedPath ? focusedNodeRef : undefined}
        role="treeitem"
        data-breadcrumb-current-location={node.path === focusedPath ? 'true' : undefined}
      >
        <button
          type="button"
          disabled={node.loading}
          onClick={() => {
            void Promise.resolve(onLoadMore(node.path)).catch((cause) => {
              console.error('[features/files/ProjectFileBreadcrumb.tsx] loading a breadcrumb directory page failed', { path: node.path, cause })
            })
          }}
          className="flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] disabled:opacity-50"
        >
          <MoreHorizontal className="size-4 shrink-0 text-[var(--nova-tree-icon)]" aria-hidden="true" />
          {t(node.loading ? 'files.tree.loadingMore' : 'files.tree.loadMore')}
        </button>
      </li>
    )
  }

  const directory = node.type === 'dir'
  const expanded = directory && expandedPaths.has(node.path)
  return (
    <li
      ref={node.path === focusedPath ? focusedNodeRef : undefined}
      role="treeitem"
      aria-expanded={directory ? expanded : undefined}
      aria-selected={!directory && selectedPath === node.path ? true : undefined}
      data-breadcrumb-current-location={node.path === focusedPath ? 'true' : undefined}
    >
      <button
        type="button"
        aria-label={directory
          ? t(expanded ? 'files.tree.collapseDirectory' : 'files.tree.expandDirectory', { name: node.name })
          : node.name}
        onClick={() => {
          if (!directory) {
            void Promise.resolve(onSelectFile(node.path))
            return
          }
          const next = new Set(expandedPaths)
          if (expanded) next.delete(node.path)
          else next.add(node.path)
          onExpandedPathsChange(next)
          if (!expanded && !node.loaded) {
            void Promise.resolve(onDirectoryExpand(node.path)).catch((cause) => {
              console.error('[features/files/ProjectFileBreadcrumb.tsx] expanding a breadcrumb directory failed', { path: node.path, cause })
            })
          }
        }}
        className={cn(
          'flex h-8 w-full min-w-0 items-center gap-1.5 rounded-md px-2 text-left text-xs outline-none transition-colors',
          'hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--nova-accent)]/45',
          pinned && 'sticky top-0 z-10 bg-[var(--nova-surface)]',
          selectedPath === node.path ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-tree-text)]',
        )}
      >
        {directory
          ? expanded
            ? <ChevronDown className="size-3.5 shrink-0 text-[var(--nova-tree-chevron)]" aria-hidden="true" />
            : <ChevronRight className="size-3.5 shrink-0 text-[var(--nova-tree-chevron)]" aria-hidden="true" />
          : <span className="size-3.5 shrink-0" aria-hidden="true" />}
        {directory
          ? expanded
            ? <FolderOpen className="size-4 shrink-0 text-[var(--nova-tree-folder)]" aria-hidden="true" />
            : <Folder className="size-4 shrink-0 text-[var(--nova-tree-folder)]" aria-hidden="true" />
          : <FileText className="size-4 shrink-0 text-[var(--nova-tree-icon)]" aria-hidden="true" />}
        <ProjectFileTreeNodeName name={node.name} type={node.type} />
      </button>
      {directory && expanded && node.children && node.children.length > 0 ? (
        <ul role="group" className="ml-4 border-l border-[var(--nova-border)] pl-1">
          {node.children.map((child) => (
            <ProjectBreadcrumbTreeNode
              key={child.id}
              node={child}
              selectedPath={selectedPath}
              focusedPath={focusedPath}
              focusedNodeRef={focusedNodeRef}
              pinned={false}
              expandedPaths={expandedPaths}
              onExpandedPathsChange={onExpandedPathsChange}
              onSelectFile={onSelectFile}
              onDirectoryExpand={onDirectoryExpand}
              onLoadMore={onLoadMore}
            />
          ))}
        </ul>
      ) : null}
    </li>
  )
}
