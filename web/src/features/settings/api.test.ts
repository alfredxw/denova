import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSettingsMergePatch, fetchSettings, invalidateSettingsCache, patchSettings, refreshSettings } from './api'
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
  beforeEach(() => {
    invalidateSettingsCache()
    apiClientMocks.requestJSON.mockReset()
  })

  it('shares concurrent and completed reads until the snapshot is refreshed', async () => {
    const firstSnapshot = { effective: { language: 'zh-CN' } } as LayeredSettings
    apiClientMocks.requestJSON.mockResolvedValueOnce(firstSnapshot)

    const first = fetchSettings()
    const concurrent = fetchSettings()

    expect(concurrent).toBe(first)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(1)
    await expect(Promise.all([first, concurrent])).resolves.toEqual([firstSnapshot, firstSnapshot])

    await expect(fetchSettings()).resolves.toBe(firstSnapshot)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(1)

    const refreshed = { effective: { language: 'en-US' } } as LayeredSettings
    apiClientMocks.requestJSON.mockResolvedValueOnce(refreshed)
    const refresh = refreshSettings()
    expect(refreshSettings()).toBe(refresh)
    await expect(refresh).resolves.toBe(refreshed)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(2)
  })

  it('starts a newer read when a later update arrives before the previous refresh finishes', async () => {
    let resolveFirst!: (value: LayeredSettings) => void
    const firstRefresh = new Promise<LayeredSettings>((resolve) => { resolveFirst = resolve })
    const latestSnapshot = { effective: { language: 'en-US' } } as LayeredSettings
    apiClientMocks.requestJSON
      .mockReturnValueOnce(firstRefresh)
      .mockResolvedValueOnce(latestSnapshot)

    const staleRequest = refreshSettings()
    await Promise.resolve()
    const latestRequest = refreshSettings()

    expect(latestRequest).not.toBe(staleRequest)
    await expect(latestRequest).resolves.toBe(latestSnapshot)
    resolveFirst({ effective: { language: 'zh-CN' } } as LayeredSettings)
    await staleRequest
    await expect(fetchSettings()).resolves.toBe(latestSnapshot)
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
    await expect(fetchSettings()).resolves.toBe(saved)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(1)
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
