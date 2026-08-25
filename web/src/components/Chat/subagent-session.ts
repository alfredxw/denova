import type { ChatMessage } from '@/lib/api'
import {
  agentSubAgentSessionKey,
  buildAgentSubAgentTimelineGroups,
  type AgentMessageView,
} from '@/lib/agent-message-view'

/** Projects one delegated invocation from the parent conversation timeline. */
export function selectSubAgentSessionViews(views: AgentMessageView[], sessionKey: string) {
  const group = buildAgentSubAgentTimelineGroups(views)
    .find(candidate => candidate.sessionKeys.includes(sessionKey))
  if (group) return group.views
  return views.filter(view => agentSubAgentSessionKey(view) === sessionKey && view.kind !== 'token-usage')
}

export function subAgentSessionKey(message?: Pick<ChatMessage, 'subagent' | 'subagent_session_id' | 'run_id' | 'agent_name' | 'root_agent_name' | 'run_path'> | null) {
  if (!message?.subagent) return ''
  if (message.subagent_session_id) return message.subagent_session_id
  return [
    message.run_id || '',
    message.root_agent_name || '',
    message.agent_name || '',
    ...(message.run_path || []),
  ].filter(Boolean).join('/')
}

export function isSubAgentTimelineMessage(message: ChatMessage) {
  if (!message.subagent) return false
  return message.role === 'assistant' || message.role === 'thinking' || message.role === 'tool_call' || message.role === 'tool_result'
}

export function buildSubAgentProgressMessage(messages: ChatMessage[]): ChatMessage | null {
  const first = messages.find((message) => message.subagent)
  if (!first) return null
  const reversed = [...messages].reverse()
  const latest = reversed.find((message) => message.role === 'assistant' && (message.content || '').trim())
    || reversed.find((message) => (message.content || (message.role === 'tool_call' || message.role === 'tool_result' ? message.name : '') || '').trim())
    || first
  const content = latest.role === 'tool_call'
    ? latest.name || latest.content || ''
    : latest.content || ''
  const sessionKey = subAgentSessionKey(first)
  return {
    id: sessionKey ? `subagent-progress:${sessionKey}` : first.id ? `${first.id}:progress` : 'subagent-progress',
    role: 'assistant',
    content,
    streaming: messages.some((message) => message.streaming === true || ('status' in message && message.status === 'running')),
    created_at: first.created_at,
    run_id: first.run_id,
    display_segment_id: first.display_segment_id,
    agent_kind: first.agent_kind,
    agent_name: first.agent_name,
    root_agent_name: first.root_agent_name,
    run_path: first.run_path,
    subagent: true,
    subagent_session_id: first.subagent_session_id,
    subagent_type: first.subagent_type,
  }
}
