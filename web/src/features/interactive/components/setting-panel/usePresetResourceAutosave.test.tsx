import { act, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useEffect } from 'react'
import { usePresetResourceAutosave } from './usePresetResourceAutosave'

interface DraftResource {
  id: string
  name: string
  updated_at?: string
}

describe('usePresetResourceAutosave', () => {
  afterEach(() => {
    vi.useRealTimers()
    controls = null
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

  it('does not save an unchanged signature', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (_id: string, payload: DraftResource, _baseRevision?: string) => ({ ...payload, updated_at: 'r2' }))
    render(<HookHarness draft={resource('preset', 'original')} baseline={resource('preset', 'original')} save={save} />)

    await advanceAutosave()
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
  valid = true,
  onAutoSaveError,
  onFlushError,
}: {
  draft: DraftResource
  baseline: DraftResource
  save: (id: string, payload: DraftResource, baseRevision?: string) => Promise<DraftResource>
  scopeKey?: string
  onSaved?: (saved: DraftResource) => void
  valid?: boolean
  onAutoSaveError?: (error: unknown) => void
  onFlushError?: (error: unknown) => void
}) {
  const autosave = usePresetResourceAutosave<DraftResource, DraftResource, DraftResource>({
    draft,
    active: true,
    scopeKey,
    valid,
    makePayload: (item) => ({ ...item }),
    signature: (value) => JSON.stringify(value),
    save,
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
  return null
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
