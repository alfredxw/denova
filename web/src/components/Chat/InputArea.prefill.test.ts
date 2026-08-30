import { describe, expect, it } from 'vitest'
import { applyInputPrefill } from './InputArea'

describe('applyInputPrefill', () => {
  it('replaces the composer when requested', () => {
    expect(applyInputPrefill('existing draft', {
      prompt: 'replacement',
      nonce: 1,
      mode: 'replace',
    })).toBe('replacement')
  })

  it('appends a quick prompt without discarding an existing draft', () => {
    expect(applyInputPrefill('existing draft  ', {
      prompt: 'quick prompt',
      nonce: 2,
      mode: 'append',
    })).toBe('existing draft\n\nquick prompt')
  })

  it('fills an empty composer instead of adding separators', () => {
    expect(applyInputPrefill('   ', {
      prompt: 'quick prompt',
      nonce: 3,
      mode: 'append',
    })).toBe('quick prompt')
  })
})
