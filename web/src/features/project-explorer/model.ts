import type {
  ProjectDirectoryPage,
  ProjectFileEntry,
  ProjectFileEntryType,
  ProjectFileTreeResolveResult,
} from '@/lib/api-client/project-files'

export const PROJECT_FILE_LOAD_MORE_PREFIX = '\u0000project-file-load-more:'

export type CachedProjectDirectory = ProjectDirectoryPage

export interface ProjectFileExplorerNode {
  id: string
  path: string
  name: string
  type: ProjectFileEntryType | 'more'
  ignored: boolean
  symlink: boolean
  loaded: boolean
  loading: boolean
  children?: ProjectFileExplorerNode[]
  /** Client-only placeholder used while a new name is being entered. */
  draft?: true
}

interface MutablePathTreeNode {
  path: string
  name: string
  type: 'file' | 'dir'
  children: Map<string, MutablePathTreeNode>
}

/** Builds a complete, read-only Project-shaped tree from a flat list of file paths. */
export function buildProjectFileTreeFromPaths(paths: readonly string[]): ProjectFileExplorerNode[] {
  const root: MutablePathTreeNode = { path: '', name: '', type: 'dir', children: new Map() }
  for (const filePath of paths) {
    const parts = filePath.split('/').filter(Boolean)
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
  return materializePathTree(root)
}

/** Returns every directory path in tree order for consumers that expand the projection by default. */
export function collectProjectFileTreeDirectoryPaths(nodes: readonly ProjectFileExplorerNode[]): string[] {
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

/** Merges a resolve response into the normalized cache, appending only cursor pages. */
export function mergeProjectDirectories(
  current: ReadonlyMap<string, CachedProjectDirectory>,
  results: readonly ProjectFileTreeResolveResult[],
  appendPath?: string,
): ReadonlyMap<string, CachedProjectDirectory> {
  const next = new Map(current)
  for (const result of results) {
    if (!result.ok) continue
    for (const directory of result.directories) {
      const previous = next.get(directory.path)
      const shouldAppend = directory.path === appendPath && previous?.revision === directory.revision
      next.set(directory.path, {
        ...directory,
        entries: shouldAppend
          ? mergeEntries(previous.entries, directory.entries)
          : directory.entries,
      })
    }
  }
  return next
}

/** Derives react-arborist data while keeping unresolved directories expandable. */
export function buildProjectFileExplorerNodes(
  path: string,
  directories: ReadonlyMap<string, CachedProjectDirectory>,
  loadingPaths: ReadonlySet<string>,
  ancestors: ReadonlySet<string> = new Set(),
): ProjectFileExplorerNode[] {
  const directory = directories.get(path)
  if (!directory || ancestors.has(path)) return []
  const nextAncestors = new Set(ancestors).add(path)
  const nodes = directory.entries.map((entry) => explorerNode(entry, directories, loadingPaths, nextAncestors))
  if (directory.children_state === 'partial' && directory.continuation) {
    nodes.push({
      id: `${PROJECT_FILE_LOAD_MORE_PREFIX}${path}`,
      path,
      name: '',
      type: 'more',
      ignored: false,
      symlink: false,
      loaded: true,
      loading: loadingPaths.has(path),
    })
  }
  return nodes
}

function explorerNode(
  entry: ProjectFileEntry,
  directories: ReadonlyMap<string, CachedProjectDirectory>,
  loadingPaths: ReadonlySet<string>,
  ancestors: ReadonlySet<string>,
): ProjectFileExplorerNode {
  const loaded = directories.has(entry.path)
  return {
    id: entry.path,
    path: entry.path,
    name: entry.name,
    type: entry.type,
    ignored: entry.ignored === true,
    symlink: entry.symlink === true,
    loaded,
    loading: loadingPaths.has(entry.path),
    children: entry.type === 'dir' && !entry.symlink
      ? buildProjectFileExplorerNodes(entry.path, directories, loadingPaths, ancestors)
      : undefined,
  }
}

function materializePathTree(parent: MutablePathTreeNode): ProjectFileExplorerNode[] {
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
      children: node.type === 'dir' ? materializePathTree(node) : undefined,
    }))
}

function mergeEntries(current: readonly ProjectFileEntry[], incoming: readonly ProjectFileEntry[]) {
  const entries = new Map(current.map((entry) => [entry.path, entry]))
  for (const entry of incoming) entries.set(entry.path, entry)
  return [...entries.values()]
}
