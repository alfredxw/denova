import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings } from './api'
import type { LayeredSettings } from './types'

const apiClientMocks = vi.hoisted(() => ({ requestJSON: vi.fn() }))

vi.mock('@/lib/api-client', () => ({
  fetchAPI: vi.fn(),
  jsonHeaders: {},
  parseSSEStream: vi.fn(),
  readErrorMessage: vi.fn(),
  requestJSON: apiClientMocks.requestJSON,
}))

describe('settings API request coalescing', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('shares concurrent reads but refreshes after the request settles', async () => {
    const firstSnapshot = { effective: { language: 'zh-CN' } } as LayeredSettings
    apiClientMocks.requestJSON.mockResolvedValueOnce(firstSnapshot)

    const first = fetchSettings()
    const concurrent = fetchSettings()

    expect(concurrent).toBe(first)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(1)
    await expect(Promise.all([first, concurrent])).resolves.toEqual([firstSnapshot, firstSnapshot])

    const refreshed = { effective: { language: 'en-US' } } as LayeredSettings
    apiClientMocks.requestJSON.mockResolvedValueOnce(refreshed)
    await expect(fetchSettings()).resolves.toBe(refreshed)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(2)
  })
})
