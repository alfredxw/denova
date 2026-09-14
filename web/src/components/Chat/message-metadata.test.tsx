import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { TrajectoryNavigationProvider } from '@/features/trajectory/trajectory-navigation'
import { MessageInlineMeta, SubAgentSessionCard } from './message-metadata'

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
afterEach(() => {
  if (originalClipboard) Object.defineProperty(navigator, 'clipboard', originalClipboard)
  else Reflect.deleteProperty(navigator, 'clipboard')
})

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

  it('reveals Run ID and copy actions together after streaming stops', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const { rerender } = render(
      <MessageInlineMeta
        projectId="project-a"
        message={{ role: 'assistant', content: '', run_id: 'run-stream-42', streaming: true }}
        content=""
        align="left"
        hideActions
      />,
    )

    expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Copy message' })).not.toBeInTheDocument()
    rerender(<MessageInlineMeta projectId="project-a" message={{ role: 'assistant', content: 'Partial reply', run_id: 'run-stream-42' }} content="Partial reply" align="left" />)
    expect(screen.getByRole('button', { name: 'Copy message' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Copy Run ID' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('run-stream-42'))
    expect(await screen.findByRole('button', { name: 'Run ID copied' })).toBeInTheDocument()
  })

  it('reports clipboard failure and never substitutes a message ID for a missing Run ID', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('Clipboard denied')) } })
    const { rerender } = render(<MessageInlineMeta message={{ role: 'error', content: 'Failed', run_id: 'failed-run' }} content="Failed" align="left" />)
    fireEvent.click(screen.getByRole('button', { name: 'Copy Run ID' }))
    expect(await screen.findByRole('button', { name: 'Failed to copy Run ID' })).toBeInTheDocument()
    rerender(<MessageInlineMeta message={{ id: 'message-only', role: 'assistant', content: 'Done' }} content="Done" align="left" />)
    expect(screen.queryByRole('button', { name: 'Copy Run ID' })).not.toBeInTheDocument()
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
