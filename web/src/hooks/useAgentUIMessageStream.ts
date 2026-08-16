import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { readUIMessageStream, type UIMessageChunk } from 'ai'
import { buildAgentMessageViews, type AgentMessageView } from '@/lib/agent-message-view'
import { AgentUIMessageNormalizer, normalizeAgentUIMessages, type AgentUIMessage } from '@/lib/agent-ui'
import { attachAgentToolInputText, recordAgentToolInputChunk } from '@/lib/agent-ui-message'
import { createRafUpdateBatcher, STREAMING_RENDER_INTERVAL_MS, type RafUpdateBatcher } from '@/lib/streaming/raf-update-batcher'

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
  const messageNormalizerRef = useRef<AgentUIMessageNormalizer | null>(null)
  messageNormalizerRef.current ??= new AgentUIMessageNormalizer()
  const messageBatcherRef = useRef<RafUpdateBatcher<AgentUIMessage[]> | null>(null)
  const messageBatcher = messageBatcherRef.current ?? createRafUpdateBatcher(rawSetMessages, {
    minIntervalMs: STREAMING_RENDER_INTERVAL_MS,
  })
  messageBatcherRef.current = messageBatcher

  const setMessages = useCallback((updater: AgentMessageUpdater) => {
    messageBatcher.discard()
    rawSetMessages((current) => {
      const next = typeof updater === 'function'
        ? (updater as (value: AgentUIMessage[]) => AgentUIMessage[])(current)
        : updater
      return messageNormalizerRef.current!.normalize(next)
    })
  }, [messageBatcher]) as Dispatch<SetStateAction<AgentUIMessage[]>>

  useEffect(() => () => messageBatcher.discard(), [messageBatcher])

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
    const inputTextByToolCall = new Map<string, string>()
    const observedStream = stream.pipeThrough(new TransformStream<UIMessageChunk, UIMessageChunk>({
      transform(chunk, controller) {
        recordAgentToolInputChunk(chunk, inputTextByToolCall)
        controller.enqueue(chunk)
      },
    }))
    try {
      for await (const message of readUIMessageStream<AgentUIMessage>({
        stream: observedStream,
        terminateOnError: true,
      })) {
        if (consumeOptions.shouldContinue && !consumeOptions.shouldContinue()) break
        const messageWithInputText = attachAgentToolInputText(message, inputTextByToolCall)
        const normalized = normalizeAgentUIMessages([messageWithInputText])[0] || messageWithInputText
        messageBatcher.enqueue(current => messageNormalizerRef.current!.normalize(upsertAgentUIMessage(current, normalized)))
        if (onView) {
          for (const view of buildAgentMessageViews([normalized])) onView(view)
        }
      }
    } finally {
      messageBatcher.flush()
      resetStreamingState()
    }
  }, [messageBatcher, onView, resetStreamingState])

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

function upsertAgentUIMessage(messages: AgentUIMessage[], next: AgentUIMessage) {
  const index = messages.findIndex(message => message.id === next.id)
  if (index < 0) return [...messages, next]
  return messages.map((message, messageIndex) => messageIndex === index ? next : message)
}
