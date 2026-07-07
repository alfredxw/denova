import { act, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useEffect } from 'react'
import { usePresetResourceAutosave } from './usePresetResourceAutosave'

interface DraftResource {
  id: string
  name: string
  tags?: string[]
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
})

let controls: ReturnType<typeof usePresetResourceAutosave<DraftResource, DraftResource, DraftResource>> | null = null

function HookHarness({
  draft,
  baseline,
  save,
  valid = true,
}: {
  draft: DraftResource
  baseline: DraftResource
  save: (id: string, payload: DraftResource, baseRevision?: string) => Promise<DraftResource>
  valid?: boolean
}) {
  const autosave = usePresetResourceAutosave<DraftResource, DraftResource, DraftResource>({
    draft,
    tagDraft: (draft.tags || []).join('，'),
    active: true,
    valid,
    makePayload: (item, tagDraft) => ({ ...item, tags: splitTags(tagDraft) }),
    signature: (value, tagDraft) => JSON.stringify({ ...value, tags: splitTags(tagDraft) }),
    save,
  })
  const baselineKey = JSON.stringify(baseline)
  useEffect(() => {
    autosave.resetBaseline(baseline, (baseline.tags || []).join('，'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autosave.resetBaseline, baselineKey])
  controls = autosave
  return null
}

function resource(id: string, name: string): DraftResource {
  return { id, name, tags: ['tag'], updated_at: 'r1' }
}

function splitTags(value: string) {
  return value
    .split(/[，,]/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}

async function advanceAutosave() {
  await advance(1300)
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}
