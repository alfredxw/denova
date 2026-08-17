import { describe, expect, it, vi } from 'vitest'
import { resolveAgentAskAndRefresh } from './agent-ask'

describe('resolveAgentAskAndRefresh', () => {
  it('returns the terminal resolution without waiting for history refresh', async () => {
    const resolution = { schema: 'ask.result.v1', id: 'ask-1', status: 'answered' as const, answers: [] }
    let finishRefresh: (() => void) | undefined
    const refreshHistory = vi.fn(() => new Promise<void>((resolve) => { finishRefresh = resolve }))
    const answer = vi.fn().mockResolvedValue(resolution)
    const cancel = vi.fn()

    await expect(resolveAgentAskAndRefresh(
      { status: 'answered', answers: [{ question_id: 'q-1', custom_input: 'done' }] },
      { answer, cancel },
      refreshHistory,
    )).resolves.toEqual(resolution)

    await vi.waitFor(() => expect(refreshHistory).toHaveBeenCalledOnce())
    expect(answer).toHaveBeenCalledWith([{ question_id: 'q-1', custom_input: 'done' }])
    expect(cancel).not.toHaveBeenCalled()
    finishRefresh?.()
  })

  it('uses the cancel transport and still refreshes history', async () => {
    const resolution = { schema: 'ask.result.v1', id: 'ask-2', status: 'cancelled' as const, cancel_reason: 'user_cancelled' }
    const refreshHistory = vi.fn()
    const cancel = vi.fn().mockResolvedValue(resolution)

    await expect(resolveAgentAskAndRefresh(
      { status: 'cancelled' },
      { answer: vi.fn(), cancel },
      refreshHistory,
    )).resolves.toEqual(resolution)

    await vi.waitFor(() => expect(refreshHistory).toHaveBeenCalledOnce())
  })
})
