import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  addAgentChatProject,
  createAgentChatSession,
  getAgentChatHistory,
  getAgentChatProjects,
  selectAgentChatProjectDirectory,
  type AgentChatProject,
} from './api'
import { persistWorkbenchState, readStoredWorkbenchState } from './tab-state'
import { closeTerminalSession, getTerminalRuntimeStatus, type TerminalSessionInfo } from './terminal/api'
import { AgentChatView } from './AgentChatView'

vi.mock('./api', () => ({
  addAgentChatProject: vi.fn(),
  createAgentChatSession: vi.fn(),
  deleteAgentChatSession: vi.fn(),
  getAgentChatHistory: vi.fn(),
  getAgentChatProjects: vi.fn(),
  renameAgentChatSession: vi.fn(),
  selectAgentChatProjectDirectory: vi.fn(),
}))

vi.mock('./AgentChatConversationTab', () => ({
  AgentChatConversationTab: ({
    workspace,
    sessionId,
    active,
    draft,
    reviewFeedback,
    onReviewFeedbackOpen,
  }: {
    workspace: string
    sessionId: string
    active: boolean
    draft?: boolean
    reviewFeedback?: Array<{
      comments: Array<{
        id: string
        body: string
        target?: { kind: 'workspace_file' | 'lore_item'; id: string }
      }>
    }>
    onReviewFeedbackOpen?: (selection: unknown, comment: unknown) => void
  }) => {
    const selection = reviewFeedback?.[0]
    const comment = selection?.comments[0]
    return (
      <div data-testid={draft ? 'draft-conversation' : `conversation:${workspace}:${sessionId}`}>
        {active ? 'active' : 'hidden'}
        {selection && comment ? (
          <button type="button" onClick={() => onReviewFeedbackOpen?.(selection, comment)}>
            open pending document feedback
          </button>
        ) : null}
      </div>
    )
  },
}))

vi.mock('./terminal/api', () => ({
  closeTerminalSession: vi.fn(),
  getTerminalRuntimeStatus: vi.fn(),
}))

function renderView(ui: ReactNode) {
  return render(<TooltipProvider delayDuration={0}>{ui}</TooltipProvider>)
}

vi.mock('./terminal/TerminalTabView', () => ({
  TerminalTabView: () => <div>terminal</div>,
  terminalTabLabel: () => 'Terminal',
}))

vi.mock('@/components/layout/adaptive-surface', () => ({
  AdaptiveSurface: ({
    left,
    right,
    rightResize,
    children,
  }: {
    left: { enabled?: boolean; content: ReactNode; desktopVisible?: boolean; desktopCollapsedContent?: ReactNode }
    right?: { content: ReactNode; desktopVisible?: boolean }
    rightResize?: { label: string }
    children: ReactNode | ((controls: {
      isMobile: boolean
      openPaneId: string | null
      openLeft: () => void
      openRight: () => void
      closePane: () => void
    }) => ReactNode)
  }) => (
    <div>
      {left.enabled === false ? null : left.desktopVisible === false ? left.desktopCollapsedContent : left.content}
      <main>{typeof children === 'function' ? children({
        isMobile: false,
        openPaneId: null,
        openLeft: vi.fn(),
        openRight: vi.fn(),
        closePane: vi.fn(),
      }) : children}</main>
      {right ? (
        <aside hidden={right.desktopVisible === false}>
          {right.desktopVisible === false || !rightResize ? null : (
            <div
              role="separator"
              aria-label={rightResize.label}
              className="nova-resize-handle nova-resize-divider nova-resize-divider-vertical w-2"
            />
          )}
          {right.content}
        </aside>
      ) : null}
    </div>
  ),
}))

function project(path: string, name: string, sessionId: string, title: string): AgentChatProject {
  return {
    id: `project-${path.split('/').pop()}`,
    type: 'book',
    status: 'available',
    path,
    name,
    current: false,
    total: 1,
    sessions: [
      {
        id: sessionId,
        title,
        active: false,
        running: false,
        message_count: 0,
        created_at: '2026-07-26T00:00:00Z',
        updated_at: '2026-07-26T00:00:00Z',
      },
    ],
  }
}

function terminalSession(id: string, ownerTabId: string | undefined, attached = 0): TerminalSessionInfo {
  return {
    id,
    owner_tab_id: ownerTabId,
    profile_id: 'shell',
    title: 'shell',
    command: '/bin/sh',
    args: [],
    cwd: '/books/a',
    workspace: '/books/a',
    cols: 80,
    rows: 24,
    created_at: '2026-07-26T00:00:00Z',
    attached,
    exited: false,
    exit_code: 0,
    token: `token-${id}`,
  }
}

describe('AgentChatView project workbenches', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.mocked(getAgentChatProjects)
      .mockReset()
      .mockResolvedValue([project('/books/a', 'Project A', 'session-a', 'Chat A'), project('/books/b', 'Project B', 'session-b', 'Chat B')])
    vi.mocked(createAgentChatSession).mockReset()
    vi.mocked(addAgentChatProject).mockReset()
    vi.mocked(selectAgentChatProjectDirectory).mockReset()
    vi.mocked(getAgentChatHistory).mockReset().mockResolvedValue({ items: [], total: 0, offset: 0, has_more: false })
    vi.mocked(closeTerminalSession).mockReset().mockResolvedValue()
    vi.mocked(getTerminalRuntimeStatus)
      .mockReset()
      .mockResolvedValue({
        enabled: true,
        shell: '/bin/sh',
        commands: [
          { id: 'codex', name: 'Codex CLI' },
          { id: 'claude', name: 'Claude Code' },
        ],
        default_cwd: '/books/a',
        max_sessions: 8,
        scrollback_kb: 256,
        sessions: [],
      })
  })

  it('adds a selected folder without asking for a name or Agent type', async () => {
    const user = userEvent.setup()
    const added = project('/projects/story', 'story', '', '')
    added.id = 'project-story'
    added.type = 'general'
    added.total = 0
    added.sessions = []
    vi.mocked(getAgentChatProjects).mockReset().mockResolvedValueOnce([]).mockResolvedValue([added])
    vi.mocked(selectAgentChatProjectDirectory).mockResolvedValue({ path: '/projects/story', canceled: false })
    vi.mocked(addAgentChatProject).mockResolvedValue(added)

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click((await screen.findAllByRole('button', { name: '添加项目' }))[0])
    await waitFor(() => expect(addAgentChatProject).toHaveBeenCalledWith('/projects/story'))
    expect(selectAgentChatProjectDirectory).toHaveBeenCalledWith(undefined)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(await screen.findByTitle('/projects/story')).toBeInTheDocument()
  })

  it('toggles the activity tree into a persistent compact rail', async () => {
    const user = userEvent.setup()
    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click(await screen.findByRole('button', { name: '隐藏活动列表' }))
    expect(screen.getByRole('button', { name: '显示活动列表' })).toBeInTheDocument()
    expect(window.localStorage.getItem('nova.agentchat.sidebarVisible.v1')).toBe('false')

    await user.click(screen.getByRole('button', { name: '显示活动列表' }))
    expect(screen.getByRole('button', { name: '隐藏活动列表' })).toBeInTheDocument()
    expect(window.localStorage.getItem('nova.agentchat.sidebarVisible.v1')).toBe('true')
  })

  it('keeps a separate tab set and mounted conversation for every selected project', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'tab-a',
              projectId: 'project-a',
              workspace: '/books/a',
              sessionId: 'session-a',
            },
          ],
          activeTabIds: { primary: 'tab-a', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
        'project-b': {
          tabs: [
            {
              kind: 'agent',
              id: 'tab-b',
              projectId: 'project-b',
              workspace: '/books/b',
              sessionId: 'session-b',
            },
          ],
          activeTabIds: { primary: 'tab-b', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click(await screen.findByRole('button', { name: /^Chat A/ }))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat A/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: /^Chat A/ })).toHaveAttribute('aria-current', 'page')

    await user.click(screen.getByRole('button', { name: /Chat B/ }))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat B/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('active')

    await user.click(screen.getByTitle('/books/a'))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat A/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('hidden')

    await waitFor(() => {
      const stored = readStoredWorkbenchState()
      expect(stored.activeProjectId).toBe('project-a')
      expect(stored.projects['project-a'].tabs).toHaveLength(1)
      expect(stored.projects['project-b'].tabs).toHaveLength(1)
    })
  })

  it('keeps a detached running conversation in the activity list after its tab closes', async () => {
    const user = userEvent.setup()
    const runningProject = project('/books/a', 'Project A', 'session-a', 'Chat A')
    runningProject.sessions[0].running = true
    vi.mocked(getAgentChatProjects).mockResolvedValue([runningProject])

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click(await screen.findByRole('button', { name: /Chat A.*运行中/ }))
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')

    await user.click(await screen.findByRole('button', { name: '关闭 Chat A' }))
    expect(screen.queryByTestId('conversation:/books/a:session-a')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Chat A.*运行中/ })).toBeInTheDocument()
  })

  it('opens one local draft without creating a backend conversation', async () => {
    const user = userEvent.setup()
    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    const createButton = await screen.findByRole('button', {
      name: '在 Project A 中新建对话',
    })
    await user.click(createButton)
    await user.click(createButton)

    expect(createAgentChatSession).not.toHaveBeenCalled()
    expect(screen.getAllByTestId('draft-conversation')).toHaveLength(1)
  })

  it('uses the full split separator as one visible resize target', async () => {
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'primary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
            {
              kind: 'agent',
              id: 'secondary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'secondary',
              sessionId: 'session-secondary',
            },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: 'secondary-tab' },
          focusedGroup: 'secondary',
          secondaryVisible: true,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    const separator = await screen.findByRole('separator', {
      name: '调整分栏宽度',
    })
    expect(separator).toHaveClass('nova-resize-handle', 'nova-resize-divider', 'nova-resize-divider-vertical', 'w-2')
  })

  it('hides and restores the secondary pane without unmounting its conversation', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'primary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
            {
              kind: 'agent',
              id: 'secondary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'secondary',
              sessionId: 'session-secondary',
            },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: 'secondary-tab' },
          focusedGroup: 'secondary',
          secondaryVisible: true,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    const hideButton = await screen.findByRole('button', { name: '隐藏右侧工作区' })
    expect(hideButton.closest('[data-agent-chat-group]')).toHaveAttribute('data-agent-chat-group', 'secondary')
    await user.click(hideButton)
    expect(screen.queryByRole('separator', { name: '调整分栏宽度' })).not.toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-secondary')).toHaveTextContent('hidden')
    await waitFor(() => expect(readStoredWorkbenchState().projects['project-a'].secondaryVisible).toBe(false))

    const showButton = screen.getByRole('button', { name: '显示右侧工作区' })
    expect(showButton.closest('[data-agent-chat-group]')).toHaveAttribute('data-agent-chat-group', 'primary')
    await user.click(showButton)
    expect(await screen.findByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-secondary')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: '隐藏右侧工作区' }).closest('[data-agent-chat-group]'))
      .toHaveAttribute('data-agent-chat-group', 'secondary')
  })

  it('lets the first secondary-pane click choose what to open there', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'primary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              sessionId: 'session-a',
            },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={(_workspace, pageId) => <div data-testid="secondary-page">{pageId}</div>}
        renderReview={() => null}
      />,
    )

    await user.click(await screen.findByRole('button', { name: '显示右侧工作区' }))
    await user.click(await screen.findByRole('menuitem', { name: '阅读器' }))

    expect(await screen.findByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()
    expect(screen.getByTestId('secondary-page')).toHaveTextContent('reader')
    expect(readStoredWorkbenchState().projects['project-a'].secondaryVisible).toBe(true)
  })

  it('releases only detached terminal sessions that no persisted tab owns', async () => {
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'terminal',
              id: 'owned-by-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              profileId: 'shell',
              title: '',
            },
            {
              kind: 'terminal',
              id: 'legacy-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              profileId: 'shell',
              title: '',
              terminalSessionId: 'legacy-session',
            },
          ],
          activeTabIds: { primary: 'owned-by-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    vi.mocked(getTerminalRuntimeStatus).mockResolvedValue({
      enabled: true,
      shell: '/bin/sh',
      commands: [
        { id: 'codex', name: 'Codex CLI' },
        { id: 'claude', name: 'Claude Code' },
      ],
      default_cwd: '/books/a',
      max_sessions: 8,
      scrollback_kb: 256,
      sessions: [
        terminalSession('owned-session', 'owned-by-tab'),
        terminalSession('legacy-session', undefined),
        terminalSession('orphan-session', 'missing-tab'),
        terminalSession('active-legacy-session', undefined, 1),
      ],
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await waitFor(() => expect(closeTerminalSession).toHaveBeenCalledWith('orphan-session'))
    expect(closeTerminalSession).toHaveBeenCalledTimes(1)
  })

  it('opens a document comment in the matching shared project page beside its conversation', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'agent-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
          ],
          activeTabIds: { primary: 'agent-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    const feedback = {
      source: 'document' as const,
      reviewThreadId: 'document-thread',
      comments: [
        {
          id: 'lore-comment',
          body: '补足人物动机',
          target: { kind: 'lore_item' as const, id: 'lin-chuan' },
        },
      ],
    }

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        documentReviewWorkspace="/books/a"
        documentReviewFeedback={feedback}
        renderPage={(_workspace, pageId, context) => (
          <div data-testid="review-target-page">
            {pageId}|{context.navigationIntent?.target.id || 'none'}|{context.navigationIntent?.commentID || 'none'}|{context.navigationIntent?.nonce || 0}
          </div>
        )}
        renderReview={() => null}
      />,
    )

    await user.click(
      await screen.findByRole('button', {
        name: 'open pending document feedback',
      }),
    )
    await waitFor(() => expect(screen.getByTestId('review-target-page')).toHaveTextContent('lore|lin-chuan|lore-comment|1'))
    expect(screen.getByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'open pending document feedback' }))
    await waitFor(() => expect(screen.getByTestId('review-target-page')).toHaveTextContent('lore|lin-chuan|lore-comment|2'))
  })
})
