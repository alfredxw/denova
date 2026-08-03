export interface ProjectFilesPreferences {
  expandedPaths: string[]
  treeVisible: boolean
}

export interface ProjectFileEditorPreferences {
  wordWrap: boolean
}

const PROJECT_FILES_PREFERENCES_VERSION = 1
const PROJECT_FILE_EDITOR_PREFERENCES_VERSION = 1
const MAX_EXPANDED_PATHS = 192

export function defaultProjectFilesPreferences(): ProjectFilesPreferences {
  return { expandedPaths: [], treeVisible: true }
}

export function defaultProjectFileEditorPreferences(): ProjectFileEditorPreferences {
  return { wordWrap: true }
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
      treeVisible: preferences.treeVisible,
    }))
  } catch {
    // Persistence is an enhancement; browsing must still work in restricted webviews.
  }
}

export function readProjectFileEditorPreferences(): ProjectFileEditorPreferences {
  const fallback = defaultProjectFileEditorPreferences()
  try {
    const raw = window.localStorage.getItem(editorPreferencesKey())
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as Partial<ProjectFileEditorPreferences>
    return { wordWrap: parsed.wordWrap !== false }
  } catch {
    return fallback
  }
}

export function persistProjectFileEditorPreferences(preferences: ProjectFileEditorPreferences) {
  try {
    window.localStorage.setItem(editorPreferencesKey(), JSON.stringify({ wordWrap: preferences.wordWrap }))
  } catch {
    // The editor remains usable with defaults in restricted webviews.
  }
}

export function removeExpandedBranch(paths: readonly string[], branch: string): string[] {
  return paths.filter((path) => path !== branch && !path.startsWith(`${branch}/`))
}

export function relocateExpandedBranch(paths: readonly string[], from: string, to: string): string[] {
  return paths.map((path) => {
    if (path === from) return to
    if (path.startsWith(`${from}/`)) return `${to}${path.slice(from.length)}`
    return path
  })
}

function preferencesKey(projectId: string) {
  return `nova.project-files.preferences.v${PROJECT_FILES_PREFERENCES_VERSION}:${projectId}`
}

function editorPreferencesKey() {
  return `nova.project-file-editor.preferences.v${PROJECT_FILE_EDITOR_PREFERENCES_VERSION}`
}
