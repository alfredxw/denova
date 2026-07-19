import { describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { isRevisionConflict, saveWithRevisionRecovery } from './revision-conflict'

describe('isRevisionConflict', () => {
  it('does not mistake another typed 409 response for a revision mismatch', () => {
    expect(isRevisionConflict(new APIError('workspace changed', {
      status: 409,
      code: 'workspace_changed',
    }))).toBe(false)
    expect(isRevisionConflict(new APIError('revision changed', {
      status: 409,
      code: 'revision_conflict',
    }))).toBe(true)
  })
})

describe('saveWithRevisionRecovery', () => {
  it('reloads and rebases again when a revision races more than once', async () => {
    const save = vi.fn<(draft: string, revision?: string) => Promise<string>>()
      .mockRejectedValueOnce(new APIError('first conflict', { status: 409 }))
      .mockRejectedValueOnce(new APIError('second conflict', { status: 412 }))
      .mockResolvedValueOnce('saved')
    const loadLatest = vi.fn()
      .mockResolvedValueOnce({ value: 'external-1', revision: 'r2' })
      .mockResolvedValueOnce({ value: 'external-2', revision: 'r3' })
    const rebase = vi.fn((baseline: string, draft: string, latest: string) => `${draft}[${baseline}->${latest}]`)

    await expect(saveWithRevisionRecovery({
      baseline: 'initial',
      draft: 'local',
      revision: 'r1',
      save,
      loadLatest,
      rebase,
    })).resolves.toBe('saved')

    expect(save).toHaveBeenNthCalledWith(1, 'local', 'r1')
    expect(save).toHaveBeenNthCalledWith(2, 'local[initial->external-1]', 'r2')
    expect(save).toHaveBeenNthCalledWith(3, 'local[initial->external-1][external-1->external-2]', 'r3')
  })

  it('does not reinterpret ordinary save failures as revision conflicts', async () => {
    const failure = new Error('offline')
    const loadLatest = vi.fn()

    await expect(saveWithRevisionRecovery({
      baseline: 'initial',
      draft: 'local',
      revision: 'r1',
      save: vi.fn().mockRejectedValue(failure),
      loadLatest,
      rebase: vi.fn(),
    })).rejects.toBe(failure)
    expect(loadLatest).not.toHaveBeenCalled()
  })

  it('waits for asynchronous recovery before retrying the write', async () => {
    const save = vi.fn<(draft: string, revision?: string) => Promise<string>>()
      .mockRejectedValueOnce(new APIError('conflict', { status: 409 }))
      .mockResolvedValueOnce('saved')
    const rebase = vi.fn(async (_baseline: string, draft: string, latest: string) => {
      await Promise.resolve()
      return `${draft}+${latest}`
    })

    await expect(saveWithRevisionRecovery({
      baseline: 'initial',
      draft: 'local',
      revision: 'r1',
      save,
      loadLatest: vi.fn().mockResolvedValue({ value: 'external', revision: 'r2' }),
      rebase,
    })).resolves.toBe('saved')

    expect(save).toHaveBeenNthCalledWith(2, 'local+external', 'r2')
  })
})
