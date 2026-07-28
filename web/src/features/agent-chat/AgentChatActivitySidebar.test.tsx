import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatProject } from './api'
import { AgentChatActivitySidebar } from './AgentChatActivitySidebar'
import type { AgentChatSidebarActivity } from './sidebar-activity'

const project: AgentChatProject = {
  path: '/books/alpha',
  name: 'Alpha',
  current: true,
  total: 2,
  sessions: [
    { id: 'active', title: 'Active chat', created_at: '', updated_at: '', message_count: 2, running: true, active: false },
    { id: 'history', title: 'Historical chat', created_at: '', updated_at: '', message_count: 1, running: false, active: false },
  ],
}

const activity: AgentChatSidebarActivity = {
  id: 'agent:active',
  workspace: project.path,
  kind: 'agent',
  title: 'Active chat',
  status: 'running',
  tabId: 'agent-tab',
  sessionId: 'active',
  group: 'primary',
  paneVisible: true,
  focused: true,
}

function renderSidebar(overrides: Partial<ComponentProps<typeof AgentChatActivitySidebar>> = {}) {
  const props: ComponentProps<typeof AgentChatActivitySidebar> = {
    projects: [project],
    activitiesByProject: new Map([[project.path, [activity]]]),
    loading: false,
    error: '',
    activeProjectPath: project.path,
    onSelectProject: vi.fn(),
    onOpenActivity: vi.fn(),
    onCreateSession: vi.fn(),
    onOpenHistory: vi.fn(),
    ...overrides,
  }
  return { ...render(<AgentChatActivitySidebar {...props} />), props }
}

describe('AgentChatActivitySidebar', () => {
  beforeEach(() => window.localStorage.clear())

  it('shows active work but keeps historical conversations out of the project tree', () => {
    renderSidebar()
    const projectButton = screen.getByTitle(project.path)
    const activityButton = screen.getByRole('button', { name: /Active chat/ })

    expect(projectButton).not.toHaveAttribute('aria-current')
    expect(projectButton.parentElement).not.toHaveClass('bg-[var(--nova-active)]')
    expect(activityButton).toHaveAttribute('aria-current', 'page')
    expect(activityButton).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.queryByText('Historical chat')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '对话历史' })).toBeInTheDocument()
  })

  it('opens an activity directly and uses the whole project row as the collapse target', async () => {
    const user = userEvent.setup()
    const { props } = renderSidebar()
    await user.click(screen.getByRole('button', { name: /Active chat/ }))
    expect(props.onOpenActivity).toHaveBeenCalledWith(project, activity)

    const projectButton = screen.getByTitle(project.path)
    await user.click(projectButton)
    expect(projectButton).toHaveAttribute('aria-expanded', 'false')
    expect(projectButton.parentElement?.querySelector('[data-slot="agent-chat-project-active-indicator"]')).not.toBeNull()
    expect(screen.queryByRole('button', { name: /Active chat/ })).not.toBeInTheDocument()
  })

  it('keeps a neutral cursor while projects are draggable in manual mode', async () => {
    const user = userEvent.setup()
    renderSidebar()
    await user.click(screen.getByRole('button', { name: '排序：最近更新' }))
    await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: '手动排序' }))
    expect(screen.getByTitle(/books\/alpha.*长按拖拽排序/)).toHaveClass('cursor-default')
  })
})
