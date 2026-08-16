import type { UIMessageChunk } from 'ai'
import type { AgentDataParts, AgentMessageMetadata, AgentUIMessage } from './agent-ui'

type AgentTextRole = 'user' | 'system' | 'assistant'
type AgentTextState = 'streaming' | 'done'
type AgentToolState = 'input-streaming' | 'input-available' | 'output-available' | 'output-error'

interface AgentTextMessageInput {
  id?: string
  role: AgentTextRole
  text: string
  state?: AgentTextState
  metadata?: AgentMessageMetadata
}

interface AgentReasoningMessageInput {
  id?: string
  text: string
  state?: AgentTextState
  metadata?: AgentMessageMetadata
}

interface AgentToolMessageInput {
  id?: string
  name: string
  state: AgentToolState
  input?: unknown
  inputText?: string
  output?: unknown
  errorText?: string
  metadata?: AgentMessageMetadata
}

interface AgentDataMessageInput {
  id?: string
  partId?: string
  type: keyof AgentDataParts
  data: Record<string, unknown>
  metadata?: AgentMessageMetadata
}

export function createAgentTextMessage({ id, role, text, state = 'done', metadata }: AgentTextMessageInput): AgentUIMessage {
  return {
    id: id || localAgentMessageID(role),
    role,
    metadata,
    parts: [{ type: 'text', text, state }],
  } as AgentUIMessage
}

export function createAgentReasoningMessage({ id, text, state = 'done', metadata }: AgentReasoningMessageInput): AgentUIMessage {
  return {
    id: id || localAgentMessageID('reasoning'),
    role: 'assistant',
    metadata,
    parts: [{ type: 'reasoning', text, state }],
  } as AgentUIMessage
}

export function createAgentToolMessage({ id, name, state, input, inputText, output, errorText, metadata }: AgentToolMessageInput): AgentUIMessage {
  const messageID = id || localAgentMessageID('tool')
  const part: Record<string, unknown> = {
    type: 'dynamic-tool',
    toolName: name || 'unknown_tool',
    toolCallId: messageID,
    state,
    input,
  }
  if (inputText !== undefined) part.inputText = inputText
  if (output !== undefined && state !== 'output-error') part.output = output
  if (errorText !== undefined || state === 'output-error') part.errorText = errorText || String(output || '')
  return { id: messageID, role: 'assistant', metadata, parts: [part] } as AgentUIMessage
}

export function createAgentDataMessage({ id, partId, type, data, metadata }: AgentDataMessageInput): AgentUIMessage {
  const messageID = id || localAgentMessageID(type)
  const dataID = partId || (typeof data.id === 'string' && data.id.trim() ? data.id.trim() : messageID)
  return {
    id: messageID,
    role: 'assistant',
    metadata,
    parts: [{ type: `data-${type}`, id: dataID, data }],
  } as AgentUIMessage
}

export function agentMessageText(message: AgentUIMessage) {
  const part = message.parts.find((candidate) => candidate.type === 'text') as Record<string, unknown> | undefined
  return typeof part?.text === 'string' ? part.text : ''
}

export function agentMessageDisplayText(message: AgentUIMessage) {
  if (message.metadata?.streaming_target_content !== undefined) return message.metadata.streaming_target_content
  const part = message.parts.find((candidate) => candidate.type === 'text' || candidate.type === 'reasoning') as Record<string, unknown> | undefined
  return typeof part?.text === 'string' ? part.text : ''
}

export function agentMessageHasDataPart(message: AgentUIMessage, type: keyof AgentDataParts) {
  return message.parts.some((part) => part.type === `data-${type}`)
}

export function parseAgentToolInput(value: string) {
  if (!value) return undefined
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

/** Returns only the exact protocol text; structured input is a separate completed-state view. */
export function agentToolInputText(part: AgentUIMessage['parts'][number]) {
  const raw = part as Record<string, unknown>
  return typeof raw.inputText === 'string' ? raw.inputText : undefined
}

/** Accumulates the protocol's append-only raw input without interpreting a tool schema. */
export function recordAgentToolInputChunk(chunk: UIMessageChunk, inputTextByToolCall: Map<string, string>) {
  if (chunk.type === 'tool-input-start') {
    inputTextByToolCall.set(chunk.toolCallId, '')
    return true
  }
  if (chunk.type !== 'tool-input-delta') return false
  inputTextByToolCall.set(
    chunk.toolCallId,
    (inputTextByToolCall.get(chunk.toolCallId) ?? '') + chunk.inputTextDelta,
  )
  return true
}

/** Adds the raw protocol input beside the SDK's parsed view for presentation. */
export function attachAgentToolInputText(
  message: AgentUIMessage,
  inputTextByToolCall: ReadonlyMap<string, string>,
): AgentUIMessage {
  let changed = false
  const parts = message.parts.map((part) => {
    const raw = part as Record<string, unknown>
    const toolCallId = typeof raw.toolCallId === 'string' ? raw.toolCallId : ''
    if (!toolCallId || !inputTextByToolCall.has(toolCallId)) return part
    const inputText = inputTextByToolCall.get(toolCallId) ?? ''
    if (raw.inputText === inputText) return part
    changed = true
    return { ...raw, inputText } as unknown as AgentUIMessage['parts'][number]
  })
  return changed ? { ...message, parts } : message
}

export function attachAgentToolInputTexts(
  messages: AgentUIMessage[],
  inputTextByToolCall: ReadonlyMap<string, string>,
) {
  if (inputTextByToolCall.size === 0) return messages
  let changed = false
  const next = messages.map((message) => {
    const attached = attachAgentToolInputText(message, inputTextByToolCall)
    if (attached !== message) changed = true
    return attached
  })
  return changed ? next : messages
}

function localAgentMessageID(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}
