import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import type { LayeredSettings, Settings, SettingsLayer } from './types'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(async () => ({ id: 'conflict-1', path: '/conflicts/conflict-1.json', storage: 'server' as const })),
}))

describe('useLayeredSettingsDraft', () => {
  beforeEach(() => {
    vi.mocked(preserveAutosaveConflict).mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('applies a pending load to the latest layer after a layer switch', async () => {
    const pending = deferred<LayeredSettings>()
    const loadSettings = vi.fn(() => pending.promise)
    const { result, rerender } = renderHook(
      ({ layer }: { layer: SettingsLayer }) => useLayeredSettingsDraft({
        layer,
        sourcePrefix: 'test-settings',
        loadSettings,
        saveUserSettings: vi.fn(),
        saveWorkspaceSettings: vi.fn(),
      }),
      { initialProps: { layer: 'user' as SettingsLayer } },
    )

    rerender({ layer: 'workspace' })
    await act(async () => {
      pending.resolve(snapshot({
        user: { language: 'zh-CN' },
        workspace: { language: 'en-US' },
      }))
      await pending.promise
    })

    await waitFor(() => expect(result.current.draft).toEqual({ language: 'en-US' }))
    expect(loadSettings).toHaveBeenCalledTimes(1)
  })

  it('ignores its own update event and reloads external updates', async () => {
    const loadSettings = vi.fn()
      .mockResolvedValueOnce(snapshot({ user: { theme: 'dark' } }))
      .mockResolvedValueOnce(snapshot({ user: { theme: 'light' } }))
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings: vi.fn(),
      saveWorkspaceSettings: vi.fn(),
    }))

    await waitFor(() => expect(result.current.draft).toEqual({ theme: 'dark' }))

    act(() => result.current.notifyUpdated())
    expect(loadSettings).toHaveBeenCalledTimes(1)

    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'another-view' } })))
    await waitFor(() => expect(result.current.draft).toEqual({ theme: 'light' }))
    expect(loadSettings).toHaveBeenCalledTimes(2)
  })

  it('saves each layer with its own revision', async () => {
    const initial = snapshot({
      user: { language: 'zh-CN' },
      workspace: { theme: 'dark' },
      revisions: { user: 'user-r1', workspace: 'workspace-r1' },
    })
    const saveUserSettings = vi.fn(async (settings: Settings) => snapshot({ ...initial, user: settings }))
    const saveWorkspaceSettings = vi.fn(async (settings: Settings) => snapshot({ ...initial, workspace: settings }))
    const loadSettings = vi.fn(async () => initial)
    const { result, rerender } = renderHook(
      ({ layer }: { layer: SettingsLayer }) => useLayeredSettingsDraft({
        layer,
        sourcePrefix: 'test-settings',
        loadSettings,
        saveUserSettings,
        saveWorkspaceSettings,
      }),
      { initialProps: { layer: 'user' as SettingsLayer } },
    )

    await waitFor(() => expect(result.current.draft).toEqual({ language: 'zh-CN' }))
    act(() => result.current.setDraft({ language: 'en-US' }))
    await act(async () => { await result.current.saveNow() })
    expect(saveUserSettings).toHaveBeenCalledWith({ language: 'en-US' }, 'user-r1')

    rerender({ layer: 'workspace' })
    await waitFor(() => expect(result.current.draft).toEqual({ theme: 'dark' }))
    act(() => result.current.setDraft({ theme: 'light' }))
    await act(async () => { await result.current.saveNow() })
    expect(saveWorkspaceSettings).toHaveBeenCalledWith({ theme: 'light' }, 'workspace-r1')
  })

  it('keeps a dirty user draft when switching layers and saves it through its own lane', async () => {
    const initial = snapshot({
      user: { language: 'zh-CN' },
      workspace: { theme: 'dark' },
      revisions: { user: 'user-r1', workspace: 'workspace-r1' },
    })
    const saveUserSettings = vi.fn(async (settings: Settings) => snapshot({
      ...initial,
      user: settings,
      revisions: { user: 'user-r2', workspace: 'workspace-r1' },
    }))
    const loadSettings = vi.fn(async () => initial)
    const saveWorkspaceSettings = vi.fn()
    const { result, rerender } = renderHook(
      ({ layer }: { layer: SettingsLayer }) => useLayeredSettingsDraft({
        layer,
        sourcePrefix: 'test-settings',
        loadSettings,
        saveUserSettings,
        saveWorkspaceSettings,
      }),
      { initialProps: { layer: 'user' as SettingsLayer } },
    )
    await waitFor(() => expect(result.current.draft).toEqual({ language: 'zh-CN' }))
    vi.useFakeTimers()

    act(() => result.current.setDraft({ language: 'en-US' }))
    rerender({ layer: 'workspace' })
    expect(result.current.draft).toEqual({ theme: 'dark' })
    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })

    expect(saveUserSettings).toHaveBeenCalledWith({ language: 'en-US' }, 'user-r1')
    rerender({ layer: 'user' })
    expect(result.current.draft).toEqual({ language: 'en-US' })
  })

  it('applies an in-flight user save that finishes while the workspace layer is active', async () => {
    const initial = snapshot({
      user: { language: 'zh-CN' },
      workspace: { theme: 'dark' },
      revisions: { user: 'user-r1', workspace: 'workspace-r1' },
    })
    const pendingSave = deferred<LayeredSettings>()
    const saveUserSettings = vi.fn(() => pendingSave.promise)
    const loadSettings = vi.fn(async () => initial)
    const saveWorkspaceSettings = vi.fn()
    const { result, rerender } = renderHook(
      ({ layer }: { layer: SettingsLayer }) => useLayeredSettingsDraft({
        layer,
        sourcePrefix: 'test-settings',
        loadSettings,
        saveUserSettings,
        saveWorkspaceSettings,
      }),
      { initialProps: { layer: 'user' as SettingsLayer } },
    )
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))

    act(() => result.current.setDraft({ language: 'en-US' }))
    let savePromise!: Promise<LayeredSettings | null>
    act(() => { savePromise = result.current.saveNow() })
    await waitFor(() => expect(saveUserSettings).toHaveBeenCalledOnce())
    rerender({ layer: 'workspace' })
    expect(result.current.draft).toEqual(initial.workspace)

    const saved = snapshot({
      ...initial,
      user: { language: 'en-US' },
      revisions: { user: 'user-r2', workspace: 'workspace-r1' },
    })
    await act(async () => {
      pendingSave.resolve(saved)
      await savePromise
    })
    rerender({ layer: 'user' })

    expect(result.current.draft).toEqual({ language: 'en-US' })
  })

  it('treats its own save response as an acknowledgement while a newer draft waits', async () => {
    const initial = snapshot({
      user: { model_profiles: [{ id: 'default', openai_model: 'model-a' }] },
      revisions: { user: 'r1' },
    })
    const firstSave = deferred<LayeredSettings>()
    const secondSave = deferred<LayeredSettings>()
    const loadSettings = vi.fn(async () => initial)
    const saveUserSettings = vi.fn()
      .mockImplementationOnce(() => firstSave.promise)
      .mockImplementationOnce(() => secondSave.promise)
    const { result, unmount } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings,
      saveWorkspaceSettings: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))
    vi.useFakeTimers()

    act(() => result.current.setDraft({ model_profiles: [{ id: 'default', openai_model: 'model-b' }] }))
    act(() => { vi.advanceTimersByTime(1100) })
    expect(saveUserSettings).toHaveBeenCalledOnce()

    act(() => result.current.setDraft({ model_profiles: [{ id: 'default', openai_model: 'model-c' }] }))
    const firstSaved = snapshot({
      user: { model_profiles: [{ id: 'default', openai_model: 'model-b' }] },
      revisions: { user: 'r2' },
    })
    await act(async () => {
      firstSave.resolve(firstSaved)
      await firstSave.promise
      await Promise.resolve()
    })

    const conflictCallCount = vi.mocked(preserveAutosaveConflict).mock.calls.length
    const currentDraft = result.current.draft
    act(() => { vi.advanceTimersByTime(1100) })
    const queuedSave = saveUserSettings.mock.calls[1]
    unmount()

    expect(conflictCallCount).toBe(0)
    expect(queuedSave).toEqual([{
      model_profiles: [{ id: 'default', openai_model: 'model-c' }],
    }, 'r2'])
    expect(currentDraft).toEqual({ model_profiles: [{ id: 'default', openai_model: 'model-c' }] })
  })

  it('applies external snapshots without writing them back when there is no local edit', async () => {
    const initial = snapshot({ user: { language: 'zh-CN', theme: 'dark' }, revisions: { user: 'r1' } })
    const external = snapshot({ user: { language: 'zh-CN', theme: 'light' }, revisions: { user: 'r2' } })
    const loadSettings = vi.fn().mockResolvedValueOnce(initial).mockResolvedValueOnce(external)
    const saveUserSettings = vi.fn()
    const saveWorkspaceSettings = vi.fn()
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings,
      saveWorkspaceSettings,
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))

    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'other-view' } })))
    await waitFor(() => expect(result.current.draft).toEqual(external.user))
    vi.useFakeTimers()
    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })

    expect(saveUserSettings).not.toHaveBeenCalled()
  })

  it('rebases local nested changes over an external snapshot before autosaving', async () => {
    const initial = snapshot({
      user: {
        agent_models: { ide: { profile_id: 'local-a' }, image: { profile_id: 'image-a' } },
      },
      revisions: { user: 'r1' },
    })
    const external = snapshot({
      user: {
        agent_models: { ide: { profile_id: 'local-a' }, image: { profile_id: 'image-b' } },
      },
      revisions: { user: 'r2' },
    })
    const loadSettings = vi.fn().mockResolvedValueOnce(initial).mockResolvedValueOnce(external)
    const saveUserSettings = vi.fn(async (settings: Settings) => snapshot({ ...external, user: settings }))
    const saveWorkspaceSettings = vi.fn()
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings,
      saveWorkspaceSettings,
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))
    vi.useFakeTimers()

    act(() => result.current.setDraft({
      agent_models: { ide: { profile_id: 'local-b' }, image: { profile_id: 'image-a' } },
    }))
    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'other-view' } })))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(result.current.draft.agent_models).toEqual({
      ide: { profile_id: 'local-b' },
      image: { profile_id: 'image-b' },
    })

    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })
    expect(saveUserSettings).toHaveBeenCalledWith({
      agent_models: {
        ide: { profile_id: 'local-b' },
        image: { profile_id: 'image-b' },
      },
    }, 'r2')
  })

  it('archives an overlapping external edit before keeping the local settings value', async () => {
    const initial = snapshot({ user: { theme: 'dark' }, revisions: { user: 'r1' } })
    const external = snapshot({ user: { theme: 'light' }, revisions: { user: 'r2' } })
    const loadSettings = vi.fn().mockResolvedValueOnce(initial).mockResolvedValueOnce(external)
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings: vi.fn(),
      saveWorkspaceSettings: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))

    act(() => result.current.setDraft({ theme: 'system' }))
    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'other-view' } })))

    await waitFor(() => expect(result.current.draft).toEqual({ theme: 'system' }))
    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'settings',
      scope: 'test-settings:user',
      id: 'user',
      base: { revision: 'r1', value: { theme: 'dark' } },
      local: { revision: 'r1', value: { theme: 'system' } },
      external: { revision: 'r2', value: { theme: 'light' } },
      merged: { revision: 'r2', value: { theme: 'system' } },
      conflict_paths: [['theme']],
    }))
  })

  it('uses the autosave queue for manual save and clears the pending timer', async () => {
    const initial = snapshot({ user: { language: 'zh-CN' }, revisions: { user: 'r1' } })
    const saveUserSettings = vi.fn(async (settings: Settings) => snapshot({ ...initial, user: settings, revisions: { user: 'r2' } }))
    const loadSettings = vi.fn(async () => initial)
    const saveWorkspaceSettings = vi.fn()
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings,
      saveWorkspaceSettings,
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))
    vi.useFakeTimers()

    act(() => result.current.setDraft({ language: 'en-US' }))
    await act(async () => { await result.current.saveNow() })
    expect(saveUserSettings).toHaveBeenCalledTimes(1)
    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })
    expect(saveUserSettings).toHaveBeenCalledTimes(1)
  })

  it('rebases and retries once with the latest revision after a settings conflict', async () => {
    const initial = snapshot({ user: { language: 'zh-CN', theme: 'dark' }, revisions: { user: 'r1' } })
    const latest = snapshot({ user: { language: 'zh-CN', theme: 'light' }, revisions: { user: 'r2' } })
    const saved = snapshot({ user: { language: 'en-US', theme: 'light' }, revisions: { user: 'r3' } })
    const loadSettings = vi.fn().mockResolvedValueOnce(initial).mockResolvedValueOnce(latest)
    const saveUserSettings = vi.fn()
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce(saved)
    const saveWorkspaceSettings = vi.fn()
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings,
      saveWorkspaceSettings,
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))
    act(() => result.current.setDraft({ language: 'en-US', theme: 'dark' }))

    await act(async () => { await result.current.saveNow() })

    expect(saveUserSettings).toHaveBeenNthCalledWith(1, { language: 'en-US', theme: 'dark' }, 'r1')
    expect(saveUserSettings).toHaveBeenNthCalledWith(2, { language: 'en-US', theme: 'light' }, 'r2')
    expect(result.current.draft).toEqual(saved.user)
  })

  it('rebases a conflict against the baseline captured when the write started', async () => {
    const initial = snapshot({ user: { language: 'zh-CN', theme: 'dark' }, revisions: { user: 'r1' } })
    const external = snapshot({ user: { language: 'zh-CN', theme: 'light' }, revisions: { user: 'r2' } })
    const latest = snapshot({ user: { language: 'zh-CN', theme: 'system' }, revisions: { user: 'r3' } })
    const saved = snapshot({ user: { language: 'en-US', theme: 'system' }, revisions: { user: 'r4' } })
    const firstSave = deferred<LayeredSettings>()
    const loadSettings = vi.fn()
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(external)
      .mockResolvedValueOnce(latest)
      .mockResolvedValueOnce(saved)
    const saveUserSettings = vi.fn()
      .mockImplementationOnce(() => firstSave.promise)
      .mockResolvedValueOnce(saved)
    const { result } = renderHook(() => useLayeredSettingsDraft({
      layer: 'user',
      sourcePrefix: 'test-settings',
      loadSettings,
      saveUserSettings,
      saveWorkspaceSettings: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft).toEqual(initial.user))
    act(() => result.current.setDraft({ language: 'en-US', theme: 'dark' }))

    let savePromise!: Promise<LayeredSettings | null>
    act(() => { savePromise = result.current.saveNow() })
    await waitFor(() => expect(saveUserSettings).toHaveBeenCalledOnce())

    act(() => window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'other-view' } })))
    await waitFor(() => expect(result.current.draft).toEqual({ language: 'en-US', theme: 'light' }))

    await act(async () => {
      firstSave.reject(new APIError('revision conflict', { status: 409 }))
      await savePromise
    })

    expect(saveUserSettings).toHaveBeenNthCalledWith(2, { language: 'en-US', theme: 'system' }, 'r3')
    expect(result.current.draft).toEqual(saved.user)
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
