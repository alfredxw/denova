import { act, renderHook } from '@testing-library/react'
import type { VirtuosoHandle } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import { useVirtuosoBottomLock } from './useVirtuosoBottomLock'

describe('useVirtuosoBottomLock', () => {
  it('does not relock from stale at-bottom callbacks after a direct content interaction', () => {
    const { result } = renderHook(() => useVirtuosoBottomLock({
      contentKey: 'turn-1',
      itemCount: 1,
      autoFollowEnabled: true,
    }))

    act(() => {
      result.current.releaseBottomLock()
      result.current.onAtBottomStateChange(false)
      result.current.onAtBottomStateChange(true)
    })

    expect(result.current.followOutput(true)).toBe(false)
  })

  it('does not follow content growth while automatic following is disabled', () => {
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ contentKey }) => useVirtuosoBottomLock({
        contentKey,
        itemCount: 1,
        autoFollowEnabled: false,
      }),
      { initialProps: { contentKey: 'idle-1' } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
    })

    rerender({ contentKey: 'idle-2' })

    expect(scrollToIndex).not.toHaveBeenCalled()
  })

  it('still positions a newly populated list after an explicit reset while automatic following is disabled', () => {
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ contentKey, itemCount, resetKey }) => useVirtuosoBottomLock({
        resetKey,
        contentKey,
        itemCount,
        autoFollowEnabled: false,
      }),
      { initialProps: { contentKey: 'empty', itemCount: 0, resetKey: 'session-1' } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
    })

    rerender({ contentKey: 'loaded', itemCount: 1, resetKey: 'session-2' })

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 'LAST', align: 'end', behavior: 'auto' })
  })

  it('resumes following when streaming starts from an idle viewport that is at the bottom', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ autoFollowEnabled, contentKey }) => useVirtuosoBottomLock({
        contentKey,
        itemCount: 1,
        autoFollowEnabled,
        resolveScroller: () => scroller,
      }),
      { initialProps: { autoFollowEnabled: false, contentKey: 'idle' } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
      result.current.releaseBottomLock()
    })

    rerender({ autoFollowEnabled: true, contentKey: 'streaming' })

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 'LAST', align: 'end', behavior: 'auto' })
  })
})
