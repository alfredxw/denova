import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import { TrajectoryNavigationProvider } from '@/features/trajectory/trajectory-navigation'
import { AgentChatListRow, type AgentChatListItem } from './AgentChatListRow'
import { buildAgentRunPresentation } from './agent-run-presentation'

beforeEach(async () => { await i18next.changeLanguage('en-US') })

function runItem(messages: AgentUIMessage[], active: boolean): AgentChatListItem {
  const run = buildAgentRunPresentation(buildAgentMessageViews(messages), 0, active)!
  return { kind: 'run', key: run.key, runId: run.runID, sections: run.sections, sourceIndex: 0 }
}

function row(item: AgentChatListItem, active: boolean, nextItem?: AgentChatListItem) {
  return <AgentChatListRow projectId="project-a" item={item} nextItem={nextItem} executionTimings={new Map()} isStreaming={active} tailFollowActive={active} activeTraceDisplay="collapsed" subAgentPresentation="card" highlightDialogue={false} />
}

it.each(['ide', 'interactive_story'])('keeps one Run reference before and after the %s reply arrives', (agentKind) => {
  const open = vi.fn()
  const messages: AgentUIMessage[] = [{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'current-run', agent_kind: agentKind },
    parts: [{ type: 'reasoning', text: 'Working through the request', state: 'done' }],
  }]
  const renderRun = (active: boolean) => (
    <TrajectoryNavigationProvider value={{ enabled: true, intent: null, open }}>
      {row(runItem(messages, active), active)}
    </TrajectoryNavigationProvider>
  )
  const { rerender } = render(renderRun(true))
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Open in Trajectory' }))
  expect(open).toHaveBeenCalledWith({ projectId: 'project-a', runId: 'current-run' })

  // Cancellation can leave a reasoning/tool-only run without terminal prose.
  rerender(renderRun(false))
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)

  messages.push({
    id: 'reply', role: 'assistant', metadata: { run_id: 'current-run', agent_kind: agentKind, display_phase: 'final' },
    parts: [{ type: 'text', text: 'Final answer', state: 'done' }],
  })
  rerender(renderRun(false))
  expect(screen.getByText('Final answer')).toBeInTheDocument()
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
  expect(screen.getAllByRole('button', { name: 'Open in Trajectory' })).toHaveLength(1)
})

it('leaves the reference on the error message when it follows a failed run', () => {
  const views = buildAgentMessageViews([{
    id: 'error', role: 'assistant', metadata: { run_id: 'failed-run' },
    parts: [{ type: 'data-agent-error', data: { message: 'Request failed' } }],
  }])
  const error: AgentChatListItem = { kind: 'message', key: 'error', view: views[0], sourceIndex: 1 }
  const run = runItem([{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'failed-run' },
    parts: [{ type: 'reasoning', text: 'Working', state: 'done' }],
  }], false)
  render(<>{row(run, false, error)}{row(error, false)}</>)
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
})

it('exposes the accepted Run before the model emits any content', () => {
  render(row({ kind: 'activity', key: 'waiting', content: 'Waiting for the model', runId: 'accepted-run' }, true))
  expect(screen.getByRole('button', { name: 'Copy Run ID' })).toBeInTheDocument()
})
