import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { AgentChatProject } from './api'
import { AgentChatProjectSwitcher } from './AgentChatProjectSwitcher'

const projects: AgentChatProject[] = [
  {
    id: 'project-a',
    type: 'book',
    path: '/books/a',
    name: 'Project A',
    status: 'available',
    current: false,
    total: 3,
    sessions: [],
  },
  {
    id: 'project-b',
    type: 'general',
    path: '/projects/b',
    name: 'Project B',
    status: 'available',
    current: false,
    total: 5,
    sessions: [],
  },
]

describe('AgentChatProjectSwitcher', () => {
  it('shows the active Project and switches only through AgentChat navigation', async () => {
    const user = userEvent.setup()
    const selectProject = vi.fn()
    render(
      <AgentChatProjectSwitcher
        navigation={{ projects, activeProjectId: 'project-a', loading: false, selectProject }}
      />,
    )

    await user.click(screen.getByRole('button', { name: '切换项目，当前：Project A' }))
    expect(screen.getByRole('menu', { name: '切换项目' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Project A/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('menuitem', { name: /Project B/ })).toHaveTextContent('5 个会话 · /projects/b')

    await user.click(screen.getByRole('menuitem', { name: /Project B/ }))
    expect(selectProject).toHaveBeenCalledWith('project-b')
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
  })
})
