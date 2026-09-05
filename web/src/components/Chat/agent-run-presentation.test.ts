import { expect, it } from 'vitest'
import { agentViewContent, buildAgentMessageViews } from '@/lib/agent-message-view'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { buildAgentRunPresentation } from './agent-run-presentation'

it.each(['ide', 'general', 'interactive_story'])('keeps completed and streaming cycles separate for %s', (agentKind) => {
  for (const streaming of [false, true]) {
    const messages: AgentUIMessage[] = [1, 2].map((cycle) => {
      const metadata = {
        run_id: 'same-run', agent_cycle: cycle, agent_kind: agentKind,
        display_segment_id: `reply-${cycle}`,
        display_phase: cycle === 1 ? 'progress' as const : 'final' as const,
      }
      return {
        id: `reply-${cycle}`,
        role: 'assistant',
        ...(streaming ? {} : { metadata }),
        parts: [{
          type: 'text', text: `Answer ${cycle}`, state: 'done',
          ...(streaming ? { providerMetadata: { agent: metadata } } : {}),
        }],
      }
    })
    const views = buildAgentMessageViews(messages)
    const first = buildAgentRunPresentation(views, 0, streaming)
    expect(first?.nextIndex).toBe(1)
    expect(first?.sections.map((section) => section.kind === 'message' && agentViewContent(section.view))).toEqual(['Answer 1'])
    const second = buildAgentRunPresentation(views, 1, streaming)
    expect(second?.nextIndex).toBe(2)
    expect(second?.sections.map((section) => section.kind === 'message' && agentViewContent(section.view))).toEqual(['Answer 2'])
  }
})

it('keeps a child journal with its own cycle inside the parent presentation', () => {
  const views = buildAgentMessageViews([
    {
      id: 'root', role: 'assistant',
      metadata: { run_id: 'parent-run', agent_cycle: 1 },
      parts: [{ type: 'text', text: 'Parent answer' }],
    },
    {
      id: 'child', role: 'assistant',
      metadata: { run_id: 'parent-run', agent_cycle: 7, subagent: true, subagent_session_id: 'child-session', agent_name: 'reviewer' },
      parts: [{ type: 'reasoning', text: 'Child reasoning' }],
    },
    {
      id: 'follow-up', role: 'assistant',
      metadata: { run_id: 'parent-run', agent_cycle: 2 },
      parts: [{ type: 'text', text: 'Follow-up answer' }],
    },
  ])
  const presentation = buildAgentRunPresentation(views, 0, true)
  expect(presentation?.nextIndex).toBe(2)
  expect(presentation?.sections.flatMap((section) => section.kind === 'process' ? section.views : [section.view])).toEqual(views.slice(0, 2))
})
