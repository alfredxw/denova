import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { TrajectoryNavigationProvider } from '@/features/trajectory/trajectory-navigation'
import { MessageInlineMeta, SubAgentSessionCard } from './message-metadata'

describe('MessageInlineMeta trajectory navigation', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('only exposes the exact Run shortcut when Developer Mode is enabled', () => {
    const open = vi.fn()
    const message = { role: 'assistant' as const, content: 'Done', run_id: 'run-42' }
    const { rerender } = render(
      <TrajectoryNavigationProvider value={{ enabled: false, intent: null, open }}>
        <MessageInlineMeta projectId="project-a" message={message} content="Done" align="left" />
      </TrajectoryNavigationProvider>,
    )

    expect(screen.queryByRole('button', { name: 'Open in Trajectory' })).not.toBeInTheDocument()

    rerender(
      <TrajectoryNavigationProvider value={{ enabled: true, intent: null, open }}>
        <MessageInlineMeta projectId="project-a" message={message} content="Done" align="left" />
      </TrajectoryNavigationProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Open in Trajectory' }))

    expect(open).toHaveBeenCalledWith({ projectId: 'project-a', runId: 'run-42' })
  })
})

describe('SubAgentSessionCard', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('shows status and keeps the child output in the detail view', () => {
    const open = vi.fn()
    const message = {
      role: 'assistant' as const,
      content: '# Live child output',
      agent_name: 'general-purpose',
      subagent: true,
      subagent_session_id: 'child-session',
      streaming: true,
    }
    render(<SubAgentSessionCard message={message} onOpen={open} />)

    const card = screen.getByRole('button', { name: 'general-purpose output' })
    expect(card).toHaveTextContent('Working')
    expect(screen.queryByText('# Live child output')).not.toBeInTheDocument()

    fireEvent.click(card)
    expect(open).toHaveBeenCalledWith(message)
  })
})
