import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSettingsMergePatch, fetchSettings, patchSettings } from './api'
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

  it('uses one partial-update endpoint for every writable layer', async () => {
    const saved = { effective: { theme: 'light' } } as LayeredSettings
    apiClientMocks.requestJSON.mockResolvedValueOnce(saved)

    await expect(patchSettings('user', { theme: 'light' }, 'user-r1')).resolves.toBe(saved)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith('/api/settings', {
      method: 'PATCH',
      headers: {},
      body: JSON.stringify({ layer: 'user', changes: { theme: 'light' }, base_revision: 'user-r1' }),
    })
  })
})

describe('createSettingsMergePatch', () => {
  it('sends only changed nested fields and represents removals as null', () => {
    const baseline = {
      theme: 'dark',
      openai_model: 'keep-model',
      agent_models: {
        ide: { profile_id: 'writer', thinking_level: 'medium' },
      },
    }
    const draft = {
      openai_model: 'keep-model',
      agent_models: {
        ide: { profile_id: 'writer', thinking_level: 'high' },
      },
    }

    expect(createSettingsMergePatch(baseline, draft)).toEqual({
      theme: null,
      agent_models: { ide: { thinking_level: 'high' } },
    })
  })

  it('replaces arrays instead of attempting index-level updates', () => {
    expect(createSettingsMergePatch(
      { model_profiles: [{ id: 'a' }, { id: 'b' }] },
      { model_profiles: [{ id: 'b' }] },
    )).toEqual({ model_profiles: [{ id: 'b' }] })
  })
})
