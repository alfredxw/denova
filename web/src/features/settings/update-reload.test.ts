import { describe, expect, it, vi } from 'vitest'
import { buildFrontendReloadURL, scheduleFrontendReloadAfterUpdate, UPDATE_RELOAD_POLL_INTERVAL_MS } from './update-reload'

describe('update frontend reload', () => {
  it('builds a cache-busting reload URL without dropping the current path or query', () => {
    const url = buildFrontendReloadURL('v0.2.0 beta', 123, 'http://localhost:8080/settings?section=updates')

    expect(url).toBe('http://localhost:8080/settings?section=updates&denova_reload=v0.2.0-beta-123')
  })

  it('waits for the restarted backend before reloading the page', async () => {
    const timers: Array<() => void> = []
    const reload = vi.fn()
    const pollBackend = vi.fn()
      .mockResolvedValueOnce(false)
      .mockRejectedValueOnce(new Error('backend restarting'))
      .mockResolvedValueOnce(true)

    scheduleFrontendReloadAfterUpdate('0.2.0', {
      pollBackend,
      reload,
      now: () => 456,
      currentHref: 'http://localhost:8080/settings',
      setTimer: (handler, delay) => {
        expect(delay).toBe(UPDATE_RELOAD_POLL_INTERVAL_MS)
        timers.push(handler)
      },
    })

    expect(timers).toHaveLength(1)

    timers.shift()?.()
    await flushMicrotasks()
    expect(reload).not.toHaveBeenCalled()
    expect(timers).toHaveLength(1)

    timers.shift()?.()
    await flushMicrotasks()
    expect(reload).not.toHaveBeenCalled()
    expect(timers).toHaveLength(1)

    timers.shift()?.()
    await flushMicrotasks()
    expect(reload).toHaveBeenCalledWith('http://localhost:8080/settings?denova_reload=0.2.0-456')
    expect(pollBackend).toHaveBeenCalledTimes(3)
  })
})

async function flushMicrotasks() {
  await Promise.resolve()
  await Promise.resolve()
}
