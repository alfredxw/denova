import { describe, expect, it } from 'vitest'
import { normalizeThinkingLevel, THINKING_LEVELS } from './thinking-levels'

describe('thinking levels', () => {
  it('keeps the complete provider-neutral order', () => {
    expect(THINKING_LEVELS).toEqual([
      'default',
      'off',
      'minimal',
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
    ])
  })

  it.each([
    ['model_default', 'default'],
    ['none', 'off'],
    ['light', 'low'],
    ['extra high', 'xhigh'],
    ['maximum', 'max'],
    ['turbo', null],
  ])('normalizes %s to %s', (input, expected) => {
    expect(normalizeThinkingLevel(input)).toBe(expected)
  })
})
