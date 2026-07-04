import type { ChatMessage, SSEEvent } from '@/lib/api'

export interface ConfigManagerToolPayload {
  id?: string
  index?: number | string
  name?: string
  args?: string
  delta?: string
  content?: string
  run_id?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
}

export type ConfigManagerChatEventMetadata = Pick<ChatMessage, 'run_id' | 'agent_name' | 'root_agent_name' | 'run_path' | 'subagent' | 'subagent_session_id' | 'subagent_type'>

interface ConfigManagerMessageOptions {
  idPrefix?: string
  toolLabel: string
  failureMessage: string
}

export function reduceConfigManagerMessages(messages: ChatMessage[], event: SSEEvent, options: ConfigManagerMessageOptions): ChatMessage[] {
  if (event.event === 'thinking') {
    const payload = parseConfigManagerPayload<ConfigManagerToolPayload>(event.data)
    return appendConfigManagerStreamingMessage(messages, 'thinking', payload?.content || '', metadataFromConfigManagerPayload(payload), options.idPrefix)
  }
  if (event.event === 'chunk') {
    const payload = parseConfigManagerPayload<ConfigManagerToolPayload>(event.data)
    return appendConfigManagerStreamingMessage(messages, 'assistant', payload?.content || '', metadataFromConfigManagerPayload(payload), options.idPrefix)
  }
  if (event.event === 'tool_call') {
    const payload = parseConfigManagerPayload<ConfigManagerToolPayload>(event.data)
    return payload ? upsertConfigManagerToolCall(messages, payload, options) : messages
  }
  if (event.event === 'tool_args_delta') {
    const payload = parseConfigManagerPayload<ConfigManagerToolPayload>(event.data)
    return payload ? appendConfigManagerToolArgs(messages, payload) : messages
  }
  if (event.event === 'tool_result') {
    const payload = parseConfigManagerPayload<ConfigManagerToolPayload>(event.data)
    return payload ? finishConfigManagerToolCall(messages, payload) : messages
  }
  if (event.event === 'token_usage') {
    const payload = parseConfigManagerPayload<Record<string, unknown>>(event.data)
    return payload ? upsertTokenUsageMessage(messages, buildTokenUsageMessage(payload)) : messages
  }
  if (event.event === 'error') {
    const payload = parseConfigManagerPayload<{ message?: string }>(event.data)
    return appendConfigManagerMessage(messages, { role: 'error', content: payload?.message || options.failureMessage }, options.idPrefix)
  }
  return messages
}

export function appendConfigManagerMessage(messages: ChatMessage[], message: ChatMessage, idPrefix = 'config-manager') {
  return [...messages, { ...message, id: message.id || `${idPrefix}-${Date.now()}-${messages.length}` }]
}

export function appendConfigManagerStreamingMessage(messages: ChatMessage[], role: ChatMessage['role'], content: string, metadata: ConfigManagerChatEventMetadata = {}, idPrefix = 'config-manager') {
  if (!content) return messages
  const last = messages[messages.length - 1]
  if (last?.role === role && last.status !== 'success' && sameConfigManagerChatEventSource(last, metadata)) {
    return [...messages.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
  }
  return [...messages, { id: `${idPrefix}-${Date.now()}-${messages.length}`, role, content, ...metadata }]
}

export function parseConfigManagerPayload<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}

export function metadataFromConfigManagerPayload(payload?: ConfigManagerToolPayload | null): ConfigManagerChatEventMetadata {
  if (!payload) return {}
  return {
    run_id: payload.run_id,
    agent_name: payload.agent_name,
    root_agent_name: payload.root_agent_name,
    run_path: payload.run_path,
    subagent: payload.subagent,
    subagent_session_id: payload.subagent_session_id,
    subagent_type: payload.subagent_type,
  }
}

export function configManagerToolKey(payload?: ConfigManagerToolPayload | null) {
  const id = readString(payload?.id)
  if (id) return id
  const index = payload?.index
  if (typeof index === 'number' && Number.isFinite(index)) return `index:${index}`
  if (typeof index === 'string' && index.trim()) return `index:${index.trim()}`
  return ''
}

function upsertConfigManagerToolCall(messages: ChatMessage[], payload: ConfigManagerToolPayload, options: ConfigManagerMessageOptions) {
  const id = configManagerToolKey(payload) || `tool-${Date.now()}-${messages.length}`
  const name = payload.name || options.toolLabel
  const existing = messages.findIndex((message) => message.id === id)
  const next: ChatMessage = { id, role: 'tool_call', content: name, name, args: payload.args || '', status: 'running', ...metadataFromConfigManagerPayload(payload) }
  if (existing >= 0) {
    return messages.map((message, index) => index === existing ? { ...message, ...next, args: message.args || next.args } : message)
  }
  return [...messages, next]
}

function appendConfigManagerToolArgs(messages: ChatMessage[], payload: ConfigManagerToolPayload) {
  const id = configManagerToolKey(payload)
  if (!id || !payload.delta) return messages
  return messages.map((message) => (
    message.id === id && message.role === 'tool_call'
      ? { ...message, args: `${message.args || ''}${payload.delta}` }
      : message
  ))
}

function finishConfigManagerToolCall(messages: ChatMessage[], payload: ConfigManagerToolPayload) {
  const id = configManagerToolKey(payload)
  if (!id) return messages
  return messages.map((message) => (
    message.id === id && message.role === 'tool_call'
      ? { ...message, status: 'success' as const, result: payload.content || '', ...metadataFromConfigManagerPayload(payload) }
      : message
  ))
}

function buildTokenUsageMessage(data: Record<string, unknown>): ChatMessage {
  const runId = readString(data.run_id)
  return {
    role: 'token_usage',
    id: runId || `token-usage-${Date.now()}`,
    content: readString(data.content),
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

function readUsageCalls(value: unknown) {
  if (!Array.isArray(value)) return undefined
  const calls = value
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
  return calls.length > 0 ? calls : undefined
}

function readStringArray(value: unknown) {
  if (!Array.isArray(value)) return undefined
  const result = value.map((item) => readString(item)).filter(Boolean)
  return result.length > 0 ? result : undefined
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readNumber(value: unknown) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

function sameConfigManagerChatEventSource(message: ChatMessage, metadata: ConfigManagerChatEventMetadata) {
  return Boolean(message.subagent) === Boolean(metadata.subagent) &&
    (message.subagent_session_id || '') === (metadata.subagent_session_id || '') &&
    (message.agent_name || '') === (metadata.agent_name || '') &&
    (message.root_agent_name || '') === (metadata.root_agent_name || '') &&
    (message.run_path || []).join('/') === (metadata.run_path || []).join('/')
}
