import { jsonHeaders, requestJSON } from '@/lib/api-client'
import { projectAPIPath } from '@/lib/api-client/project-scope'
import type {
  ConversationConfigBinding,
  ConversationConfigChanges,
  ConversationConfigSnapshot,
} from './types'

export function fetchConversationConfig(binding: ConversationConfigBinding): Promise<ConversationConfigSnapshot> {
  const { path, transportBinding } = conversationConfigTransport(binding)
  return requestJSON(`${path}?${conversationConfigQuery(transportBinding).toString()}`)
}

export function patchConversationConfig(
  binding: ConversationConfigBinding,
  changes: ConversationConfigChanges,
  baseRevision: number,
): Promise<ConversationConfigSnapshot> {
  const { path, transportBinding } = conversationConfigTransport(binding)
  return requestJSON(path, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ binding: transportBinding, base_revision: baseRevision, changes }),
  })
}

function conversationConfigTransport(binding: ConversationConfigBinding) {
  const projectId = binding.project_id?.trim() || ''
  if (!projectId) return { path: '/api/conversation-config', transportBinding: binding }
  const { project_id: _projectId, ...transportBinding } = binding
  return { path: projectAPIPath(projectId, 'conversation-config'), transportBinding }
}

function conversationConfigQuery(binding: ConversationConfigBinding) {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(binding)) {
    if (value) query.set(key, value)
  }
  return query
}
