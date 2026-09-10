import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { queryClient } from '@/lib/query-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'
import type { LayeredSettings, Settings } from './types'
import type { SettingsTarget } from './query'
import { fetchSettings, patchSettings } from './api'

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(async () => ({ id: 'conflict', path: '/conflicts/settings.json', storage: 'server' as const })),
}))

const initial: Settings = {
  agent_models: { ide: { thinking_level: 'low' }, interactive_story: { thinking_level: 'low' } },
}
const edited: Settings = {
  agent_models: { ide: { thinking_level: 'high' }, interactive_story: { thinking_level: 'high' } },
}

function snapshot(user: Settings, revision: string): LayeredSettings {
  return {
    default: {}, global: {}, user, workspace: {}, effective: user, revisions: { user: revision },
    paths: { denova_dir: '', nova_dir: '', user_config: '', workspace_config: '' },
    resolved_agent_tool_manifests: {}, resolved_agent_contexts: {},
  }
}

describe('layered settings partial saves', () => {
  beforeEach(() => {
    queryClient.clear()
    vi.mocked(preserveAutosaveConflict).mockClear()
  })

  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  for (const [name, target, url] of [
    ['global writing settings', { kind: 'global' }, '/api/settings'],
    ['project game settings', { kind: 'project', projectId: 'game' }, '/api/projects/game/settings'],
  ] as const) {
    it(`reconciles partial saves before reverting and retrying ${name}`, async () => {
      let current = snapshot(initial, 'r1')
      const patches: { changes: Settings; base_revision: string }[] = []
      vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
        expect(input).toBe(url)
        if (init?.method === 'PATCH') {
          const patch = JSON.parse(init.body as string)
          patches.push(patch)
          if (patches.length === 1) {
            current = snapshot({
              agent_models: { ide: { thinking_level: 'low' }, interactive_story: { thinking_level: 'high' } },
            }, 'r2')
            return Response.json({ error: 'Writing profile conflicted', code: 'settings_file_save_failed', details: { files: [
              { path: 'agents/main/game.toml', status: 'saved' },
              { path: 'agents/main/writing.toml', status: 'failed', code: 'revision_conflict' },
            ] } }, { status: 409 })
          }
          current = snapshot(initial, 'r3')
        }
        return Response.json(current)
      }))
      const { result } = renderHook(() => useLayeredSettingsDraft({ target: target as SettingsTarget, layer: 'user', sourcePrefix: name }))
      await waitFor(() => expect(result.current.layered?.revisions?.user).toBe('r1'))
      vi.useFakeTimers()

      act(() => result.current.setDraft(edited))
      await act(async () => { await vi.advanceTimersByTimeAsync(1000) })

      expect(result.current.layered?.user).toEqual(current.user)
      expect(result.current.draft).toEqual(edited)
      expect(result.current.autosaveStatus).toBe('error')
      await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
      expect(patches).toHaveLength(1)

      act(() => result.current.setDraft(initial))
      await act(async () => { await result.current.saveNow() })
      expect(patches[1]).toMatchObject({
        changes: { agent_models: { interactive_story: { thinking_level: 'low' } } },
        base_revision: 'r2',
      })
      expect(patches[1].changes.agent_models).not.toHaveProperty('ide')
      expect(result.current.draft).toEqual(initial)
      expect(result.current.layered?.user).toEqual(initial)
      expect(result.current.autosaveStatus).toBe('saved')
    })
  }

  it('discards a withdrawn failed draft when retrying a save with no committed changes', async () => {
    const save = vi.fn().mockRejectedValue(new Error('Request rejected'))
    const loadSettings = async () => snapshot(initial, 'r1')
    const { result } = renderHook(() => useLayeredSettingsDraft({
      target: { kind: 'global' }, layer: 'user', sourcePrefix: 'revert',
      loadSettings, saveUserSettings: save,
    }))
    await waitFor(() => expect(result.current.layered).not.toBeNull())
    vi.useFakeTimers()
    act(() => result.current.setDraft(edited))
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(save).toHaveBeenCalledOnce()
    act(() => result.current.setDraft(initial))
    await act(async () => { await result.current.saveNow() })
    expect(save).toHaveBeenCalledOnce()
  })

  it('preserves a revert and newer edits while reading back a partial 400 result', async () => {
    let current = snapshot(initial, 'r1')
    const partialResponse = deferred<Response>()
    const readback = deferred<Response>()
    const patches: { changes: Settings; base_revision: string }[] = []
    vi.stubGlobal('fetch', vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') {
        const patch = JSON.parse(init.body as string)
        patches.push(patch)
        if (patches.length === 1) return partialResponse.promise
        current = snapshot({ ...initial, theme: 'dark' }, 'r3')
      }
      if (patches.length === 1) return readback.promise
      return Response.json(current)
    }))
    const { result } = renderHook(() => useLayeredSettingsDraft({ target: { kind: 'global' }, layer: 'user', sourcePrefix: 'during-save' }))
    await waitFor(() => expect(result.current.layered?.revisions?.user).toBe('r1'))
    vi.useFakeTimers()
    act(() => result.current.setDraft(edited))
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    act(() => result.current.setDraft(initial))
    await act(async () => {
      partialResponse.resolve(Response.json({ error: 'Invalid game profile', code: 'settings_file_save_failed', details: { files: [
        { path: 'agents/main/writing.toml', status: 'saved' },
        { path: 'agents/main/game.toml', status: 'failed', code: 'invalid_profile' },
      ] } }, { status: 400 }))
    })
    act(() => result.current.setDraft({ ...initial, theme: 'dark' }))
    current = snapshot({
      agent_models: { ide: { thinking_level: 'high' }, interactive_story: { thinking_level: 'low' } },
    }, 'r2')
    await act(async () => { readback.resolve(Response.json(current)) })
    expect(result.current.layered?.user).toEqual(current.user)
    expect(result.current.draft).toEqual({ ...initial, theme: 'dark' })
    expect(result.current.error).toContain('agents/main/writing.toml')
    expect(result.current.error).toContain('agents/main/game.toml')
    expect(result.current.autosaveStatus).toBe('error')
    await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
    expect(patches).toHaveLength(1)
    await act(async () => { await result.current.saveNow() })
    expect(patches[1]).toMatchObject({
      changes: { theme: 'dark', agent_models: { ide: { thinking_level: 'low' } } }, base_revision: 'r2',
    })
    expect(patches[1].changes.agent_models).not.toHaveProperty('interactive_story')
    expect(result.current.autosaveStatus).toBe('saved')
  })

  it('archives external conflicts and preserves edits made during conflict recovery', async () => {
    const archive = deferred<{ id: string; path: string; storage: 'server' }>()
    vi.mocked(preserveAutosaveConflict).mockImplementationOnce(() => archive.promise)
    let current = snapshot(initial, 'r1')
    const fetch = vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') {
        current = snapshot({
          agent_models: { ide: { thinking_level: 'medium' }, interactive_story: { thinking_level: 'high' } },
          ui_font_size: 16,
        }, 'r2')
        return Response.json({ error: 'Conflict', code: 'settings_file_save_failed', details: { files: [
          { path: 'agents/main/game.toml', status: 'saved' },
          { path: 'agents/main/writing.toml', status: 'failed', code: 'revision_conflict' },
        ] } }, { status: 409 })
      }
      return Response.json(current)
    })
    vi.stubGlobal('fetch', fetch)
    const { result } = renderHook(() => useLayeredSettingsDraft({ target: { kind: 'global' }, layer: 'user', sourcePrefix: 'conflict' }))
    await waitFor(() => expect(result.current.layered?.revisions?.user).toBe('r1'))
    vi.useFakeTimers()
    act(() => result.current.setDraft(edited))
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(preserveAutosaveConflict).toHaveBeenCalledOnce()
    act(() => result.current.setDraft({ ...initial, theme: 'dark' }))
    await act(async () => { archive.resolve({ id: 'conflict', path: '/conflicts/settings.json', storage: 'server' }) })
    expect(result.current.draft).toEqual({ ...initial, theme: 'dark', ui_font_size: 16 })
    expect(result.current.layered?.user).toEqual(current.user)
    expect(result.current.autosaveStatus).toBe('error')
  })

  it('publishes canonical partial effects for other settings consumers while retaining the failure', async () => {
    const latest = snapshot(edited, 'r2')
    vi.stubGlobal('fetch', vi.fn(async (_input: string, init?: RequestInit) => init?.method === 'PATCH'
      ? Response.json({ error: 'Invalid game profile', code: 'settings_file_save_failed', details: { files: [
        { path: 'config.toml', status: 'saved' },
        { path: 'agents/main/game.toml', status: 'failed', code: 'invalid_profile' },
      ] } }, { status: 400 })
      : Response.json(latest)))
    await expect(patchSettings('user', { theme: 'dark' }, 'r1')).rejects.toMatchObject({
      status: 400, code: 'settings_file_save_failed', snapshot: latest,
    })
    expect(await fetchSettings()).toEqual(latest)
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('does not restart autosave when revision recovery ends in a partial failure', async () => {
    let current = snapshot(initial, 'r1')
    let saves = 0
    vi.stubGlobal('fetch', vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') {
        saves++
        if (saves === 1) {
          current = snapshot({ ...initial, ui_font_size: 16 }, 'r2')
          return Response.json({ error: 'Settings changed', code: 'revision_conflict' }, { status: 409 })
        }
        current = snapshot({
          agent_models: { ide: { thinking_level: 'high' }, interactive_story: { thinking_level: 'low' } },
          ui_font_size: 16,
        }, 'r3')
        return Response.json({ error: 'Invalid profile', code: 'settings_file_save_failed', details: { files: [
          { path: 'agents/main/writing.toml', status: 'saved' },
          { path: 'agents/main/game.toml', status: 'failed', code: 'invalid_profile' },
        ] } }, { status: 400 })
      }
      return Response.json(current)
    }))
    const { result } = renderHook(() => useLayeredSettingsDraft({ target: { kind: 'global' }, layer: 'user', sourcePrefix: 'revision-then-partial' }))
    await waitFor(() => expect(result.current.layered?.revisions?.user).toBe('r1'))
    vi.useFakeTimers()
    act(() => result.current.setDraft(edited))
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(saves).toBe(2)
    expect(result.current.autosaveStatus).toBe('error')
    expect(result.current.layered?.user).toEqual(current.user)
    expect(result.current.draft).toEqual({ ...edited, ui_font_size: 16 })
    await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
    expect(saves).toBe(2)
  })

  it('does not reuse a read started before the partial commit', async () => {
    const oldRead = deferred<Response>()
    const latest = snapshot(edited, 'r2')
    let reads = 0
    vi.stubGlobal('fetch', vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === 'PATCH') return Response.json({ error: 'Partial failure', code: 'settings_file_save_failed', details: { files: [
        { path: 'agents/main/writing.toml', status: 'saved' },
        { path: 'agents/main/game.toml', status: 'failed', code: 'invalid_profile' },
      ] } }, { status: 400 })
      reads++
      return reads === 1 ? oldRead.promise : Response.json(latest)
    }))
    const stale = fetchSettings().catch(() => undefined)
    await waitFor(() => expect(reads).toBe(1))
    await expect(patchSettings('user', edited, 'r1')).rejects.toMatchObject({ snapshot: latest })
    oldRead.resolve(Response.json(snapshot(initial, 'r1')))
    await stale
    expect(await fetchSettings()).toEqual(latest)
    expect(reads).toBe(2)
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(res => { resolve = res })
  return { promise, resolve }
}
