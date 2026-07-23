import type { ChatMessage } from '@/lib/api'
import type { TokenUsageEvent } from '../../types'

export function buildTokenUsageMessage(data: Record<string, unknown> | TokenUsageEvent, fallbackId?: string): ChatMessage {
  const runId = readString(data.run_id)
  return {
    role: 'token_usage',
    id: runId || fallbackId || `token-usage-${Date.now()}`,
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

export function upsertTokenUsageMessage(messages: ChatMessage[], next: ChatMessage) {
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

export function mergeTokenUsageMessages(persisted: ChatMessage[], live: ChatMessage[]) {
  return live.reduce((messages, message) => upsertTokenUsageMessage(messages, message), [...persisted])
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
