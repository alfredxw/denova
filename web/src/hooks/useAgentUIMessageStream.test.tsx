import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UIMessageChunk } from 'ai'
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
    streamMocks.readUIMessageStream.mockReset()
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

  it('preserves the exact append-only input text beside parsed tool snapshots', async () => {
    const firstDelta = '{"path":"chapters/ch01.md","edits":[{"old_string":"旧正文'
    const secondDelta = '仍在生成'
    streamMocks.readUIMessageStream.mockImplementation(({ stream }: { stream: ReadableStream<UIMessageChunk> }) => (async function* () {
      const reader = stream.getReader()
      let inputVersion = 0
      while (true) {
        const { done, value } = await reader.read()
        if (done) return
        if (value.type !== 'tool-input-start' && value.type !== 'tool-input-delta') continue
        if (value.type === 'tool-input-delta') inputVersion += 1
        yield toolMessage({ parsedVersion: inputVersion })
      }
    })())
    const onView = vi.fn()
    const { result } = renderHook(() => useAgentUIMessageStream({ onView }))
    const stream = new ReadableStream<UIMessageChunk>({
      start(controller) {
        controller.enqueue({ type: 'tool-input-start', toolCallId: 'tool-1', toolName: 'edit', dynamic: true })
        controller.enqueue({ type: 'tool-input-delta', toolCallId: 'tool-1', inputTextDelta: firstDelta })
        controller.enqueue({ type: 'tool-input-delta', toolCallId: 'tool-1', inputTextDelta: secondDelta })
        controller.close()
      },
    })

    await act(async () => {
      await result.current.consumeAgentUIStream(stream)
    })

    expect(onView.mock.calls.map(([view]) => view.inputText)).toEqual(['', firstDelta, `${firstDelta}${secondDelta}`])
    expect(result.current.messages[0].parts[0]).toMatchObject({
      input: { parsedVersion: 2 },
      inputText: `${firstDelta}${secondDelta}`,
    })
  })
})

function textMessage(text: string): AgentUIMessage {
  return {
    id: 'assistant-1',
    role: 'assistant',
    parts: [{ type: 'text', text }],
  } as AgentUIMessage
}

function toolMessage(input: unknown): AgentUIMessage {
  return {
    id: 'assistant-tool',
    role: 'assistant',
    parts: [{ type: 'dynamic-tool', toolName: 'edit', toolCallId: 'tool-1', state: 'input-streaming', input }],
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
