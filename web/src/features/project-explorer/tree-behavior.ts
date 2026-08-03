import type { NodeApi, TreeApi } from 'react-arborist'
import type { ProjectFileExplorerNode } from './model'
import { projectParentPath, removeNestedProjectPaths } from './operations'

/** Resolves toolbar and keyboard operations from Explorer selection, never stale DOM focus. */
export function insertionDirectory(tree: TreeApi<ProjectFileExplorerNode> | null): string {
  const node = tree?.mostRecentNode
  if (!node || node.data.type === 'more' || node.data.draft) return ''
  return node.data.type === 'dir' ? node.data.path : projectParentPath(node.data.path)
}

/** Restores selection after a filesystem mutation once every resulting node exists. */
export function applyExplorerSelection(
  tree: TreeApi<ProjectFileExplorerNode> | null,
  paths: readonly string[],
): boolean {
  if (!tree) return false
  if (paths.length === 0) {
    tree.deselectAll()
    return true
  }
  const nodes = paths.map((path) => tree.get(path))
  if (nodes.some((node) => !node)) return false
  const last = nodes.at(-1)!
  tree.openParents(last)
  tree.setSelection({ ids: [...paths], anchor: paths[0], mostRecent: paths.at(-1)! })
  tree.focus(last, { scroll: false })
  void tree.scrollTo(last, 'smart')
  return true
}

export function selectionAfterDeletion(
  tree: TreeApi<ProjectFileExplorerNode> | null,
  paths: readonly string[],
): string[] {
  if (!tree) return []
  const removed = removeNestedProjectPaths(paths)
  const isRemoved = (path: string) => removed.some((root) => path === root || path.startsWith(`${root}/`))
  const firstIndex = Math.min(...removed.map((path) => tree.get(path)?.rowIndex ?? Number.POSITIVE_INFINITY))
  const candidates = tree.visibleNodes.filter((node) => (
    node.data.type !== 'more' && !node.data.draft && !isRemoved(node.data.path)
  ))
  const next = candidates.find((node) => (node.rowIndex ?? -1) > firstIndex)
  const previous = candidates.findLast((node) => (node.rowIndex ?? Number.POSITIVE_INFINITY) < firstIndex)
  const fallback = next ?? previous
  return fallback ? [fallback.data.path] : []
}

export function actionableFocusedNode(tree: TreeApi<ProjectFileExplorerNode>) {
  const node = tree.focusedNode
  return node && node.data.type !== 'more' && !node.data.draft ? node : null
}

export function actionableSelection(tree: TreeApi<ProjectFileExplorerNode>): string[] {
  const selected = tree.selectedNodes
    .map((node) => node.data)
    .filter((node) => node.type !== 'more' && !node.draft)
    .map((node) => node.path)
  if (selected.length > 0) return removeNestedProjectPaths(selected)
  const focused = actionableFocusedNode(tree)
  return focused ? [focused.data.path] : []
}

export function disableProjectFileEdit(node: ProjectFileExplorerNode) {
  return node.type !== 'file' && node.type !== 'dir'
}

export function disableProjectFileDrag(node: ProjectFileExplorerNode) {
  return node.type === 'more' || node.draft === true
}

export function disableProjectFileDrop({
  parentNode,
  dragNodes,
}: {
  parentNode: NodeApi<ProjectFileExplorerNode>
  dragNodes: NodeApi<ProjectFileExplorerNode>[]
}) {
  if (!parentNode.isRoot && (parentNode.data.type !== 'dir' || parentNode.data.symlink || parentNode.data.draft === true)) return true
  const target = parentNode.isRoot ? '' : parentNode.data.path
  const sources = dragNodes.map((node) => node.data.path)
  if (sources.some((source) => target === source || target.startsWith(`${source}/`))) return true
  return sources.every((source) => projectParentPath(source) === target)
}
