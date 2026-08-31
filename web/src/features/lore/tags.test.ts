import { describe, expect, it } from 'vitest'
import { hasLoreProtagonistTag, splitLoreTags, toggleLoreProtagonistTag } from './tags'

describe('Lore protagonist tags', () => {
  it('recognizes both supported protagonist tag spellings', () => {
    expect(hasLoreProtagonistTag(['同伴', '主角'])).toBe(true)
    expect(hasLoreProtagonistTag(['Protagonist'])).toBe(true)
    expect(hasLoreProtagonistTag(['同伴'])).toBe(false)
  })

  it('adds or removes only the protagonist tag', () => {
    expect(toggleLoreProtagonistTag(['同伴'])).toEqual(['同伴', '主角'])
    expect(toggleLoreProtagonistTag(['主角', '同伴', 'protagonist'])).toEqual(['同伴'])
  })

  it('uses the same comma parsing as Lore autosave', () => {
    expect(splitLoreTags('同伴， 主角,航海士')).toEqual(['同伴', '主角', '航海士'])
  })
})
