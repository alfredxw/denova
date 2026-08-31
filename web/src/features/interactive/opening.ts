import type { InitialActorTraitRoll, StoryCheckSettings, StoryDirectorModuleRefs, StoryImageSettings, StoryOpeningConfig, StoryPlanningMode, StoryProtagonist, StoryStateSchemaPolicy } from './types'

export interface StoryCreateInput {
  title: string
  custom_agent_id?: string
  origin: string
  protagonist?: StoryProtagonist
  story_teller_id: string
  story_director_id: string
  planning_mode: StoryPlanningMode
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars: number
  choice_count: number
  image_settings?: StoryImageSettings
  check_settings?: StoryCheckSettings
  opening?: StoryOpeningConfig
  initial_trait_rolls?: InitialActorTraitRoll[]
  state_schema_policy?: StoryStateSchemaPolicy
}

const STORY_OPENING_TEXT_LIMIT = 4000
export const DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS = 2000
export const DEFAULT_INTERACTIVE_CHOICE_COUNT = 5
export const MIN_INTERACTIVE_CHOICE_COUNT = 2
export const MAX_INTERACTIVE_CHOICE_COUNT = 10
export const INTERACTIVE_OPENING_PRESET_PATH = 'setting/interactive-openings.json'
export const LEGACY_INTERACTIVE_OPENING_PRESET_PATH = 'setting/interactive-opening.md'
export const INTERACTIVE_OPENING_PRESET_UPDATED_EVENT = 'nova:interactive-opening-preset-updated'
export const INTERACTIVE_OPENING_PRESET_ENTRY_ID = '__interactive_opening_preset__'

export interface BookOpeningPreset {
  id: string
  title: string
  content: string
}

interface BookOpeningPresetFile {
  version: number
  presets: BookOpeningPreset[]
}

export function parseBookOpeningPresets(content: string): BookOpeningPreset[] {
  const trimmed = content.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed) as Partial<BookOpeningPresetFile> | BookOpeningPreset[]
    const sourcePresets = Array.isArray(parsed) ? parsed : parsed.presets
    if (Array.isArray(sourcePresets)) return normalizeBookOpeningPresets(sourcePresets)
  } catch {
    return normalizeBookOpeningPresets([{ id: 'legacy', title: '默认开场白', content: trimmed }])
  }
  return []
}

export function serializeBookOpeningPresets(presets: BookOpeningPreset[]) {
  return `${JSON.stringify({ version: 1, presets: normalizeBookOpeningPresets(presets) }, null, 2)}\n`
}

function normalizeBookOpeningPresets(presets: Array<Partial<BookOpeningPreset>>): BookOpeningPreset[] {
  const seen = new Set<string>()
  return presets
    .map((preset, index) => {
      const fallbackId = `opening-${index + 1}`
      let id = (preset.id || fallbackId).trim() || fallbackId
      while (seen.has(id)) id = `${id}-${index + 1}`
      seen.add(id)
      return {
        id,
        title: truncateStoryOpeningTitle(preset.title || `开场白 ${index + 1}`),
        content: truncateStoryOpeningText(preset.content || ''),
      }
    })
    .filter((preset) => preset.title || preset.content)
}

export function newBookOpeningPreset(title = '新开场白'): BookOpeningPreset {
  return {
    id: createOpeningPresetId(),
    title,
    content: '',
  }
}

export function truncateStoryOpeningText(text: string) {
  const trimmed = text.trim()
  if (trimmed.length <= STORY_OPENING_TEXT_LIMIT) return trimmed
  return trimmed.slice(0, STORY_OPENING_TEXT_LIMIT)
}

function truncateStoryOpeningTitle(text: string) {
  const trimmed = text.trim()
  if (trimmed.length <= 80) return trimmed
  return trimmed.slice(0, 80)
}

function createOpeningPresetId() {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `opening-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}
