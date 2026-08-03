import { useCallback, useEffect, useState } from 'react'

export interface ProjectExplorerPreferences {
  expandedPaths: string[]
  treeVisible: boolean
}

const PROJECT_EXPLORER_PREFERENCES_VERSION = 1
const MAX_EXPANDED_PATHS = 192

export function readProjectExplorerPreferences(
  projectId: string,
  initialExpandedPaths: readonly string[] = [],
): ProjectExplorerPreferences {
  const fallback = defaultPreferences(initialExpandedPaths)
  try {
    const raw = window.localStorage.getItem(preferencesKey(projectId))
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as Partial<ProjectExplorerPreferences>
    return {
      expandedPaths: normalizeExpandedPaths(parsed.expandedPaths),
      treeVisible: parsed.treeVisible !== false,
    }
  } catch {
    return fallback
  }
}

export function persistProjectExplorerPreferences(
  projectId: string,
  preferences: ProjectExplorerPreferences,
) {
  try {
    window.localStorage.setItem(preferencesKey(projectId), JSON.stringify({
      expandedPaths: normalizeExpandedPaths(preferences.expandedPaths),
      treeVisible: preferences.treeVisible,
    }))
  } catch {
    // Browsing remains available when persistence is blocked by the host webview.
  }
}

/** Owns project-scoped Explorer layout state for every surface using the shared tree. */
export function useProjectExplorerPreferences(
  projectId: string,
  initialExpandedPaths: readonly string[] = [],
) {
  // Callers keep one hook instance bound to one stable Project ID. Surfaces that can switch
  // projects key their Explorer by Project ID so state from two projects is never interleaved.
  const [preferences, setPreferences] = useState<ProjectExplorerPreferences>(() => (
    readProjectExplorerPreferences(projectId, initialExpandedPaths)
  ))

  useEffect(() => {
    persistProjectExplorerPreferences(projectId, preferences)
  }, [preferences, projectId])

  const setTreeVisible = useCallback((treeVisible: boolean) => {
    setPreferences((current) => ({ ...current, treeVisible }))
  }, [])
  const setDirectoryExpanded = useCallback((path: string, expanded: boolean) => {
    setPreferences((current) => {
      const paths = new Set(current.expandedPaths)
      if (expanded) paths.add(path)
      else paths.delete(path)
      return { ...current, expandedPaths: [...paths] }
    })
  }, [])
  const collapseAll = useCallback(() => {
    setPreferences((current) => ({ ...current, expandedPaths: [] }))
  }, [])
  const removeBranch = useCallback((branch: string) => {
    setPreferences((current) => ({
      ...current,
      expandedPaths: current.expandedPaths.filter((path) => path !== branch && !path.startsWith(`${branch}/`)),
    }))
  }, [])
  const relocateBranch = useCallback((from: string, to: string) => {
    setPreferences((current) => ({
      ...current,
      expandedPaths: current.expandedPaths.map((path) => {
        if (path === from) return to
        if (path.startsWith(`${from}/`)) return `${to}${path.slice(from.length)}`
        return path
      }),
    }))
  }, [])

  return {
    preferences,
    setTreeVisible,
    setDirectoryExpanded,
    collapseAll,
    removeBranch,
    relocateBranch,
  }
}

function defaultPreferences(initialExpandedPaths: readonly string[]): ProjectExplorerPreferences {
  return { expandedPaths: normalizeExpandedPaths(initialExpandedPaths), treeVisible: true }
}

function normalizeExpandedPaths(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return [...new Set(value.filter((path): path is string => typeof path === 'string' && Boolean(path)))]
    .slice(-MAX_EXPANDED_PATHS)
}

function preferencesKey(projectId: string) {
  return `nova.project-explorer.preferences.v${PROJECT_EXPLORER_PREFERENCES_VERSION}:${projectId}`
}
