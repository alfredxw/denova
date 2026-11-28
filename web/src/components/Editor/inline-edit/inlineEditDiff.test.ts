import { describe, expect, it } from 'vitest'
import { computeInlineEditDiff } from './inlineEditDiff'

describe('computeInlineEditDiff', () => {
  it('produces fine-grained Chinese word diff instead of replacing the entire sentence', () => {
    const parts = computeInlineEditDiff(
      '温照野站在栈桥尽头，低声说：“潮汐快涨了。”',
      '温照野站在栈桥尽头，按住剑柄说：“潮汐要涨了。”',
      'zh-CN',
    )

    expect(parts.some((p) => p.kind === 'equal' && p.value.includes('温照野站在栈桥尽头'))).toBe(true)
    expect(parts.some((p) => p.kind === 'removed' && p.value.includes('低声'))).toBe(true)
    expect(parts.some((p) => p.kind === 'added' && p.value.includes('按住剑柄'))).toBe(true)
  })

  it('falls back to character diff when Intl.Segmenter is unavailable', () => {
    const parts = computeInlineEditDiff(
      '夜雾沉沉。',
      '夜雾浓重。',
      'zh-CN',
      null,
    )

    expect(parts).toEqual([
      { kind: 'equal', value: '夜雾' },
      { kind: 'removed', value: '沉沉' },
      { kind: 'added', value: '浓重' },
      { kind: 'equal', value: '。' },
    ])
  })
})
