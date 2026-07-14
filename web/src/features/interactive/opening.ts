import type { TFunction } from 'i18next'
import type { InitialActorTraitRoll, StoryDirectorModuleRefs, StoryImageSettings, StoryOpeningConfig, StorySummary } from './types'

export interface StoryCreateInput {
  title: string
  origin: string
  story_teller_id: string
  story_director_id: string
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars: number
  choice_count: number
  image_settings?: StoryImageSettings
  opening?: StoryOpeningConfig
  initial_trait_rolls?: InitialActorTraitRoll[]
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

const STORY_OPENING_PRESETS = [
  {
    id: 'arrival',
    zh: '夜雨把旧城的招牌洗得发亮。你在一间还未打烊的小店门前停下，掌心里那枚陌生的钥匙正变得滚烫。',
    en: 'Rain polishes the old city signs. You stop outside a shop that has not closed, while the unfamiliar key in your palm grows warm.',
  },
  {
    id: 'wake-up',
    zh: '你在陌生房间醒来，窗外没有太阳，只有一轮低悬的红月。床头的纸条写着：别相信第一个敲门的人。',
    en: 'You wake in an unfamiliar room. There is no sun outside, only a low red moon. A note by the bed says: do not trust the first knock.',
  },
  {
    id: 'invitation',
    zh: '午夜十二点，一封没有署名的邀请函从门缝滑进来。信纸上只有一行字：如果你还想知道真相，就独自来钟楼。',
    en: 'At midnight, an unsigned invitation slides under the door. It contains one line: if you still want the truth, come to the clock tower alone.',
  },
] as const

function defaultStoryOpening(): StoryOpeningConfig {
  return { mode: 'ai' }
}

function normalizeStoryOpening(opening?: Partial<StoryOpeningConfig> | null): StoryOpeningConfig {
  const mode = opening?.mode === 'preset' || opening?.mode === 'custom' ? opening.mode : 'ai'
  if (mode === 'preset') {
    return {
      mode,
      preset_id: opening?.preset_id?.trim() || STORY_OPENING_PRESETS[0].id,
      preset_text: truncateStoryOpeningText(opening?.preset_text || STORY_OPENING_PRESETS[0].zh),
    }
  }
  if (mode === 'custom') {
    return {
      mode,
      custom_text: truncateStoryOpeningText(opening?.custom_text || ''),
    }
  }
  return defaultStoryOpening()
}

function storyOpeningSourceText(opening: StoryOpeningConfig | undefined) {
  const normalized = normalizeStoryOpening(opening)
  if (normalized.mode === 'preset') return normalized.preset_text?.trim() || ''
  if (normalized.mode === 'custom') return normalized.custom_text?.trim() || ''
  return ''
}

export function buildOpeningPrompt(story: StorySummary | undefined, t: TFunction, sourceOpening?: Partial<StoryOpeningConfig>, source: 'story' | 'book_preset' = 'story') {
  const opening = normalizeStoryOpening(sourceOpening || story?.opening)
  const title = story?.title?.trim() || t('storyStage.opening.untitledStory')
  const origin = story?.origin?.trim()
  const sourceText = storyOpeningSourceText(opening)
  if (opening.mode === 'preset') {
    if (source === 'book_preset') {
      return t('storyStage.opening.promptBookPreset', { title, origin: origin || t('storyStage.opening.noOrigin'), opening: sourceText })
    }
    return t('storyStage.opening.promptPreset', { title, origin: origin || t('storyStage.opening.noOrigin'), opening: sourceText })
  }
  if (opening.mode === 'custom') {
    return t('storyStage.opening.promptCustom', { title, origin: origin || t('storyStage.opening.noOrigin'), opening: sourceText })
  }
  return t('storyStage.opening.promptAI', { title, origin: origin || t('storyStage.opening.noOrigin') })
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
