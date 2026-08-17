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

const terminalActivity: AgentChatSidebarActivity = {
  id: 'terminal:shell-tab',
  projectId: project.id,
  workspace: project.path,
  kind: 'terminal',
  title: 'Shell',
  status: 'ready',
  tabId: 'shell-tab',
  group: 'secondary',
  paneVisible: true,
  focused: false,
}

function sidebarProps(overrides: Partial<ComponentProps<typeof AgentChatActivitySidebar>> = {}): ComponentProps<typeof AgentChatActivitySidebar> {
  return {
    projects: [project],
    activitiesByProject: new Map([[project.id, [activity, terminalActivity]]]),
    loading: false,
    error: '',
    activeProjectId: project.id,
    onSelectProject: vi.fn(),
    onOpenActivity: vi.fn(),
    onOpenSession: vi.fn(),
    onRenameSession: vi.fn(),
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

  it('keeps the current Project selected and exposes recent conversation navigation', async () => {
    const user = userEvent.setup()
    const { props } = renderSidebar()
    const projectButton = screen.getByRole('button', { name: '收起 Alpha' })
    const projectHistory = screen.getByRole('button', { name: '打开 Alpha 的全部会话' })
    const activityButton = screen.getByRole('button', { name: /Active chat/ })
    const terminalButton = screen.getByRole('button', { name: /Shell/ })

    expect(screen.queryByText('已打开')).not.toBeInTheDocument()
    expect(screen.queryByText('最近会话')).not.toBeInTheDocument()
    expect(projectButton).toHaveAttribute('aria-current', 'location')
    expect(projectButton.parentElement).toHaveAttribute('data-current-project', 'true')
    expect(projectButton.parentElement).toHaveClass('bg-[var(--nova-surface-2)]')
    expect(activityButton).toHaveAttribute('aria-current', 'page')
    expect(activityButton).not.toHaveAttribute('title')
    expect(activityButton).toHaveClass('bg-[var(--nova-active)]')
    const recentSessionButton = screen.getByRole('button', { name: '打开 Historical chat' })
    expect(recentSessionButton).not.toHaveAttribute('title')
    await user.click(recentSessionButton)
    expect(props.onOpenSession).toHaveBeenCalledWith(project, project.sessions[1])
    await user.click(terminalButton)
    expect(props.onOpenActivity).toHaveBeenCalledWith(project, terminalActivity)
    await user.click(projectHistory)
    expect(props.onOpenHistory).toHaveBeenCalledWith(project)
    expect(screen.getByRole('button', { name: '对话历史' })).toBeInTheDocument()
  })

  it('keeps all child rows on one compact text baseline while distinguishing terminals typographically', () => {
    renderSidebar()
    const projectButton = screen.getByRole('button', { name: '收起 Alpha' })
    const projectHistory = screen.getByRole('button', { name: '打开 Alpha 的全部会话' })
    const projectActions = screen.getByRole('button', { name: 'Alpha 的项目操作' })
    const newConversation = screen.getByRole('button', { name: '在 Alpha 中新建对话' })
    const directButtons = projectButton.parentElement?.querySelectorAll(':scope > button')
    const conversation = screen.getByRole('button', { name: /Active chat/ })
    const terminal = screen.getByRole('button', { name: /Shell/ })

    expect(directButtons?.[1]).toBe(projectHistory)
    expect(directButtons?.[2]).toBe(projectActions)
    expect(directButtons?.[3]).toBe(newConversation)
    expect(projectButton).toHaveTextContent('Alpha')
    expect(projectButton).not.toHaveTextContent('2')
    expect(projectButton).toHaveClass('focus-visible:bg-[var(--nova-active)]')
    expect(projectButton.className).not.toContain('focus-visible:ring')
    expect(projectHistory).toHaveClass('opacity-0', 'group-hover:opacity-100')
    expect(projectHistory.querySelector('svg')).not.toBeNull()
    expect(conversation).toHaveClass('pl-1.5')
    expect(conversation.className).not.toContain('focus-visible:ring')
    expect(conversation.querySelector('svg')).toBeNull()
    expect(conversation.querySelector('span.min-w-0')).toHaveClass('text-[11px]')
    expect(terminal).toHaveClass('pl-1.5')
    expect(terminal.className).not.toContain('focus-visible:ring')
    expect(terminal.querySelector('svg')).toBeNull()
    expect(terminal.querySelector('span.min-w-0')).toHaveClass('font-mono', 'text-[10px]')
    expect(screen.queryByText('全部会话')).not.toBeInTheDocument()
  })

  it('offers rename from both Project and conversation context menus', async () => {
    const user = userEvent.setup()
    const { props } = renderSidebar()

    fireEvent.contextMenu(screen.getByRole('button', { name: '收起 Alpha' }))
    await user.click(await screen.findByRole('menuitem', { name: '重命名项目' }))
    expect(props.onRenameProject).toHaveBeenCalledWith(project)

    const conversation = screen.getByRole('button', { name: /Historical chat/ })
    fireEvent.contextMenu(conversation)
    await user.click(await screen.findByRole('menuitem', { name: '重命名会话' }))
    expect(props.onRenameSession).toHaveBeenCalledWith(project, project.sessions[1])
  })

  it('shows Project details in a right-side card without a tooltip', async () => {
    const user = userEvent.setup()
    renderSidebar()
    const projectButton = screen.getByRole('button', { name: '收起 Alpha' })

    expect(projectButton).not.toHaveAttribute('title')
    expect(screen.queryByText(project.path)).not.toBeInTheDocument()
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()

    await user.hover(projectButton)
    const path = await screen.findByText(project.path, undefined, { timeout: 1600 })
    const details = path.closest('[data-slot="agent-chat-project-details"]')
    expect(details).toHaveAttribute('data-side', 'right')
    expect(details).toHaveTextContent(project.name)
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  it('keeps a recent conversation in place when it gains open-session state', () => {
    const view = renderSidebar()
    const conversationOrder = () => Array.from(
      document.querySelectorAll('[data-slot="agent-chat-conversation-row"]'),
      (row) => row.textContent,
    )
    expect(conversationOrder()).toEqual(['Active chat运行中', 'Historical chat'])

    const historicalActivity: AgentChatSidebarActivity = {
      ...activity,
      id: 'agent:history',
      title: 'Historical chat',
      tabId: 'history-tab',
      sessionId: 'history',
      focused: true,
    }
    view.rerender(
      <AgentChatActivitySidebar
        {...view.props}
        activitiesByProject={new Map([[
          project.id,
          [{ ...activity, paneVisible: false, focused: false }, historicalActivity, terminalActivity],
        ]])}
      />,
    )

    expect(conversationOrder()).toEqual(['Active chat运行中', 'Historical chat运行中'])
    expect(screen.getByRole('button', { name: /Historical chat/ })).toHaveAttribute('aria-current', 'page')
  })

  it('keeps an active conversation visible beyond the recent-session limit', () => {
    const olderActiveSession = {
      ...project.sessions[1],
      id: 'older-active',
      title: 'Older active chat',
    }
    const extendedProject: AgentChatProject = {
      ...project,
      total: 7,
      sessions: [
        ...project.sessions,
        { ...project.sessions[1], id: 'third', title: 'Third recent chat' },
        { ...project.sessions[1], id: 'fourth', title: 'Fourth recent chat' },
        { ...project.sessions[1], id: 'fifth', title: 'Fifth recent chat' },
        { ...project.sessions[1], id: 'sixth', title: 'Sixth recent chat' },
        olderActiveSession,
      ],
    }
    const olderActivity: AgentChatSidebarActivity = {
      ...activity,
      id: 'agent:older-active',
      title: olderActiveSession.title,
      sessionId: olderActiveSession.id,
      tabId: 'older-active-tab',
      paneVisible: false,
      focused: false,
    }

    renderSidebar({
      projects: [extendedProject],
      activitiesByProject: new Map([[extendedProject.id, [activity, terminalActivity, olderActivity]]]),
    })

    expect(document.querySelectorAll('[data-slot="agent-chat-conversation-row"]')).toHaveLength(6)
    expect(screen.getByRole('button', { name: /Older active chat/ })).toBeInTheDocument()
  })

  it('shows five recent conversations by default and reveals the next page inline', async () => {
    const user = userEvent.setup()
    const sessions = Array.from({ length: 8 }, (_, index) => ({
      ...project.sessions[1],
      id: `session-${index + 1}`,
      title: `Chat ${index + 1}`,
    }))
    const extendedProject: AgentChatProject = {
      ...project,
      total: sessions.length,
      sessions,
    }

    renderSidebar({
      projects: [extendedProject],
      activitiesByProject: new Map([[extendedProject.id, []]]),
    })

    expect(document.querySelectorAll('[data-slot="agent-chat-conversation-row"]')).toHaveLength(5)
    const showMore = screen.getByRole('button', { name: '展示 Alpha 的更多会话' })
    expect(showMore.parentElement).not.toHaveClass('border-t')
    expect(screen.queryByText('全部会话')).not.toBeInTheDocument()

    await user.click(showMore)

    expect(document.querySelectorAll('[data-slot="agent-chat-conversation-row"]')).toHaveLength(8)
    expect(screen.queryByRole('button', { name: '展示 Alpha 的更多会话' })).not.toBeInTheDocument()
    expect(showMore).toHaveClass('text-[var(--nova-text-faint)]')
    expect(showMore.className).not.toContain('hover:bg-')

    await user.click(screen.getByRole('button', { name: '收起 Alpha' }))
    await user.click(screen.getByRole('button', { name: '展开 Alpha' }))

    expect(document.querySelectorAll('[data-slot="agent-chat-conversation-row"]')).toHaveLength(5)
    expect(screen.getByRole('button', { name: '展示 Alpha 的更多会话' })).toBeInTheDocument()
  })

  it('keeps a not-yet-persisted conversation draft in the same flat list', () => {
    const draftActivity: AgentChatSidebarActivity = {
      ...activity,
      id: 'agent:draft-session',
      title: 'New draft chat',
      sessionId: 'draft-session',
      tabId: 'draft-tab',
      paneVisible: false,
      focused: false,
      status: 'idle',
    }

    renderSidebar({
      activitiesByProject: new Map([[project.id, [activity, draftActivity, terminalActivity]]]),
    })

    expect(screen.getByRole('button', { name: /New draft chat/ })).toBeInTheDocument()
    expect(screen.queryByText('已打开')).not.toBeInTheDocument()
    expect(screen.queryByText('最近会话')).not.toBeInTheDocument()
  })

  it('starts with only the current Project expanded and reveals another Project when selected', async () => {
    const user = userEvent.setup()
    const beta: AgentChatProject = {
      ...project,
      id: 'project-beta',
      path: '/books/beta',
      name: 'Beta',
      current: false,
      sessions: [{ ...project.sessions[1], id: 'beta-history', title: 'Beta history' }],
      total: 1,
    }
    const { props } = renderSidebar({
      projects: [project, beta],
      activitiesByProject: new Map([[project.id, [activity]], [beta.id, []]]),
    })

    expect(screen.queryByRole('button', { name: '打开 Beta history' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开 Beta' }))
    expect(props.onSelectProject).toHaveBeenCalledWith(beta)
    expect(screen.getByRole('button', { name: '打开 Beta history' })).toBeInTheDocument()
  })

  it('toggles from the full Project row while preserving the current marker', async () => {
    const user = userEvent.setup()
    const { props } = renderSidebar()
    await user.click(screen.getByRole('button', { name: /Active chat/ }))
    expect(props.onOpenSession).toHaveBeenCalledWith(project, project.sessions[0])

    const projectButton = screen.getByRole('button', { name: '收起 Alpha' })
    const projectContent = document.querySelector('[data-slot="agent-chat-project-content"]')
    expect(projectButton).toHaveAccessibleName('收起 Alpha')
    expect(projectButton.querySelector('[data-slot="agent-chat-project-chevron"]')?.closest('button')).toBe(projectButton)
    expect(projectContent).toHaveAttribute('data-state', 'open')
    expect(projectContent).toHaveClass('grid-rows-[1fr]', 'duration-[var(--nova-motion-fast)]')
    await user.click(projectButton)
    expect(props.onSelectProject).toHaveBeenCalledWith(project)
    expect(projectButton).toHaveAccessibleName('展开 Alpha')
    expect(projectButton).toHaveAttribute('aria-expanded', 'false')
    expect(projectContent).toHaveAttribute('data-state', 'closed')
    expect(projectContent).toHaveAttribute('inert')
    expect(projectContent).toHaveClass('grid-rows-[0fr]', 'opacity-0')
    expect(projectButton.parentElement?.querySelector('[data-slot="agent-chat-project-active-indicator"]')).toBeNull()
    expect(screen.queryByRole('button', { name: /Active chat/ })).not.toBeInTheDocument()

    await user.click(projectButton)
    expect(projectButton).toHaveAccessibleName('收起 Alpha')
    expect(screen.getByRole('button', { name: /Active chat/ })).toBeInTheDocument()
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
    const projectButton = screen.getByRole('button', { name: '收起 Alpha' })
    expect(projectButton).toHaveClass('cursor-default')
    expect(projectButton).not.toHaveAttribute('title')
    expect(projectButton).toHaveAttribute('aria-description', '长按拖拽排序')
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
      const expand = screen.getByRole('button', { name: '显示项目导航' })
      const rail = expand.parentElement!

      fireEvent.mouseEnter(rail)
      fireEvent.focus(expand)
      fireEvent.click(expand)
      act(() => vi.advanceTimersByTime(200))

      expect(onExpand).toHaveBeenCalledTimes(1)
      expect(screen.queryByRole('button', { name: '收起 Alpha' })).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('opens the project-tree peek after deliberate hover intent', () => {
    vi.useFakeTimers()
    try {
      render(
        <AgentChatSidebarRail
          {...sidebarProps()}
          onExpand={vi.fn()}
          onCreateDefaultSession={vi.fn()}
          createDisabled={false}
        />,
      )
      const rail = screen.getByRole('button', { name: '显示项目导航' }).parentElement!

      fireEvent.mouseEnter(rail)
      act(() => vi.advanceTimersByTime(119))
      expect(screen.queryByRole('button', { name: '收起 Alpha' })).not.toBeInTheDocument()

      act(() => vi.advanceTimersByTime(1))
      expect(screen.getByRole('button', { name: '收起 Alpha' })).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })
})
