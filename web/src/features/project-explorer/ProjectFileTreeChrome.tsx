import type { RowRendererProps } from 'react-arborist'
import { cn } from '@/lib/utils'
import type { ProjectFileExplorerNode } from './model'

/** Shared selection, focus, hover, and drop-target treatment for Project-shaped trees. */
export function ProjectFileTreeRow({ node, attrs, innerRef, children }: RowRendererProps<ProjectFileExplorerNode>) {
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
        // Keyboard focus uses the same quiet fill language as hover; reserve the bright ring for drop targeting.
        node.isFocused && !node.isSelected && 'bg-[var(--nova-hover)] text-[var(--nova-text)]',
        !node.isSelected && !node.isFocused && 'hover:bg-[var(--nova-hover)]',
        node.isDragging && 'opacity-40',
        node.willReceiveDrop && 'bg-[var(--nova-active)] ring-1 ring-inset ring-[var(--nova-accent)]',
      )}
    >
      {children}
    </div>
  )
}

/** Keeps file extensions visible while long stems truncate, matching the main Project explorer. */
export function ProjectFileTreeNodeName({ name, type }: { name: string; type: 'file' | 'dir' }) {
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
