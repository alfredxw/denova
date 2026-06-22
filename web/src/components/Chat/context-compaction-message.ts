import type { ChatMessage } from '@/lib/api'

export function createContextCompactionMessageId(counterRef: { current: number }) {
  counterRef.current += 1
  return `context-compaction:${Date.now()}:${counterRef.current}`
}

export function buildContextCompactionMessage(data: Record<string, unknown>, id: string): ChatMessage {
  const status = readString(data.status)
  const messageStatus = status === 'completed' ? 'success' : status === 'failed' ? 'error' : 'running'
  return {
    role: 'context_compaction',
    id,
    status: messageStatus,
    content: readString(data.summary) || readString(data.delta),
    phase: readString(data.phase),
    attempt: readNumber(data.attempt),
    tokens_before: readNumber(data.tokens_before),
    tokens_after: readNumber(data.tokens_after),
    context_window_tokens: readNumber(data.context_window_tokens),
    threshold: readNumber(data.threshold),
    target_ratio: readNumber(data.target_ratio),
    epoch: readNumber(data.epoch),
    source_message_count: readNumber(data.source_message_count),
    message_count_before: readNumber(data.message_count_before),
    message_count_after: readNumber(data.message_count_after),
    skipped_reason: readString(data.skipped_reason),
    streaming: messageStatus === 'running',
  }
}

export function upsertContextCompactionMessage(messages: ChatMessage[], next: ChatMessage) {
  if (!next.id) return [...messages, next]
  let found = false
  const updated = messages.map(message => {
    if (message.role !== 'context_compaction' || message.id !== next.id) return message
    found = true
    const hasNewAttempt = next.attempt !== undefined && message.attempt !== undefined && next.attempt !== message.attempt
    const content = next.status === 'success' && next.content
      ? next.content
      : hasNewAttempt
        ? next.content || ''
        : next.content
          ? `${message.content || ''}${next.content}`
          : message.content
    return {
      ...message,
      ...next,
      attempt: next.attempt || message.attempt,
      content,
    }
  })
  return found ? updated : [...messages, next]
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}
