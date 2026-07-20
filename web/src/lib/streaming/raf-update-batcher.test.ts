import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRafUpdateBatcher } from './raf-update-batcher'

describe('createRafUpdateBatcher', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('commits queued state updates once with their original order', () => {
    const scheduledFrames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      scheduledFrames.push(callback)
      return 1
    }))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    let value = 1
    const commits: number[] = []
    const batcher = createRafUpdateBatcher<number>((update) => {
      value = update(value)
      commits.push(value)
    })

    batcher.enqueue(current => current + 1)
    batcher.enqueue(current => current * 3)

    expect(commits).toEqual([])
    expect(scheduledFrames).toHaveLength(1)
    scheduledFrames[0](0)
    expect(commits).toEqual([6])
  })

  it('flushes pending updates immediately at a stream boundary', () => {
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 7))
    const cancelAnimationFrame = vi.fn()
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)
    let value = ''
    const batcher = createRafUpdateBatcher<string>((update) => {
      value = update(value)
    })

    batcher.enqueue(current => `${current}first`)
    batcher.enqueue(current => `${current}-last`)
    batcher.flush()

    expect(value).toBe('first-last')
    expect(cancelAnimationFrame).toHaveBeenCalledWith(7)
  })

  it('discards pending updates when their owner is disposed', () => {
    const scheduledFrames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
      scheduledFrames.push(callback)
      return 9
    }))
    const cancelAnimationFrame = vi.fn()
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)
    let value = 'stable'
    const batcher = createRafUpdateBatcher<string>((update) => {
      value = update(value)
    })

    batcher.enqueue(() => 'stale')
    batcher.discard()
    scheduledFrames[0](0)

    expect(value).toBe('stable')
    expect(cancelAnimationFrame).toHaveBeenCalledWith(9)
  })
})
