import type { ChatMessage } from '@/lib/api'

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
  const assistant = messages.find((message) => message.role === 'assistant' && (message.content || '').trim())
  if (assistant) return assistant
  const latest = [...messages].reverse().find((message) => (message.content || message.name || '').trim()) || first
  const content = latest.role === 'tool_call'
    ? latest.name || latest.content || ''
    : latest.content || ''
  return {
    ...first,
    id: first.id ? `${first.id}:progress` : `subagent-progress:${subAgentSessionKey(first)}`,
    role: 'assistant',
    content,
    streaming: messages.some((message) => message.streaming !== false),
  }
}
