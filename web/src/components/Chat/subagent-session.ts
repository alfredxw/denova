import type { ChatMessage, SubAgentStatus } from '@/lib/api'
import {
  agentSubAgentSessionKey,
  buildAgentSubAgentTimelineGroups,
  readSubAgentStatus,
  type AgentMessageView,
} from '@/lib/agent-message-view'

/** Projects one delegated invocation from the parent conversation timeline. */
export function selectSubAgentSessionViews(views: AgentMessageView[], sessionKey: string) {
  const group = buildAgentSubAgentTimelineGroups(views)
    .find(candidate => candidate.key === sessionKey)
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

/** Reads the one unambiguous delegated invocation returned by task.start. */
export function taskSubAgentSessionKey(result: string) {
  if (!result.trim()) return ''
  try {
    const parsed = JSON.parse(result) as {
      results?: Array<{ task?: { ref?: { session?: unknown; run?: unknown } } }>
    }
    const keys = new Set<string>()
    for (const item of parsed.results || []) {
      const session = typeof item.task?.ref?.session === 'string' ? item.task.ref.session.trim() : ''
      const run = typeof item.task?.ref?.run === 'string' ? item.task.ref.run.trim() : ''
      if (session && run) keys.add(`${session}/${run}`)
    }
    return keys.size === 1 ? [...keys][0] : ''
  } catch {
    return ''
  }
}

export function isSubAgentTimelineMessage(message: ChatMessage) {
  if (!message.subagent) return false
  return message.role === 'assistant' || message.role === 'thinking' || message.role === 'tool_call' || message.role === 'tool_result'
}

/** Status cards never need the child prose or serialized tool arguments/results. */
export function buildSubAgentSummaryMessage(views: AgentMessageView[]): ChatMessage | null {
  const first = views.find((view) => view.metadata.subagent)
  if (!first) return null
  const metadata = first.metadata
  const id = first.partId || first.messageId
  const sessionKey = subAgentSessionKey(metadata)
  const subAgentStatus = subAgentStatusFromViews(views)
  const streaming = subAgentStatus
    ? isActiveSubAgentStatus(subAgentStatus)
    : views.some((view) => view.streaming || view.status === 'running')
  return {
    id: sessionKey ? `subagent-summary:${sessionKey}` : id ? `${id}:summary` : 'subagent-summary',
    role: 'assistant',
    content: '',
    streaming,
    created_at: metadata.created_at,
    run_id: metadata.run_id,
    display_segment_id: metadata.display_segment_id,
    agent_kind: metadata.agent_kind,
    agent_name: metadata.agent_name,
    root_agent_name: metadata.root_agent_name,
    run_path: metadata.run_path,
    subagent: true,
    subagent_session_id: metadata.subagent_session_id,
    subagent_type: metadata.subagent_type,
    subagent_status: subAgentStatus,
  }
}

export function subAgentStatusFromViews(views: AgentMessageView[]): SubAgentStatus | undefined {
  const settled = [...views].reverse().find((view) => view.kind === 'subagent-status')
  const status = readSubAgentStatus(settled?.data.status)
  if (status) return status
  if (views.some((view) => view.kind === 'ask' && view.streaming)) return 'waiting_input'
  if (views.some((view) => view.streaming)) return 'running'
  return undefined
}

export function subAgentStatusTranslationKey(status: SubAgentStatus | undefined, running: boolean) {
  const resolved = status ?? (running ? 'running' : 'completed')
  return `chat.subagent.status.${resolved}`
}

export function isActiveSubAgentStatus(status: SubAgentStatus) {
  return status === 'running' || status === 'waiting_input' || status === 'aborting'
}
