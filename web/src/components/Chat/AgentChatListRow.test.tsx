import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import { TrajectoryNavigationProvider } from '@/features/trajectory/trajectory-navigation'
import { AgentChatListRow, type AgentChatListItem } from './AgentChatListRow'
import { buildAgentRunPresentation } from './agent-run-presentation'

beforeEach(async () => { await i18next.changeLanguage('en-US') })

it('keeps historical turn media actions available without allowing narrative mutations', () => {
  const view = buildAgentMessageViews([{
    id: 'old-reply', role: 'assistant', metadata: { turn_id: 'old-turn', agent_kind: 'interactive_story', display_phase: 'final' },
    parts: [{ type: 'text', text: 'An earlier scene', state: 'done' }],
  }])[0]
  const generateImage = vi.fn()
  const readAloud = vi.fn()
  const renderRow = (streaming: boolean) => (
    <AgentChatListRow item={{ kind: 'message', key: view.key, view, sourceIndex: 0 }}
      executionTimings={new Map()} isStreaming={streaming} tailFollowActive={false}
      activeTraceDisplay="collapsed" subAgentPresentation="card" highlightDialogue={false}
      canMutateMessage={() => false} onEditAssistantReply={vi.fn()} onRegenerateMessage={vi.fn()}
      onGenerateInteractiveImage={generateImage} onReadAloud={readAloud} />
  )
  const { rerender } = render(renderRow(false))
  fireEvent.click(screen.getByRole('button', { name: 'Generate interactive image' }))
  fireEvent.click(screen.getByRole('button', { name: i18next.t('speech.read') }))
  expect(generateImage).toHaveBeenCalledWith(view)
  expect(readAloud).toHaveBeenCalledWith(view)
  expect(screen.queryByRole('button', { name: 'Edit AI reply' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Regenerate this turn' })).not.toBeInTheDocument()

  rerender(renderRow(true))
  expect(screen.queryByRole('button', { name: 'Generate interactive image' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: i18next.t('speech.read') })).toBeInTheDocument()
})

function runItem(messages: AgentUIMessage[], active: boolean): AgentChatListItem {
  const run = buildAgentRunPresentation(buildAgentMessageViews(messages), 0, active)!
  return { kind: 'run', key: run.key, runId: run.runID, sections: run.sections, sourceIndex: 0 }
}

function row(item: AgentChatListItem, active: boolean, nextItem?: AgentChatListItem) {
  return <AgentChatListRow projectId="project-a" item={item} nextItem={nextItem} executionTimings={new Map()} isStreaming={active} tailFollowActive={active} activeTraceDisplay="collapsed" subAgentPresentation="card" highlightDialogue={false} />
}

it.each(['ide', 'interactive_story'])('shows one Run reference only after the %s output stops', (agentKind) => {
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
  expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Open in Trajectory' })).not.toBeInTheDocument()

  // Cancellation can leave a reasoning/tool-only run without terminal prose.
  rerender(renderRun(false))
  expect(screen.getAllByRole('button', { name: 'Copy Run ID' })).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Open in Trajectory' }))
  expect(open).toHaveBeenCalledWith({ projectId: 'project-a', runId: 'current-run' })

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

it('hides Run actions while waiting for the model to emit content', () => {
  render(row({ kind: 'activity', key: 'waiting', content: 'Waiting for the model', runId: 'accepted-run' }, true))
  expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
})

it('keeps Run actions hidden when execution remains active between streamed parts', () => {
  const item = runItem([{
    id: 'thinking', role: 'assistant', metadata: { run_id: 'active-run' },
    parts: [{ type: 'reasoning', text: 'Checking the result', state: 'done' }],
  }], true)
  render(row(item, false))
  expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
})
