import { useCallback, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { readUIMessageStream, type UIMessageChunk } from 'ai'
import { buildAgentMessageViews, type AgentMessageView } from '@/lib/agent-message-view'
import { normalizeAgentUIMessages, type AgentDataParts, type AgentMessageMetadata, type AgentUIMessage } from '@/lib/agent-ui'

interface AgentUIMessageStreamOptions {
  onView?: (view: AgentMessageView) => void
}

type AgentMessageUpdater = SetStateAction<AgentUIMessage[]>
interface ConsumeAgentUIStreamOptions {
  shouldContinue?: () => boolean
}

export function useAgentUIMessageStream(options: AgentUIMessageStreamOptions = {}) {
  const { onView } = options
  const [messages, rawSetMessages] = useState<AgentUIMessage[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [activityContent, setActivityContent] = useState('')
  const abortControllerRef = useRef<AbortController | null>(null)

  const setMessages = useCallback((updater: AgentMessageUpdater) => {
    rawSetMessages((current) => {
      const next = typeof updater === 'function'
        ? (updater as (value: AgentUIMessage[]) => AgentUIMessage[])(current)
        : updater
      return normalizeAgentUIMessages(next)
    })
  }, []) as Dispatch<SetStateAction<AgentUIMessage[]>>

  const resetStreamingState = useCallback(() => {
    setIsStreaming(false)
    setActivityContent('')
    abortControllerRef.current = null
  }, [])

  const setAbortController = useCallback((controller: AbortController | null) => {
    abortControllerRef.current = controller
  }, [])

  const abortLocalStream = useCallback(() => {
    abortControllerRef.current?.abort()
    resetStreamingState()
  }, [resetStreamingState])

  const consumeAgentUIStream = useCallback(async (stream: ReadableStream<UIMessageChunk>, consumeOptions: ConsumeAgentUIStreamOptions = {}) => {
    setIsStreaming(true)
    setActivityContent('')
    try {
      for await (const message of readUIMessageStream<AgentUIMessage>({
        stream,
        terminateOnError: true,
      })) {
        if (consumeOptions.shouldContinue && !consumeOptions.shouldContinue()) break
        const normalized = normalizeAgentUIMessages([message])[0] || message
        rawSetMessages(current => normalizeAgentUIMessages(upsertAgentUIMessage(current, normalized)))
        if (onView) {
          for (const view of buildAgentMessageViews([normalized])) onView(view)
        }
      }
    } finally {
      resetStreamingState()
    }
  }, [onView, resetStreamingState])

  return {
    messages,
    setMessages,
    isStreaming,
    activityContent,
    consumeAgentUIStream,
    resetStreamingState,
    setAbortController,
    abortLocalStream,
  }
}

export function createAgentTextMessage(role: 'user' | 'system' | 'assistant', content: string, metadata?: AgentMessageMetadata): AgentUIMessage {
  return {
    id: localAgentMessageID(role),
    role,
    metadata,
    parts: [{ type: 'text', text: content }],
  } as AgentUIMessage
}

export function createAgentDataMessage(type: keyof AgentDataParts, data: Record<string, unknown>, metadata?: AgentMessageMetadata): AgentUIMessage {
  const partType = `data-${type}` as const
  return {
    id: localAgentMessageID(type),
    role: 'assistant',
    metadata,
    parts: [{ type: partType, id: localAgentMessageID(type), data }],
  } as AgentUIMessage
}

function upsertAgentUIMessage(messages: AgentUIMessage[], next: AgentUIMessage) {
  const index = messages.findIndex(message => message.id === next.id)
  if (index < 0) return [...messages, next]
  return messages.map((message, messageIndex) => messageIndex === index ? next : message)
}

function localAgentMessageID(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}
