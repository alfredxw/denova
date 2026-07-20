import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useVirtuosoBottomLock } from './useVirtuosoBottomLock'

describe('useVirtuosoBottomLock', () => {
  it('does not relock from stale at-bottom callbacks after a direct content interaction', () => {
    const { result } = renderHook(() => useVirtuosoBottomLock({
      contentKey: 'turn-1',
      itemCount: 1,
    }))

    act(() => {
      result.current.releaseBottomLock()
      result.current.onAtBottomStateChange(false)
      result.current.onAtBottomStateChange(true)
    })

    expect(result.current.followOutput(true)).toBe(false)
  })
})
