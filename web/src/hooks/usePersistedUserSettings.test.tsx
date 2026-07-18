import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings, updateUserSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { APIError } from '@/lib/api-client'
import { usePersistedUserSettings } from './usePersistedUserSettings'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}))

const defaults = {
  ide_story_teller_id: 'classic',
  ide_image_preset_id: 'game-cg',
} as const

describe('usePersistedUserSettings', () => {
  beforeEach(() => {
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(updateUserSettings).mockReset()
  })

  it('loads all configured values from one settings snapshot', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(snapshot({
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
    }))

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.values).toEqual({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    })
    expect(fetchSettings).toHaveBeenCalledOnce()
  })

  it('serializes different setting writes and rebases each patch on the latest revision', async () => {
    let current = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const firstUpdate = deferred<LayeredSettings>()
    vi.mocked(fetchSettings).mockImplementation(async () => current)
    vi.mocked(updateUserSettings)
      .mockImplementationOnce(() => firstUpdate.promise)
      .mockImplementationOnce(async (settings) => {
        current = snapshot({
          user: settings,
          effective: settings,
          revisions: { user: 'r3' },
        })
        return current
      })

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))

    let tellerSave!: Promise<boolean>
    let presetSave!: Promise<boolean>
    act(() => {
      tellerSave = result.current.persist('ide_story_teller_id', 'slow-burn')
      presetSave = result.current.persist('ide_image_preset_id', 'cinematic')
    })

    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1))
    expect(fetchSettings).toHaveBeenCalledTimes(2)
    expect(result.current.values).toEqual({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    })

    current = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    await act(async () => {
      firstUpdate.resolve(current)
      expect(await tellerSave).toBe(true)
    })

    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(2))
    expect(fetchSettings).toHaveBeenCalledTimes(3)
    expect(updateUserSettings).toHaveBeenNthCalledWith(2, {
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    }, 'r2')
    await act(async () => expect(await presetSave).toBe(true))
  })

  it('keeps optimistic values while an external reload arrives during a save', async () => {
    let current = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const pendingUpdate = deferred<LayeredSettings>()
    vi.mocked(fetchSettings).mockImplementation(async () => current)
    vi.mocked(updateUserSettings).mockImplementation(() => pendingUpdate.promise)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))

    let save!: Promise<boolean>
    act(() => {
      save = result.current.persist('ide_story_teller_id', 'slow-burn')
    })
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledOnce())

    current = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r2' },
    })
    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'settings-page' } })))

    await waitFor(() => expect(result.current.values).toEqual({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    }))

    current = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r3' },
    })
    await act(async () => {
      pendingUpdate.resolve(current)
      expect(await save).toBe(true)
    })
    expect(result.current.values.ide_story_teller_id).toBe('slow-burn')
  })

  it('rebases and retries a setting write after a revision conflict', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const latest = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r2' },
    })
    const saved = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r3' },
    })
    vi.mocked(fetchSettings)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(latest)
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce(saved)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))

    let save!: Promise<boolean>
    act(() => { save = result.current.persist('ide_story_teller_id', 'slow-burn') })
    await act(async () => expect(await save).toBe(true))

    expect(updateUserSettings).toHaveBeenNthCalledWith(1, {
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'game-cg',
    }, 'r1')
    expect(updateUserSettings).toHaveBeenNthCalledWith(2, {
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    }, 'r2')
    expect(result.current.values).toEqual({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    })
  })
})

function snapshot(patch: Partial<LayeredSettings>): LayeredSettings {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective: {},
    paths: {
      denova_dir: '',
      nova_dir: '',
      user_config: '',
      workspace_config: '',
    },
    ...patch,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
