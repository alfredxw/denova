export const STORY_STATE_DISPLAY_STORAGE_KEY = 'nova.interactive.storyStateDisplay.v1'
export const OPEN_DIRECTOR_STATE_EVENT = 'nova:interactive-open-director-state'

/** Sets the main-stage default for each new turn; manual panel state remains local to that turn. */
export type StoryStateDisplayPreference = 'preview' | 'expanded' | 'collapsed' | 'director-only'

export const DEFAULT_STORY_STATE_DISPLAY: StoryStateDisplayPreference = 'preview'

export function readStoryStateDisplayPreference(): StoryStateDisplayPreference {
  if (typeof window === 'undefined') return DEFAULT_STORY_STATE_DISPLAY
  try {
    const value = window.localStorage.getItem(STORY_STATE_DISPLAY_STORAGE_KEY)
    return isStoryStateDisplayPreference(value) ? value : DEFAULT_STORY_STATE_DISPLAY
  } catch (error) {
    console.warn('[interactive-story-state] failed to read display preference', { key: STORY_STATE_DISPLAY_STORAGE_KEY, error })
    return DEFAULT_STORY_STATE_DISPLAY
  }
}

export function writeStoryStateDisplayPreference(value: StoryStateDisplayPreference) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, value)
  } catch (error) {
    console.warn('[interactive-story-state] failed to persist display preference', { key: STORY_STATE_DISPLAY_STORAGE_KEY, value, error })
  }
}

function isStoryStateDisplayPreference(value: string | null): value is StoryStateDisplayPreference {
  return value === 'preview' || value === 'expanded' || value === 'collapsed' || value === 'director-only'
}
