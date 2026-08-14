import type { AgentRunTrace, AgentRunTraceRecord } from '@/lib/api'
import type { TrajectoryAnalysis, TrajectorySpan } from './trajectory-analysis'
import { objectValue } from './trajectory-analysis'

export type TrajectoryContentKind = 'system' | 'user' | 'context' | 'assistant' | 'tool'

export interface TrajectoryToolDefinition {
  name: string
  description: string
  parameters: unknown
  parametersError: string
  extra: Record<string, unknown>
}

export interface TrajectoryToolCall {
  id: string
  name: string
  arguments: string
  raw: Record<string, unknown>
}

export interface TrajectoryContentEntry {
  id: string
  kind: TrajectoryContentKind
  requestIndex: number
  requestID: string
  messageIndex: number
  label: string
  content: string
  reasoning: string
  status: string
  toolName: string
  toolCallID: string
  toolCall: TrajectoryToolCall | null
  toolCalls: TrajectoryToolCall[]
  tools: TrajectoryToolDefinition[]
  raw: unknown
  source: Record<string, unknown>
  span: TrajectorySpan | null
  inputRecord: AgentRunTraceRecord
  outputRecord: AgentRunTraceRecord | null
  previousContent: string
  previousTools: TrajectoryToolDefinition[]
}

export interface TrajectoryRequest {
  id: string
  index: number
  span: TrajectorySpan | null
  inputRecord: AgentRunTraceRecord
  outputRecord: AgentRunTraceRecord | null
  entries: TrajectoryContentEntry[]
}

export interface TrajectoryContentAnalysis {
  available: boolean
  requests: TrajectoryRequest[]
  entries: TrajectoryContentEntry[]
}

interface CapturedMessage {
  role: string
  content: string
  reasoning_content?: string
  name?: string
  tool_calls?: unknown[]
  tool_call_id?: string
  tool_name?: string
  tool_result?: unknown
  extra?: unknown
  [key: string]: unknown
}

interface RequestCapture {
  id: string
  index: number
  span: TrajectorySpan | null
  inputRecord: AgentRunTraceRecord
  outputRecord: AgentRunTraceRecord | null
  messages: CapturedMessage[]
  tools: TrajectoryToolDefinition[]
  inputContent: Record<string, unknown>
  outputContent: Record<string, unknown>
}

/** Reconstructs the model-visible semantic ledger from developer trace records. */
export function analyzeTrajectoryContent(trace: AgentRunTrace, analysis: TrajectoryAnalysis): TrajectoryContentAnalysis {
  const spans = new Map(analysis.spans.map((span) => [span.id, span]))
  const toolSpans = new Map<string, TrajectorySpan>()
  for (const span of analysis.spans) {
    if (span.category !== 'tool') continue
    for (const key of [exactString(span.attrs.provider_call_id), exactString(span.attrs.execution_id)]) {
      if (key) toolSpans.set(key, span)
    }
  }
  const outputs = new Map<string, AgentRunTraceRecord>()
  for (const record of trace.records) {
    if (record.type !== 'llm_output') continue
    const data = objectValue(record.data)
    outputs.set(requestKey(data), record)
  }

  const captures = trace.records.flatMap((record, index): RequestCapture[] => {
    if (record.type !== 'llm_input') return []
    const data = objectValue(record.data)
    const content = objectValue(data.content)
    const spanID = exactString(data.span_id)
    const callID = exactString(data.call_id)
    const outputRecord = outputs.get(requestKey(data)) ?? null
    return [{
      id: callID || spanID || `request-${index + 1}`,
      index: 0,
      span: spans.get(spanID) ?? null,
      inputRecord: record,
      outputRecord,
      messages: arrayValue(content.messages).map(messageValue).filter((message): message is CapturedMessage => message !== null),
      tools: arrayValue(content.tools).map(toolValue).filter((tool): tool is TrajectoryToolDefinition => tool !== null),
      inputContent: content,
      outputContent: objectValue(objectValue(outputRecord?.data).content),
    }]
  }).sort(compareCapture)

  const toolCalls = new Map<string, TrajectoryToolCall>()
  const requests: TrajectoryRequest[] = []
  let previousMessages: CapturedMessage[] = []
  let previousOutput: CapturedMessage | null = null
  let previousSystem = ''
  let previousTools: TrajectoryToolDefinition[] = []

  for (const [captureIndex, capture] of captures.entries()) {
    capture.index = captureIndex + 1
    const entries: TrajectoryContentEntry[] = []
    const systemMessages = capture.messages.filter((message) => message.role === 'system')
    const system = systemMessages.map((message) => message.content).join('\n\n')
    const toolsChanged = stableJSON(capture.tools) !== stableJSON(previousTools)
    if (captureIndex === 0 || system !== previousSystem || toolsChanged) {
      entries.push(contentEntry(capture, {
        kind: 'system',
        messageIndex: 0,
        label: captureIndex === 0 ? 'Initial System Prompt' : 'System Prompt Update',
        content: system,
        tools: capture.tools,
        raw: systemMessages,
        source: {
          trace_record: capture.inputRecord,
          model_config: capture.inputContent.model_config,
          message_count: capture.inputContent.message_count,
          tool_count: capture.inputContent.tool_count,
        },
        previousContent: previousSystem,
        previousTools,
      }))
    }

    const prefixLength = sharedPrefixLength(previousMessages, capture.messages)
    if (captureIndex > 0 && prefixLength < previousMessages.length) {
      entries.push(contentEntry(capture, {
        kind: 'context',
        messageIndex: prefixLength,
        label: 'Input Snapshot Changed',
        content: '',
        raw: capture.messages,
        source: {
          reason: 'The model-visible message prefix changed between requests, usually because context was rebuilt or compacted.',
          previous_message_count: previousMessages.length,
          current_message_count: capture.messages.length,
          shared_prefix_messages: prefixLength,
          trace_record: capture.inputRecord,
        },
      }))
    }

    const newMessages = captureIndex === 0 ? capture.messages : capture.messages.slice(prefixLength)
    for (const [messageOffset, message] of newMessages.entries()) {
      if (message.role === 'system') continue
      if (message.role === 'assistant' && previousOutput && messageFingerprint(message) === messageFingerprint(previousOutput)) continue
      const entry = entryFromMessage(capture, message, prefixLength + messageOffset, toolCalls, toolSpans)
      entries.push(entry)
      for (const call of entry.toolCalls) toolCalls.set(call.id, call)
    }

    const outputMessage = messageValue(capture.outputContent.message)
    if (outputMessage) {
      const entry = entryFromMessage(capture, outputMessage, capture.messages.length, toolCalls, toolSpans, true)
      entries.push(entry)
      for (const call of entry.toolCalls) toolCalls.set(call.id, call)
      previousOutput = outputMessage
    }

    requests.push({
      id: capture.id,
      index: capture.index,
      span: capture.span,
      inputRecord: capture.inputRecord,
      outputRecord: capture.outputRecord,
      entries,
    })
    previousMessages = capture.messages
    previousSystem = system
    previousTools = capture.tools
  }

  return {
    available: captures.length > 0,
    requests,
    entries: requests.flatMap((request) => request.entries),
  }
}

function contentEntry(capture: RequestCapture, value: Partial<TrajectoryContentEntry> & Pick<TrajectoryContentEntry, 'kind' | 'messageIndex' | 'label'>): TrajectoryContentEntry {
  return {
    id: `${capture.id}:${value.kind}:${value.messageIndex}`,
    kind: value.kind,
    requestIndex: capture.index,
    requestID: capture.id,
    messageIndex: value.messageIndex,
    label: value.label,
    content: value.content ?? '',
    reasoning: value.reasoning ?? '',
    status: value.status ?? (capture.span?.status || exactString(capture.outputContent.status) || 'recorded'),
    toolName: value.toolName ?? '',
    toolCallID: value.toolCallID ?? '',
    toolCall: value.toolCall ?? null,
    toolCalls: value.toolCalls ?? [],
    tools: value.tools ?? [],
    raw: value.raw ?? null,
    source: {
      request_index: capture.index,
      request_id: capture.id,
      span_id: capture.span?.id ?? exactString(objectValue(capture.inputRecord.data).span_id),
      ...value.source,
    },
    span: value.span === undefined ? capture.span : value.span,
    inputRecord: capture.inputRecord,
    outputRecord: capture.outputRecord,
    previousContent: value.previousContent ?? '',
    previousTools: value.previousTools ?? [],
  }
}

function entryFromMessage(
  capture: RequestCapture,
  message: CapturedMessage,
  messageIndex: number,
  callsByID: ReadonlyMap<string, TrajectoryToolCall>,
  toolSpansByCallID: ReadonlyMap<string, TrajectorySpan>,
  isOutput = false,
) {
  const toolCalls = arrayValue(message.tool_calls).map(toolCallValue).filter((call): call is TrajectoryToolCall => call !== null)
  const kind = messageKind(message, isOutput)
  const toolCallID = exactString(message.tool_call_id)
  const toolCall = toolCallID ? callsByID.get(toolCallID) ?? null : null
  const toolName = exactString(message.tool_name) || exactString(message.name) || toolCall?.name || ''
  const source = {
    role: message.role,
    name: message.name,
    tool_call_id: toolCallID,
    tool_name: toolName,
    extra: message.extra,
    trace_record: isOutput ? capture.outputRecord : capture.inputRecord,
  }
  return contentEntry(capture, {
    kind,
    messageIndex,
    label: messageLabel(kind, capture.index, toolName, isOutput),
    content: message.content,
    reasoning: exactString(message.reasoning_content),
    status: isOutput ? exactString(capture.outputContent.status) || capture.span?.status || 'success' : 'included',
    toolName,
    toolCallID,
    toolCall,
    toolCalls,
    tools: kind === 'tool' ? capture.tools : [],
    raw: message,
    source,
    span: kind === 'tool' ? toolSpansByCallID.get(toolCallID) ?? capture.span : capture.span,
  })
}

function messageKind(message: CapturedMessage, isOutput: boolean): TrajectoryContentKind {
  if (isOutput || message.role === 'assistant') return 'assistant'
  if (message.role === 'tool') return 'tool'
  if (message.role === 'system') return 'system'
  if (message.role === 'user' && isContextMessage(message)) return 'context'
  return 'user'
}

function isContextMessage(message: CapturedMessage) {
  const extra = objectValue(message.extra)
  return Object.keys(extra).some((key) => key.startsWith('agent.context') || key.includes('context_placement'))
}

function messageLabel(kind: TrajectoryContentKind, requestIndex: number, toolName: string, isOutput: boolean) {
  if (kind === 'assistant') return isOutput ? `Request #${requestIndex}` : 'Assistant History'
  if (kind === 'tool') return toolName || 'Tool Result'
  if (kind === 'context') return 'Model Context'
  if (kind === 'user') return 'User Message'
  return 'System Prompt'
}

function toolValue(value: unknown): TrajectoryToolDefinition | null {
  const tool = objectValue(value)
  const name = exactString(tool.name)
  if (!name) return null
  return {
    name,
    description: exactString(tool.description),
    parameters: tool.parameters ?? null,
    parametersError: exactString(tool.parameters_error),
    extra: objectValue(tool.extra),
  }
}

function toolCallValue(value: unknown): TrajectoryToolCall | null {
  const call = objectValue(value)
  const fn = objectValue(call.function)
  const id = exactString(call.id)
  const name = exactString(fn.name)
  if (!id && !name) return null
  return { id, name, arguments: exactString(fn.arguments), raw: call }
}

function messageValue(value: unknown): CapturedMessage | null {
  const message = objectValue(value)
  const role = exactString(message.role)
  if (!role) return null
  return { ...message, role, content: exactString(message.content) }
}

function sharedPrefixLength(left: readonly CapturedMessage[], right: readonly CapturedMessage[]) {
  const limit = Math.min(left.length, right.length)
  let index = 0
  while (index < limit && messageFingerprint(left[index]) === messageFingerprint(right[index])) index++
  return index
}

function messageFingerprint(message: CapturedMessage) {
  return stableJSON(message)
}

function compareCapture(left: RequestCapture, right: RequestCapture) {
  const leftSpan = left.span?.startedAt ?? Date.parse(left.inputRecord.created_at)
  const rightSpan = right.span?.startedAt ?? Date.parse(right.inputRecord.created_at)
  return leftSpan - rightSpan || left.inputRecord.created_at.localeCompare(right.inputRecord.created_at)
}

function requestKey(data: Record<string, unknown>) {
  return exactString(data.call_id) || exactString(data.span_id)
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function exactString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function stableJSON(value: unknown) {
  return JSON.stringify(value, (_key, item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return item
    return Object.fromEntries(Object.entries(item as Record<string, unknown>).sort(([left], [right]) => left.localeCompare(right)))
  })
}
