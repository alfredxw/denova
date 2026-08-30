import type { TFunction } from 'i18next'
import type { StoryDirector } from './types'

export const DEFAULT_GAME_PRESET_ID = 'default'

/** Localize the untouched built-in preset while preserving creator overrides. */
export function gamePresetName(preset: StoryDirector, t: TFunction): string {
  if (preset.id === DEFAULT_GAME_PRESET_ID && !preset.custom && !preset.builtin_overridden) {
    return t('storyPicker.defaultStoryDirector')
  }
  return preset.name || preset.id
}
