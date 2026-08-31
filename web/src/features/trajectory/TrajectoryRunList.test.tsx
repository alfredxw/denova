import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import type { GlobalAgentRunTraceSummary } from '@/lib/api'
import { TrajectoryRunList } from './TrajectoryRunList'

describe('TrajectoryRunList', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('shows each project once and keeps session identifiers out of the visual hierarchy', () => {
    const runs = [
      run({ id: 'selected-run', session_id: 'session-first', session_title: 'Read the outline', trajectory_uri: 'trajectory:selected', agent_kind: 'ide' }),
      run({ id: 'second-run', session_id: 'session-second', session_title: 'Continue the chapter', trajectory_uri: 'trajectory:second', agent_kind: 'writer' }),
      run({ id: 'no-session-one', session_id: undefined, session_title: undefined, trajectory_uri: 'trajectory:no-session-one' }),
      run({ id: 'no-session-two', session_id: undefined, session_title: undefined, trajectory_uri: 'trajectory:no-session-two' }),
      run({ id: 'agents-run', project_id: 'agents', project_name: 'Agents', session_id: 'session-agents', session_title: 'Profile review', trajectory_uri: 'trajectory:agents' }),
    ]

    render(<TrajectoryRunList runs={runs} selectedRunURI="trajectory:selected" onSelect={vi.fn()} />)

    expect(screen.getAllByText('TestBook')).toHaveLength(1)
    expect(screen.getAllByText('Agents')).toHaveLength(1)
    expect(screen.getByText('Read the outline')).toBeInTheDocument()
    expect(screen.getByText('Continue the chapter')).toBeInTheDocument()
    expect(screen.queryByText('session-first')).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /TestBook · No session · 2 runs/ })).toHaveLength(1)
    expect(screen.getByText('ide')).toBeInTheDocument()
    expect(screen.queryByText('writer')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /TestBook · Continue the chapter .* 1 runs/ }))

    expect(screen.getByText('writer')).toBeInTheDocument()
  })
})

function run(overrides: Partial<GlobalAgentRunTraceSummary>): GlobalAgentRunTraceSummary {
  return {
    id: 'run-id',
    created_at: '2026-08-26T15:34:51Z',
    path: 'trajectory.jsonl',
    status: 'success',
    events: 12,
    context_parts: 3,
    duration_ms: 250_000,
    llm_calls: 21,
    tool_calls: 27,
    content_captured: true,
    agent_kind: 'ide',
    project_id: 'test-book',
    project_name: 'TestBook',
    session_id: 'session-default',
    session_title: 'Session title',
    trajectory_uri: 'trajectory:default',
    ...overrides,
  }
}
