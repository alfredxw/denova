import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChapterIllustration, ChatMessage, SSEEvent } from '@/lib/api'
import { buildContextCompactionMessage, createContextCompactionMessageId, upsertContextCompactionMessage } from '@/components/Chat/context-compaction-message'

interface AgentEventStreamOptions {
  onAgentFileChange?: (path?: string) => void | Promise<void>
  onEvent?: (event: SSEEvent, data: Record<string, unknown>) => void
}

interface ConsumeAgentStreamOptions {
  clearInputsOnFinish?: () => void
  showAbortMessage?: boolean
}

interface ToolCallInfo {
  id: string
  name: string
  args: string
}

type StreamSegmentRole = 'assistant' | 'thinking'
type EventMetadata = Pick<ChatMessage, 'run_id' | 'agent_name' | 'root_agent_name' | 'run_path' | 'subagent' | 'subagent_session_id' | 'subagent_type'>
type EventDisplayMetadata = Pick<ChatMessage, 'sse_hidden_fields' | 'sse_hidden_reason' | 'sse_display_notice' | 'sse_generated_chars'>

const PLAN_PREAMBLE_MAX_CHARS = 1200
const PLAN_THINKING_BUFFER_MAX_CHARS = 2000
const PLAN_THINKING_PREVIEW_MAX_CHARS = 160
const PLAN_PROTOCOL_TOOL_EVENT_ID = 'plan_protocol_tool'

/** Shared SSE consumer for Agent-like streams. It keeps text/thinking/tool events on one timeline. */
export function useAgentEventStream(options: AgentEventStreamOptions = {}) {
  const { t } = useTranslation()
  const { onAgentFileChange, onEvent } = options
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [activityContent, setActivityContent] = useState('')
  const abortControllerRef = useRef<AbortController | null>(null)
  const currentSegmentIdRef = useRef<string | null>(null)
  const currentSegmentRoleRef = useRef<StreamSegmentRole | null>(null)
  const currentSegmentSourceRef = useRef<string | null>(null)
  const segmentIdCounterRef = useRef(0)
  const pendingToolCallsRef = useRef<Record<string, ToolCallInfo>>({})
  const toolCallQueueRef = useRef<string[]>([])
  const toolKeyToMessageIdRef = useRef<Record<string, string>>({})
  const toolIdCounterRef = useRef(0)
  const currentCompactionMessageIdRef = useRef<string | null>(null)
  const compactionIdCounterRef = useRef(0)
  const segmentBufferRef = useRef<Record<string, string>>({})
  const segmentRafRef = useRef<number | null>(null)
  const segmentPromoteRafRef = useRef<number | null>(null)
  const deltaBufferRef = useRef<Record<string, string>>({})
  const deltaRafRef = useRef<number | null>(null)
  const planThinkingBufferRef = useRef('')
  const planThinkingPreviewRef = useRef('')
  const discardNextAssistantAfterPlanRef = useRef(false)
  const discardAssistantSegmentIdsRef = useRef<Set<string>>(new Set())

  const resetStreamingState = useCallback(() => {
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    currentSegmentIdRef.current = null
    currentSegmentRoleRef.current = null
    currentSegmentSourceRef.current = null
    pendingToolCallsRef.current = {}
    toolCallQueueRef.current = []
    toolKeyToMessageIdRef.current = {}
    currentCompactionMessageIdRef.current = null
    segmentBufferRef.current = {}
    deltaBufferRef.current = {}
    planThinkingBufferRef.current = ''
    planThinkingPreviewRef.current = ''
    discardNextAssistantAfterPlanRef.current = false
    discardAssistantSegmentIdsRef.current = new Set()
    if (segmentRafRef.current !== null) {
      cancelAnimationFrame(segmentRafRef.current)
      segmentRafRef.current = null
    }
    if (segmentPromoteRafRef.current !== null) {
      cancelAnimationFrame(segmentPromoteRafRef.current)
      segmentPromoteRafRef.current = null
    }
    if (deltaRafRef.current !== null) {
      cancelAnimationFrame(deltaRafRef.current)
      deltaRafRef.current = null
    }
    setIsStreaming(false)
    setActivityContent('')
  }, [])

  const setAbortController = useCallback((controller: AbortController | null) => {
    abortControllerRef.current = controller
  }, [])

  const abortLocalStream = useCallback(() => {
    abortControllerRef.current?.abort()
  }, [])

  const markPendingToolsAsError = useCallback(() => {
    const pendingIds = new Set(Object.keys(pendingToolCallsRef.current))
    if (pendingIds.size === 0) return
    setMessages(prev => prev.map(message => (
      message.role === 'tool_call' && message.id && pendingIds.has(message.id)
        ? { ...message, status: 'error', streaming: false }
        : message
    )))
  }, [])

  const markPendingToolsAsSuccess = useCallback(() => {
    const pendingIds = new Set(Object.keys(pendingToolCallsRef.current))
    if (pendingIds.size === 0) return
    setMessages(prev => prev.map(message => (
      message.role === 'tool_call' && message.id && pendingIds.has(message.id)
        ? { ...message, status: message.status === 'error' ? 'error' : 'success', streaming: false }
        : message
    )))
    pendingToolCallsRef.current = {}
    toolCallQueueRef.current = []
  }, [])

  const flushToolArgBuffer = useCallback(() => {
    const buffered = { ...deltaBufferRef.current }
    deltaBufferRef.current = {}
    if (deltaRafRef.current !== null) {
      cancelAnimationFrame(deltaRafRef.current)
      deltaRafRef.current = null
    }
    if (Object.keys(buffered).length === 0) return
    setMessages(prev => prev.map(message => {
      if (message.role === 'tool_call' && message.id && buffered[message.id]) {
        const args = (message.args || '') + buffered[message.id]
        return { ...message, args, content: buildToolContent(message.name || 'unknown_tool', args), subagent_type: message.name === 'task' ? parseTaskSubagentType(args) : message.subagent_type }
      }
      return message
    }))
  }, [])

  const scheduleStreamingSegmentPromotion = useCallback(() => {
    if (segmentPromoteRafRef.current !== null) return
    segmentPromoteRafRef.current = requestAnimationFrame(() => {
      segmentPromoteRafRef.current = null
      setMessages(prev => promoteStreamingTargets(prev))
    })
  }, [])

  const flushStreamingSegmentBuffer = useCallback(() => {
    const buffered = { ...segmentBufferRef.current }
    segmentBufferRef.current = {}
    if (segmentRafRef.current !== null) {
      cancelAnimationFrame(segmentRafRef.current)
      segmentRafRef.current = null
    }
    if (Object.keys(buffered).length === 0) return
    setMessages(prev => updateStreamingSegments(prev, buffered))
    scheduleStreamingSegmentPromotion()
  }, [scheduleStreamingSegmentPromotion])

  const finishCurrentSegment = useCallback((options: { discardPlanPreamble?: boolean } = {}) => {
    const segmentId = currentSegmentIdRef.current
    if (!segmentId) return
    const role = currentSegmentRoleRef.current
    const shouldDiscardPlanFollowup = role === 'assistant' && discardAssistantSegmentIdsRef.current.has(segmentId)
    flushStreamingSegmentBuffer()
    currentSegmentIdRef.current = null
    currentSegmentRoleRef.current = null
    currentSegmentSourceRef.current = null
    discardAssistantSegmentIdsRef.current.delete(segmentId)
    setMessages(prev => {
      const finalized = finalizeStreamingSegment(prev, segmentId)
      return (options.discardPlanPreamble || shouldDiscardPlanFollowup) && role === 'assistant'
        ? discardPlanPreambleSegment(finalized, segmentId)
        : finalized
    })
  }, [flushStreamingSegmentBuffer])

  const appendStreamingSegment = useCallback((role: StreamSegmentRole, text: string, metadata: EventMetadata = {}) => {
    if (!text) return
    const sourceKey = segmentSourceKey(metadata)
    if (currentSegmentRoleRef.current !== role || currentSegmentSourceRef.current !== sourceKey || !currentSegmentIdRef.current) {
      finishCurrentSegment()
      currentSegmentIdRef.current = createSegmentId(role, segmentIdCounterRef)
      currentSegmentRoleRef.current = role
      currentSegmentSourceRef.current = sourceKey
      const segmentId = currentSegmentIdRef.current
      if (!segmentId) return
      if (role === 'assistant' && discardNextAssistantAfterPlanRef.current) {
        discardAssistantSegmentIdsRef.current.add(segmentId)
        discardNextAssistantAfterPlanRef.current = false
      }
      setMessages(prev => appendStreamingSegmentMessage(prev, role, segmentId, text, metadata))
      scheduleStreamingSegmentPromotion()
      return
    }
    const segmentId = currentSegmentIdRef.current
    if (!segmentId) return
    segmentBufferRef.current[segmentId] = (segmentBufferRef.current[segmentId] || '') + text
    if (segmentRafRef.current === null) {
      segmentRafRef.current = requestAnimationFrame(() => flushStreamingSegmentBuffer())
    }
  }, [finishCurrentSegment, flushStreamingSegmentBuffer])

  const upsertPlanProtocolToolCard = useCallback((
    role: 'plan_question' | 'proposed_plan',
    rawContent: string,
    data: Record<string, unknown>,
    metadata: EventMetadata,
  ) => {
    const content = extractPlanProtocolToolContent(role, rawContent)
    if (!content) return
    const id = createPlanCardMessageId(role, { ...data, id: PLAN_PROTOCOL_TOOL_EVENT_ID }, metadata)
    discardNextAssistantAfterPlanRef.current = true
    setMessages(prev => upsertPlanCardMessage(prev, {
      content,
      id,
      role,
      status: 'success',
      streaming: false,
      ...metadata,
    }))
  }, [])

  const consumeAgentStream = useCallback(async (
    stream: ReadableStream<SSEEvent>,
    consumeOptions: ConsumeAgentStreamOptions = {},
  ) => {
    pendingToolCallsRef.current = {}
    toolCallQueueRef.current = []
    toolKeyToMessageIdRef.current = {}
    currentSegmentIdRef.current = null
    currentSegmentRoleRef.current = null
    currentSegmentSourceRef.current = null
    currentCompactionMessageIdRef.current = null
    segmentBufferRef.current = {}
    planThinkingBufferRef.current = ''
    planThinkingPreviewRef.current = ''
    discardNextAssistantAfterPlanRef.current = false
    discardAssistantSegmentIdsRef.current = new Set()
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
          case 'chunk': {
            appendStreamingSegment('assistant', readString(data.content), metadata)
            setActivityContent('')
            break
          }
          case 'thinking': {
            const content = readString(data.content)
            appendStreamingSegment('thinking', content, metadata)
            if (!metadata.subagent) {
              const nextPreview = updatePlanThinkingPreview(planThinkingBufferRef.current, content)
              planThinkingBufferRef.current = nextPreview.buffer
              if (nextPreview.preview && nextPreview.preview !== planThinkingPreviewRef.current) {
                planThinkingPreviewRef.current = nextPreview.preview
                setMessages(prev => updateLatestRunningPlanThinkingPreview(prev, nextPreview.preview))
              }
            }
            setActivityContent(t('chat.activity.thinking'))
            break
          }
          case 'tool_call': {
            finishCurrentSegment()
            const toolName = readString(data.name) || 'unknown_tool'
            const args = readString(data.args)
            const planRole = planRoleForProtocolTool(toolName)
            if (planRole) {
              upsertPlanProtocolToolCard(planRole, args, data, metadata)
              setActivityContent('')
              break
            }
            const toolKey = getToolEventKey(data)
            const existingToolId = toolKey ? toolKeyToMessageIdRef.current[toolKey] : undefined
            const toolId = existingToolId || createToolMessageId(toolKey, toolIdCounterRef)
            if (toolKey) {
              toolKeyToMessageIdRef.current = { ...toolKeyToMessageIdRef.current, [toolKey]: toolId }
            }
            pendingToolCallsRef.current = {
              ...pendingToolCallsRef.current,
              [toolId]: { id: toolId, name: toolName, args },
            }
            if (!toolCallQueueRef.current.includes(toolId)) {
              toolCallQueueRef.current = [...toolCallQueueRef.current, toolId]
            }
            setActivityContent('')
            setMessages(prev => upsertToolCallMessage(prev, {
              role: 'tool_call',
              content: buildToolContent(toolName, args),
              id: toolId,
              name: toolName,
              args,
              status: 'running',
              ...metadata,
              ...readEventDisplayMetadata(data),
              subagent_type: toolName === 'task' ? parseTaskSubagentType(args) : metadata.subagent_type,
            }))
            break
          }
          case 'tool_result': {
            const resultToolName = readString(data.name)
            if (isPlanProtocolToolName(resultToolName)) {
              setActivityContent('')
              break
            }
            flushToolArgBuffer()
            const content = readString(data.content)
            const illustration = readChapterIllustration(data.illustration)
            const toolId = findToolMessageId(data, toolKeyToMessageIdRef.current, toolCallQueueRef.current, pendingToolCallsRef.current)
            const toolCall = toolId ? pendingToolCallsRef.current[toolId] : undefined
            const toolName = readString(data.name) || toolCall?.name || ''
            if (toolId) {
              const { [toolId]: _, ...restPending } = pendingToolCallsRef.current
              pendingToolCallsRef.current = restPending
              toolCallQueueRef.current = toolCallQueueRef.current.filter(id => id !== toolId)
            }
            setActivityContent('')
            if (toolId) {
              setMessages(prev => prev.map(message => (
                message.role === 'tool_call' && message.id === toolId
                  ? { ...message, status: 'success', result: content, illustration, streaming: false, ...metadata }
                  : message
              )))
            } else {
              setMessages(prev => [...prev, { role: 'tool_result', content, illustration, ...metadata }])
            }
            if (toolCall && isFileMutationTool(toolCall.name)) {
              void onAgentFileChange?.(extractToolPath(toolCall.args))
            }
            if (illustration) {
              void onAgentFileChange?.(illustration.meta_path || illustration.image_path)
            }
            if (isLoreMutationTool(toolName)) {
              notifyLoreUpdated(readStringArray(data.item_ids))
            }
            break
          }
          case 'tool_args_delta': {
            if (isPlanProtocolToolName(readString(data.name))) {
              break
            }
            const delta = readString(data.delta)
            const displayMetadata = readEventDisplayMetadata(data)
            const toolId = findToolMessageId(data, toolKeyToMessageIdRef.current, toolCallQueueRef.current, pendingToolCallsRef.current)
            if (toolId) {
              const pending = pendingToolCallsRef.current[toolId]
              if (delta) {
                if (pending) {
                  pending.args = (pending.args || '') + delta
                }
                deltaBufferRef.current[toolId] = (deltaBufferRef.current[toolId] || '') + delta
                if (deltaRafRef.current === null) {
                  deltaRafRef.current = requestAnimationFrame(flushToolArgBuffer)
                }
              }
              if (Object.keys(displayMetadata).length > 0) {
                setMessages(prev => prev.map(message => (
                  message.role === 'tool_call' && message.id === toolId
                    ? { ...message, ...displayMetadata }
                    : message
                )))
              }
            }
            break
          }
          case 'context_compaction': {
            finishCurrentSegment()
            const status = readString(data.status)
            const compactionId: string = currentCompactionMessageIdRef.current || createContextCompactionMessageId(compactionIdCounterRef)
            currentCompactionMessageIdRef.current = compactionId
            setMessages(prev => upsertContextCompactionMessage(prev, buildContextCompactionMessage(data, compactionId)))
            setActivityContent('')
            if (status === 'completed' || status === 'failed') {
              currentCompactionMessageIdRef.current = null
            }
            break
          }
          case 'plan_question':
          case 'proposed_plan': {
            finishCurrentSegment({ discardPlanPreamble: true })
            const content = readString(data.content)
            const role = event.event === 'plan_question' ? 'plan_question' : 'proposed_plan'
            const status = normalizePlanCardStatus(readString(data.status), content)
            const id = createPlanCardMessageId(role, data, metadata)
            discardNextAssistantAfterPlanRef.current = true
            if (status === 'running') {
              planThinkingBufferRef.current = ''
              planThinkingPreviewRef.current = ''
            }
            setMessages(prev => upsertPlanCardMessage(prev, {
              content,
              id,
              role,
              status,
              streaming: status === 'running',
              thinking_preview: undefined,
              ...metadata,
            }))
            setActivityContent('')
            break
          }
          case 'token_usage': {
            finishCurrentSegment()
            setMessages(prev => upsertTokenUsageMessage(prev, buildTokenUsageMessage(data)))
            break
          }
          case 'done': {
            markPendingToolsAsSuccess()
            setActivityContent('')
            break
          }
          case 'aborted': {
            markPendingToolsAsError()
            setActivityContent(t('chat.activity.aborted'))
            break
          }
          case 'error': {
            markPendingToolsAsError()
            setActivityContent('')
            setMessages(prev => [...prev, { role: 'error', content: readString(data.message) || readString(data.error) || t('chat.activity.unknownError') }])
            break
          }
        }
      }

      flushToolArgBuffer()
      flushStreamingSegmentBuffer()
      finishCurrentSegment()
      markPendingToolsAsSuccess()
      consumeOptions.clearInputsOnFinish?.()
    } catch (e) {
      markPendingToolsAsError()
      flushToolArgBuffer()
      flushStreamingSegmentBuffer()
      finishCurrentSegment()
      if (isAbortError(e)) {
        setActivityContent(t('chat.activity.aborted'))
        if (consumeOptions.showAbortMessage) {
          setMessages(prev => [...prev, { role: 'system', content: t('chat.activity.abortMessage') }])
        }
      } else {
        setMessages(prev => [...prev, { role: 'error', content: t('chat.activity.requestFailed', { error: String(e) }) }])
      }
      consumeOptions.clearInputsOnFinish?.()
    } finally {
      abortControllerRef.current = null
      pendingToolCallsRef.current = {}
      toolCallQueueRef.current = []
      toolKeyToMessageIdRef.current = {}
      currentCompactionMessageIdRef.current = null
      planThinkingBufferRef.current = ''
      planThinkingPreviewRef.current = ''
      discardNextAssistantAfterPlanRef.current = false
      discardAssistantSegmentIdsRef.current = new Set()
      flushToolArgBuffer()
      flushStreamingSegmentBuffer()
      finishCurrentSegment()
      setIsStreaming(false)
      setActivityContent('')
    }
  }, [
    appendStreamingSegment,
    finishCurrentSegment,
    flushStreamingSegmentBuffer,
    flushToolArgBuffer,
    markPendingToolsAsError,
    markPendingToolsAsSuccess,
    onAgentFileChange,
    onEvent,
    t,
    upsertPlanProtocolToolCard,
  ])

  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort()
      abortControllerRef.current = null
    }
  }, [])

  return {
    messages,
    setMessages: setMessages as Dispatch<SetStateAction<ChatMessage[]>>,
    isStreaming,
    activityContent,
    consumeAgentStream,
    resetStreamingState,
    setAbortController,
    abortLocalStream,
  }
}

export function normalizeRepeatedMessages(messages: ChatMessage[]) {
  const normalized: ChatMessage[] = []
  for (const message of messages) {
    const prev = normalized[normalized.length - 1]
    if (
      prev &&
      prev.role === message.role &&
      normalizeMessageContent(prev.content || '') === normalizeMessageContent(message.content || '')
    ) {
      continue
    }
    normalized.push(message)
  }
  return normalized
}

export function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
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

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readChapterIllustration(value: unknown): ChapterIllustration | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const data = value as Record<string, unknown>
  const schema = readString(data.schema)
  const imagePath = readString(data.image_path)
  if (schema !== 'chapter_illustration.v1' || !imagePath) return undefined
  return {
    schema,
    chapter_path: readString(data.chapter_path),
    image_path: imagePath,
    meta_path: readString(data.meta_path),
    markdown: readString(data.markdown),
    alt_text: readString(data.alt_text),
    profile_id: readString(data.profile_id),
    provider: readString(data.provider),
    model: readString(data.model),
    size: readString(data.size) || undefined,
    quality: readString(data.quality) || undefined,
    output_format: readString(data.output_format) || undefined,
    created_at: readString(data.created_at) || undefined,
    revised_prompt: readString(data.revised_prompt) || undefined,
    mime_type: readString(data.mime_type) || undefined,
    size_bytes: readNumber(data.size_bytes),
  }
}

function readBool(value: unknown) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') return value === 'true'
  return false
}

function readNumber(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return undefined
}

function readEventMetadata(data: Record<string, unknown>): EventMetadata {
  const runPath = readStringArray(data.run_path)
  const metadata: EventMetadata = {
    run_id: readString(data.run_id) || undefined,
    agent_name: readString(data.agent_name) || undefined,
    root_agent_name: readString(data.root_agent_name) || undefined,
    run_path: runPath.length > 0 ? runPath : undefined,
    subagent: readBool(data.subagent),
    subagent_session_id: readString(data.subagent_session_id) || undefined,
  }
  const subagentType = readString(data.subagent_type) || parseTaskSubagentType(readString(data.args))
  if (subagentType) metadata.subagent_type = subagentType
  return metadata
}

function readEventDisplayMetadata(data: Record<string, unknown>): EventDisplayMetadata {
  const hiddenFields = readStringArray(data.sse_hidden_fields)
  const metadata: EventDisplayMetadata = {}
  if (hiddenFields.length > 0) metadata.sse_hidden_fields = hiddenFields
  const hiddenReason = readString(data.sse_hidden_reason)
  if (hiddenReason) metadata.sse_hidden_reason = hiddenReason
  const displayNotice = readString(data.sse_display_notice)
  if (displayNotice) metadata.sse_display_notice = displayNotice
  const generatedChars = readNumber(data.sse_generated_chars)
  if (generatedChars !== undefined) metadata.sse_generated_chars = generatedChars
  return metadata
}

function segmentSourceKey(metadata: EventMetadata) {
  const path = metadata.run_path?.join('/') || ''
  return `${metadata.subagent ? 'sub' : 'root'}:${metadata.subagent_session_id || ''}:${metadata.agent_name || ''}:${path}`
}

function getToolEventKey(data: Record<string, unknown>): string | undefined {
  const source = segmentSourceKey(readEventMetadata(data))
  if (typeof data.id === 'string' && data.id) return `${source}:id:${data.id}`
  if (typeof data.index === 'number') return `${source}:index:${data.index}`
  if (typeof data.index === 'string' && data.index) return `${source}:index:${data.index}`
  return undefined
}

function createToolMessageId(toolKey: string | undefined, counterRef: { current: number }) {
  counterRef.current += 1
  const suffix = toolKey ? toolKey.replace(/[^a-zA-Z0-9:_-]/g, '_') : `local:${counterRef.current}`
  return `tool:${Date.now()}:${suffix}:${counterRef.current}`
}

function createSegmentId(role: StreamSegmentRole, counterRef: { current: number }) {
  counterRef.current += 1
  return `segment:${role}:${Date.now()}:${counterRef.current}`
}

function createPlanCardMessageId(role: 'plan_question' | 'proposed_plan', data: Record<string, unknown>, metadata: EventMetadata) {
  const rawID = readString(data.id)
  if (!rawID) return `${role}-${Date.now()}`
  const runID = (metadata.run_id || '').replace(/[^a-zA-Z0-9:_-]/g, '_')
  const sourcePrefix = runID ? `run:${runID}:` : ''
  const source = `${sourcePrefix}${segmentSourceKey(metadata)}`.replace(/[^a-zA-Z0-9:_-]/g, '_')
  const safeID = rawID.replace(/[^a-zA-Z0-9:_-]/g, '_')
  return `plan:${source}:${role}:${safeID}`
}

function appendStreamingSegmentMessage(
  messages: ChatMessage[],
  role: StreamSegmentRole,
  id: string,
  text: string,
  metadata: EventMetadata,
) {
  return [...messages, { role, id, content: '', streaming_target_content: text, streaming: true, ...metadata }]
}

function updateStreamingSegments(messages: ChatMessage[], buffered: Record<string, string>) {
  return messages.map(message => (
    message.id && buffered[message.id]
      ? { ...message, streaming_target_content: (message.streaming_target_content || message.content || '') + buffered[message.id], streaming: true }
      : message
  ))
}

function buildTokenUsageMessage(data: Record<string, unknown>): ChatMessage {
  const runId = readString(data.run_id)
  return {
    role: 'token_usage',
    id: runId || `token-usage-${Date.now()}`,
    content: '',
    run_id: runId,
    agent_kind: readString(data.agent_kind),
    prompt_tokens: readNumber(data.prompt_tokens),
    cached_prompt_tokens: readNumber(data.cached_prompt_tokens),
    uncached_prompt_tokens: readNumber(data.uncached_prompt_tokens),
    cache_hit_rate: readNumber(data.cache_hit_rate),
    completion_tokens: readNumber(data.completion_tokens),
    reasoning_tokens: readNumber(data.reasoning_tokens),
    total_tokens: readNumber(data.total_tokens),
    model_calls: readNumber(data.model_calls),
    generated_bytes: readNumber(data.generated_bytes),
    usage_calls: readUsageCalls(data.usage_calls),
    created_at: readString(data.created_at) || new Date().toISOString(),
  }
}

function readUsageCalls(value: unknown) {
  if (!Array.isArray(value)) return undefined
  return value
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const call = item as Record<string, unknown>
      return {
        index: readNumber(call.index),
        created_at: readString(call.created_at),
        finish_reason: readString(call.finish_reason),
        requested_tools: readStringArray(call.requested_tools),
        after_tools: readStringArray(call.after_tools),
        prompt_tokens: readNumber(call.prompt_tokens),
        cached_prompt_tokens: readNumber(call.cached_prompt_tokens),
        uncached_prompt_tokens: readNumber(call.uncached_prompt_tokens),
        cache_hit_rate: readNumber(call.cache_hit_rate),
        completion_tokens: readNumber(call.completion_tokens),
        reasoning_tokens: readNumber(call.reasoning_tokens),
        total_tokens: readNumber(call.total_tokens),
      }
    })
    .filter((call): call is NonNullable<typeof call> => Boolean(call))
}

function upsertTokenUsageMessage(messages: ChatMessage[], next: ChatMessage) {
  if (!next.run_id) return [...messages, next]
  let found = false
  const updated = messages.map((message) => {
    if (message.role === 'token_usage' && message.run_id === next.run_id) {
      found = true
      return { ...message, ...next }
    }
    return message
  })
  return found ? updated : [...updated, next]
}

function normalizePlanCardStatus(raw: string, content: string): ChatMessage['status'] {
  if (raw === 'running' || raw === 'success' || raw === 'error') return raw
  return content ? 'success' : 'running'
}

function planRoleForProtocolTool(name: string): 'plan_question' | 'proposed_plan' | undefined {
  if (name === 'plan_questions' || name === 'plan_question') return 'plan_question'
  if (name === 'proposed_plan') return 'proposed_plan'
  return undefined
}

export function isPlanProtocolToolName(name: string) {
  return Boolean(planRoleForProtocolTool(name))
}

function extractPlanProtocolToolContent(role: 'plan_question' | 'proposed_plan', rawContent: string) {
  const content = rawContent.trim()
  if (!content || role === 'plan_question') return content
  try {
    const data = JSON.parse(content) as Record<string, unknown>
    for (const key of ['content', 'plan', 'markdown', 'proposal', 'summary']) {
      const value = data[key]
      if (typeof value === 'string' && value.trim()) return value.trim()
    }
  } catch {
    // Keep the original string; this is only a display fallback for malformed protocol tool calls.
  }
  return content
}

function updatePlanThinkingPreview(buffer: string, delta: string) {
  const nextBuffer = truncateLeadingChars(`${buffer}${delta}`, PLAN_THINKING_BUFFER_MAX_CHARS)
  const lines = nextBuffer
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(Boolean)
  const preview = truncateTrailingChars(lines[lines.length - 1] || '', PLAN_THINKING_PREVIEW_MAX_CHARS)
  return { buffer: nextBuffer, preview }
}

function updateLatestRunningPlanThinkingPreview(messages: ChatMessage[], preview: string) {
  if (!preview) return messages
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if ((message.role === 'plan_question' || message.role === 'proposed_plan') && (message.status === 'running' || message.streaming)) {
      if (message.thinking_preview === preview) return messages
      const updated = [...messages]
      updated[index] = { ...message, thinking_preview: preview }
      return updated
    }
  }
  return messages
}

function truncateLeadingChars(value: string, maxChars: number) {
  return value.length > maxChars ? value.slice(value.length - maxChars) : value
}

function truncateTrailingChars(value: string, maxChars: number) {
  return value.length > maxChars ? `${value.slice(0, maxChars - 1)}…` : value
}

function upsertPlanCardMessage(messages: ChatMessage[], next: ChatMessage) {
  if (next.status === 'error' && !next.content) {
    return next.id ? messages.filter(message => message.id !== next.id) : messages
  }
  if (!next.content && next.status !== 'running') {
    return next.id ? messages.filter(message => message.id !== next.id) : messages
  }
  if (!next.id) return [...messages, next]
  let found = false
  const updated = messages.map((message) => {
    if (message.id !== next.id) return message
    found = true
    return {
      ...message,
      ...next,
      content: next.content || message.content || '',
    }
  })
  return found ? updated : [...updated, next]
}

function discardPlanPreambleSegment(messages: ChatMessage[], segmentId: string) {
  return messages.filter(message => {
    if (message.id !== segmentId || message.role !== 'assistant') return true
    return !isLikelyPlanPreamble(message.content || '')
  })
}

function isLikelyPlanPreamble(content: string) {
  const text = content.trim()
  if (!text || text.length > PLAN_PREAMBLE_MAX_CHARS) return false
  if (text.includes('```')) return false
  return /计划|规划|方案|确认|问题|补充|不确定|提问|基于你的回答|before (the )?plan|need (to )?(confirm|clarify|ask)|question|proposal|proposed plan/i.test(text)
}

function finalizeStreamingSegment(messages: ChatMessage[], id: string) {
  return messages.map(message => (
    message.id === id ? { ...promoteStreamingTarget(message), streaming: false } : message
  ))
}

function promoteStreamingTargets(messages: ChatMessage[]) {
  let changed = false
  const nextMessages = messages.map((message) => {
    if (message.streaming_target_content === undefined) return message
    changed = true
    return promoteStreamingTarget(message)
  })
  return changed ? nextMessages : messages
}

function promoteStreamingTarget(message: ChatMessage): ChatMessage {
  if (message.streaming_target_content === undefined) return message
  const { streaming_target_content, ...rest } = message
  return { ...rest, content: streaming_target_content }
}

function findToolMessageId(
  data: Record<string, unknown>,
  keyToMessageId: Record<string, string>,
  fallbackQueue: string[],
  pendingToolCalls: Record<string, ToolCallInfo>,
) {
  const toolKey = getToolEventKey(data)
  if (toolKey && keyToMessageId[toolKey]) return keyToMessageId[toolKey]
  if (toolKey) return undefined
  const name = readString(data.name)
  if (name) {
    const queuedMatches = fallbackQueue.filter(id => pendingToolCalls[id]?.name === name)
    if (queuedMatches.length === 1) return queuedMatches[0]
    if (queuedMatches.length > 1) return undefined
    const pendingMatches = Object.entries(pendingToolCalls).filter(([, call]) => call.name === name)
    if (pendingMatches.length === 1) return pendingMatches[0][0]
    if (pendingMatches.length > 1) return undefined
  }
  return fallbackQueue[0]
}

function buildToolContent(name: string, args: string) {
  return args ? `${name}\n${args}` : name
}

function parseTaskSubagentType(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    return typeof data.subagent_type === 'string' ? data.subagent_type : ''
  } catch {
    const match = args.match(/"subagent_type"\s*:\s*"([^"]+)"/)
    return match?.[1] || ''
  }
}

function upsertToolCallMessage(messages: ChatMessage[], next: ChatMessage) {
  if (!next.id) return [...messages, next]
  let found = false
  const updated = messages.map(message => {
    if (message.role !== 'tool_call' || message.id !== next.id) return message
    found = true
    const args = next.args || message.args || ''
    const name = next.name || message.name
    return {
      ...message,
      ...next,
      name,
      args,
      content: buildToolContent(name || 'unknown_tool', args),
      status: message.status === 'success' ? message.status : next.status,
      result: message.result,
    }
  })
  return found ? updated : [...messages, next]
}

function normalizeMessageContent(content: string) {
  return content.trim().replace(/\s+/g, ' ')
}

function isFileMutationTool(name: string) {
  return ['write_file', 'create_file', 'edit_file', 'replace_file', 'delete_file', 'rename_file'].includes(name)
}

function isLoreMutationTool(name: string) {
  return name === 'write_lore_items'
}

function notifyLoreUpdated(itemIds: string[] = []) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: { item_ids: itemIds } }))
}

function readStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

function extractToolPath(args: string): string | undefined {
  if (!args) return undefined
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const value = data.path || data.file_path || data.to || data.from
    return typeof value === 'string' ? value : undefined
  } catch {
    const match = args.match(/"(?:path|file_path)"\s*:\s*"([^"]+)"/)
    return match?.[1]
  }
}
