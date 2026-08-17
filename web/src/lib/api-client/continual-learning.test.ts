import { afterEach, describe, expect, it, vi } from 'vitest'
import { runHarnessOptimizer } from './continual-learning'

describe('Harness Optimizer evidence scope', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('omits evidence for automatic global discovery', async () => {
    const fetchMock = vi.fn(async () => new Response('stream', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await runHarnessOptimizer('command-auto', '')

    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(JSON.parse(String(init.body))).toEqual({ command_id: 'command-auto', instruction: '' })
  })

  it('sends only the explicitly selected Run URIs', async () => {
    const fetchMock = vi.fn(async () => new Response('stream', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const evidence = [
      'trajectory://projects/first/runs/run-1',
      'trajectory://projects/second/runs/run-2',
    ]

    await runHarnessOptimizer('command-scoped', 'Inspect failures.', evidence)

    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(JSON.parse(String(init.body))).toEqual({
      command_id: 'command-scoped',
      instruction: 'Inspect failures.',
      evidence,
    })
  })
})
