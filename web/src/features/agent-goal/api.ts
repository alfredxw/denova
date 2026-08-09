import { jsonHeaders, requestJSON } from '@/lib/api-client'
import { projectAPIPath } from '@/lib/api-client/project-scope'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'
import type { ConversationGoal, ConversationGoalAction } from './types'

interface GoalEnvelope {
  goal: ConversationGoal | null
}

export async function fetchConversationGoal(binding: ConversationConfigBinding): Promise<ConversationGoal | null> {
  const { path, transportBinding } = goalTransport(binding)
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(transportBinding)) {
    if (value) query.set(key, value)
  }
  return (await requestJSON<GoalEnvelope>(`${path}?${query.toString()}`)).goal
}

export async function mutateConversationGoal(
  binding: ConversationConfigBinding,
  action: ConversationGoalAction,
  expectedRevision: number,
  objective?: string,
): Promise<ConversationGoal> {
  const { path, transportBinding } = goalTransport(binding)
  const envelope = await requestJSON<GoalEnvelope>(path, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      binding: transportBinding,
      action,
      expected_revision: expectedRevision,
      ...(objective === undefined ? {} : { objective }),
    }),
  })
  if (!envelope.goal) throw new Error('Conversation goal mutation returned no goal')
  return envelope.goal
}

function goalTransport(binding: ConversationConfigBinding) {
  const projectId = binding.project_id?.trim() || ''
  if (!projectId) throw new Error('Conversation goal requires a Project ID')
  const { project_id: _projectId, ...transportBinding } = binding
  return { path: projectAPIPath(projectId, 'conversation-goal'), transportBinding }
}
