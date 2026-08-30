import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '@/lib/api'
import type { AgentMessageView, AgentMessageViewKind } from '@/lib/agent-message-view'
import { buildSubAgentProgressMessage, selectSubAgentSessionViews, subAgentStatusFromViews } from './subagent-session'

describe('selectSubAgentSessionViews', () => {
  it('projects every contiguous slice of one legacy delegated invocation', () => {
    const first = subAgentView('first', 'child-session-1')
    const second = subAgentView('second', 'child-session-2')
    const other = subAgentView('other', 'other-session', { agentName: 'reviewer' })

    expect(selectSubAgentSessionViews([first, second, other], 'child-session-2')).toEqual([first, second])
  })

  it('does not cross a root Agent event when matching an older ancestry key', () => {
    const first = subAgentView('first', 'child-session-1')
    const root = subAgentView('root', '', { subagent: false })
    const second = subAgentView('second', 'child-session-2')

    expect(selectSubAgentSessionViews([first, root, second], 'child-session-2')).toEqual([second])
  })

  it('retains the exact terminal status in the shared SubAgent card projection', () => {
    const content = subAgentView('content', 'child-session')
    const settled = { ...subAgentView('settled', 'child-session', { kind: 'subagent-status' }), data: { status: 'failed' } }
    expect(subAgentStatusFromViews([content, settled])).toBe('failed')

    const progress = buildSubAgentProgressMessage([
      { role: 'assistant', content: 'partial output', streaming: true, subagent: true, subagent_session_id: 'child-session' },
      { role: 'system', content: '', subagent: true, subagent_session_id: 'child-session', subagent_status: 'failed' },
    ] as ChatMessage[])
    expect(progress).toMatchObject({ role: 'assistant', content: 'partial output', subagent_status: 'failed', streaming: false })
  })
})

function subAgentView(id: string, sessionID: string, options: { agentName?: string; subagent?: boolean; kind?: AgentMessageViewKind } = {}) {
  const subagent = options.subagent ?? true
  return {
    key: id,
    kind: options.kind ?? 'assistant',
    messageId: id,
    partId: id,
    metadata: {
      run_id: 'run-1',
      agent_name: options.agentName ?? 'researcher',
      root_agent_name: 'writer',
      run_path: ['writer', options.agentName ?? 'researcher'],
      subagent,
      subagent_session_id: sessionID || undefined,
    },
    streaming: false,
  } as AgentMessageView
}
