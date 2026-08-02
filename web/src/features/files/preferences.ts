export interface ProjectFilesPreferences {
  expandedPaths: string[]
  showIgnored: boolean
  treeVisible: boolean
}

const PROJECT_FILES_PREFERENCES_VERSION = 1
const MAX_EXPANDED_PATHS = 256

export function defaultProjectFilesPreferences(): ProjectFilesPreferences {
  return { expandedPaths: [], showIgnored: false, treeVisible: true }
}

export function readProjectFilesPreferences(projectId: string): ProjectFilesPreferences {
  const fallback = defaultProjectFilesPreferences()
  try {
    const raw = window.localStorage.getItem(preferencesKey(projectId))
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as Partial<ProjectFilesPreferences>
    return {
      expandedPaths: Array.isArray(parsed.expandedPaths)
        ? parsed.expandedPaths.filter((path): path is string => typeof path === 'string' && Boolean(path)).slice(0, MAX_EXPANDED_PATHS)
        : [],
      showIgnored: parsed.showIgnored === true,
      treeVisible: parsed.treeVisible !== false,
    }
  } catch {
    return fallback
  }
}

export function persistProjectFilesPreferences(projectId: string, preferences: ProjectFilesPreferences) {
  try {
    window.localStorage.setItem(preferencesKey(projectId), JSON.stringify({
      expandedPaths: [...new Set(preferences.expandedPaths)].slice(-MAX_EXPANDED_PATHS),
      showIgnored: preferences.showIgnored,
      treeVisible: preferences.treeVisible,
    }))
  } catch {
    // Persistence is an enhancement; browsing must still work in restricted webviews.
  }
}

function preferencesKey(projectId: string) {
  return `nova.project-files.preferences.v${PROJECT_FILES_PREFERENCES_VERSION}:${projectId}`
}
