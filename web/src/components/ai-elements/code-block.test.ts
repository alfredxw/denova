import { beforeEach, describe, expect, it, vi } from 'vitest'

const shikiMock = vi.hoisted(() => ({
  codeToTokens: vi.fn((code: string) => ({
    bg: 'transparent',
    fg: 'inherit',
    tokens: [[{ content: code, color: 'inherit' }]],
  })),
}))

vi.mock('shiki', () => ({
  createHighlighter: vi.fn(async () => ({
    getLoadedLanguages: () => ['typescript'],
    codeToTokens: shikiMock.codeToTokens,
  })),
}))

describe('highlightCode', () => {
  beforeEach(() => {
    shikiMock.codeToTokens.mockClear()
  })

  it('does not collide when equal-length code only differs outside the old prefix/suffix key', async () => {
    const { highlightCode } = await import('./code-block')
    const prefix = 'a'.repeat(100)
    const suffix = 'z'.repeat(100)
    const first = `${prefix}first${suffix}`
    const second = `${prefix}other${suffix}`

    const firstTokens = await resolveHighlight(highlightCode, first)
    const secondTokens = await resolveHighlight(highlightCode, second)

    expect(firstTokens.tokens[0][0].content).toBe(first)
    expect(secondTokens.tokens[0][0].content).toBe(second)
  })

  it('shares one in-flight tokenization for concurrent consumers of the same code', async () => {
    const { highlightCode } = await import('./code-block')
    const code = `const concurrent_${Date.now()} = true`

    const [first, second] = await Promise.all([
      resolveHighlight(highlightCode, code),
      resolveHighlight(highlightCode, code),
    ])

    expect(first).toEqual(second)
    expect(shikiMock.codeToTokens.mock.calls.filter(([value]) => value === code)).toHaveLength(1)
  })

  it('evicts old tokenized sources instead of growing for the lifetime of the app', async () => {
    const { highlightCode } = await import('./code-block')
    const first = `const oldest_${Date.now()} = true`
    await resolveHighlight(highlightCode, first)

    for (let index = 0; index < 70; index += 1) {
      await resolveHighlight(highlightCode, `const cache_entry_${Date.now()}_${index} = true`)
    }
    await resolveHighlight(highlightCode, first)

    expect(shikiMock.codeToTokens.mock.calls.filter(([value]) => value === first)).toHaveLength(2)
  })
})

type HighlightCode = typeof import('./code-block')['highlightCode']
type TokenizedCode = NonNullable<ReturnType<HighlightCode>>

function resolveHighlight(highlightCode: HighlightCode, code: string) {
  return new Promise<TokenizedCode>((resolve) => {
    const cached = highlightCode(code, 'typescript', resolve)
    if (cached) resolve(cached)
  })
}
