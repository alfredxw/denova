import type { DragPreviewProps } from 'react-arborist'
import { FileText, Folder } from 'lucide-react'
import { memo, useContext } from 'react'
import { ProjectExplorerRenderContext } from './ProjectExplorerNode'

const PREVIEW_WIDTH = 288
const PREVIEW_HEIGHT = 36
const PREVIEW_GAP = 12
const VIEWPORT_GAP = 8

/** Lightweight pointer-anchored preview; rendering a full tree row here makes drag updates expensive. */
export const ProjectExplorerDragPreview = memo(function ProjectExplorerDragPreview({
  mouse,
  id,
  dragIds,
  isDragging,
}: DragPreviewProps) {
  const context = useContext(ProjectExplorerRenderContext)
  const node = id ? context?.nodesById.get(id) : null
  if (!isDragging || !mouse || !node || node.type === 'more' || node.draft) return null

  const viewportWidth = typeof window === 'undefined' ? Number.POSITIVE_INFINITY : window.innerWidth
  const viewportHeight = typeof window === 'undefined' ? Number.POSITIVE_INFINITY : window.innerHeight
  const x = Math.max(VIEWPORT_GAP, Math.min(mouse.x + PREVIEW_GAP, viewportWidth - PREVIEW_WIDTH - VIEWPORT_GAP))
  const y = Math.max(VIEWPORT_GAP, Math.min(mouse.y + PREVIEW_GAP, viewportHeight - PREVIEW_HEIGHT - VIEWPORT_GAP))

  return (
    <div
      aria-hidden="true"
      data-testid="project-explorer-drag-preview"
      className="pointer-events-none fixed left-0 top-0 z-[100] flex h-8 w-72 max-w-[calc(100vw-1rem)] items-center gap-2 overflow-hidden rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-xs text-[var(--nova-text)] shadow-lg"
      style={{
        contain: 'layout paint style',
        transform: `translate3d(${x}px, ${y}px, 0)`,
        willChange: 'transform',
      }}
    >
      {node.type === 'dir'
        ? <Folder className="size-4 shrink-0 text-[var(--nova-tree-icon)]" />
        : <FileText className="size-4 shrink-0 text-[var(--nova-tree-icon)]" />}
      <span className="min-w-0 flex-1 truncate">{node.name}</span>
      {dragIds.length > 1 ? (
        <span className="flex min-w-5 shrink-0 items-center justify-center rounded-full bg-[var(--nova-active)] px-1 text-[10px] tabular-nums">
          {dragIds.length}
        </span>
      ) : null}
    </div>
  )
})
