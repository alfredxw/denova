export interface ProjectFileEditorPreferences {
  wordWrap: boolean
}

const PROJECT_FILE_EDITOR_PREFERENCES_VERSION = 1

export function defaultProjectFileEditorPreferences(): ProjectFileEditorPreferences {
  return { wordWrap: true }
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

function editorPreferencesKey() {
  return `nova.project-file-editor.preferences.v${PROJECT_FILE_EDITOR_PREFERENCES_VERSION}`
}
