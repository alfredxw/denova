export function canonicalFileTreePath(path: string, directory: boolean): string {
  const normalized = path.replace(/^\/+|\/+$/g, '')
  return directory && normalized ? `${normalized}/` : normalized
}

export function applicationFileTreePath(path: string): string {
  return path.replace(/\/+$/g, '')
}

/** Resolves the initial-expansion option into the explicit paths resetPaths expects. */
export function expandedFileTreePathsForReset(
  paths: readonly string[],
  initialExpansion: 'closed' | 'open' | number,
  initialExpandedPaths: readonly string[],
): string[] {
  if (initialExpansion === 'closed') return [...initialExpandedPaths]
  const expanded = new Set(initialExpandedPaths)
  for (const path of directoryPathsIn(paths)) {
    if (initialExpansion === 'open' || path.split('/').filter(Boolean).length <= initialExpansion) expanded.add(path)
  }
  return [...expanded]
}

export async function writeClipboardText(value: string) {
  if (!navigator.clipboard?.writeText) throw new Error('Clipboard API is unavailable')
  await navigator.clipboard.writeText(value)
}

function directoryPathsIn(paths: readonly string[]): string[] {
  const directories = new Set<string>()
  for (const path of paths) {
    const directory = path.endsWith('/')
    const parts = path.split('/').filter(Boolean)
    const directoryParts = directory ? parts.length : Math.max(0, parts.length - 1)
    for (let depth = 1; depth <= directoryParts; depth += 1) {
      directories.add(`${parts.slice(0, depth).join('/')}/`)
    }
  }
  return [...directories]
}
