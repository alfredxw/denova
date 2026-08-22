import type {
  DragPreviewProps,
  NodeApi,
  NodeRendererProps,
  RowRendererProps,
  TreeApi,
} from 'react-arborist'
import { Tree } from 'react-arborist'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentPropsWithRef,
  type ElementType,
  type ReactNode,
  type RefObject,
} from 'react'
import { cn } from '@/lib/utils'
import type { ProjectFileExplorerNode } from './model'

interface ProjectFileTreeViewProps extends Omit<ComponentPropsWithRef<'div'>, 'children' | 'onSelect' | 'onToggle'> {
  nodes: readonly ProjectFileExplorerNode[]
  treeRef: RefObject<TreeApi<ProjectFileExplorerNode> | null>
  selectedPath?: string | null
  expandedPaths?: readonly string[]
  ariaLabel: string
  rowHeight?: number
  indent?: number
  overscanCount?: number
  disableEdit?: boolean | ((node: ProjectFileExplorerNode) => boolean)
  disableDrag?: boolean | ((node: ProjectFileExplorerNode) => boolean)
  disableDrop?: boolean | ((args: {
    parentNode: NodeApi<ProjectFileExplorerNode>
    dragNodes: NodeApi<ProjectFileExplorerNode>[]
    index: number
  }) => boolean)
  onActivate?: (node: NodeApi<ProjectFileExplorerNode>) => void
  onToggle?: (id: string) => void
  onScrollOffsetChange?: (scrollOffset: number) => void
  onRename?: (args: { node: NodeApi<ProjectFileExplorerNode>; name: string }) => void | Promise<void>
  onMove?: (args: { dragNodes: NodeApi<ProjectFileExplorerNode>[]; parentId: string | null; index: number }) => void | Promise<void>
  renderNode: ElementType<NodeRendererProps<ProjectFileExplorerNode>>
  renderRow: ElementType<RowRendererProps<ProjectFileExplorerNode>>
  renderDragPreview?: ElementType<DragPreviewProps>
  renderTree?: (tree: ReactNode) => ReactNode
  overlay?: ReactNode
}

/**
 * Shared virtualized viewport for Project-shaped file trees.
 *
 * Consumers own their node chrome and actions, while sizing, selection, directory
 * expansion, density, and virtualization stay identical across Files and Review.
 */
export function ProjectFileTreeView({
  nodes,
  treeRef,
  selectedPath,
  expandedPaths = [],
  ariaLabel,
  rowHeight = 26,
  indent = 14,
  overscanCount = 12,
  disableEdit,
  disableDrag,
  disableDrop,
  onActivate,
  onToggle,
  onScrollOffsetChange,
  onRename,
  onMove,
  renderNode,
  renderRow,
  renderDragPreview,
  renderTree,
  overlay,
  ref: forwardedRef,
  className,
  ...hostProps
}: ProjectFileTreeViewProps) {
  const hostRef = useRef<HTMLDivElement>(null)
  // Radix `asChild` triggers inject a ref for menu behavior. Preserve it while keeping
  // the local host ref attached so ResizeObserver can size the virtualized tree.
  const composedHostRef = useCallback((node: HTMLDivElement | null) => {
    hostRef.current = node
    if (typeof forwardedRef === 'function') {
      const cleanup = forwardedRef(node)
      if (typeof cleanup === 'function') {
        return () => {
          hostRef.current = null
          cleanup()
        }
      }
    } else if (forwardedRef) {
      forwardedRef.current = node
    }
  }, [forwardedRef])
  const size = useElementSize(hostRef)
  const initialOpenState = useMemo(
    () => Object.fromEntries(expandedPaths.map((path) => [path, true])),
    [expandedPaths],
  )

  const tree = (
    <Tree<ProjectFileExplorerNode>
      ref={treeRef}
      data={nodes}
      idAccessor="id"
      childrenAccessor="children"
      width={Math.max(size.width, 1)}
      height={Math.max(size.height, 1)}
      rowHeight={rowHeight}
      indent={indent}
      overscanCount={overscanCount}
      openByDefault={false}
      selection={selectedPath ?? undefined}
      initialOpenState={initialOpenState}
      aria-label={ariaLabel}
      disableEdit={disableEdit}
      disableDrag={disableDrag}
      disableDrop={disableDrop}
      onActivate={onActivate}
      onToggle={onToggle}
      onScroll={onScrollOffsetChange ? ({ scrollOffset }) => onScrollOffsetChange(scrollOffset) : undefined}
      onRename={onRename}
      onMove={onMove}
      renderRow={renderRow}
      renderDragPreview={renderDragPreview}
    >
      {renderNode}
    </Tree>
  )

  return (
    <div
      ref={composedHostRef}
      className={cn('relative h-full min-h-0 min-w-0 overflow-hidden', className)}
      {...hostProps}
    >
      {renderTree ? renderTree(tree) : tree}
      {overlay}
    </div>
  )
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
