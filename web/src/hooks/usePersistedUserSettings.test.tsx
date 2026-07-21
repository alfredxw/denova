import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings, updateUserSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { usePersistedUserSettings } from './usePersistedUserSettings'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(async () => ({ id: 'settings-conflict', path: '/conflicts/settings-conflict.json', storage: 'server' as const })),
}))

const defaults = {
  ide_story_teller_id: 'classic',
  ide_image_preset_id: 'game-cg',
} as const

describe('usePersistedUserSettings', () => {
  beforeEach(() => {
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(updateUserSettings).mockReset()
    vi.mocked(preserveAutosaveConflict).mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
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

  it('updates optimistically but persists only after the edit delay', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const saved = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    vi.mocked(fetchSettings).mockResolvedValue(initial)
    vi.mocked(updateUserSettings).mockResolvedValue(saved)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let save!: Promise<boolean>
    act(() => { save = result.current.persist('ide_story_teller_id', 'slow-burn') })
    expect(result.current.values.ide_story_teller_id).toBe('slow-burn')

    await vi.advanceTimersByTimeAsync(999)
    expect(updateUserSettings).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    await act(async () => expect(await save).toBe(true))
    expect(updateUserSettings).toHaveBeenCalledOnce()
  })

  it('coalesces different pending preferences into one latest settings patch', async () => {
    let current = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    vi.mocked(fetchSettings).mockImplementation(async () => current)
    vi.mocked(updateUserSettings).mockImplementation(async (settings) => {
      current = snapshot({
        user: settings,
        effective: settings,
        revisions: { user: 'r2' },
      })
      return current
    })

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let tellerSave!: Promise<boolean>
    let presetSave!: Promise<boolean>
    act(() => {
      tellerSave = result.current.persist('ide_story_teller_id', 'slow-burn')
      presetSave = result.current.persist('ide_image_preset_id', 'cinematic')
    })

    expect(result.current.values).toEqual({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
      expect(await tellerSave).toBe(true)
      expect(await presetSave).toBe(true)
    })

    expect(fetchSettings).toHaveBeenCalledTimes(2)
    expect(updateUserSettings).toHaveBeenCalledOnce()
    expect(updateUserSettings).toHaveBeenCalledWith({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    }, 'r1')
  })

  it('keeps one request in flight and saves only the latest edit waiting behind it', async () => {
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
        current = snapshot({ user: settings, effective: settings, revisions: { user: 'r3' } })
        return current
      })

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let firstSave!: Promise<boolean>
    act(() => { firstSave = result.current.persist('ide_story_teller_id', 'slow-burn') })
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(updateUserSettings).toHaveBeenCalledOnce()

    let middleSave!: Promise<boolean>
    let latestSave!: Promise<boolean>
    act(() => {
      middleSave = result.current.persist('ide_story_teller_id', 'epic')
      latestSave = result.current.persist('ide_story_teller_id', 'minimal')
    })
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(updateUserSettings).toHaveBeenCalledOnce()

    current = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    await act(async () => {
      firstUpdate.resolve(current)
      expect(await firstSave).toBe(true)
      await vi.advanceTimersByTimeAsync(0)
    })

    expect(updateUserSettings).toHaveBeenCalledTimes(2)
    expect(updateUserSettings).toHaveBeenLastCalledWith({
      ide_story_teller_id: 'minimal',
      ide_image_preset_id: 'game-cg',
    }, 'r2')
    await act(async () => {
      expect(await middleSave).toBe(true)
      expect(await latestSave).toBe(true)
    })
    expect(preserveAutosaveConflict).not.toHaveBeenCalled()
  })

  it('still archives a different setting changed externally while its local edit waits', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const external = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r2' },
    })
    const firstSaved = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r3' },
    })
    const finalSaved = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'illustrated' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'illustrated' },
      revisions: { user: 'r4' },
    })
    let current = initial
    const firstUpdate = deferred<LayeredSettings>()
    vi.mocked(fetchSettings).mockImplementation(async () => current)
    vi.mocked(updateUserSettings)
      .mockImplementationOnce(() => firstUpdate.promise)
      .mockImplementationOnce(async () => finalSaved)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let firstOperation!: Promise<boolean>
    act(() => { firstOperation = result.current.persist('ide_story_teller_id', 'slow-burn') })
    current = external
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(updateUserSettings).toHaveBeenCalledOnce()

    let secondOperation!: Promise<boolean>
    act(() => { secondOperation = result.current.persist('ide_image_preset_id', 'illustrated') })
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    current = firstSaved
    await act(async () => {
      firstUpdate.resolve(firstSaved)
      expect(await firstOperation).toBe(true)
      expect(await secondOperation).toBe(true)
    })

    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      conflict_paths: [['ide_image_preset_id']],
      base: { revision: 'r1', value: expect.objectContaining({ ide_image_preset_id: 'game-cg' }) },
      external: { revision: 'r3', value: expect.objectContaining({ ide_image_preset_id: 'cinematic' }) },
    }))
  })

  it('keeps an after-delay user preference when the stable owner changes workspace', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const saved = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    vi.mocked(fetchSettings).mockResolvedValue(initial)
    vi.mocked(updateUserSettings).mockResolvedValue(saved)

    const { result, rerender } = renderHook(
      ({ workspace }) => usePersistedUserSettings({ workspace, defaults }),
      { initialProps: { workspace: '/book-a' } },
    )
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let operation!: Promise<boolean>
    act(() => { operation = result.current.persist('ide_story_teller_id', 'slow-burn') })
    rerender({ workspace: '/book-b' })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
      expect(await operation).toBe(true)
    })

    expect(updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({ ide_story_teller_id: 'slow-burn' }), 'r1')
    expect(result.current.values.ide_story_teller_id).toBe('slow-burn')
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
    vi.useFakeTimers()

    let save!: Promise<boolean>
    act(() => {
      save = result.current.persist('ide_story_teller_id', 'slow-burn')
    })
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(updateUserSettings).toHaveBeenCalledOnce()

    current = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'cinematic' },
      revisions: { user: 'r2' },
    })
    await act(async () => {
      window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'settings-page' } }))
      await Promise.resolve()
    })

    expect(result.current.values).toEqual({
      ide_story_teller_id: 'slow-burn',
      ide_image_preset_id: 'cinematic',
    })

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
    vi.useFakeTimers()

    let save!: Promise<boolean>
    act(() => { save = result.current.persist('ide_story_teller_id', 'slow-burn') })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
      expect(await save).toBe(true)
    })

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

  it('archives a same-key external edit detected during the delay before applying the local preference', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const external = snapshot({
      user: { ide_story_teller_id: 'epic' },
      effective: { ide_story_teller_id: 'epic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    const saved = snapshot({
      user: { ide_story_teller_id: 'slow-burn' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r3' },
    })
    vi.mocked(fetchSettings).mockResolvedValueOnce(initial).mockResolvedValueOnce(external)
    vi.mocked(updateUserSettings).mockResolvedValue(saved)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let operation!: Promise<boolean>
    act(() => { operation = result.current.persist('ide_story_teller_id', 'slow-burn') })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
      expect(await operation).toBe(true)
    })

    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'settings',
      scope: '/book:user',
      id: 'user',
      base: { revision: 'r1', value: expect.objectContaining({ ide_story_teller_id: 'classic' }) },
      local: { revision: 'r1', value: expect.objectContaining({ ide_story_teller_id: 'slow-burn' }) },
      external: { revision: 'r2', value: expect.objectContaining({ ide_story_teller_id: 'epic' }) },
      merged: { revision: 'r2', value: expect.objectContaining({ ide_story_teller_id: 'slow-burn' }) },
      conflict_paths: [['ide_story_teller_id']],
    }))
    expect(updateUserSettings).toHaveBeenCalledWith({ ide_story_teller_id: 'slow-burn' }, 'r2')
  })

  it('does not overwrite or clear a local preference when conflict preservation fails', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const external = snapshot({
      user: { ide_story_teller_id: 'epic' },
      effective: { ide_story_teller_id: 'epic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    const saved = snapshot({
      user: { ide_story_teller_id: 'slow-burn' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r3' },
    })
    vi.mocked(fetchSettings).mockResolvedValueOnce(initial).mockResolvedValue(external)
    vi.mocked(preserveAutosaveConflict)
      .mockRejectedValueOnce(new Error('archive offline'))
      .mockResolvedValueOnce({ id: 'settings-conflict', path: '/conflicts/settings-conflict.json', storage: 'server' })
    vi.mocked(updateUserSettings).mockResolvedValue(saved)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let operation!: Promise<boolean>
    let settled = false
    act(() => { operation = result.current.persist('ide_story_teller_id', 'slow-burn') })
    void operation.then(() => { settled = true })
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })

    expect(settled).toBe(false)
    expect(result.current.values.ide_story_teller_id).toBe('slow-burn')
    expect(result.current.isSaving('ide_story_teller_id')).toBe(true)
    expect(updateUserSettings).not.toHaveBeenCalled()

    await act(async () => {
      expect(await result.current.flushPending()).toBe(true)
      expect(await operation).toBe(true)
    })
    expect(preserveAutosaveConflict).toHaveBeenCalledTimes(2)
    expect(updateUserSettings).toHaveBeenCalledWith({ ide_story_teller_id: 'slow-burn' }, 'r2')
    warn.mockRestore()
  })

  it('keeps a failed delayed preference pending and retries it without rolling back the local value', async () => {
    const initial = snapshot({
      user: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'classic', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r1' },
    })
    const saved = snapshot({
      user: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      effective: { ide_story_teller_id: 'slow-burn', ide_image_preset_id: 'game-cg' },
      revisions: { user: 'r2' },
    })
    vi.mocked(fetchSettings).mockResolvedValue(initial)
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(saved)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)

    const { result } = renderHook(() => usePersistedUserSettings({ workspace: '/book', defaults }))
    await waitFor(() => expect(result.current.loading).toBe(false))
    vi.useFakeTimers()

    let save!: Promise<boolean>
    let settled = false
    act(() => { save = result.current.persist('ide_story_teller_id', 'slow-burn') })
    void save.then(() => { settled = true })
    expect(result.current.isSaving('ide_story_teller_id')).toBe(true)
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })

    expect(settled).toBe(false)
    expect(result.current.values.ide_story_teller_id).toBe('slow-burn')
    expect(result.current.isSaving('ide_story_teller_id')).toBe(true)

    await act(async () => {
      expect(await result.current.flushPending()).toBe(true)
      expect(await save).toBe(true)
    })
    expect(updateUserSettings).toHaveBeenCalledTimes(2)
    expect(result.current.isSaving('ide_story_teller_id')).toBe(false)
    warn.mockRestore()
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
