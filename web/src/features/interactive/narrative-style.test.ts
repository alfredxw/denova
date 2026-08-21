import { describe, expect, it } from 'vitest'
import type { Teller } from './types'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from './narrative-style'

const teller = (id: string): Teller => ({
  version: 1,
  id,
  name: id,
  description: '',
  context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' },
  slots: [],
  custom: true,
})

describe('narrative style resolution', () => {
  it('keeps a valid previous selection and otherwise resolves rhythm independently of list order', () => {
    const tellers = [teller('classic'), teller(DEFAULT_NARRATIVE_STYLE_ID), teller('custom')]
    expect(resolveNarrativeStyle(tellers, 'classic')?.id).toBe('classic')
    expect(resolveNarrativeStyle(tellers, 'custom')?.id).toBe('custom')
    expect(resolveNarrativeStyle(tellers, '')?.id).toBe(DEFAULT_NARRATIVE_STYLE_ID)
  })
})
