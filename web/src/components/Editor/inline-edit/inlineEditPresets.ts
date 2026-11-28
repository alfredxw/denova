import type { InlineEditMode } from '@/lib/api-client/inline-edit'
import type { LoreEntityMatcher } from '@/features/lore/lore-entity-matcher'

export interface InlineEditPreset {
  id: string
  labelKey: string
  instructionKey: string
}

const REWRITE_PRESETS: Record<InlineEditMode, readonly InlineEditPreset[]> = {
  writing: [
    { id: 'polish', labelKey: 'editor.inlineEdit.preset.polish', instructionKey: 'editor.inlineEdit.presetPrompt.polish' },
    { id: 'expand', labelKey: 'editor.inlineEdit.preset.expand', instructionKey: 'editor.inlineEdit.presetPrompt.expand' },
    { id: 'shorten', labelKey: 'editor.inlineEdit.preset.shorten', instructionKey: 'editor.inlineEdit.presetPrompt.shorten' },
    { id: 'dialogue', labelKey: 'editor.inlineEdit.preset.dialogue', instructionKey: 'editor.inlineEdit.presetPrompt.dialogue' },
    { id: 'deai', labelKey: 'editor.inlineEdit.preset.deai', instructionKey: 'editor.inlineEdit.presetPrompt.deai' },
  ],
  game_turn: [
    { id: 'polish', labelKey: 'editor.inlineEdit.preset.polish', instructionKey: 'editor.inlineEdit.presetPrompt.polish' },
    { id: 'expand', labelKey: 'editor.inlineEdit.preset.expand', instructionKey: 'editor.inlineEdit.presetPrompt.expand' },
    { id: 'shorten', labelKey: 'editor.inlineEdit.preset.shorten', instructionKey: 'editor.inlineEdit.presetPrompt.shorten' },
    { id: 'dialogue', labelKey: 'editor.inlineEdit.preset.dialogue', instructionKey: 'editor.inlineEdit.presetPrompt.dialogue' },
    { id: 'deai', labelKey: 'editor.inlineEdit.preset.deai', instructionKey: 'editor.inlineEdit.presetPrompt.deai' },
  ],
  lore: [
    { id: 'polish', labelKey: 'editor.inlineEdit.preset.polish', instructionKey: 'editor.inlineEdit.presetPrompt.lorePolish' },
    { id: 'expand', labelKey: 'editor.inlineEdit.preset.loreExpand', instructionKey: 'editor.inlineEdit.presetPrompt.loreExpand' },
    { id: 'shorten', labelKey: 'editor.inlineEdit.preset.loreDistill', instructionKey: 'editor.inlineEdit.presetPrompt.loreDistill' },
  ],
}

const INSERT_PRESETS: Record<InlineEditMode, readonly InlineEditPreset[]> = {
  writing: [
    { id: 'continue', labelKey: 'editor.inlineEdit.preset.continue', instructionKey: 'editor.inlineEdit.presetPrompt.continue' },
    { id: 'scene', labelKey: 'editor.inlineEdit.preset.sceneDetail', instructionKey: 'editor.inlineEdit.presetPrompt.sceneDetail' },
    { id: 'transition', labelKey: 'editor.inlineEdit.preset.transition', instructionKey: 'editor.inlineEdit.presetPrompt.transition' },
  ],
  game_turn: [
    { id: 'continue', labelKey: 'editor.inlineEdit.preset.continue', instructionKey: 'editor.inlineEdit.presetPrompt.continue' },
    { id: 'scene', labelKey: 'editor.inlineEdit.preset.sceneDetail', instructionKey: 'editor.inlineEdit.presetPrompt.sceneDetail' },
  ],
  lore: [
    { id: 'expand', labelKey: 'editor.inlineEdit.preset.loreExpand', instructionKey: 'editor.inlineEdit.presetPrompt.loreInsertDetail' },
    { id: 'hooks', labelKey: 'editor.inlineEdit.preset.loreHooks', instructionKey: 'editor.inlineEdit.presetPrompt.loreHooks' },
  ],
}

export function inlineEditPresetsFor(mode: InlineEditMode, hasSelection: boolean): readonly InlineEditPreset[] {
  return hasSelection ? REWRITE_PRESETS[mode] : INSERT_PRESETS[mode]
}

export const INLINE_EDIT_BEFORE_BUDGET = 2000
export const INLINE_EDIT_AFTER_BUDGET = 1500
export const INLINE_EDIT_MAX_LORE_IDS = 6

/**
 * Extracts bounded unique Lore IDs appearing in the selection and its nearby
 * context window, prioritizing entities mentioned inside the selection itself.
 */
export function matchInlineEditLoreIds(
  matcher: LoreEntityMatcher | null,
  selection: string,
  before: string,
  after: string,
  maxItems = INLINE_EDIT_MAX_LORE_IDS,
): string[] {
  if (!matcher || !matcher.hasTerms || maxItems <= 0) return []
  const seen = new Set<string>()
  const ordered: string[] = []
  const collect = (text: string) => {
    if (!text || ordered.length >= maxItems) return
    for (const match of matcher.findMatches(text)) {
      if (seen.has(match.itemId)) continue
      seen.add(match.itemId)
      ordered.push(match.itemId)
      if (ordered.length >= maxItems) break
    }
  }
  collect(selection)
  collect(before.slice(-800))
  collect(after.slice(0, 600))
  return ordered
}
