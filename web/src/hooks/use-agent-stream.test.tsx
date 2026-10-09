import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ChatTransport, UIMessageChunk } from 'ai'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { useAgentStream } from './use-agent-stream'

function connection() {
  let controller!: ReadableStreamDefaultController<UIMessageChunk>
  const cancel = vi.fn()
  const stream = new ReadableStream<UIMessageChunk>({ start(value) { controller = value }, cancel })
  return { stream, controller, cancel }
}

function setup(transport: ChatTransport<AgentUIMessage>) {
  const options = { transport, throttle: 80, onData: vi.fn(), onError: vi.fn(), onFinish: vi.fn() }
  return { ...renderHook(() => useAgentStream(options)), options }
}

describe('Agent display connection', () => {
  it('keeps history intact through submission, data events and final text', async () => {
    const active = connection()
    const transport = { sendMessages: vi.fn().mockResolvedValue(active.stream), reconnectToStream: vi.fn() }
    const { result, options } = setup(transport)
    const history: AgentUIMessage = { id: 'earlier', role: 'assistant', parts: [{ type: 'text', text: 'Earlier answer.' }] }
    act(() => result.current.setMessages([history]))
    let completed!: Promise<void>
    await act(async () => { completed = result.current.sendMessage({ role: 'user', parts: [{ type: 'text', text: 'Continue' }] }, { body: { command_id: 'command' } }) })
    expect(result.current.status).toBe('submitted')
    expect(transport.sendMessages.mock.calls[0][0]).toMatchObject({ body: { command_id: 'command' }, messages: [{ role: 'user' }] })
    await act(async () => {
      active.controller.enqueue({ type: 'start', messageId: 'response' })
      active.controller.enqueue({ type: 'data-agent-activity', transient: true, data: { event: 'agent_cycle_started' } })
      active.controller.enqueue({ type: 'text-start', id: 'answer' })
      active.controller.enqueue({ type: 'text-delta', id: 'answer', delta: 'New answer.' })
      active.controller.enqueue({ type: 'finish' })
      active.controller.close()
      await completed
    })
    expect(result.current.messages[0]).toBe(history)
    expect(result.current.messages).toHaveLength(3)
    expect(result.current.messages[2].parts[0]).toMatchObject({ text: 'New answer.', state: 'done' })
    expect(result.current.status).toBe('ready')
    expect(options.onData).toHaveBeenCalledOnce()
    expect(options.onFinish).toHaveBeenCalledOnce()
    expect(options.onError).not.toHaveBeenCalled()
  })

  it('flushes partial text on stop and resumes without changing the previous answer', async () => {
    const first = connection()
    const second = connection()
    const { result, options } = setup({ sendMessages: vi.fn().mockResolvedValue(first.stream), reconnectToStream: vi.fn().mockResolvedValue(second.stream) })
    let sending!: Promise<void>
    await act(async () => { sending = result.current.sendMessage({ role: 'user', parts: [] }) })
    await act(async () => {
      first.controller.enqueue({ type: 'text-start', id: 'partial' })
      first.controller.enqueue({ type: 'text-delta', id: 'partial', delta: 'Before stop' })
    })
    await act(async () => { result.current.stop(); await sending })
    const partial = result.current.messages.at(-1)
    expect(partial?.parts[0]).toMatchObject({ text: 'Before stop', state: 'done' })
    expect(first.cancel).toHaveBeenCalledOnce()
    let resumed!: Promise<void>
    await act(async () => { resumed = result.current.resumeStream() })
    await act(async () => {
      second.controller.enqueue({ type: 'text-start', id: 'continued' })
      second.controller.enqueue({ type: 'text-delta', id: 'continued', delta: 'After reconnect' })
      second.controller.close()
      await resumed
    })
    expect(result.current.messages.at(-2)).toBe(partial)
    expect(result.current.messages.at(-1)?.parts[0]).toMatchObject({ text: 'After reconnect', state: 'done' })
    expect(options.onError).not.toHaveBeenCalled()
  })

  it('cancels a late reconnect after unmount without publishing or finishing', async () => {
    const active = connection()
    let resolve!: (stream: ReadableStream<UIMessageChunk>) => void
    const pending = new Promise<ReadableStream<UIMessageChunk>>(done => { resolve = done })
    const { result, unmount, options } = setup({ sendMessages: vi.fn(), reconnectToStream: vi.fn().mockReturnValue(pending) })
    let resumed!: Promise<void>
    act(() => { resumed = result.current.resumeStream() })
    unmount()
    await act(async () => { resolve(active.stream); await resumed })
    expect(active.cancel).toHaveBeenCalledOnce()
    expect(options.onFinish).not.toHaveBeenCalled()
    expect(options.onError).not.toHaveBeenCalled()
  })
})
