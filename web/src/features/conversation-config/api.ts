import { jsonHeaders, requestJSON } from '@/lib/api-client'
import type {
  ConversationConfigBinding,
  ConversationConfigChanges,
  ConversationConfigSnapshot,
} from './types'

export function fetchConversationConfig(binding: ConversationConfigBinding): Promise<ConversationConfigSnapshot> {
  return requestJSON(`/api/conversation-config?${conversationConfigQuery(binding).toString()}`)
}

export function patchConversationConfig(
  binding: ConversationConfigBinding,
  changes: ConversationConfigChanges,
  baseRevision: number,
): Promise<ConversationConfigSnapshot> {
  return requestJSON('/api/conversation-config', {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ binding, base_revision: baseRevision, changes }),
  })
}

function conversationConfigQuery(binding: ConversationConfigBinding) {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(binding)) {
    if (value) query.set(key, value)
  }
  return query
}
