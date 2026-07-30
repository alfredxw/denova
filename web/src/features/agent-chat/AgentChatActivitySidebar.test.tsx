import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatProject } from './api'
import { AgentChatActivitySidebar, AgentChatSidebarRail } from './AgentChatActivitySidebar'
import type { AgentChatSidebarActivity } from './sidebar-activity'

const project: AgentChatProject = {
  id: 'project-alpha',
  type: 'book',
  status: 'available',
  path: '/books/alpha',
  name: 'Alpha',
  current: true,
  total: 2,
  sessions: [
    {
      id: 'active',
      title: 'Active chat',
      created_at: '',
      updated_at: '',
      message_count: 2,
      running: true,
      active: false,
    },
    {
      id: 'history',
      title: 'Historical chat',
      created_at: '',
      updated_at: '',
      message_count: 1,
      running: false,
      active: false,
    },
  ],
}

const activity: AgentChatSidebarActivity = {
  id: 'agent:active',
  projectId: project.id,
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

function sidebarProps(overrides: Partial<ComponentProps<typeof AgentChatActivitySidebar>> = {}): ComponentProps<typeof AgentChatActivitySidebar> {
  return {
    projects: [project],
    activitiesByProject: new Map([[project.id, [activity]]]),
    loading: false,
    error: '',
    activeProjectId: project.id,
    onSelectProject: vi.fn(),
    onOpenActivity: vi.fn(),
    onCreateSession: vi.fn(),
    onOpenHistory: vi.fn(),
    onAddProject: vi.fn(),
    projectDirectoryBusy: false,
    onRenameProject: vi.fn(),
    onRelinkProject: vi.fn(),
    onArchiveProject: vi.fn(),
    ...overrides,
  }
}

function renderSidebar(overrides: Partial<ComponentProps<typeof AgentChatActivitySidebar>> = {}) {
  const props = sidebarProps(overrides)
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
    await user.click(
      within(await screen.findByRole('menu')).getByRole('menuitem', {
        name: '手动排序',
      }),
    )
    expect(screen.getByTitle(/books\/alpha.*长按拖拽排序/)).toHaveClass('cursor-default')
  })

  it('does not mount the full peek tree when the expand action is clicked directly', () => {
    vi.useFakeTimers()
    try {
      const onExpand = vi.fn()
      render(
        <AgentChatSidebarRail
          {...sidebarProps()}
          onExpand={onExpand}
          onCreateDefaultSession={vi.fn()}
          createDisabled={false}
        />,
      )
      const expand = screen.getByRole('button', { name: '显示活动列表' })
      const rail = expand.parentElement!

      fireEvent.mouseEnter(rail)
      fireEvent.focus(expand)
      fireEvent.click(expand)
      act(() => vi.advanceTimersByTime(200))

      expect(onExpand).toHaveBeenCalledTimes(1)
      expect(screen.queryByTitle(project.path)).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })
})
