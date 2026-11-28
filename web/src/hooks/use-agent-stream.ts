import { randomUUID } from '@/lib/uuid'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChatRequestOptions, ChatTransport, UIMessageChunk } from 'ai'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { AgentStreamProjection, applyAgentStreamUpdates, isAgentDataChunk } from '@/lib/agent-stream-projection'
import { createRafUpdateBatcher } from '@/lib/streaming/raf-update-batcher'

type Status = 'ready' | 'submitted' | 'streaming' | 'error'
/** Only a protocol terminal chunk certifies that the display reached its end. */
export type AgentStreamCompletion = 'terminal' | 'disconnected' | null
type MessageUpdate = AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])
type DataPart = Extract<AgentUIMessage['parts'][number], { type: `data-${string}` }>
interface StreamOptions {
  transport: ChatTransport<AgentUIMessage> & {
    reconnectToStream(options: Parameters<ChatTransport<AgentUIMessage>['reconnectToStream']>[0], signal?: AbortSignal): Promise<ReadableStream<UIMessageChunk> | null>
  }
  throttle: number
  onData: (part: DataPart) => void
  onError: (error: Error) => void
  onFinish: () => void
}
interface Connection {
  controller: AbortController
  projection: AgentStreamProjection
  reader?: ReadableStreamDefaultReader<UIMessageChunk>
}

/** Owns only the browser's display connection. Command admission, cancellation
 * of execution and durable recovery remain in the existing product hooks. */
export function useAgentStream(options: StreamOptions) {
  const callbacks = useRef(options)
  callbacks.current = options
  const [messages, publishMessages] = useState<AgentUIMessage[]>([])
  const currentMessages = useRef(messages)
  const [status, setStatus] = useState<Status>('ready')
  const [completion, setCompletion] = useState<AgentStreamCompletion>(null)
  const [error, setError] = useState<Error>()
  const connection = useRef<Connection | null>(null)
  const chatID = useRef(randomUUID())
  const setMessages = useCallback((update: MessageUpdate) => {
    const next = typeof update === 'function' ? update(currentMessages.current) : update
    currentMessages.current = next
    publishMessages(next)
  }, [])
  const batcher = useMemo(() => createRafUpdateBatcher<AgentUIMessage[]>(setMessages, {
    minIntervalMs: options.throttle,
  }), [setMessages, options.throttle])

  const disconnect = useCallback(() => {
    const active = connection.current
    connection.current = null
    if (active) {
      active.controller.abort()
      void active.reader?.cancel().catch(() => {})
      active.projection.finish()
      batcher.flush()
      setMessages(current => applyAgentStreamUpdates(current, active.projection.takeUpdates()))
    }
  }, [batcher, setMessages])

  const stop = useCallback(() => {
    disconnect()
    setCompletion('disconnected')
    setStatus('ready')
  }, [disconnect])

  const connect = useCallback(async (mode: 'submit' | 'resume', request: (active: Connection) => Promise<ReadableStream<UIMessageChunk> | null>): Promise<boolean> => {
    disconnect()
    const active: Connection = { controller: new AbortController(), projection: new AgentStreamProjection() }
    connection.current = active
    // A reconnect belongs to an accepted task. Preserve its display status
    // until attachment resolves so recovery cannot mistake it for a new POST.
    if (mode === 'submit') {
      setCompletion(null)
      setError(undefined)
      setStatus('submitted')
    }
    try {
      const stream = await request(active)
      if (connection.current !== active) {
        await stream?.cancel()
        return true
      }
      if (!stream) { setStatus('ready'); return true }
      active.reader = stream.getReader()
      if (mode === 'resume') {
        setCompletion(null)
        setError(undefined)
        setStatus('streaming')
      }
      let completion: AgentStreamCompletion = 'disconnected'
      while (connection.current === active) {
        const { done, value } = await active.reader.read()
        if (done || connection.current !== active) break
        setStatus('streaming')
        active.projection.consume(value)
        if (value.type === 'finish' || value.type === 'abort') completion = 'terminal'
        batcher.enqueue(current => applyAgentStreamUpdates(current, active.projection.takeUpdates()))
        if (isAgentDataChunk(value)) callbacks.current.onData(value as DataPart)
      }
      if (connection.current === active) {
        setCompletion(completion)
        setStatus('ready')
      }
      return true
    } catch (cause) {
      if (connection.current !== active || active.controller.signal.aborted) return true
      const failure = cause instanceof Error ? cause : new Error(String(cause))
      console.warn('[use-agent-stream.ts] Agent display stream failed', failure)
      setError(failure)
      setStatus('error')
      callbacks.current.onError(failure)
      return false
    } finally {
      if (connection.current === active) {
        active.projection.finish()
        batcher.flush()
        setMessages(current => applyAgentStreamUpdates(current, active.projection.takeUpdates()))
        connection.current = null
        // Failed attachments have no response to finish. Their owner refreshes
        // the active projection instead of firing completion side effects.
        if (mode === 'submit' || active.reader) callbacks.current.onFinish()
      }
      void active.reader?.cancel().catch(() => {})
    }
  }, [batcher, disconnect, setMessages])

  const sendMessage = useCallback((message: Omit<AgentUIMessage, 'id'> & { id?: string }, request: ChatRequestOptions = {}) => {
    const user = { ...message, id: message.id || randomUUID() }
    setMessages(current => [...current, user])
    return connect('submit', active => options.transport.sendMessages({
      ...request, trigger: 'submit-message', chatId: chatID.current, messageId: user.id,
      // Denova submits just the new input; history is owned by the server.
      messages: [user], abortSignal: active.controller.signal,
    }))
  }, [connect, options.transport, setMessages])
  const resumeStream = useCallback(() => connect('resume', active => options.transport.reconnectToStream({ chatId: chatID.current }, active.controller.signal)), [connect, options.transport])

  useEffect(() => () => {
    const active = connection.current
    connection.current = null
    active?.controller.abort()
    void active?.reader?.cancel().catch(() => {})
    batcher.discard()
  }, [batcher, options.transport])

  return { messages, setMessages, sendMessage, resumeStream, stop, status, error, completion }
}
