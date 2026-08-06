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

  it('restores a locked streaming viewport after its persistent tab becomes visible again', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ visible }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled: true,
        visible,
        resolveScroller: () => scroller,
      }),
      { initialProps: { visible: true } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
    })
    flushAnimationFrames(frames)
    scrollToIndex.mockClear()

    rerender({ visible: false })
    scrollHeight = 680
    rerender({ visible: true })
    flushAnimationFrames(frames)

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 'LAST', align: 'end', behavior: 'auto' })
  })

  it('preserves a manual scroll-away while a persistent tab is hidden and restored', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 680 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 300
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ visible }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled: true,
        visible,
        resolveScroller: () => scroller,
      }),
      { initialProps: { visible: true } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
      result.current.releaseBottomLock()
    })
    flushAnimationFrames(frames)
    scrollToIndex.mockClear()

    rerender({ visible: false })
    rerender({ visible: true })
    act(() => result.current.onAtBottomStateChange(true))
    flushAnimationFrames(frames)

    expect(scrollToIndex).not.toHaveBeenCalled()
    expect(result.current.followOutput(true)).toBe(false)
  })

  it('synchronously compensates a committed streaming row height change before browser measurement', () => {
    let observerCallback: ResizeObserverCallback | undefined
    const observe = vi.fn()
    vi.stubGlobal('ResizeObserver', class ResizeObserverMock {
      constructor(callback: ResizeObserverCallback) {
        observerCallback = callback
      }

      observe = observe
      unobserve = vi.fn()
      disconnect = vi.fn()
    })
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const streamingRow = document.createElement('div')
    let rowHeight = 40
    streamingRow.getBoundingClientRect = () => ({
      bottom: 60 + rowHeight - (scroller.scrollTop - 400),
      height: rowHeight,
    }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))

    act(() => result.current.streamingRowRef(streamingRow))
    expect(observe).toHaveBeenCalledWith(streamingRow)

    rowHeight = 70
    scrollHeight = 530
    act(() => result.current.syncStreamingTailLayout())

    expect(scroller.scrollTop).toBe(430)

    act(() => result.current.releaseBottomLock())
    rowHeight = 100
    scrollHeight = 560
    act(() => observerCallback?.([{ target: streamingRow } as unknown as ResizeObserverEntry], {} as ResizeObserver))

    expect(scroller.scrollTop).toBe(430)
  })

  it('keeps the height baseline across a transient callback-ref release of the same row', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const streamingRow = document.createElement('div')
    let rowHeight = 40
    streamingRow.getBoundingClientRect = () => ({
      bottom: 60 + rowHeight - (scroller.scrollTop - 400),
      height: rowHeight,
    }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))

    act(() => result.current.streamingRowRef(streamingRow))
    act(() => result.current.streamingRowRef(null))
    rowHeight = 70
    scrollHeight = 530
    act(() => {
      result.current.streamingRowRef(streamingRow)
      result.current.syncStreamingTailLayout()
    })

    expect(scroller.scrollTop).toBe(430)
  })

  it('preserves the bottom anchor when the streaming tail row is replaced', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
    scroller.scrollTop = 400
    const activityRow = document.createElement('div')
    activityRow.getBoundingClientRect = () => ({ bottom: 100, height: 28 }) as DOMRect
    const thinkingRow = document.createElement('div')
    thinkingRow.getBoundingClientRect = () => ({ bottom: 170, height: 98 }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))

    act(() => result.current.streamingRowRef(activityRow))
    scrollHeight = 570
    act(() => {
      result.current.streamingRowRef(null)
      result.current.streamingRowRef(thinkingRow)
      result.current.syncStreamingTailLayout()
    })

    expect(scroller.scrollTop).toBe(470)
  })

  it('starts a fresh tail anchor when the message list reset key changes', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 800 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
    scroller.scrollTop = 400
    const firstSessionRow = document.createElement('div')
    firstSessionRow.getBoundingClientRect = () => ({ bottom: 100 }) as DOMRect
    const nextSessionRow = document.createElement('div')
    nextSessionRow.getBoundingClientRect = () => ({ bottom: 300 }) as DOMRect
    const { result, rerender } = renderHook(
      ({ resetKey }) => useVirtuosoBottomLock({
        resetKey,
        itemCount: 1,
        autoFollowEnabled: true,
        resolveScroller: () => scroller,
      }),
      { initialProps: { resetKey: 'session-a' } },
    )

    act(() => result.current.streamingRowRef(firstSessionRow))
    rerender({ resetKey: 'session-b' })
    act(() => {
      result.current.streamingRowRef(nextSessionRow)
      result.current.syncStreamingTailLayout()
    })

    expect(scroller.scrollTop).toBe(400)
  })

  it('keeps imperative navigation relative to the current data', () => {
    const scrollToIndex = vi.fn()
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 3,
      autoFollowEnabled: false,
    }))
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
      result.current.scrollToIndex(1, { align: 'start', behavior: 'smooth' })
    })

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 1, align: 'start', behavior: 'smooth' })
  })
})

function flushAnimationFrames(frames: FrameRequestCallback[]) {
  act(() => {
    const callbacks = frames.splice(0)
    callbacks.forEach((callback) => callback(0))
  })
}
