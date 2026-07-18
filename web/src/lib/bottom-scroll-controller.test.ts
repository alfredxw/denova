import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDeferredBottomScrollScheduler, isElementNearBottom } from './bottom-scroll-controller'

describe('bottom scroll controller', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('measures distance from the visible bottom', () => {
    const element = { scrollHeight: 400, scrollTop: 288, clientHeight: 100 } as HTMLElement
    expect(isElementNearBottom(element, 12)).toBe(true)
    element.scrollTop = 280
    expect(isElementNearBottom(element, 12)).toBe(false)
  })

  it('runs immediately, across two frames, and once after layout settles', () => {
    vi.useFakeTimers()
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    }))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const scrollNow = vi.fn()
    const scheduler = createDeferredBottomScrollScheduler()

    scheduler.schedule(scrollNow, () => true)
    expect(scrollNow).toHaveBeenCalledTimes(1)
    frames.shift()?.(0)
    frames.shift()?.(0)
    expect(scrollNow).toHaveBeenCalledTimes(3)
    vi.advanceTimersByTime(80)
    expect(scrollNow).toHaveBeenCalledTimes(4)
  })

  it('cancels deferred work when the lock is released', () => {
    vi.useFakeTimers()
    const frames: FrameRequestCallback[] = []
    const cancelFrame = vi.fn()
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    }))
    vi.stubGlobal('cancelAnimationFrame', cancelFrame)
    const scrollNow = vi.fn()
    const scheduler = createDeferredBottomScrollScheduler()

    scheduler.schedule(scrollNow, () => true)
    scheduler.cancel()
    vi.advanceTimersByTime(80)

    expect(cancelFrame).toHaveBeenCalledWith(1)
    expect(scrollNow).toHaveBeenCalledTimes(1)
  })
})
