import type { AgentUIMessage } from '@/lib/agent-ui'
import { createAgentDataMessage } from '@/lib/agent-ui-message'

export function createContextCompactionMessageId(counterRef: { current: number }) {
  counterRef.current += 1
  return `context-compaction:${Date.now()}:${counterRef.current}`
}

export function buildContextCompactionMessage(data: Record<string, unknown>, id: string): AgentUIMessage {
  const status = readString(data.status)
  const messageStatus = status === 'completed' ? 'success' : status === 'failed' ? 'error' : 'running'
  return createAgentDataMessage({
    id,
    type: 'agent-context-compaction',
    data: {
      id,
      role: 'context_compaction',
      status: messageStatus,
      content: readString(data.summary) || readString(data.delta),
      phase: readString(data.phase),
      attempt: readNumber(data.attempt),
      tokens_before: readNumber(data.tokens_before),
      tokens_after: readNumber(data.tokens_after),
      projected_tokens_before: readNumber(data.projected_tokens_before),
      projected_tokens_after: readNumber(data.projected_tokens_after),
      reserved_completion_tokens: readNumber(data.reserved_completion_tokens),
      reserved_tool_result_tokens: readNumber(data.reserved_tool_result_tokens),
      context_window_tokens: readNumber(data.context_window_tokens),
      threshold: readNumber(data.threshold),
      target_ratio: readNumber(data.target_ratio),
      epoch: readNumber(data.epoch),
      source_message_count: readNumber(data.source_message_count),
      message_count_before: readNumber(data.message_count_before),
      message_count_after: readNumber(data.message_count_after),
      skipped_reason: readString(data.skipped_reason),
    },
  })
}

export function upsertContextCompactionMessage(messages: AgentUIMessage[], next: AgentUIMessage) {
  const nextData = contextCompactionData(next)
  if (!nextData || !next.id) return [...messages, next]
  let found = false
  const updated = messages.map(message => {
    const currentData = contextCompactionData(message)
    if (!currentData || message.id !== next.id) return message
    found = true
    const nextAttempt = readNumber(nextData.attempt)
    const currentAttempt = readNumber(currentData.attempt)
    const nextContent = readString(nextData.content)
    const currentContent = readString(currentData.content)
    const hasNewAttempt = nextAttempt !== undefined && currentAttempt !== undefined && nextAttempt !== currentAttempt
    const content = nextData.status === 'success' && nextContent
      ? nextContent
      : hasNewAttempt
        ? nextContent
        : nextContent
          ? `${currentContent}${nextContent}`
          : currentContent
    return createAgentDataMessage({
      id: message.id,
      partId: contextCompactionPartID(message),
      type: 'agent-context-compaction',
      metadata: message.metadata,
      data: {
        ...currentData,
        ...nextData,
        attempt: nextAttempt || currentAttempt,
        content,
      },
    })
  })
  return found ? updated : [...messages, next]
}

export function settleContextCompactionMessages(messages: AgentUIMessage[], status: 'success' | 'error') {
  return messages.map((message) => {
    const data = contextCompactionData(message)
    if (!data) return message
    return createAgentDataMessage({
      id: message.id,
      partId: contextCompactionPartID(message),
      type: 'agent-context-compaction',
      metadata: message.metadata,
      data: { ...data, status },
    })
  })
}

function contextCompactionData(message: AgentUIMessage): Record<string, unknown> | undefined {
  const part = message.parts.find((candidate) => candidate.type === 'data-agent-context-compaction') as Record<string, unknown> | undefined
  return part?.data && typeof part.data === 'object' && !Array.isArray(part.data) ? part.data as Record<string, unknown> : undefined
}

function contextCompactionPartID(message: AgentUIMessage) {
  const part = message.parts.find((candidate) => candidate.type === 'data-agent-context-compaction') as Record<string, unknown> | undefined
  return typeof part?.id === 'string' ? part.id : message.id
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}
