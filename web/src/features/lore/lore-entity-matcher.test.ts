import { describe, expect, it } from 'vitest'
import type { LoreItem } from '@/lib/api'
import {
  canQuickCreateLoreName,
  createLoreEntityMatcher,
  isMatchableLoreTerm,
  loreLensExcerpt,
} from './lore-entity-matcher'

function loreItem(overrides: Partial<LoreItem> & Pick<LoreItem, 'id' | 'name'>): LoreItem {
  return {
    id: overrides.id,
    name: overrides.name,
    type: overrides.type ?? 'character',
    type_source: overrides.type_source ?? 'manual',
    enabled: overrides.enabled ?? true,
    importance: overrides.importance ?? 'important',
    tags: overrides.tags ?? [],
    brief_description: overrides.brief_description ?? '',
    keywords: overrides.keywords ?? [],
    load_mode: overrides.load_mode ?? 'auto',
    content: overrides.content ?? '',
    created_at: '',
    updated_at: '',
  }
}

describe('lore-entity-matcher', () => {
  it('filters single-character CJK and too-short ASCII terms to avoid noise', () => {
    expect(isMatchableLoreTerm('剑')).toBe(false)
    expect(isMatchableLoreTerm('温照野')).toBe(true)
    expect(isMatchableLoreTerm('AI')).toBe(false)
    expect(isMatchableLoreTerm('Lin')).toBe(true)
    expect(isMatchableLoreTerm('[[引用]]')).toBe(false)
  })

  it('prefers longest matches and prevents shorter prefixes from lighting up on repeated long mentions', () => {
    const matcher = createLoreEntityMatcher([
      loreItem({ id: 'sect', name: '玄天宗' }),
      loreItem({ id: 'leader', name: '玄天宗主' }),
    ], { enabled: true, matchKeywords: false })

    expect(matcher).not.toBeNull()
    const seen = new Set<string>()
    const matches = matcher!.findMatches('玄天宗主走进大殿，玄天宗主笑了，随后看向玄天宗弟子。', seen)
    expect(matches.map((m) => [m.itemId, m.matchedTerm])).toEqual([
      ['leader', '玄天宗主'],
      ['sect', '玄天宗'],
    ])
  })

  it('respects ASCII word boundaries, keyword toggles, and disabled items', () => {
    const items = [
      loreItem({ id: 'lin', name: 'Lin', keywords: ['星海使者', '剑'] }),
      loreItem({ id: 'disabled', name: '温照野', enabled: false }),
    ]
    const nameOnly = createLoreEntityMatcher(items, { enabled: true, matchKeywords: false })!
    expect(nameOnly.findMatches('Ling met lin in online chat. 星海使者与温照野。').map((m) => m.matchedTerm)).toEqual(['lin'])

    const withKeywords = createLoreEntityMatcher(items, { enabled: true, matchKeywords: true })!
    expect(withKeywords.findMatches('星海使者拔剑。').map((m) => [m.itemId, m.matchedTerm])).toEqual([
      ['lin', '星海使者'],
    ])
  })

  it('validates quick-create candidates and produces clean excerpts', () => {
    const existing = [loreItem({ id: 'hero', name: '温照野', content: '# 标题\n\n**擅长**使用[[潮汐术]]。' })]
    expect(canQuickCreateLoreName('青岚剑宗', existing)).toBe(true)
    expect(canQuickCreateLoreName('温照野', existing)).toBe(false)
    expect(canQuickCreateLoreName('剑', existing)).toBe(false)
    expect(canQuickCreateLoreName('多行\n内容', existing)).toBe(false)
    expect(loreLensExcerpt(existing[0])).toBe('标题 擅长使用潮汐术。')
  })
})
