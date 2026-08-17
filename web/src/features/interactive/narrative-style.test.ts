import { describe, expect, it } from 'vitest'
import type { Teller } from './types'
import { DEFAULT_NARRATIVE_STYLE_ID, narrativeStylesForMode, resolveNarrativeStyle } from './narrative-style'

const teller = (id: string, modes?: Array<'writing' | 'game'>): Teller => ({
  version: 1,
  id,
  name: id,
  description: '',
  modes,
  context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' },
  slots: [],
  custom: true,
})

describe('narrative style mode resolution', () => {
  it('treats legacy styles as shared and filters explicitly scoped styles', () => {
    const tellers = [teller('legacy'), teller('writing-only', ['writing']), teller('game-only', ['game'])]
    expect(narrativeStylesForMode(tellers, 'writing').map((item) => item.id)).toEqual(['legacy', 'writing-only'])
    expect(narrativeStylesForMode(tellers, 'game').map((item) => item.id)).toEqual(['legacy', 'game-only'])
  })

  it('keeps a valid previous selection and otherwise resolves rhythm independently of list order', () => {
    const tellers = [teller('classic'), teller(DEFAULT_NARRATIVE_STYLE_ID), teller('writing-only', ['writing'])]
    expect(resolveNarrativeStyle(tellers, 'classic', 'game')?.id).toBe('classic')
    expect(resolveNarrativeStyle(tellers, 'writing-only', 'game')?.id).toBe(DEFAULT_NARRATIVE_STYLE_ID)
    expect(resolveNarrativeStyle(tellers, '', 'game')?.id).toBe(DEFAULT_NARRATIVE_STYLE_ID)
  })
})
