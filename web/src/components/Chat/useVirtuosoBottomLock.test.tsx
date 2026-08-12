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
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
    let rowBottom = 100
    const row = document.createElement('div')
    row.getBoundingClientRect = () => ({ bottom: rowBottom }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))

    act(() => {
      result.current.streamingRowRef(row)
      result.current.releaseBottomLock()
      result.current.onAtBottomStateChange(false)
      result.current.onAtBottomStateChange(true)
    })
    rowBottom = 130
    act(() => result.current.syncStreamingTailLayout())

    expect(scroller.scrollTop).toBe(400)
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

  it('cancels a pending automatic bottom follow when streaming ends before footer content mounts', () => {
    const scrollToIndex = vi.fn()
    const { result, rerender } = renderHook(
      ({ autoFollowEnabled }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled,
      }),
      { initialProps: { autoFollowEnabled: true } },
    )
    act(() => {
      result.current.virtuosoRef.current = { scrollToIndex } as unknown as VirtuosoHandle
    })

    rerender({ autoFollowEnabled: false })
    flushAnimationFrames(frames)

    expect(scrollToIndex).not.toHaveBeenCalled()
  })

  it('preserves the viewport when completed footer content changes the measured layout', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 700 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const { result, rerender } = renderHook(
      ({ autoFollowEnabled }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled,
        resolveScroller: () => scroller,
      }),
      { initialProps: { autoFollowEnabled: true } },
    )
    act(() => {
      result.current.onScroll({ currentTarget: scroller } as never)
    })

    // Virtuoso may correct the viewport while replacing the live row and
    // mounting the completed state footer in the same React commit.
    scroller.scrollTop = 310
    rerender({ autoFollowEnabled: false })

    expect(scroller.scrollTop).toBe(600)
  })

  it('does not mistake a layout-induced upward scroll for user intent', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
    scroller.scrollTop = 400
    let rowBottom = 100
    const row = document.createElement('div')
    row.getBoundingClientRect = () => ({ bottom: rowBottom }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))
    flushAnimationFrames(frames)
    act(() => result.current.streamingRowRef(row))

    scroller.scrollTop = 300
    act(() => {
      result.current.onScroll({ currentTarget: scroller } as never)
    })
    rowBottom = 130
    act(() => result.current.syncStreamingTailLayout())
    expect(scroller.scrollTop).toBe(330)

    act(() => {
      result.current.onWheel({ deltaY: -10 } as never)
    })
    rowBottom = 160
    act(() => result.current.syncStreamingTailLayout())
    expect(scroller.scrollTop).toBe(330)
  })

  it('unlocks from an explicit upward touch-scroll gesture', () => {
    const scroller = document.createElement('div')
    const content = document.createElement('div')
    scroller.appendChild(content)
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
    let rowBottom = 100
    const row = document.createElement('div')
    row.getBoundingClientRect = () => ({ bottom: rowBottom }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))

    act(() => {
      result.current.streamingRowRef(row)
      result.current.onPointerDown({
        target: content,
        currentTarget: scroller,
        pointerType: 'touch',
        clientY: 100,
      } as never)
      result.current.onPointerMove({ clientY: 116 } as never)
      result.current.onPointerUp()
    })
    rowBottom = 130
    act(() => result.current.syncStreamingTailLayout())

    expect(scroller.scrollTop).toBe(400)
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

  it('anchors a new stream directly without delegating to Virtuoso bottom following', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 390
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

    expect(scroller.scrollTop).toBe(400)
    expect(scrollToIndex).not.toHaveBeenCalled()
  })

  it('preserves an existing bottom lock when the first streamed batch changes layout before anchoring', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
    scroller.scrollTop = 400
    const { rerender } = renderHook(
      ({ autoFollowEnabled }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled,
        resolveScroller: () => scroller,
      }),
      { initialProps: { autoFollowEnabled: false } },
    )
    flushAnimationFrames(frames)

    // React can commit a multiline first batch before the streaming runway is
    // anchored. Geometry is temporarily away from the end even though the user
    // never expressed upward-scroll intent.
    scrollHeight = 560
    rerender({ autoFollowEnabled: true })

    expect(scroller.scrollTop).toBe(460)
  })

  it('allocates a viewport-relative runway for a new streaming response', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 1500 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 900 })
    setScrollerViewport(scroller, 900)
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      bottomInsetPx: 120,
      resolveScroller: () => scroller,
    }))

    expect(result.current.streamingSpacerPx).toBeCloseTo(576)
    expect(scroller.scrollTop).toBe(600)
  })

  it('removes the response runway and returns a locked viewport to the real bottom when output completes', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 1500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 900 })
    setScrollerViewport(scroller, 900)
    const { result, rerender } = renderHook(
      ({ autoFollowEnabled }) => useVirtuosoBottomLock({
        itemCount: 1,
        autoFollowEnabled,
        bottomInsetPx: 120,
        resolveScroller: () => scroller,
      }),
      { initialProps: { autoFollowEnabled: true } },
    )
    expect(result.current.streamingSpacerPx).toBeCloseTo(576)
    expect(scroller.scrollTop).toBe(600)

    scrollHeight = 1600
    rerender({ autoFollowEnabled: false })

    expect(result.current.streamingSpacerPx).toBeUndefined()

    // The virtualizer reports the total again after the temporary 576px
    // runway collapses back to the persistent 120px composer inset.
    scrollHeight = 1144
    act(() => result.current.syncIdleBottomLayout())
    expect(scroller.scrollTop).toBe(244)
  })

  it('does not restore idle bottom lock after explicit upward scrolling', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    scroller.scrollTop = 400
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: false,
      resolveScroller: () => scroller,
    }))
    flushAnimationFrames(frames)

    act(() => {
      result.current.onWheel({ deltaY: -20 } as never)
      scroller.scrollTop = 280
    })
    scrollHeight = 620
    act(() => result.current.syncIdleBottomLayout())

    expect(scroller.scrollTop).toBe(280)
  })

  it('restores a locked streaming viewport after its persistent tab becomes visible again', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
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
    setScrollerViewport(scroller, 100)
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

  it('retries multiline growth after Virtuoso publishes the remaining scroll range', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    let scrollTop = 400
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
    Object.defineProperty(scroller, 'scrollTop', {
      configurable: true,
      get: () => scrollTop,
      set: value => {
        scrollTop = Math.max(0, Math.min(value, scrollHeight - scroller.clientHeight))
      },
    })
    const streamingRow = document.createElement('div')
    let rowHeight = 40
    streamingRow.getBoundingClientRect = () => ({
      bottom: 60 + rowHeight - (scrollTop - 400),
      height: rowHeight,
    }) as DOMRect
    const { result } = renderHook(() => useVirtuosoBottomLock({
      itemCount: 1,
      autoFollowEnabled: true,
      resolveScroller: () => scroller,
    }))

    act(() => result.current.streamingRowRef(streamingRow))

    // Three rendered lines arrive together, but Virtuoso exposes only half of
    // their scroll range during the row's layout commit.
    rowHeight = 100
    scrollHeight = 530
    act(() => result.current.syncStreamingTailLayout())
    expect(scrollTop).toBe(430)

    // The row no longer changes size when Virtuoso publishes the remaining
    // range. The controller must still finish the unapplied compensation.
    scrollHeight = 560
    act(() => result.current.syncStreamingTailLayout())
    expect(scrollTop).toBe(460)
  })

  it('keeps the viewport follow target across a transient callback-ref release', () => {
    const scroller = document.createElement('div')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
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
    setScrollerViewport(scroller, 100)
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

  it('starts a fresh streaming runway when the message list reset key changes', () => {
    const scroller = document.createElement('div')
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, value: 800 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    setScrollerViewport(scroller, 100)
    scroller.scrollTop = 400
    const firstSessionRow = document.createElement('div')
    firstSessionRow.getBoundingClientRect = () => ({ bottom: 100 }) as DOMRect
    const nextSessionRow = document.createElement('div')
    nextSessionRow.getBoundingClientRect = () => ({ bottom: 100 }) as DOMRect
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

    expect(scroller.scrollTop).toBe(700)
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

function setScrollerViewport(scroller: HTMLElement, height: number) {
  scroller.getBoundingClientRect = () => ({ top: 0, bottom: height, height }) as DOMRect
}
