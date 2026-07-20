import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { useAgentUIMessageStream } from './useAgentUIMessageStream'

const streamMocks = vi.hoisted(() => ({
  readUIMessageStream: vi.fn(),
}))

vi.mock('ai', () => ({
  readUIMessageStream: streamMocks.readUIMessageStream,
}))

describe('useAgentUIMessageStream', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps intermediate stream snapshots out of React state until the frame boundary', async () => {
    const reachedBoundary = deferred<void>()
    const continueStream = deferred<void>()
    streamMocks.readUIMessageStream.mockReturnValue((async function* () {
      yield textMessage('draft')
      yield textMessage('draft updated')
      reachedBoundary.resolve()
      await continueStream.promise
      yield textMessage('final')
    })())
    const { result } = renderHook(() => useAgentUIMessageStream())
    let consuming: Promise<void>

    act(() => {
      consuming = result.current.consumeAgentUIStream(new ReadableStream())
    })
    await act(async () => {
      await reachedBoundary.promise
    })

    expect(result.current.messages).toEqual([])

    await act(async () => {
      continueStream.resolve()
      await consuming
    })
    expect(messageText(result.current.messages[0])).toBe('final')
  })
})

function textMessage(text: string): AgentUIMessage {
  return {
    id: 'assistant-1',
    role: 'assistant',
    parts: [{ type: 'text', text }],
  } as AgentUIMessage
}

function messageText(message?: AgentUIMessage) {
  const part = message?.parts[0] as { text?: string } | undefined
  return part?.text || ''
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((next) => {
    resolve = next
  })
  return { promise, resolve }
}
