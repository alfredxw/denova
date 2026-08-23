import type { ProjectFileExplorerNode } from './model'

export interface ProjectFileClipboard {
  mode: 'copy' | 'cut'
  paths: string[]
}

export interface ProjectFileTransfer {
  source: string
  destination: string
}

/** Builds collision-free copy targets and rejects moving a folder into itself. */
export function buildProjectFilePastePlan(
  nodes: readonly ProjectFileExplorerNode[],
  clipboard: ProjectFileClipboard,
  targetDirectory: string,
): ProjectFileTransfer[] {
  const occupied = new Set(childrenAt(nodes, targetDirectory).map((node) => node.path))
  const transfers: ProjectFileTransfer[] = []
  for (const source of removeNestedProjectPaths(clipboard.paths)) {
    if (source === targetDirectory || targetDirectory.startsWith(`${source}/`)) continue
    const sourceNode = findProjectFileNode(nodes, source)
    let destination = joinProjectPath(targetDirectory, projectBaseName(source))
    if (clipboard.mode === 'copy' && occupied.has(destination)) {
      destination = nextCopyPath(targetDirectory, projectBaseName(source), sourceNode?.type === 'file', occupied)
    }
    if (clipboard.mode === 'cut' && source === destination) continue
    occupied.add(destination)
    transfers.push({ source, destination })
  }
  return transfers
}

export function findProjectFileNode(
  nodes: readonly ProjectFileExplorerNode[],
  path: string,
): ProjectFileExplorerNode | null {
  for (const node of nodes) {
    if (node.path === path) return node
    const nested = node.children ? findProjectFileNode(node.children, path) : null
    if (nested) return nested
  }
  return null
}

export function removeNestedProjectPaths(paths: readonly string[]): string[] {
  const sorted = [...new Set(paths)].sort((left, right) => left.length - right.length)
  return sorted.filter((path, index) => !sorted.slice(0, index).some((parent) => path.startsWith(`${parent}/`)))
}

export function projectParentPath(path: string): string {
  const separator = path.lastIndexOf('/')
  return separator < 0 ? '' : path.slice(0, separator)
}

export function projectBaseName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

export function joinProjectPath(parent: string, name: string): string {
  return parent ? `${parent}/${name}` : name
}

export function absoluteProjectPath(workspace: string, path: string): string {
  const root = workspace.replace(/[\\/]+$/, '')
  return root ? `${root}/${path}` : path
}

function childrenAt(nodes: readonly ProjectFileExplorerNode[], path: string): readonly ProjectFileExplorerNode[] {
  if (!path) return nodes
  return findProjectFileNode(nodes, path)?.children ?? []
}

function nextCopyPath(
  parent: string,
  name: string,
  isFile: boolean,
  occupied: ReadonlySet<string>,
): string {
  const dot = isFile ? name.lastIndexOf('.') : -1
  const stem = dot > 0 ? name.slice(0, dot) : name
  const extension = dot > 0 ? name.slice(dot) : ''
  for (let copy = 1; ; copy += 1) {
    const suffix = copy === 1 ? ' copy' : ` copy ${copy}`
    const candidate = joinProjectPath(parent, `${stem}${suffix}${extension}`)
    if (!occupied.has(candidate)) return candidate
  }
}
