import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useIsMobile } from './useIsMobile'

describe('useIsMobile', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it.each([
    [390, true],
    [768, true],
    [800, true],
    [900, true],
    [1023, true],
    [1024, false],
    [1488, false],
  ])('uses the compact workspace at a %ipx viewport: %s', (width, expected) => {
    const matchMedia = vi.fn((query: string) => createMediaQueryList(query, width))
    vi.stubGlobal('matchMedia', matchMedia)

    const { result } = renderHook(() => useIsMobile())

    expect(result.current).toBe(expected)
    expect(matchMedia).toHaveBeenCalledWith('(max-width: 1023px)')
  })
})

function createMediaQueryList(query: string, width: number): MediaQueryList {
  return {
    matches: query === '(max-width: 1023px)' && width <= 1023,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }
}
