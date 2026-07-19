export const STORY_STATE_DISPLAY_STORAGE_KEY = 'nova.interactive.storyStateDisplay.v1'

/** Sets the main-stage default for each new turn; manual panel state remains local to that turn. */
export type StoryStateDisplayPreference = 'visible' | 'collapsed' | 'director-only'

export const DEFAULT_STORY_STATE_DISPLAY: StoryStateDisplayPreference = 'visible'

export function readStoryStateDisplayPreference(): StoryStateDisplayPreference {
  if (typeof window === 'undefined') return DEFAULT_STORY_STATE_DISPLAY
  try {
    const value = window.localStorage.getItem(STORY_STATE_DISPLAY_STORAGE_KEY)
    return normalizeStoryStateDisplayPreference(value) || DEFAULT_STORY_STATE_DISPLAY
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

/** Legacy 'preview'/'expanded' values fold into 'visible' since the grouped ledger replaced the height-clamped preview. */
function normalizeStoryStateDisplayPreference(value: string | null): StoryStateDisplayPreference | null {
  if (value === 'visible' || value === 'collapsed' || value === 'director-only') return value
  if (value === 'preview' || value === 'expanded') return 'visible'
  return null
}
