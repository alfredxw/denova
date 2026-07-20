import { act, renderHook } from '@testing-library/react'
import type { VirtuosoHandle } from 'react-virtuoso'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useVirtuosoBottomLock } from './useVirtuosoBottomLock'

describe('useVirtuosoBottomLock', () => {
  let frames: FrameRequestCallback[]

  beforeEach(() => {
    frames = []
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    }))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
  })

  afterEach(() => vi.unstubAllGlobals())

  it('does not relock from stale at-bottom callbacks after a direct content interaction', () => {
    const { result } = renderHook(() => useVirtuosoBottomLock({
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
      ({ itemCount }) => useVirtuosoBottomLock({
        itemCount,
        autoFollowEnabled: false,
      }),
      { initialProps: { itemCount: 1 } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
    })
    flushAnimationFrames(frames)
    scrollToIndex.mockClear()

    rerender({ itemCount: 2 })
    flushAnimationFrames(frames)

    expect(scrollToIndex).not.toHaveBeenCalled()
  })

  it('still positions a newly populated list after an explicit reset while automatic following is disabled', () => {
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ itemCount, resetKey }) => useVirtuosoBottomLock({
        resetKey,
        itemCount,
        autoFollowEnabled: false,
      }),
      { initialProps: { itemCount: 0, resetKey: 'session-1' } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
    })

    rerender({ itemCount: 1, resetKey: 'session-2' })
    flushAnimationFrames(frames)

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 'LAST', align: 'end', behavior: 'auto' })
  })

  it('resumes following when streaming starts from an idle viewport that is at the bottom', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ autoFollowEnabled }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled,
        resolveScroller: () => scroller,
      }),
      { initialProps: { autoFollowEnabled: false } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
      result.current.releaseBottomLock()
    })

    rerender({ autoFollowEnabled: true })
    flushAnimationFrames(frames)

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 'LAST', align: 'end', behavior: 'auto' })
  })

  it('translates relative navigation targets into the virtualizer absolute index window', () => {
    const scrollToIndex = vi.fn()
    const { result } = renderHook(() => useVirtuosoBottomLock({
      firstItemIndex: 1_000_000,
      itemCount: 3,
      autoFollowEnabled: false,
    }))
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
      result.current.scrollToIndex(1, { align: 'start', behavior: 'smooth' })
    })

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 1_000_001, align: 'start', behavior: 'smooth' })
  })
})

function flushAnimationFrames(frames: FrameRequestCallback[]) {
  act(() => {
    const callbacks = frames.splice(0)
    callbacks.forEach((callback) => callback(0))
  })
}
