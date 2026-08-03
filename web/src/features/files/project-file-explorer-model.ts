import type {
  ProjectDirectoryPage,
  ProjectFileEntry,
  ProjectFileEntryType,
  ProjectFileTreeResolveResult,
} from './api'

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

function mergeEntries(current: readonly ProjectFileEntry[], incoming: readonly ProjectFileEntry[]) {
  const entries = new Map(current.map((entry) => [entry.path, entry]))
  for (const entry of incoming) entries.set(entry.path, entry)
  return [...entries.values()]
}
