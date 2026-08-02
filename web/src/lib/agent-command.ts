import type { TFunction } from 'i18next'
import type { AgentRuntimeQueuedCommand } from '@/lib/api'

/** Stable fingerprint for retaining one idempotency key across an uncertain retry. */
export function agentCommandRetryKey(operationID: string, type: string, payload: unknown) {
  return JSON.stringify([operationID, type, payload])
}

/** Retain uncertain command identities without allowing retry state to grow forever. */
export function rememberAgentCommandID(
  commands: Map<string, string>,
  retryKey: string,
  create: () => string,
  maxEntries = 32,
) {
  const existing = commands.get(retryKey)
  if (existing) return existing
  const commandID = create()
  commands.set(retryKey, commandID)
  while (commands.size > Math.max(1, maxEntries)) {
    const oldest = commands.keys().next().value
    if (oldest === undefined) break
    commands.delete(oldest)
  }
  return commandID
}

/** A 4xx response proves the command was rejected; network/5xx outcomes remain uncertain. */
export function isKnownAgentCommandOutcome(error: unknown) {
  if (!error || typeof error !== 'object') return false
  const status = Number((error as { status?: unknown }).status)
  return Number.isFinite(status) && status >= 400 && status < 500
}

/** Map stable runtime error codes to the active UI locale. */
export function agentCommandErrorMessage(error: unknown, t: TFunction) {
  const code = error && typeof error === 'object' && typeof (error as { code?: unknown }).code === 'string'
    ? (error as { code: string }).code
    : ''
  switch (code) {
    case 'agent_runtime.target_operation_mismatch':
      return t('chat.runtime.operationChanged')
    case 'agent_runtime.queue_conflict':
      return t('chat.runtime.queueConflict')
    case 'agent_runtime.invalid_phase':
      return t('chat.runtime.operationUnavailable')
    case 'agent_runtime.busy':
      return t('chat.runtime.busy')
    case 'agent_runtime.invalid_command':
    case 'agent_runtime.command_conflict':
      return t('chat.runtime.invalidCommand')
    default: {
      const detail = error instanceof Error ? error.message : t('chat.runtime.commandFailed')
      return t('chat.activity.requestFailed', { error: detail })
    }
  }
}

/** Optimistically project one accepted command without duplicating queue entries on retry. */
export function mergeProjectedAgentQueue(
  queue: AgentRuntimeQueuedCommand[] | undefined,
  next: AgentRuntimeQueuedCommand,
) {
  return [...(queue || []).filter((item) => item.command_id !== next.command_id), next]
}
