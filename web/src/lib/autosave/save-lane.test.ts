import { afterEach, describe, expect, it, vi } from 'vitest'
import { createSaveLane } from './save-lane'

function deferred() {
  let resolve!: () => void
  let reject!: (error: unknown) => void
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('save lane', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('saves an edit only after the configured delay', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('draft')

    await vi.advanceTimersByTimeAsync(99)
    expect(save).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(save).toHaveBeenCalledOnce()
    expect(save).toHaveBeenCalledWith({ scopeKey: 'document:a', value: 'draft' })
  })

  it('restarts the delay and keeps only the latest edited snapshot', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('first')
    await vi.advanceTimersByTimeAsync(80)
    lane.edit('latest')
    await vi.advanceTimersByTimeAsync(99)
    expect(save).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(save).toHaveBeenCalledOnce()
    expect(save).toHaveBeenCalledWith({ scopeKey: 'document:a', value: 'latest' })
  })

  it('uses an updated delay without recreating the lane', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.setDelayMs(250)
    lane.edit('draft')

    await vi.advanceTimersByTimeAsync(249)
    expect(save).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(save).toHaveBeenCalledOnce()
  })

  it('cancels a delayed edit without affecting the active scope', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('discarded draft')
    lane.cancel()
    await vi.advanceTimersByTimeAsync(500)

    expect(save).not.toHaveBeenCalled()
    expect(lane.getSnapshot()).toMatchObject({ scopeKey: 'document:a', status: 'saved' })
  })

  it('reports work from an older scope until its in-flight request settles', async () => {
    vi.useFakeTimers()
    const saving = deferred()
    const lane = createSaveLane<string>({ delayMs: 100, save: () => saving.promise })

    lane.reset('document:old')
    lane.edit('draft')
    await vi.advanceTimersByTimeAsync(100)
    lane.reset('document:new')

    expect(lane.hasWork()).toBe(true)
    saving.resolve()
    await vi.waitFor(() => expect(lane.hasWork()).toBe(false))
  })

  it('replaces a pending draft after external reload without restarting the user-edit delay', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('local on r1')
    await vi.advanceTimersByTimeAsync(80)
    expect(lane.reload('rebased on r2')).toBe(true)
    await vi.advanceTimersByTimeAsync(19)
    expect(save).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)

    expect(save).toHaveBeenCalledWith({ scopeKey: 'document:a', value: 'rebased on r2' })
  })

  it('runs at most one save at a time and coalesces waiting edits to the latest snapshot', async () => {
    vi.useFakeTimers()
    const firstSave = deferred()
    const latestSave = deferred()
    const save = vi.fn()
      .mockReturnValueOnce(firstSave.promise)
      .mockReturnValueOnce(latestSave.promise)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('first')
    await vi.advanceTimersByTimeAsync(100)
    expect(save).toHaveBeenCalledOnce()

    lane.edit('second')
    lane.edit('latest')
    await vi.advanceTimersByTimeAsync(100)
    expect(save).toHaveBeenCalledOnce()

    firstSave.resolve()
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save).toHaveBeenLastCalledWith({ scopeKey: 'document:a', value: 'latest' })
    latestSave.resolve()
  })

  it('publishes pending, saving, and saved states for an edit', async () => {
    vi.useFakeTimers()
    const saving = deferred()
    const lane = createSaveLane<string>({ delayMs: 100, save: () => saving.promise })
    const observed: string[] = []

    lane.reset('document:a')
    const unsubscribe = lane.subscribe(() => observed.push(lane.getSnapshot().status))
    lane.edit('draft')
    await vi.advanceTimersByTimeAsync(100)
    saving.resolve()
    await vi.waitFor(() => expect(lane.getSnapshot().status).toBe('saved'))

    expect(observed).toEqual(['pending', 'saving', 'saved'])
    unsubscribe()
  })

  it('flushes the latest edit through the same serialized save queue', async () => {
    vi.useFakeTimers()
    const firstSave = deferred()
    const latestSave = deferred()
    const save = vi.fn()
      .mockReturnValueOnce(firstSave.promise)
      .mockReturnValueOnce(latestSave.promise)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('first')
    await vi.advanceTimersByTimeAsync(100)
    lane.edit('latest')

    let flushed = false
    const flush = lane.flush().then(() => { flushed = true })
    expect(save).toHaveBeenCalledOnce()

    firstSave.resolve()
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save).toHaveBeenLastCalledWith({ scopeKey: 'document:a', value: 'latest' })
    expect(flushed).toBe(false)

    latestSave.resolve()
    await flush
    expect(flushed).toBe(true)
  })

  it('retains a failed snapshot in error state so flush can retry it', async () => {
    vi.useFakeTimers()
    const failure = new Error('offline')
    const save = vi.fn()
      .mockRejectedValueOnce(failure)
      .mockResolvedValueOnce(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('draft')
    await vi.advanceTimersByTimeAsync(100)
    await vi.waitFor(() => expect(lane.getSnapshot().status).toBe('error'))
    expect(lane.getSnapshot().error).toBe(failure)

    await lane.flush()
    expect(save).toHaveBeenCalledTimes(2)
    expect(save).toHaveBeenLastCalledWith({ scopeKey: 'document:a', value: 'draft' })
    expect(lane.getSnapshot()).toMatchObject({ status: 'saved', error: null })
  })

  it('keeps the latest edit blocked until the lane is unblocked and a fresh delay elapses', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('first')
    lane.block('invalid draft')
    lane.edit('latest')
    await vi.advanceTimersByTimeAsync(500)
    expect(save).not.toHaveBeenCalled()
    expect(lane.getSnapshot()).toMatchObject({ status: 'blocked', error: 'invalid draft' })

    lane.unblock()
    await vi.advanceTimersByTimeAsync(99)
    expect(save).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(save).toHaveBeenCalledWith({ scopeKey: 'document:a', value: 'latest' })
  })

  it('rejects flush while blocked instead of reporting an unsaved draft as complete', async () => {
    const lane = createSaveLane<string>({ delayMs: 100, save: vi.fn() })
    lane.reset('document:a')
    lane.edit('invalid draft')
    lane.block('invalid')

    await expect(lane.flush()).rejects.toMatchObject({
      name: 'SaveLaneBlockedError',
      reason: 'invalid',
    })
    expect(lane.hasWork()).toBe(true)
  })

  it('does not let an old scope result update the reset scope', async () => {
    vi.useFakeTimers()
    const oldSave = deferred()
    const currentSave = deferred()
    const save = vi.fn()
      .mockReturnValueOnce(oldSave.promise)
      .mockReturnValueOnce(currentSave.promise)
    const onSaved = vi.fn()
    const lane = createSaveLane<string, void>({ delayMs: 100, save, onSaved })

    lane.reset('document:old')
    lane.edit('old draft')
    await vi.advanceTimersByTimeAsync(100)

    lane.reset('document:current')
    lane.edit('current draft')
    await vi.advanceTimersByTimeAsync(100)
    expect(lane.getSnapshot()).toMatchObject({ scopeKey: 'document:current', status: 'pending' })

    oldSave.resolve()
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(onSaved).not.toHaveBeenCalled()
    expect(lane.getSnapshot()).toMatchObject({ scopeKey: 'document:current', status: 'saving' })

    currentSave.resolve()
    await vi.waitFor(() => expect(lane.getSnapshot().status).toBe('saved'))
    expect(onSaved).toHaveBeenCalledOnce()
    expect(onSaved).toHaveBeenCalledWith(
      { scopeKey: 'document:current', value: 'current draft' },
      undefined,
    )
  })

  it('does not publish an old scope failure into the reset scope', async () => {
    vi.useFakeTimers()
    const oldSave = deferred()
    const currentSave = deferred()
    const onError = vi.fn()
    const save = vi.fn()
      .mockReturnValueOnce(oldSave.promise)
      .mockReturnValueOnce(currentSave.promise)
    const lane = createSaveLane<string>({ delayMs: 100, save, onError })

    lane.reset('document:old')
    lane.edit('old draft')
    await vi.advanceTimersByTimeAsync(100)
    lane.reset('document:current')
    lane.edit('current draft')
    await vi.advanceTimersByTimeAsync(100)

    oldSave.reject(new Error('old failure'))
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(onError).not.toHaveBeenCalled()
    expect(lane.getSnapshot()).toMatchObject({ scopeKey: 'document:current', status: 'saving' })

    currentSave.resolve()
    await vi.waitFor(() => expect(lane.getSnapshot().status).toBe('saved'))
  })

  it('returns the last persisted result from a flush', async () => {
    const saved = { revision: 'revision-2' }
    const lane = createSaveLane<string, typeof saved>({
      delayMs: 100,
      save: vi.fn().mockResolvedValue(saved),
    })

    lane.reset('document:a')
    lane.edit('draft')

    await expect(lane.flush()).resolves.toBe(saved)
    await expect(lane.flush()).resolves.toBeNull()
  })

  it('can dispose by discarding a delayed edit', async () => {
    vi.useFakeTimers()
    const save = vi.fn().mockResolvedValue(undefined)
    const lane = createSaveLane<string>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('draft')
    await expect(lane.dispose('discard')).resolves.toBeNull()
    await vi.advanceTimersByTimeAsync(100)

    expect(save).not.toHaveBeenCalled()
    expect(() => lane.edit('later')).toThrow('Cannot edit a disposed save lane')
  })

  it('can dispose by flushing a delayed edit first', async () => {
    const saved = { revision: 'revision-3' }
    const save = vi.fn().mockResolvedValue(saved)
    const lane = createSaveLane<string, typeof saved>({ delayMs: 100, save })

    lane.reset('document:a')
    lane.edit('draft')

    await expect(lane.dispose('flush')).resolves.toBe(saved)
    expect(save).toHaveBeenCalledWith({ scopeKey: 'document:a', value: 'draft' })
    expect(() => lane.reset('document:b')).toThrow('Cannot reset a disposed save lane')
  })

  it('settles flush and exposes an observer failure instead of wedging after a successful save', async () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const observerFailure = new Error('saved observer failed')
    const lane = createSaveLane<string, string>({
      delayMs: 100,
      save: vi.fn().mockResolvedValue('persisted'),
      onSaved: () => { throw observerFailure },
    })
    lane.reset('document:a')
    lane.edit('draft')

    await expect(lane.flush()).resolves.toBe('persisted')
    expect(lane.getSnapshot()).toMatchObject({ status: 'error', error: observerFailure })
    expect(lane.hasWork()).toBe(false)
    log.mockRestore()
  })

  it('retains a failed request even when its error observer throws', async () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const saveFailure = new Error('offline')
    const lane = createSaveLane<string>({
      delayMs: 100,
      save: vi.fn().mockRejectedValueOnce(saveFailure).mockResolvedValueOnce(undefined),
      onError: () => { throw new Error('error observer failed') },
    })
    lane.reset('document:a')
    lane.edit('draft')

    await expect(lane.flush()).resolves.toBeNull()
    expect(lane.getSnapshot()).toMatchObject({ status: 'error', error: saveFailure })
    expect(lane.hasWork()).toBe(true)
    await expect(lane.flush()).resolves.toBeUndefined()
    expect(lane.getSnapshot().status).toBe('saved')
    log.mockRestore()
  })
})
