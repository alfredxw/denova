import { useCallback, useEffect, useState } from 'react'

interface MultiFileDiffNavigationOptions {
  identity: string
  paths: readonly string[]
  preferredPath?: string | null
}

/** Shared active-file, collapse, and navigator state for virtualized multi-file diffs. */
export function useMultiFileDiffNavigation({ identity, paths, preferredPath }: MultiFileDiffNavigationOptions) {
  const [activePath, setActivePath] = useState('')
  const [collapsedPaths, setCollapsedPaths] = useState<ReadonlySet<string>>(() => new Set())
  const [navigatorVisible, setNavigatorVisible] = useState(true)

  useEffect(() => {
    setActivePath('')
    setCollapsedPaths(new Set())
    setNavigatorVisible(true)
  }, [identity])

  useEffect(() => {
    if (!paths.length) {
      setActivePath('')
      return
    }
    if (paths.includes(activePath)) return
    setActivePath(preferredPath && paths.includes(preferredPath) ? preferredPath : paths[0])
  }, [activePath, paths, preferredPath])

  useEffect(() => {
    const available = new Set(paths)
    setCollapsedPaths((current) => {
      const next = new Set([...current].filter((path) => available.has(path)))
      return next.size === current.size ? current : next
    })
  }, [paths])

  const allDiffsCollapsed = paths.length > 0 && paths.every((path) => collapsedPaths.has(path))

  const toggleFile = useCallback((path: string) => {
    setActivePath(path)
    setCollapsedPaths((current) => {
      const next = new Set(current)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }, [])

  const toggleAllDiffs = useCallback(() => {
    setCollapsedPaths(allDiffsCollapsed ? new Set() : new Set(paths))
  }, [allDiffsCollapsed, paths])

  const selectFile = useCallback((path: string) => {
    setActivePath(path)
    setCollapsedPaths((current) => {
      if (!current.has(path)) return current
      const next = new Set(current)
      next.delete(path)
      return next
    })
  }, [])

  return {
    activePath,
    collapsedPaths,
    allDiffsCollapsed,
    navigatorVisible,
    setActivePath,
    setNavigatorVisible,
    toggleFile,
    toggleAllDiffs,
    selectFile,
  }
}

export type MultiFileDiffNavigation = ReturnType<typeof useMultiFileDiffNavigation>
