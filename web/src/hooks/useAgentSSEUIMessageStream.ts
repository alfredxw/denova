import { useCallback, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import type { SSEEvent } from '@/lib/api'
import { normalizeAgentUIMessages, type AgentMessageMetadata, type AgentUIMessage } from '@/lib/agent-ui'
import { createAgentDataMessage } from './useAgentUIMessageStream'

interface AgentSSEUIMessageStreamOptions {
  onEvent?: (event: SSEEvent, data: Record<string, unknown>) => void
}

type AgentMessageUpdater = SetStateAction<AgentUIMessage[]>

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

  const setMessages = useCallback((updater: AgentMessageUpdater) => {
    rawSetMessages((current) => {
      const next = typeof updater === 'function'
        ? (updater as (value: AgentUIMessage[]) => AgentUIMessage[])(current)
        : updater
      return normalizeAgentUIMessages(next)
    })
  }, []) as Dispatch<SetStateAction<AgentUIMessage[]>>

  const resetStreamingState = useCallback(() => {
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    textSegmentIDRef.current = ''
    reasoningSegmentIDRef.current = ''
    toolInputsRef.current = {}
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
    setIsStreaming(true)
    setActivityContent(t('chat.activity.connecting'))
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
          case 'chunk':
            appendStreamingText(rawSetMessages, textSegmentIDRef, 'text', readString(data.content), metadata)
            setActivityContent('')
            break
          case 'thinking':
            appendStreamingText(rawSetMessages, reasoningSegmentIDRef, 'reasoning', readString(data.content), metadata)
            setActivityContent(t('chat.activity.thinking'))
            break
          case 'tool_call':
            upsertTool(rawSetMessages, toolInputsRef, toolCounterRef, data, metadata, 'input-available')
            setActivityContent('')
            break
          case 'tool_args_delta':
            appendToolArgs(rawSetMessages, toolInputsRef, toolCounterRef, data, metadata)
            break
          case 'tool_result':
            upsertTool(rawSetMessages, toolInputsRef, toolCounterRef, data, metadata, data.status === 'error' ? 'output-error' : 'output-available')
            setActivityContent('')
            break
          case 'done':
            finishStreamingParts(rawSetMessages)
            setActivityContent('')
            break
          case 'aborted':
            finishStreamingParts(rawSetMessages)
            setActivityContent(t('chat.activity.aborted'))
            break
          case 'error':
            finishStreamingParts(rawSetMessages)
            setMessages(prev => [...prev, createAgentDataMessage('agent-error', { content: readString(data.message) || readString(data.error) || t('chat.activity.unknownError') })])
            setActivityContent('')
            break
          default:
            setActivityContent('')
            break
        }
      }
      finishStreamingParts(rawSetMessages)
    } catch (error) {
      finishStreamingParts(rawSetMessages)
      if (isAbortError(error)) {
        setActivityContent(t('chat.activity.aborted'))
      } else {
        setMessages(prev => [...prev, createAgentDataMessage('agent-error', { content: t('chat.activity.requestFailed', { error: String(error) }) })])
      }
    } finally {
      abortControllerRef.current = null
      textSegmentIDRef.current = ''
      reasoningSegmentIDRef.current = ''
      toolInputsRef.current = {}
      setIsStreaming(false)
      setActivityContent('')
    }
  }, [onEvent, setMessages, t])

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

function appendStreamingText(
  setMessages: Dispatch<SetStateAction<AgentUIMessage[]>>,
  segmentIDRef: { current: string },
  partType: 'text' | 'reasoning',
  content: string,
  metadata?: AgentMessageMetadata,
) {
  if (!content) return
  if (!segmentIDRef.current) {
    segmentIDRef.current = localSSEMessageID(partType)
    const part = partType === 'text'
      ? { type: 'text', text: content, state: 'streaming' }
      : { type: 'reasoning', text: content, state: 'streaming' }
    setMessages(current => normalizeAgentUIMessages([...current, {
      id: segmentIDRef.current,
      role: 'assistant',
      metadata,
      parts: [part],
    } as AgentUIMessage]))
    return
  }
  const messageID = segmentIDRef.current
  setMessages(current => normalizeAgentUIMessages(current.map((message) => {
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
  })))
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

function appendToolArgs(
  setMessages: Dispatch<SetStateAction<AgentUIMessage[]>>,
  toolInputsRef: { current: Record<string, string> },
  counterRef: { current: number },
  data: Record<string, unknown>,
  metadata: AgentMessageMetadata,
) {
  const toolID = toolEventID(data, counterRef)
  const delta = readString(data.delta)
  if (!delta) return
  toolInputsRef.current[toolID] = `${toolInputsRef.current[toolID] || ''}${delta}`
  upsertTool(setMessages, toolInputsRef, counterRef, data, metadata, 'input-streaming')
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
