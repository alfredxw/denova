import type { AgentRunTrace, AgentRunTraceRecord } from '@/lib/api'
import type { TrajectoryAnalysis, TrajectorySpan } from './trajectory-analysis'
import { objectValue } from './trajectory-analysis'
import type {
  TrajectoryContentAnalysis,
  TrajectoryContentEntry,
  TrajectoryContentKind,
  TrajectoryConversationNode,
  TrajectoryDirection,
  TrajectoryRequest,
  TrajectoryToolCall,
  TrajectoryToolDefinition,
  TrajectoryToolExchange,
  TrajectoryToolOutput,
} from './trajectory-content-types'

export type {
  TrajectoryContentAnalysis,
  TrajectoryContentEntry,
  TrajectoryContentKind,
  TrajectoryConversationNode,
  TrajectoryDirection,
  TrajectoryRequest,
  TrajectoryToolCall,
  TrajectoryToolDefinition,
  TrajectoryToolExchange,
  TrajectoryToolOutput,
} from './trajectory-content-types'

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

/** Reconstructs a readable conversation plus the exact per-request message source. */
export function analyzeTrajectoryContent(trace: AgentRunTrace, analysis: TrajectoryAnalysis): TrajectoryContentAnalysis {
  const spans = new Map(analysis.spans.map((span) => [span.id, span]))
  const toolSpans = indexToolSpans(analysis.spans)
  const toolOutputs = indexToolOutputs(trace.records, spans, toolSpans)
  const outputs = new Map<string, AgentRunTraceRecord>()
  for (const record of trace.records) {
    if (record.type !== 'llm_output') continue
    outputs.set(requestKey(objectValue(record.data)), record)
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

  const callsByID = new Map<string, TrajectoryToolCall>()
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
        direction: 'input',
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
        direction: 'input',
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
      const entry = entryFromMessage(capture, message, prefixLength + messageOffset, callsByID, toolSpans, 'input')
      entries.push(entry)
      rememberToolCalls(callsByID, entry)
    }

    const outputMessage = messageValue(capture.outputContent.message)
    if (outputMessage) {
      const entry = entryFromMessage(capture, outputMessage, capture.messages.length, callsByID, toolSpans, 'output')
      entries.push(entry)
      rememberToolCalls(callsByID, entry)
      previousOutput = outputMessage
    }

    const debugCalls = new Map<string, TrajectoryToolCall>()
    const debugInputEntries = capture.messages.map((message, messageIndex) => {
      const entry = entryFromMessage(capture, message, messageIndex, debugCalls, toolSpans, 'input', `debug-input-${messageIndex}`)
      rememberToolCalls(debugCalls, entry)
      return entry
    })
    const debugOutputEntries = outputMessage
      ? [entryFromMessage(capture, outputMessage, capture.messages.length, debugCalls, toolSpans, 'output', 'debug-output')]
      : []

    requests.push({
      id: capture.id,
      index: capture.index,
      span: capture.span,
      inputRecord: capture.inputRecord,
      outputRecord: capture.outputRecord,
      tools: capture.tools,
      entries,
      inputNodes: [],
      outputNodes: [],
      debugInputEntries,
      debugOutputEntries,
    })
    previousMessages = capture.messages
    previousSystem = system
    previousTools = capture.tools
  }

  const entries = requests.flatMap((request) => request.entries)
  const exchanges = buildToolExchanges(entries, requests, toolOutputs, toolSpans)
  const groupsByCaller = new Map<string, TrajectoryToolExchange[]>()
  const orphanGroups = new Map<string, TrajectoryToolExchange[]>()
  const pairedResults = new Set<string>()
  for (const exchange of exchanges) {
    if (exchange.caller) addMapArray(groupsByCaller, exchange.caller.id, exchange)
    else if (exchange.result) addMapArray(orphanGroups, exchange.result.id, exchange)
    if (exchange.result) pairedResults.add(exchange.result.id)
  }
  for (const request of requests) {
    request.inputNodes = conversationNodes(request.entries, 'input', groupsByCaller, orphanGroups, pairedResults)
    request.outputNodes = conversationNodes(request.entries, 'output', groupsByCaller, orphanGroups, pairedResults)
  }

  return { available: captures.length > 0, requests, entries, toolCalls: exchanges }
}

function buildToolExchanges(
  entries: TrajectoryContentEntry[],
  requests: TrajectoryRequest[],
  outputs: ReadonlyMap<string, TrajectoryToolOutput>,
  toolSpans: ReadonlyMap<string, TrajectorySpan>,
) {
  const requestTools = new Map(requests.map((request) => [request.id, request.tools]))
  const exchanges: TrajectoryToolExchange[] = []
  const byCallID = new Map<string, TrajectoryToolExchange>()
  for (const entry of entries) {
    for (const [index, call] of entry.toolCalls.entries()) {
      const key = call.id || `${entry.id}:${index}`
      const exchange: TrajectoryToolExchange = {
        id: `${entry.id}:tool:${key}`,
        call,
        caller: entry,
        result: null,
        output: outputs.get(call.id) ?? null,
        definition: requestTools.get(entry.requestID)?.find((tool) => tool.name === call.name) ?? null,
        span: toolSpans.get(call.id) ?? outputs.get(call.id)?.span ?? null,
      }
      exchanges.push(exchange)
      if (call.id) byCallID.set(call.id, exchange)
    }
  }
  for (const entry of entries) {
    if (entry.kind !== 'tool') continue
    let exchange = entry.toolCallID ? byCallID.get(entry.toolCallID) : undefined
    if (!exchange) {
      const call: TrajectoryToolCall = entry.toolCall ?? {
        id: entry.toolCallID,
        name: entry.toolName,
        arguments: '',
        raw: {},
      }
      exchange = {
        id: `${entry.id}:tool-result`,
        call,
        caller: null,
        result: null,
        output: outputs.get(entry.toolCallID) ?? null,
        definition: requestTools.get(entry.requestID)?.find((tool) => tool.name === entry.toolName) ?? null,
        span: toolSpans.get(entry.toolCallID) ?? entry.span,
      }
      exchanges.push(exchange)
      if (entry.toolCallID) byCallID.set(entry.toolCallID, exchange)
    }
    exchange.result = entry
    exchange.span ??= entry.span
  }
  return exchanges
}

function conversationNodes(
  entries: TrajectoryContentEntry[],
  direction: TrajectoryDirection,
  groupsByCaller: ReadonlyMap<string, TrajectoryToolExchange[]>,
  orphanGroups: ReadonlyMap<string, TrajectoryToolExchange[]>,
  pairedResults: ReadonlySet<string>,
) {
  const nodes: TrajectoryConversationNode[] = []
  for (const entry of entries) {
    if (entry.direction !== direction) continue
    const callerGroup = groupsByCaller.get(entry.id) ?? []
    const orphanGroup = orphanGroups.get(entry.id) ?? []
    if (!pairedResults.has(entry.id) && (entry.kind !== 'assistant' || entry.content || entry.reasoning || callerGroup.length === 0)) {
      nodes.push({ type: 'message', id: entry.id, entry })
    }
    if (callerGroup.length > 0) nodes.push({ type: 'tool-group', id: `${entry.id}:tools`, calls: callerGroup })
    if (orphanGroup.length > 0) nodes.push({ type: 'tool-group', id: `${entry.id}:orphan-tools`, calls: orphanGroup })
  }
  return nodes
}

function contentEntry(
  capture: RequestCapture,
  value: Partial<TrajectoryContentEntry> & Pick<TrajectoryContentEntry, 'kind' | 'direction' | 'messageIndex' | 'label'>,
  idSuffix = '',
): TrajectoryContentEntry {
  return {
    id: `${capture.id}:${value.direction}:${value.kind}:${value.messageIndex}${idSuffix ? `:${idSuffix}` : ''}`,
    kind: value.kind,
    direction: value.direction,
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
      direction: value.direction,
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
  direction: TrajectoryDirection,
  idSuffix = '',
) {
  const toolCalls = arrayValue(message.tool_calls).map(toolCallValue).filter((call): call is TrajectoryToolCall => call !== null)
  const kind = messageKind(message, direction)
  const toolCallID = exactString(message.tool_call_id)
  const toolCall = toolCallID ? callsByID.get(toolCallID) ?? null : null
  const toolName = exactString(message.tool_name) || exactString(message.name) || toolCall?.name || ''
  return contentEntry(capture, {
    kind,
    direction,
    messageIndex,
    label: messageLabel(kind, capture.index, toolName, direction),
    content: message.content,
    reasoning: exactString(message.reasoning_content),
    status: direction === 'output' ? exactString(capture.outputContent.status) || capture.span?.status || 'success' : 'included',
    toolName,
    toolCallID,
    toolCall,
    toolCalls,
    tools: kind === 'system' || kind === 'tool' ? capture.tools : [],
    raw: message,
    source: {
      role: message.role,
      name: message.name,
      tool_call_id: toolCallID,
      tool_name: toolName,
      extra: message.extra,
      trace_record: direction === 'output' ? capture.outputRecord : capture.inputRecord,
    },
    span: kind === 'tool' ? toolSpansByCallID.get(toolCallID) ?? capture.span : capture.span,
  }, idSuffix)
}

function indexToolSpans(spans: TrajectorySpan[]) {
  const result = new Map<string, TrajectorySpan>()
  for (const span of spans) {
    if (span.category !== 'tool') continue
    for (const key of [exactString(span.attrs.provider_call_id), exactString(span.attrs.execution_id)]) {
      if (key) result.set(key, span)
    }
  }
  return result
}

function indexToolOutputs(records: AgentRunTraceRecord[], spans: ReadonlyMap<string, TrajectorySpan>, toolSpans: ReadonlyMap<string, TrajectorySpan>) {
  const result = new Map<string, TrajectoryToolOutput>()
  for (const record of records) {
    if (record.type !== 'tool_output') continue
    const data = objectValue(record.data)
    const content = objectValue(data.content)
    const callID = exactString(content.provider_call_id) || exactString(data.call_id)
    const executionID = exactString(content.execution_id)
    const output: TrajectoryToolOutput = {
      callID,
      executionID,
      name: exactString(content.tool_name),
      status: exactString(content.status) || 'recorded',
      content: exactString(content.result),
      error: exactString(content.error),
      truncated: content.truncated === true,
      originalBytes: numberValue(content.original_bytes),
      returnedBytes: numberValue(content.returned_bytes),
      span: spans.get(exactString(data.span_id)) ?? toolSpans.get(callID) ?? toolSpans.get(executionID) ?? null,
      raw: record,
    }
    for (const key of [callID, executionID, exactString(data.call_id)]) {
      if (key) result.set(key, output)
    }
  }
  return result
}

function messageKind(message: CapturedMessage, direction: TrajectoryDirection): TrajectoryContentKind {
  if (direction === 'output' || message.role === 'assistant') return 'assistant'
  if (message.role === 'tool') return 'tool'
  if (message.role === 'system') return 'system'
  if (message.role === 'user' && isContextMessage(message)) return 'context'
  return 'user'
}

function isContextMessage(message: CapturedMessage) {
  return Object.keys(objectValue(message.extra)).some((key) => key.startsWith('agent.context') || key.includes('context_placement'))
}

function messageLabel(kind: TrajectoryContentKind, requestIndex: number, toolName: string, direction: TrajectoryDirection) {
  if (kind === 'assistant') return direction === 'output' ? `Request #${requestIndex}` : 'Assistant History'
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

function rememberToolCalls(callsByID: Map<string, TrajectoryToolCall>, entry: TrajectoryContentEntry) {
  for (const call of entry.toolCalls) if (call.id) callsByID.set(call.id, call)
}

function addMapArray<T>(map: Map<string, T[]>, key: string, value: T) {
  const values = map.get(key)
  if (values) values.push(value)
  else map.set(key, [value])
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

function numberValue(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function stableJSON(value: unknown) {
  return JSON.stringify(value, (_key, item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return item
    return Object.fromEntries(Object.entries(item as Record<string, unknown>).sort(([left], [right]) => left.localeCompare(right)))
  })
}
