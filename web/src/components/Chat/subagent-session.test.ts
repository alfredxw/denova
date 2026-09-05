import { describe, expect, it } from 'vitest'
import { buildAgentSubAgentTimelineGroups, type AgentMessageView, type AgentMessageViewKind } from '@/lib/agent-message-view'
import { buildSubAgentSummaryMessage, selectSubAgentSessionViews, subAgentStatusFromViews } from './subagent-session'

describe('selectSubAgentSessionViews', () => {
  it('isolates interleaved concurrent sessions of the same SubAgent type', () => {
    const firstA = subAgentView('first-a', 'child-session-a', { content: 'A thinks ', kind: 'reasoning' })
    const firstB = subAgentView('first-b', 'child-session-b', { content: 'B answer ' })
    const secondA = subAgentView('second-a', 'child-session-a', { content: 'again', kind: 'reasoning' })
    const secondB = subAgentView('second-b', 'child-session-b', { content: 'done' })
    const views = [firstA, firstB, secondA, secondB]

    expect(selectSubAgentSessionViews(views, 'child-session-a')).toEqual([{ ...firstA, content: 'A thinks again' }])
    expect(selectSubAgentSessionViews(views, 'child-session-b')).toEqual([{ ...firstB, content: 'B answer done' }])
    expect(buildAgentSubAgentTimelineGroups(views).map(group => ({ key: group.key, indexes: group.viewIndexes, content: group.views.map(view => view.content) }))).toEqual([
      { key: 'child-session-a', indexes: [0, 2], content: ['A thinks again'] },
      { key: 'child-session-b', indexes: [1, 3], content: ['B answer done'] },
    ])
  })

  it('retains the exact terminal status in the shared SubAgent card projection', () => {
    const content = subAgentView('content', 'child-session')
    const settled = { ...subAgentView('settled', 'child-session', { kind: 'subagent-status' }), data: { status: 'failed' } }
    expect(subAgentStatusFromViews([content, settled])).toBe('failed')

    const summary = buildSubAgentSummaryMessage([{ ...content, streaming: true }, settled])
    expect(summary).toMatchObject({ role: 'assistant', content: '', subagent_status: 'failed', streaming: false })
  })

  it('projects only identity and lifecycle facts without reading tool payloads', () => {
    const tool: AgentMessageView = {
      ...subAgentView('write-call', 'child-session', { kind: 'tool' }),
      status: 'running',
      get input() { throw new Error('Status cards must not read tool input') },
      get output() { throw new Error('Status cards must not read tool output') },
    }
    expect(buildSubAgentSummaryMessage([tool])).toMatchObject({
      id: 'subagent-summary:child-session',
      role: 'assistant',
      content: '',
      agent_name: 'researcher',
      subagent_session_id: 'child-session',
      streaming: true,
    })
    const waiting = { ...subAgentView('ask', 'child-session', { kind: 'ask' }), streaming: true }
    expect(buildSubAgentSummaryMessage([tool, waiting])).toMatchObject({
      subagent_status: 'waiting_input',
      streaming: true,
    })
  })
})

function subAgentView(id: string, sessionID: string, options: { agentName?: string; content?: string; subagent?: boolean; kind?: AgentMessageViewKind } = {}) {
  const subagent = options.subagent ?? true
  return {
    key: id,
    kind: options.kind ?? 'assistant',
    messageId: id,
    partId: id,
    content: options.content ?? id,
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
