import type { StoryCheckSettings } from './types'

export function normalizeStoryCheckSettings(value?: Partial<StoryCheckSettings> | null): StoryCheckSettings {
  return {
    difficulty_shift: value?.difficulty_shift ?? 0,
    roll_modifier: value?.roll_modifier ?? 0,
    rule_state_consumption_mode: value?.rule_state_consumption_mode || 'hybrid_auto',
    rule_visibility_mode: value?.rule_visibility_mode || 'audit_only',
  }
}
