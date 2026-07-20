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

  it('coalesces bottom scrolling into one animation-frame write', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    }))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const scrollNow = vi.fn()
    const scheduler = createDeferredBottomScrollScheduler()

    scheduler.schedule(scrollNow, () => true)
    expect(scrollNow).not.toHaveBeenCalled()
    frames.shift()?.(0)
    expect(scrollNow).toHaveBeenCalledTimes(1)
  })

  it('keeps only the latest pending scroll request in the same frame', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    }))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const first = vi.fn()
    const latest = vi.fn()
    const scheduler = createDeferredBottomScrollScheduler()

    scheduler.schedule(first, () => true)
    scheduler.schedule(latest, () => true)
    frames.forEach((callback) => callback(0))

    expect(first).not.toHaveBeenCalled()
    expect(latest).toHaveBeenCalledTimes(1)
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
    frames.shift()?.(0)

    expect(cancelFrame).toHaveBeenCalledWith(1)
    expect(scrollNow).not.toHaveBeenCalled()
  })
})
