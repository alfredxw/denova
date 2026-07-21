import { useCallback, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import type { SSEEvent } from '@/lib/api'
import { normalizeAgentUIMessages, type AgentMessageMetadata, type AgentUIMessage } from '@/lib/agent-ui'
import { createAgentDataMessage } from './useAgentUIMessageStream'

interface AgentSSEUIMessageStreamOptions {
  onEvent?: (event: SSEEvent, data: Record<string, unknown>) => void
}

type AgentMessageUpdater = SetStateAction<AgentUIMessage[]>

/** A buffered streaming text chunk awaiting rAF flush. */
interface PendingTextChunk {
  segmentIDRef: { current: string }
  partType: 'text' | 'reasoning'
  content: string
  metadata?: AgentMessageMetadata
}

/** A buffered tool args delta awaiting rAF flush. */
interface PendingToolArgsDelta {
  toolID: string
  data: Record<string, unknown>
  metadata: AgentMessageMetadata
}

type PendingChunk =
  | { kind: 'text'; chunk: PendingTextChunk }
  | { kind: 'tool_args'; delta: PendingToolArgsDelta }

export function useAgentSSEUIMessageStream(options: AgentSSEUIMessageStreamOptions = {}) {
  const { t } = useTranslation()
  const { onEvent } = options
  const [messages, rawSetMessages] = useState<AgentUIMessage[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [activityContent, setActivityContent] = useState('')
  const abortControllerRef = useRef<AbortController | null>(null)
  const textSegmentIDRef = useRef('')
  const reasoningSegmentIDRef = useRef('')
  const toolInputsRef = useRef<Record<string, string>>({})
  const toolCounterRef = useRef(0)
  // rAF batch buffer: streaming deltas are queued here and flushed once per frame.
  const pendingChunksRef = useRef<PendingChunk[]>([])
  const rafRef = useRef<number>(0)

  const setMessages = useCallback((updater: AgentMessageUpdater) => {
    rawSetMessages((current) => {
      const next = typeof updater === 'function'
        ? (updater as (value: AgentUIMessage[]) => AgentUIMessage[])(current)
        : updater
      return normalizeAgentUIMessages(next)
    })
  }, []) as Dispatch<SetStateAction<AgentUIMessage[]>>

  /** Flush all buffered streaming chunks into a single setState (no normalization). */
  const flushPendingChunks = useCallback(() => {
    if (rafRef.current) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = 0
    }
    const pending = pendingChunksRef.current
    if (pending.length === 0) return
    pendingChunksRef.current = []
    rawSetMessages(current => {
      let next = current
      for (const entry of pending) {
        if (entry.kind === 'text') {
          next = applyTextChunk(next, entry.chunk)
        } else {
          next = applyToolArgsDelta(next, entry.delta, toolInputsRef)
        }
      }
      return next
    })
  }, [])

  /** Schedule a rAF flush if one is not already pending. */
  const scheduleFlush = useCallback(() => {
    if (rafRef.current) return
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = 0
      flushPendingChunks()
    })
  }, [flushPendingChunks])

  const resetStreamingState = useCallback(() => {
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    textSegmentIDRef.current = ''
    reasoningSegmentIDRef.current = ''
    toolInputsRef.current = {}
    if (rafRef.current) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = 0
    }
    pendingChunksRef.current = []
    setIsStreaming(false)
    setActivityContent('')
  }, [])

  const setAbortController = useCallback((controller: AbortController | null) => {
    abortControllerRef.current = controller
  }, [])

  const abortLocalStream = useCallback(() => {
    abortControllerRef.current?.abort()
  }, [])

  const consumeAgentSSEStream = useCallback(async (stream: ReadableStream<SSEEvent>) => {
    textSegmentIDRef.current = ''
    reasoningSegmentIDRef.current = ''
    toolInputsRef.current = {}
    pendingChunksRef.current = []
    setIsStreaming(true)
    setActivityContent(t('chat.activity.thinking'))
    try {
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const event = value as SSEEvent
        const data = parseEventData(event.data)
        const metadata = readEventMetadata(data)
        onEvent?.(event, data)
        switch (event.event) {
          case 'chunk': {
            const content = readString(data.content)
            if (content) {
              pendingChunksRef.current.push({ kind: 'text', chunk: { segmentIDRef: textSegmentIDRef, partType: 'text', content, metadata } })
              scheduleFlush()
            }
            setActivityContent('')
            break
          }
          case 'thinking': {
            const content = readString(data.content)
            if (content) {
              pendingChunksRef.current.push({ kind: 'text', chunk: { segmentIDRef: reasoningSegmentIDRef, partType: 'reasoning', content, metadata } })
              scheduleFlush()
            }
            setActivityContent(t('chat.activity.thinking'))
            break
          }
          case 'tool_call':
            flushPendingChunks()
            upsertTool(rawSetMessages, toolInputsRef, toolCounterRef, data, metadata, 'input-available')
            setActivityContent('')
            break
          case 'tool_args_delta': {
            const toolID = toolEventID(data, toolCounterRef)
            const delta = readString(data.delta)
            if (delta) {
              toolInputsRef.current[toolID] = `${toolInputsRef.current[toolID] || ''}${delta}`
              pendingChunksRef.current.push({ kind: 'tool_args', delta: { toolID, data, metadata } })
              scheduleFlush()
            }
            break
          }
          case 'tool_result':
            flushPendingChunks()
            upsertTool(rawSetMessages, toolInputsRef, toolCounterRef, data, metadata, data.status === 'error' ? 'output-error' : 'output-available')
            setActivityContent('')
            break
          case 'done':
            flushPendingChunks()
            finishStreamingParts(rawSetMessages)
            setActivityContent('')
            break
          case 'aborted':
            flushPendingChunks()
            finishStreamingParts(rawSetMessages)
            setActivityContent(t('chat.activity.aborted'))
            break
          case 'error':
            flushPendingChunks()
            finishStreamingParts(rawSetMessages)
            setMessages(prev => [...prev, createAgentDataMessage('agent-error', { content: readString(data.message) || readString(data.error) || t('chat.activity.unknownError') })])
            setActivityContent('')
            break
          default:
            setActivityContent('')
            break
        }
      }
      flushPendingChunks()
      finishStreamingParts(rawSetMessages)
    } catch (error) {
      flushPendingChunks()
      finishStreamingParts(rawSetMessages)
      if (isAbortError(error)) {
        setActivityContent(t('chat.activity.aborted'))
      } else {
        setMessages(prev => [...prev, createAgentDataMessage('agent-error', { content: t('chat.activity.requestFailed', { error: String(error) }) })])
      }
    } finally {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = 0
      }
      pendingChunksRef.current = []
      abortControllerRef.current = null
      textSegmentIDRef.current = ''
      reasoningSegmentIDRef.current = ''
      toolInputsRef.current = {}
      setIsStreaming(false)
      setActivityContent('')
    }
  }, [onEvent, setMessages, t, flushPendingChunks, scheduleFlush])

  return {
    messages,
    setMessages,
    isStreaming,
    activityContent,
    consumeAgentSSEStream,
    resetStreamingState,
    setAbortController,
    abortLocalStream,
  }
}

/**
 * Apply a buffered text chunk to the message array without normalization.
 * First chunk for a segment creates a new message; subsequent chunks append text.
 */
function applyTextChunk(messages: AgentUIMessage[], chunk: PendingTextChunk): AgentUIMessage[] {
  const { segmentIDRef, partType, content, metadata } = chunk
  if (!segmentIDRef.current) {
    segmentIDRef.current = localSSEMessageID(partType)
    const part = partType === 'text'
      ? { type: 'text', text: content, state: 'streaming' }
      : { type: 'reasoning', text: content, state: 'streaming' }
    return [...messages, {
      id: segmentIDRef.current,
      role: 'assistant',
      metadata,
      parts: [part],
    } as AgentUIMessage]
  }
  const messageID = segmentIDRef.current
  return messages.map((message) => {
    if (message.id !== messageID) return message
    return {
      ...message,
      metadata: { ...message.metadata, ...metadata },
      parts: message.parts.map((part, index) => {
        if (index !== 0) return part
        const raw = part as Record<string, unknown>
        return { ...raw, text: `${readString(raw.text)}${content}`, state: 'streaming' } as AgentUIMessage['parts'][number]
      }),
    } as AgentUIMessage
  })
}

/**
 * Apply a buffered tool args delta by upserting the tool card without normalization.
 */
function applyToolArgsDelta(
  messages: AgentUIMessage[],
  delta: PendingToolArgsDelta,
  toolInputsRef: { current: Record<string, string> },
): AgentUIMessage[] {
  const { toolID, data, metadata } = delta
  const toolName = readString(data.name) || 'unknown_tool'
  const inputText = toolInputsRef.current[toolID] || ''
  const part: Record<string, unknown> = {
    type: 'dynamic-tool',
    toolName,
    toolCallId: toolID,
    state: 'input-streaming',
    input: parseJSONValue(inputText),
    providerMetadata: { agent: metadata },
  }
  return upsertAgentMessage(messages, {
    id: toolID,
    role: 'assistant',
    metadata,
    parts: [part as AgentUIMessage['parts'][number]],
  } as AgentUIMessage)
}

function upsertTool(
  setMessages: Dispatch<SetStateAction<AgentUIMessage[]>>,
  toolInputsRef: { current: Record<string, string> },
  counterRef: { current: number },
  data: Record<string, unknown>,
  metadata: AgentMessageMetadata,
  state: string,
) {
  const toolID = toolEventID(data, counterRef)
  const toolName = readString(data.name) || 'unknown_tool'
  const inputText = readString(data.args) || toolInputsRef.current[toolID] || ''
  if (inputText) toolInputsRef.current[toolID] = inputText
  const output = readString(data.result) || readString(data.content)
  const part: Record<string, unknown> = {
    type: 'dynamic-tool',
    toolName,
    toolCallId: toolID,
    state,
    input: parseJSONValue(inputText),
    providerMetadata: { agent: metadata },
  }
  if (state === 'output-error') part.errorText = output
  else if (output) part.output = output

  setMessages(current => normalizeAgentUIMessages(upsertAgentMessage(current, {
    id: toolID,
    role: 'assistant',
    metadata,
    parts: [part as AgentUIMessage['parts'][number]],
  } as AgentUIMessage)))
}


function finishStreamingParts(setMessages: Dispatch<SetStateAction<AgentUIMessage[]>>) {
  setMessages(current => normalizeAgentUIMessages(current.map((message) => ({
    ...message,
    parts: message.parts.map((part) => {
      const raw = part as Record<string, unknown>
      if ((raw.type === 'text' || raw.type === 'reasoning') && raw.state === 'streaming') {
        return { ...raw, state: 'done' } as AgentUIMessage['parts'][number]
      }
      if ((raw.type === 'dynamic-tool' || readString(raw.type).startsWith('tool-')) && raw.state === 'input-streaming') {
        return { ...raw, state: 'input-available' } as AgentUIMessage['parts'][number]
      }
      return part
    }),
  } as AgentUIMessage))))
}

function upsertAgentMessage(messages: AgentUIMessage[], next: AgentUIMessage) {
  const index = messages.findIndex(message => message.id === next.id)
  if (index < 0) return [...messages, next]
  return messages.map((message, messageIndex) => messageIndex === index ? next : message)
}

function toolEventID(data: Record<string, unknown>, counterRef: { current: number }) {
  const id = readString(data.id) || readString(data.tool_call_id) || readString(data.toolCallId)
  if (id) return id
  counterRef.current += 1
  return `sse-tool-${counterRef.current}`
}

function parseEventData(raw: string): Record<string, unknown> {
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw) as unknown
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : { message: raw }
  } catch {
    return { message: raw }
  }
}

function readEventMetadata(data: Record<string, unknown>): AgentMessageMetadata {
  const metadata: AgentMessageMetadata = {
    run_id: readString(data.run_id) || undefined,
    agent_kind: readString(data.agent_kind) || undefined,
    agent_name: readString(data.agent_name) || undefined,
    root_agent_name: readString(data.root_agent_name) || undefined,
    run_path: readStringArray(data.run_path),
    subagent: data.subagent === true || undefined,
    subagent_session_id: readString(data.subagent_session_id) || undefined,
    subagent_type: readString(data.subagent_type) || undefined,
  }
  return Object.fromEntries(Object.entries(metadata).filter(([, value]) => value !== undefined && value !== '')) as AgentMessageMetadata
}

function parseJSONValue(value: string) {
  if (!value) return undefined
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const result = value.filter((item): item is string => typeof item === 'string')
  return result.length ? result : undefined
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}

function localSSEMessageID(prefix: string) {
  return `sse-${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}
