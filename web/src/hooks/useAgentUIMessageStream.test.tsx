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

  it('uses the AI SDK parsed tool input without a second raw-text state', async () => {
    const firstInput = { path: 'chapters/ch01.md', content: '正在' }
    const finalInput = { path: 'chapters/ch01.md', content: '正在生成' }
    streamMocks.readUIMessageStream.mockReturnValue((async function* () {
      yield toolMessage(firstInput)
      yield toolMessage(finalInput)
    })())
    const onView = vi.fn()
    const { result } = renderHook(() => useAgentUIMessageStream({ onView }))
    const stream = new ReadableStream<UIMessageChunk>()

    await act(async () => {
      await result.current.consumeAgentUIStream(stream)
    })

    expect(streamMocks.readUIMessageStream).toHaveBeenCalledWith({ stream, terminateOnError: true })
    expect(onView.mock.calls.map(([view]) => view.input)).toEqual([firstInput, finalInput])
    expect(result.current.messages[0].parts[0]).toMatchObject({
      input: finalInput,
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
