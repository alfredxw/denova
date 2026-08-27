import type { FileTree, GitStatusEntry } from '@pierre/trees'
import { Copy, ExternalLink, Loader2, MoreHorizontal } from 'lucide-react'
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { FileTreeMenu, FileTreeMenuItem, FileTreeMenuSeparator } from '@/components/file-tree/FileTreeMenu'
import { NovaFileTree } from '@/components/file-tree/NovaFileTree'
import { applicationFileTreePath, canonicalFileTreePath, writeClipboardText } from '@/components/file-tree/paths'
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
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import { projectFileTreeProjection } from '@/features/project-explorer/model'
import { absoluteProjectPath } from '@/features/project-explorer/operations'

interface ProjectFileBreadcrumbProps {
  workspace: string
  nodes: readonly ProjectFileExplorerNode[]
  selectedPath: string | null
  loading: boolean
  gitStatus?: readonly GitStatusEntry[]
  onSelectFile: (path: string) => unknown
  onDirectoryExpand: (path: string) => void | Promise<void>
  onLoadMore: (path: string) => void | Promise<void>
}

interface BreadcrumbSegment {
  key: string
  label: string
  parentPath: string | null
}

interface ProjectFileSnapshotBreadcrumbProps {
  workspace: string
  nodes: readonly ProjectFileExplorerNode[]
  selectedPath: string
  onSelectFile: (path: string) => unknown
}

/** Interactive breadcrumb backed by a complete, already-resolved Project tree. */
export function ProjectFileSnapshotBreadcrumb({
  workspace,
  nodes,
  selectedPath,
  onSelectFile,
}: ProjectFileSnapshotBreadcrumbProps) {
  return (
    <ProjectFileBreadcrumb
      workspace={workspace}
      nodes={nodes}
      selectedPath={selectedPath}
      loading={false}
      onSelectFile={onSelectFile}
      onDirectoryExpand={ignoreResolvedTreeRequest}
      onLoadMore={ignoreResolvedTreeRequest}
    />
  )
}

/** Compact path navigation with per-segment file browsing. */
export function ProjectFileBreadcrumb({
  workspace,
  nodes,
  selectedPath,
  loading,
  gitStatus = [],
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
          } else if (isCurrent) {
            defaultExpandedPath = segment.parentPath
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
                    <div className="flex h-[min(60dvh,30rem)] min-h-40 flex-col overflow-hidden overscroll-contain">
                      {loading && nodes.length === 0 ? (
                        <div className="px-3 py-5 text-center text-xs text-[var(--nova-text-faint)]">
                          {t('files.tree.loading')}
                        </div>
                      ) : menuNodes.length > 0 ? (
                        <ProjectBreadcrumbTree
                          nodes={menuNodes}
                          workspace={workspace}
                          selectedPath={selectedPath}
                          defaultExpandedPath={defaultExpandedPath}
                          focusedPath={focusedPath}
                          onSelectFile={(path) => {
                            setOpenSegment(null)
                            return onSelectFile(path)
                          }}
                          onDirectoryExpand={onDirectoryExpand}
                          onLoadMore={onLoadMore}
                          gitStatus={gitStatus}
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
  workspace,
  selectedPath,
  defaultExpandedPath,
  focusedPath,
  onSelectFile,
  onDirectoryExpand,
  onLoadMore,
  gitStatus,
}: {
  nodes: readonly ProjectFileExplorerNode[]
  workspace: string
  selectedPath: string | null
  defaultExpandedPath: string | null
  focusedPath: string | null
  onSelectFile: (path: string) => unknown
  onDirectoryExpand: (path: string) => void | Promise<void>
  onLoadMore: (path: string) => void | Promise<void>
  gitStatus: readonly GitStatusEntry[]
}) {
  const { t } = useTranslation()
  const treeRef = useRef<FileTree | null>(null)
  const projection = useMemo(() => projectFileTreeProjection(nodes), [nodes])
  const initialExpandedPaths = useMemo(
    () => defaultExpandedPath ? [canonicalFileTreePath(defaultExpandedPath, true)] : [],
    [defaultExpandedPath],
  )
  const mergedGitStatus = useMemo(() => {
    const statuses = new Map(gitStatus.map((entry) => [entry.path, entry]))
    for (const node of projection.nodesByPath.values()) {
      if (!node.ignored) continue
      const path = canonicalFileTreePath(node.path, node.type === 'dir')
      statuses.set(path, { path, status: 'ignored' })
    }
    return [...statuses.values()]
  }, [gitStatus, projection.nodesByPath])
  const loadingMore = projection.incompletePaths.some((path) => findMoreNode(nodes, path)?.loading)
  const selectedPaths = useMemo(() => selectedPath ? [selectedPath] : [], [selectedPath])

  useEffect(() => {
    if (!focusedPath) return
    const node = projection.nodesByPath.get(focusedPath)
    const path = canonicalFileTreePath(focusedPath, node?.type === 'dir')
    if (treeRef.current?.getItem(path)) treeRef.current.scrollToPath(path, { focus: false, offset: 'center' })
  }, [focusedPath, projection.nodesByPath])

  return (
    <>
      <NovaFileTree
        ref={treeRef}
        paths={projection.paths}
        presorted
        ariaLabel={t('files.breadcrumb.browser')}
        searchLabel={t('files.tree.search')}
        initialExpansion="closed"
        initialExpandedPaths={initialExpandedPaths}
        selectedPaths={selectedPaths}
        gitStatus={mergedGitStatus}
        onSelectionChange={(paths) => {
          const path = applicationFileTreePath(paths.at(-1) ?? '')
          if (projection.nodesByPath.get(path)?.type === 'file') void Promise.resolve(onSelectFile(path))
        }}
        onDirectoryExpand={(path) => onDirectoryExpand(applicationFileTreePath(path))}
        renderContextMenu={(item, context) => {
          const path = applicationFileTreePath(item.path)
          const node = projection.nodesByPath.get(path)
          if (!node) return null
          const closeThen = (action: () => void) => {
            context.close()
            action()
          }
          return (
            <FileTreeMenu anchorRect={context.anchorRect}>
              {node.type === 'file' ? (
                <>
                  <FileTreeMenuItem onClick={() => closeThen(() => { void Promise.resolve(onSelectFile(path)) })}><ExternalLink />{t('changes.openFile')}</FileTreeMenuItem>
                  <FileTreeMenuSeparator />
                </>
              ) : null}
              <FileTreeMenuItem onClick={() => closeThen(() => copyBreadcrumbPath(absoluteProjectPath(workspace, path), path, t('files.tree.copyPathFailed')))}><Copy />{t('sidebar.copyPath')}</FileTreeMenuItem>
              <FileTreeMenuItem onClick={() => closeThen(() => copyBreadcrumbPath(path, path, t('files.tree.copyPathFailed')))}><Copy />{t('sidebar.copyRelativePath')}</FileTreeMenuItem>
            </FileTreeMenu>
          )
        }}
      />
      {projection.incompletePaths.length > 0 ? (
        <button
          type="button"
          disabled={loadingMore}
          className="mt-1 flex h-7 shrink-0 items-center justify-center gap-1 rounded text-[11px] text-[var(--nova-accent)] hover:bg-[var(--nova-hover)] disabled:opacity-50"
          onClick={() => {
            void Promise.resolve(onLoadMore(projection.incompletePaths[0])).catch((cause) => {
              console.error('[features/files/ProjectFileBreadcrumb.tsx] loading a breadcrumb directory page failed', { path: projection.incompletePaths[0], cause })
            })
          }}
        >
          {loadingMore ? <Loader2 className="size-3.5 animate-spin" /> : <MoreHorizontal className="size-3.5" />}
          {t(loadingMore ? 'files.tree.loadingMore' : 'files.tree.loadMore')}
        </button>
      ) : null}
    </>
  )
}

function ignoreResolvedTreeRequest() {}

function findMoreNode(nodes: readonly ProjectFileExplorerNode[], path: string): ProjectFileExplorerNode | null {
  for (const node of nodes) {
    if (node.type === 'more' && node.path === path) return node
    const child = node.children ? findMoreNode(node.children, path) : null
    if (child) return child
  }
  return null
}

function copyBreadcrumbPath(value: string, path: string, failureMessage: string) {
  void writeClipboardText(value).catch((cause) => {
    console.error('[features/files/ProjectFileBreadcrumb.tsx] copying a breadcrumb path failed', { path, cause })
    toast.error(failureMessage)
  })
}
