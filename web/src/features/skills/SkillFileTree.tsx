import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { FileNode } from '@/hooks/useWorkspace'
import { cn } from '@/lib/utils'

interface SkillFileTreeProps {
  nodes: readonly FileNode[]
  selectedFile: string
  defaultExpandedPaths?: readonly string[]
  onSelectFile: (path: string) => void
}

const fileNameCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** A deliberately read-only tree for browsing the files that belong to one Skill. */
export function SkillFileTree({
  nodes,
  selectedFile,
  defaultExpandedPaths = [],
  onSelectFile,
}: SkillFileTreeProps) {
  const expandedPaths = useMemo(() => new Set(defaultExpandedPaths), [defaultExpandedPaths])
  return (
    <ul role="tree" className="space-y-0.5">
      {sortNodes(nodes).map((node) => (
        <SkillFileTreeNode
          key={node.name}
          node={node}
          path={node.name}
          selectedFile={selectedFile}
          defaultExpandedPaths={expandedPaths}
          onSelectFile={onSelectFile}
        />
      ))}
    </ul>
  )
}

function SkillFileTreeNode({
  node,
  path,
  selectedFile,
  defaultExpandedPaths,
  onSelectFile,
}: {
  node: FileNode
  path: string
  selectedFile: string
  defaultExpandedPaths: ReadonlySet<string>
  onSelectFile: (path: string) => void
}) {
  const directory = node.type === 'dir'
  const [expanded, setExpanded] = useState(() => directory && defaultExpandedPaths.has(path))
  const children = directory ? sortNodes(node.children ?? []) : []

  return (
    <li role="treeitem" aria-expanded={directory ? expanded : undefined}>
      <button
        type="button"
        className={cn(
          'flex h-7 w-full min-w-0 items-center gap-1 rounded px-1.5 text-left text-xs text-[var(--nova-tree-text)]',
          selectedFile === path ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'hover:bg-[var(--nova-hover)]',
        )}
        onClick={() => {
          if (directory) setExpanded((current) => !current)
          else onSelectFile(path)
        }}
      >
        {directory
          ? expanded
            ? <ChevronDown className="size-3.5 shrink-0 text-[var(--nova-tree-chevron)]" />
            : <ChevronRight className="size-3.5 shrink-0 text-[var(--nova-tree-chevron)]" />
          : <span className="size-3.5 shrink-0" />}
        {directory
          ? expanded
            ? <FolderOpen className="size-4 shrink-0 text-[var(--nova-tree-folder)]" />
            : <Folder className="size-4 shrink-0 text-[var(--nova-tree-folder)]" />
          : <FileText className="size-4 shrink-0 text-[var(--nova-tree-icon)]" />}
        <span className="min-w-0 truncate">{node.name}</span>
      </button>
      {directory && expanded && children.length > 0 ? (
        <ul role="group" className="ml-3">
          {children.map((child) => {
            const childPath = `${path}/${child.name}`
            return (
              <SkillFileTreeNode
                key={childPath}
                node={child}
                path={childPath}
                selectedFile={selectedFile}
                defaultExpandedPaths={defaultExpandedPaths}
                onSelectFile={onSelectFile}
              />
            )
          })}
        </ul>
      ) : null}
    </li>
  )
}

function sortNodes(nodes: readonly FileNode[]) {
  return [...nodes].sort((left, right) => {
    if (left.type !== right.type) return left.type === 'dir' ? -1 : 1
    return fileNameCollator.compare(left.name, right.name)
  })
}
