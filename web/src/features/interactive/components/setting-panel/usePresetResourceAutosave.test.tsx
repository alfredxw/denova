import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useEffect } from 'react'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { createPresetConflictResolver, usePresetResourceAutosave } from './usePresetResourceAutosave'

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

interface DraftResource {
  id: string
  name: string
  prompt?: string
  updated_at?: string
}

describe('createPresetConflictResolver', () => {
  it('archives an overlapping conflict before preferring the local field and retrying', async () => {
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-1',
      path: 'conflicts/conflict-1.json',
      storage: 'server',
    })
    const baseline = resource('preset', 'original', 'r1')
    const local = resource('preset', 'local edit', 'r1')
    const external = resource('preset', 'external edit', 'r2')
    const resolve = createPresetConflictResolver(
      async () => [external],
      (draft) => draft,
      { resource: 'story_director', scope: '/books/demo' },
    )

    const result = await resolve({
      error: new APIError('revision conflict', { status: 409 }),
      baseline,
      draft: local,
      payload: local,
      baseRevision: 'r1',
    })

    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'story_director',
      scope: '/books/demo',
      id: 'preset',
      base: { revision: 'r1', value: baseline },
      local: { revision: 'r1', value: local },
      external: { revision: 'r2', value: external },
      merged: {
        revision: 'r2',
        value: expect.objectContaining({ name: 'local edit', updated_at: 'r2' }),
      },
      strategy: 'merge_non_overlap_prefer_local',
      conflict_paths: [['name']],
    }))
    expect(result).toEqual({
      payload: expect.objectContaining({ name: 'local edit', updated_at: 'r2' }),
      baseRevision: 'r2',
    })
  })
})

describe('usePresetResourceAutosave', () => {
  afterEach(() => {
    vi.useRealTimers()
    controls = null
  })

  it('exposes autosave state, including validation blocks', async () => {
    vi.useFakeTimers()
    const pendingSave = deferred<DraftResource>()
    const baseline = resource('preset', 'original')
    const view = render(<HookHarness draft={baseline} baseline={baseline} save={() => pendingSave.promise} />)

    expect(screen.getByTestId('autosave-status')).toHaveTextContent('saved')
    view.rerender(<HookHarness draft={resource('preset', 'changed')} baseline={baseline} save={() => pendingSave.promise} />)
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('pending')

    view.rerender(<HookHarness draft={resource('preset', 'changed')} baseline={baseline} save={() => pendingSave.promise} valid={false} />)
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('blocked')

    view.rerender(<HookHarness draft={resource('preset', 'changed')} baseline={baseline} save={() => pendingSave.promise} valid />)
    await advanceAutosave()
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('saving')

    pendingSave.resolve(resource('preset', 'changed', 'r2'))
    await act(async () => { await pendingSave.promise })
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('saved')
  })

  it('does not enqueue an initially invalid draft and rejects navigation flush', async () => {
    vi.useFakeTimers()
    const save = vi.fn()
    render(
      <HookHarness
        draft={resource('preset', 'invalid local')}
        baseline={resource('preset', 'original')}
        save={save}
        valid={false}
      />,
    )

    expect(screen.getByTestId('autosave-status')).toHaveTextContent('blocked')
    await expect(controls?.flushPending()).rejects.toMatchObject({ name: 'SaveLaneBlockedError' })
    await advanceAutosave()
    expect(save).not.toHaveBeenCalled()
  })

  it('does not enqueue a dirty baseline while the adapter is inactive', async () => {
    vi.useFakeTimers()
    const save = vi.fn()
    render(
      <HookHarness
        draft={resource('preset', 'local')}
        baseline={resource('preset', 'original')}
        save={save}
        active={false}
      />,
    )

    await advanceAutosave()
    expect(save).not.toHaveBeenCalled()
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('saved')
  })

  it('debounces edits and saves the latest draft once', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    const view = render(<HookHarness draft={resource('preset', 'original')} baseline={resource('preset', 'original')} save={save} />)

    view.rerender(<HookHarness draft={resource('preset', 'first')} baseline={resource('preset', 'original')} save={save} />)
    await advance(500)
    view.rerender(<HookHarness draft={resource('preset', 'latest')} baseline={resource('preset', 'original')} save={save} />)

    await advanceAutosave()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenLastCalledWith('preset', expect.objectContaining({ name: 'latest' }), 'r1')
  })

  it('keeps the original user-edit deadline when an external baseline rebases the draft', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource) => ({ ...payload, updated_at: 'r3' }))
    const initial = resource('preset', 'original', 'r1')
    const view = render(<HookHarness draft={initial} baseline={initial} save={save} />)

    view.rerender(<HookHarness draft={resource('preset', 'local', 'r1')} baseline={initial} save={save} />)
    await advance(600)
    const external = resource('preset', 'external', 'r2')
    const rebased = resource('preset', 'local + external', 'r2')
    view.rerender(<HookHarness draft={rebased} baseline={external} save={save} />)

    await advance(599)
    expect(save).not.toHaveBeenCalled()
    await advance(1)
    expect(save).toHaveBeenCalledWith('preset', expect.objectContaining({ name: 'local + external' }), 'r2')
  })

  it('never pairs a pre-reload draft with the newer external revision', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource) => ({ ...payload, updated_at: 'r3' }))
    const initial = resource('preset', 'original', 'r1')
    const localBeforeReload = resource('preset', 'local before reload', 'r1')
    const view = render(<HookHarness draft={initial} baseline={initial} save={save} />)

    view.rerender(<HookHarness draft={localBeforeReload} baseline={initial} save={save} />)
    await advance(600)
    view.rerender(
      <HookHarness
        draft={localBeforeReload}
        baseline={resource('preset', 'external', 'r2')}
        save={save}
      />,
    )

    await act(async () => {
      await controls?.flushPending()
    })

    expect(save).toHaveBeenCalledWith(
      'preset',
      expect.objectContaining({ name: 'local before reload', updated_at: 'r1' }),
      'r1',
    )
  })

  it('keeps the old revision request while the external rebase is still awaiting recovery', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource) => ({ ...payload, updated_at: 'r3' }))
    const initial = resource('preset', 'original', 'r1')
    const localBeforeReload = resource('preset', 'local before reload', 'r1')
    const view = render(<HookHarness draft={initial} baseline={initial} save={save} />)

    view.rerender(<HookHarness draft={localBeforeReload} baseline={initial} save={save} />)
    await advance(600)

    const external = resource('preset', 'external', 'r2')
    view.rerender(<HookHarness draft={localBeforeReload} baseline={external} save={save} />)
    // Recovery/archive is still awaiting while the user keeps editing the r1 draft.
    view.rerender(
      <HookHarness
        draft={resource('preset', 'latest edit while awaiting', 'r1')}
        baseline={external}
        save={save}
      />,
    )
    await advance(600)

    expect(save).toHaveBeenCalledWith(
      'preset',
      expect.objectContaining({ name: 'local before reload', updated_at: 'r1' }),
      'r1',
    )
    expect(save).not.toHaveBeenCalledWith(
      'preset',
      expect.objectContaining({ name: 'latest edit while awaiting' }),
      'r2',
    )
  })

  it('upgrades the pending request when the matching rebased draft arrives with unchanged content', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource) => ({ ...payload, updated_at: 'r3' }))
    const initial = resource('preset', 'original', 'r1')
    const localBeforeReload = resource('preset', 'local', 'r1')
    const view = render(<HookHarness draft={initial} baseline={initial} save={save} />)

    view.rerender(<HookHarness draft={localBeforeReload} baseline={initial} save={save} />)
    await advance(600)
    const external = resource('preset', 'original', 'r2')
    view.rerender(<HookHarness draft={localBeforeReload} baseline={external} save={save} />)
    view.rerender(
      <HookHarness
        draft={resource('preset', 'local', 'r2')}
        baseline={external}
        save={save}
      />,
    )

    await advance(600)

    expect(save).toHaveBeenCalledWith(
      'preset',
      expect.objectContaining({ name: 'local', updated_at: 'r2' }),
      'r2',
    )
  })

  it('does not save an unchanged signature', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    render(<HookHarness draft={resource('preset', 'original')} baseline={resource('preset', 'original')} save={save} />)

    await advanceAutosave()
    expect(save).not.toHaveBeenCalled()
  })

  it('does not write an unchanged resource when manually flushed', async () => {
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    const baseline = resource('preset', 'original')
    render(<HookHarness draft={baseline} baseline={baseline} save={save} />)

    await act(async () => {
      expect(await controls?.saveNow('manual')).toBeNull()
    })

    expect(save).not.toHaveBeenCalled()
  })

  it('manual save cancels the pending autosave', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    render(<HookHarness draft={resource('preset', 'changed')} baseline={resource('preset', 'original')} save={save} />)

    await act(async () => {
      await controls?.saveNow('manual')
    })
    await advanceAutosave()

    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenLastCalledWith('preset', expect.objectContaining({ name: 'changed' }), 'r1')
  })

  it('flushPending clears the timer and saves before switching resources', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    render(<HookHarness draft={resource('preset', 'changed')} baseline={resource('preset', 'original')} save={save} />)

    await act(async () => {
      await controls?.flushPending()
    })
    await advanceAutosave()

    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenLastCalledWith('preset', expect.objectContaining({ name: 'changed' }), 'r1')
  })

  it('waits for an in-flight autosave without applying its revision to a new resource baseline', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<DraftResource>()
    const save = vi.fn(async (id: string, payload: DraftResource, _baseRevision?: string) => {
      if (id === 'first') return firstSave.promise
      return { ...payload, updated_at: 'second-r2' }
    })
    const firstBaseline = resource('first', 'original', 'first-r1')
    const view = render(
      <HookHarness
        draft={resource('first', 'changed', 'first-r1')}
        baseline={firstBaseline}
        save={save}
      />,
    )

    await advanceAutosave()
    expect(save).toHaveBeenCalledTimes(1)

    const secondBaseline = resource('second', 'original', 'second-r1')
    view.rerender(<HookHarness draft={secondBaseline} baseline={secondBaseline} save={save} />)
    const flushResult = controls?.flushPending()
    expect(flushResult).not.toBeNull()
    let flushed = false
    void flushResult?.then(() => { flushed = true })
    await act(async () => { await Promise.resolve() })
    expect(flushed).toBe(false)

    firstSave.resolve(resource('first', 'changed', 'first-r2'))
    await act(async () => { await flushResult })
    expect(flushed).toBe(true)

    view.rerender(
      <HookHarness
        draft={resource('second', 'changed', 'second-r1')}
        baseline={secondBaseline}
        save={save}
      />,
    )
    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][0]).toBe('second')
    expect(save.mock.calls[1][2]).toBe('second-r1')
  })

  it('ignores an in-flight save after the workspace scope changes', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<DraftResource>()
    const onSaved = vi.fn()
    const save = vi.fn(async (_id: string, payload: DraftResource) => {
      if (save.mock.calls.length === 1) return firstSave.promise
      return { ...payload, updated_at: 'workspace-b-r2' }
    })
    const workspaceABaseline = resource('shared-id', 'workspace-a', 'workspace-a-r1')
    const view = render(
      <HookHarness
        scopeKey="workspace-a"
        draft={resource('shared-id', 'changed-a', 'workspace-a-r1')}
        baseline={workspaceABaseline}
        save={save}
        onSaved={onSaved}
      />,
    )

    const staleResult = controls?.saveNow('manual')
    await act(async () => { await Promise.resolve() })
    expect(save).toHaveBeenCalledTimes(1)

    const workspaceBBaseline = resource('shared-id', 'workspace-b', 'workspace-b-r1')
    view.rerender(
      <HookHarness
        scopeKey="workspace-b"
        draft={workspaceBBaseline}
        baseline={workspaceBBaseline}
        save={save}
        onSaved={onSaved}
      />,
    )
    firstSave.resolve(resource('shared-id', 'changed-a', 'workspace-a-r2'))
    await act(async () => { await staleResult })
    expect(onSaved).not.toHaveBeenCalled()

    view.rerender(
      <HookHarness
        scopeKey="workspace-b"
        draft={resource('shared-id', 'changed-b', 'workspace-b-r1')}
        baseline={workspaceBBaseline}
        save={save}
        onSaved={onSaved}
      />,
    )
    await act(async () => { await controls?.saveNow('manual') })
    expect(save).toHaveBeenLastCalledWith('shared-id', expect.objectContaining({ name: 'changed-b' }), 'workspace-b-r1')
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('finishes an old scope conflict with the save and resolver captured for that resource', async () => {
    const firstAttempt = deferred<DraftResource>()
    const saveA = vi.fn()
      .mockImplementationOnce(() => firstAttempt.promise)
      .mockResolvedValueOnce(resource('a', 'rebased-a', 'a-r3'))
    const saveB = vi.fn().mockResolvedValue(resource('b', 'saved-b', 'b-r2'))
    const resolveA = vi.fn().mockResolvedValue({ payload: resource('a', 'rebased-a', 'a-r2'), baseRevision: 'a-r2' })
    const resolveB = vi.fn()
    const baselineA = resource('a', 'original-a', 'a-r1')
    const view = render(
      <HookHarness draft={resource('a', 'local-a', 'a-r1')} baseline={baselineA} save={saveA} resolveConflict={resolveA} />,
    )

    void controls?.saveNow('manual')
    await act(async () => { await Promise.resolve() })
    const baselineB = resource('b', 'original-b', 'b-r1')
    view.rerender(
      <HookHarness draft={baselineB} baseline={baselineB} save={saveB} resolveConflict={resolveB} scopeKey="workspace-b" />,
    )

    firstAttempt.reject(new Error('stale revision'))
    await vi.waitFor(() => expect(saveA).toHaveBeenCalledTimes(2))
    expect(resolveA).toHaveBeenCalledOnce()
    expect(resolveB).not.toHaveBeenCalled()
    expect(saveB).not.toHaveBeenCalled()
  })

  it('cancels autosave while invalid without losing the dirty draft', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    const view = render(<HookHarness draft={resource('preset', 'changed')} baseline={resource('preset', 'original')} save={save} />)

    view.rerender(<HookHarness draft={resource('preset', 'changed')} baseline={resource('preset', 'original')} save={save} valid={false} />)
    await advanceAutosave()
    expect(save).not.toHaveBeenCalled()

    view.rerender(<HookHarness draft={resource('preset', 'changed')} baseline={resource('preset', 'original')} save={save} valid />)
    await advanceAutosave()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenLastCalledWith('preset', expect.objectContaining({ name: 'changed' }), 'r1')
  })

  it('uses the saved resource revision as the next base revision', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: save.mock.calls.length === 1 ? 'r2' : 'r3' }))
    const view = render(<HookHarness draft={resource('preset', 'first')} baseline={resource('preset', 'original')} save={save} />)

    await act(async () => {
      await controls?.saveNow('manual')
    })
    view.rerender(<HookHarness draft={resource('preset', 'second')} baseline={resource('preset', 'original')} save={save} />)
    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[0][2]).toBe('r1')
    expect(save.mock.calls[1][2]).toBe('r2')
  })

  it('reloads, rebases, and retries a revision conflict without entering an error state', async () => {
    const conflict = new Error('revision conflict')
    const baseline = resource('preset', 'original', 'r1')
    const changed = resource('preset', 'local edit', 'r1')
    const save = vi.fn()
      .mockRejectedValueOnce(conflict)
      .mockImplementationOnce(async (_id: string, payload: DraftResource) => ({ ...payload, updated_at: 'r3' }))
    const resolveConflict = vi.fn(async () => ({
      payload: resource('preset', 'local edit', 'r2'),
      baseRevision: 'r2',
    }))
    const onSaved = vi.fn()
    render(
      <HookHarness
        draft={changed}
        baseline={baseline}
        save={save}
        resolveConflict={resolveConflict}
        onSaved={onSaved}
      />,
    )

    await act(async () => {
      expect(await controls?.saveNow('manual')).toEqual(resource('preset', 'local edit', 'r3'))
    })

    expect(resolveConflict).toHaveBeenCalledWith(expect.objectContaining({
      error: conflict,
      baseline,
      draft: changed,
      payload: changed,
      baseRevision: 'r1',
    }))
    expect(save).toHaveBeenNthCalledWith(1, 'preset', changed, 'r1')
    expect(save).toHaveBeenNthCalledWith(2, 'preset', resource('preset', 'local edit', 'r2'), 'r2')
    expect(onSaved).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('saved')
  })

  it('reloads and retries again when another writer advances the revision during conflict recovery', async () => {
    const firstConflict = new Error('first conflict')
    const secondConflict = new Error('second conflict')
    const baseline = resource('preset', 'original', 'r1')
    const changed = resource('preset', 'local edit', 'r1')
    const save = vi.fn()
      .mockRejectedValueOnce(firstConflict)
      .mockRejectedValueOnce(secondConflict)
      .mockImplementationOnce(async (_id: string, payload: DraftResource) => ({ ...payload, updated_at: 'r4' }))
    const resolveConflict = vi.fn()
      .mockResolvedValueOnce({ payload: resource('preset', 'local edit', 'r2'), baseRevision: 'r2' })
      .mockResolvedValueOnce({ payload: resource('preset', 'local edit', 'r3'), baseRevision: 'r3' })
    render(<HookHarness draft={changed} baseline={baseline} save={save} resolveConflict={resolveConflict} />)

    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(resolveConflict).toHaveBeenCalledTimes(2)
    expect(resolveConflict).toHaveBeenNthCalledWith(2, expect.objectContaining({
      error: secondConflict,
      payload: resource('preset', 'local edit', 'r2'),
      baseRevision: 'r2',
    }))
    expect(save).toHaveBeenNthCalledWith(3, 'preset', resource('preset', 'local edit', 'r3'), 'r3')
    expect(screen.getByTestId('autosave-status')).toHaveTextContent('saved')
  })

  it('does not retry an unchanged draft when the server normalizes its response', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({
      ...payload,
      name: `${payload.name} normalized`,
      updated_at: 'r2',
    }))
    const baseline = resource('preset', 'original')
    const changed = resource('preset', 'changed')
    const view = render(<HookHarness draft={changed} baseline={baseline} save={save} />)

    await act(async () => {
      await controls?.saveNow('auto')
    })
    view.rerender(<HookHarness draft={changed} baseline={baseline} save={save} />)
    await advanceAutosave()

    expect(save).toHaveBeenCalledTimes(1)
  })

  it('serializes saves from the same hook and advances their base revision', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<DraftResource>()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => {
      if (save.mock.calls.length === 1) return firstSave.promise
      return { ...payload, updated_at: 'r3' }
    })
    const view = render(<HookHarness draft={resource('preset', 'first')} baseline={resource('preset', 'original')} save={save} />)

    const firstResult = controls?.saveNow('manual')
    view.rerender(<HookHarness draft={resource('preset', 'second')} baseline={resource('preset', 'original')} save={save} />)
    const secondResult = controls?.saveNow('manual')
    await act(async () => { await Promise.resolve() })
    expect(save).toHaveBeenCalledTimes(1)

    firstSave.resolve(resource('preset', 'first', 'r2'))
    await act(async () => {
      await firstResult
      await secondResult
    })

    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][2]).toBe('r2')
  })

  it('uses the first successful draft as the r2 merge baseline for a queued conflict', async () => {
    const firstSave = deferred<DraftResource>()
    const conflict = new Error('queued conflict')
    const save = vi.fn()
      .mockImplementationOnce(() => firstSave.promise)
      .mockRejectedValueOnce(conflict)
      .mockResolvedValueOnce(resource('preset', 'second', 'r3'))
    const resolveConflict = vi.fn().mockResolvedValue({
      payload: resource('preset', 'second', 'r2'),
      baseRevision: 'r2',
    })
    const initial = resource('preset', 'original', 'r1')
    const first = resource('preset', 'first', 'r1')
    const view = render(<HookHarness draft={first} baseline={initial} save={save} resolveConflict={resolveConflict} />)

    const firstResult = controls?.saveNow('manual')
    view.rerender(<HookHarness draft={resource('preset', 'second', 'r1')} baseline={initial} save={save} resolveConflict={resolveConflict} />)
    const secondResult = controls?.saveNow('manual')
    firstSave.resolve(resource('preset', 'first', 'r2'))
    await act(async () => {
      await firstResult
      await secondResult
    })

    expect(resolveConflict).toHaveBeenCalledWith(expect.objectContaining({
      error: conflict,
      baseline: resource('preset', 'first', 'r2'),
      baseRevision: 'r2',
    }))
  })

  it('does not advance a queued full snapshot past a conflict-transformed save baseline', async () => {
    const firstConflict = new Error('external writer advanced to r2')
    const queuedConflict = new Error('queued draft must rebase')
    const recovery = deferred<{ payload: DraftResource; baseRevision: string }>()
    const baseline = { ...resource('preset', 'N0', 'r1'), prompt: 'P0' }
    const first = { ...resource('preset', 'N1', 'r1'), prompt: 'P0' }
    const second = { ...resource('preset', 'N2', 'r1'), prompt: 'P0' }
    const mergedFirst = { ...resource('preset', 'N1', 'r2'), prompt: 'P1' }
    const mergedSecond = { ...resource('preset', 'N2', 'r3'), prompt: 'P1' }
    const save = vi.fn()
      .mockRejectedValueOnce(firstConflict)
      .mockResolvedValueOnce({ ...mergedFirst, updated_at: 'r3' })
      .mockRejectedValueOnce(queuedConflict)
      .mockResolvedValueOnce({ ...mergedSecond, updated_at: 'r4' })
    const resolveConflict = vi.fn()
      .mockImplementationOnce(() => recovery.promise)
      .mockResolvedValueOnce({ payload: mergedSecond, baseRevision: 'r3' })
    const view = render(
      <HookHarness draft={first} baseline={baseline} save={save} resolveConflict={resolveConflict} />,
    )

    const firstResult = controls?.saveNow('manual')
    await vi.waitFor(() => expect(resolveConflict).toHaveBeenCalledTimes(1))
    view.rerender(
      <HookHarness draft={second} baseline={baseline} save={save} resolveConflict={resolveConflict} />,
    )
    const secondResult = controls?.saveNow('manual')
    recovery.resolve({ payload: mergedFirst, baseRevision: 'r2' })

    await act(async () => {
      await firstResult
      await secondResult
    })

    expect(save.mock.calls[2][2]).toBe('r1')
    expect(save).toHaveBeenLastCalledWith(
      'preset',
      expect.objectContaining({ name: 'N2', prompt: 'P1' }),
      'r3',
    )
  })

  it('manual retry keeps the old safe request while an external rebase draft is pending', async () => {
    const failure = new Error('archive unavailable')
    const initial = resource('preset', 'original', 'r1')
    const local = resource('preset', 'local', 'r1')
    const external = resource('preset', 'external', 'r2')
    const save = vi.fn(async (_id: string, _payload: DraftResource, _baseRevision?: string) => { throw failure })
    const view = render(<HookHarness draft={local} baseline={initial} save={save} />)

    await expect(controls?.saveNow('manual')).rejects.toBe(failure)
    view.rerender(<HookHarness draft={local} baseline={external} save={save} />)
    await expect(controls?.retry()).rejects.toBe(failure)

    expect(save).toHaveBeenCalledTimes(2)
    expect(save.mock.calls[1][2]).toBe('r1')
  })

  it('does not let an older in-flight response replace a newer external baseline', async () => {
    const staleSave = deferred<DraftResource>()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => {
      if (save.mock.calls.length === 1) return staleSave.promise
      return { ...payload, updated_at: 'external-r3' }
    })
    const onSaved = vi.fn()
    const initial = resource('preset', 'original', 'r1')
    const view = render(
      <HookHarness
        draft={resource('preset', 'local edit', 'r1')}
        baseline={initial}
        save={save}
        onSaved={onSaved}
      />,
    )

    const staleResult = controls?.saveNow('manual')
    await act(async () => { await Promise.resolve() })
    const external = resource('preset', 'external edit', 'external-r2')
    view.rerender(<HookHarness draft={external} baseline={external} save={save} onSaved={onSaved} />)

    staleSave.resolve(resource('preset', 'local edit', 'r2'))
    await act(async () => { await staleResult })
    expect(onSaved).not.toHaveBeenCalled()

    view.rerender(
      <HookHarness
        draft={resource('preset', 'next local edit', 'external-r2')}
        baseline={external}
        save={save}
        onSaved={onSaved}
      />,
    )
    await act(async () => { await controls?.saveNow('manual') })

    expect(save.mock.calls[1][2]).toBe('external-r2')
  })

  it('propagates failures to flush and manual callers while reporting auto-save errors', async () => {
    vi.useFakeTimers()
    const failure = new Error('save failed')
    const onAutoSaveError = vi.fn()
    const onFlushError = vi.fn()
    const save = vi.fn(async () => { throw failure })
    const view = render(
      <HookHarness
        draft={resource('preset', 'changed')}
        baseline={resource('preset', 'original')}
        save={save}
        onAutoSaveError={onAutoSaveError}
        onFlushError={onFlushError}
      />,
    )

    await advanceAutosave()
    await act(async () => { await Promise.resolve() })
    expect(onAutoSaveError).toHaveBeenCalledWith(failure)

    view.rerender(
      <HookHarness
        draft={resource('preset', 'changed again')}
        baseline={resource('preset', 'original')}
        save={save}
        onAutoSaveError={onAutoSaveError}
        onFlushError={onFlushError}
      />,
    )
    await expect(controls?.flushPending()).rejects.toBe(failure)
    expect(onFlushError).toHaveBeenCalledWith(failure)
    await expect(controls?.saveNow('manual')).rejects.toBe(failure)
  })
})

let controls: ReturnType<typeof usePresetResourceAutosave<DraftResource, DraftResource, DraftResource>> | null = null

function HookHarness({
  draft,
  baseline,
  save,
  scopeKey = 'workspace',
  onSaved,
  active = true,
  valid = true,
  resolveConflict,
  onAutoSaveError,
  onFlushError,
}: {
  draft: DraftResource
  baseline: DraftResource
  save: (id: string, payload: DraftResource, baseRevision?: string) => Promise<DraftResource>
  scopeKey?: string
  onSaved?: (saved: DraftResource) => void
  active?: boolean
  valid?: boolean
  resolveConflict?: (context: {
    error: unknown
    baseline: DraftResource | null
    draft: DraftResource
    payload: DraftResource
    baseRevision: string
  }) => Promise<{ payload: DraftResource; baseRevision?: string } | null>
  onAutoSaveError?: (error: unknown) => void
  onFlushError?: (error: unknown) => void
}) {
  const autosave = usePresetResourceAutosave<DraftResource, DraftResource, DraftResource>({
    draft,
    active,
    scopeKey,
    valid,
    makePayload: (item) => ({ ...item }),
    signature: (value) => JSON.stringify({ id: value.id, name: value.name, prompt: value.prompt }),
    save,
    resolveConflict,
    onSaved,
    onAutoSaveError,
    onFlushError,
  })
  const baselineKey = JSON.stringify(baseline)
  useEffect(() => {
    autosave.resetBaseline(baseline)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autosave.resetBaseline, baselineKey])
  controls = autosave
  return <output data-testid="autosave-status">{autosave.status}</output>
}

function resource(id: string, name: string, updatedAt = 'r1'): DraftResource {
  return { id, name, updated_at: updatedAt }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function advanceAutosave() {
  await advance(1300)
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}
