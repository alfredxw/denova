import { describe, expect, it } from 'vitest'
import { normalizeThinkingLevel, THINKING_LEVELS } from './thinking-levels'

describe('thinking levels', () => {
  it('excludes minimal from the selectable vocabulary', () => {
    expect(THINKING_LEVELS).toEqual(['default', 'off', 'low', 'medium', 'high', 'xhigh', 'max'])
  })

  it('normalizes saved minimal selections to low', () => {
    expect(normalizeThinkingLevel('minimal')).toBe('low')
  })
})
