import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AgentChatProjectDetailsCard } from './AgentChatProjectDetailsCard'

const project = { id: 'project', name: 'Project', path: '/workspace/project', type: 'general' as const,
  status: 'available' as const, current: false, total: 0, sessions: [] }

afterEach(() => vi.useRealTimers())

it('does not open a delayed project card after pointer and focus have left', async () => {
  vi.useFakeTimers()
  render(<><AgentChatProjectDetailsCard project={project} active manualSorting={false}>
    <button>Project</button>
  </AgentChatProjectDetailsCard><button>Composer</button></>)
  const trigger = screen.getByRole('button', { name: 'Project' })
  // Clicking a row schedules opening from both pointer entry and focus.
  fireEvent.pointerEnter(trigger, { pointerType: 'mouse' })
  act(() => trigger.focus())
  fireEvent.pointerLeave(trigger, { pointerType: 'mouse' })
  act(() => screen.getByRole('button', { name: 'Composer' }).focus())
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(screen.queryByText(project.path)).not.toBeInTheDocument()
})

it('keeps project details accessible from keyboard focus', async () => {
  vi.useFakeTimers()
  render(<AgentChatProjectDetailsCard project={project} active manualSorting={false}>
    <button>Project</button>
  </AgentChatProjectDetailsCard>)
  act(() => screen.getByRole('button', { name: 'Project' }).focus())
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(screen.getByText(project.path)).toBeVisible()
})
