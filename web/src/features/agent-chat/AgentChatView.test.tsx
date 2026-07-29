import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { createAgentChatSession, getAgentChatHistory, getAgentChatProjects, type AgentChatProject } from './api'
import { persistWorkbenchState, readStoredWorkbenchState } from './tab-state'
import { closeTerminalSession, getTerminalRuntimeStatus, type TerminalSessionInfo } from './terminal/api'
import { AgentChatView } from './AgentChatView'

vi.mock('./api', () => ({
  createAgentChatSession: vi.fn(),
  deleteAgentChatSession: vi.fn(),
  getAgentChatHistory: vi.fn(),
  getAgentChatProjects: vi.fn(),
  renameAgentChatSession: vi.fn(),
}))

vi.mock('./AgentChatConversationTab', () => ({
  AgentChatConversationTab: ({ workspace, sessionId, active, draft, reviewFeedback, onReviewFeedbackOpen }: {
    workspace: string
    sessionId: string
    active: boolean
    draft?: boolean
    reviewFeedback?: Array<{ comments: Array<{ id: string; body: string; target?: { kind: 'workspace_file' | 'lore_item'; id: string } }> }>
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
  AdaptiveSurface: ({ left, children }: {
    left: { enabled?: boolean; content: ReactNode }
    children: ReactNode | ((controls: { isMobile: boolean; openLeft: () => void }) => ReactNode)
  }) => (
    <div>
      {left.enabled === false ? null : left.content}
      <main>{typeof children === 'function' ? children({ isMobile: false, openLeft: vi.fn() }) : children}</main>
    </div>
  ),
}))

function project(path: string, name: string, sessionId: string, title: string): AgentChatProject {
  return {
    path,
    name,
    current: false,
    total: 1,
    sessions: [{
      id: sessionId,
      title,
      active: false,
      running: false,
      message_count: 0,
      created_at: '2026-07-26T00:00:00Z',
      updated_at: '2026-07-26T00:00:00Z',
    }],
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
    vi.mocked(getAgentChatProjects).mockReset().mockResolvedValue([
      project('/books/a', 'Project A', 'session-a', 'Chat A'),
      project('/books/b', 'Project B', 'session-b', 'Chat B'),
    ])
    vi.mocked(createAgentChatSession).mockReset()
    vi.mocked(getAgentChatHistory).mockReset().mockResolvedValue({ items: [], total: 0, offset: 0, has_more: false })
    vi.mocked(closeTerminalSession).mockReset().mockResolvedValue()
    vi.mocked(getTerminalRuntimeStatus).mockReset().mockResolvedValue({
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

  it('keeps a separate tab set and mounted conversation for every selected project', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectPath: '/books/a',
      projects: {
        '/books/a': {
          tabs: [{ kind: 'agent', id: 'tab-a', workspace: '/books/a', sessionId: 'session-a' }],
          activeTabIds: { primary: 'tab-a', secondary: null },
          focusedGroup: 'primary',
        },
        '/books/b': {
          tabs: [{ kind: 'agent', id: 'tab-b', workspace: '/books/b', sessionId: 'session-b' }],
          activeTabIds: { primary: 'tab-b', secondary: null },
          focusedGroup: 'primary',
        },
      },
    })
    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={() => null}
      />,
    )

    await user.click(await screen.findByRole('button', { name: /^Chat A/ }))
    expect(within(screen.getAllByRole('tablist')[0]).getByRole('tab', { name: /Chat A/ })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: /^Chat A/ })).toHaveAttribute('aria-current', 'page')

    await user.click(screen.getByRole('button', { name: /Chat B/ }))
    expect(within(screen.getAllByRole('tablist')[0]).getByRole('tab', { name: /Chat B/ })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('active')

    await user.click(screen.getByTitle('/books/a'))
    expect(within(screen.getAllByRole('tablist')[0]).getByRole('tab', { name: /Chat A/ })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('hidden')

    await waitFor(() => {
      const stored = readStoredWorkbenchState()
      expect(stored.activeProjectPath).toBe('/books/a')
      expect(stored.projects['/books/a'].tabs).toHaveLength(1)
      expect(stored.projects['/books/b'].tabs).toHaveLength(1)
    })
  })

  it('keeps a detached running conversation in the activity list after its tab closes', async () => {
    const user = userEvent.setup()
    const runningProject = project('/books/a', 'Project A', 'session-a', 'Chat A')
    runningProject.sessions[0].running = true
    vi.mocked(getAgentChatProjects).mockResolvedValue([runningProject])

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={() => null}
      />,
    )

    await user.click(await screen.findByRole('button', { name: /Chat A.*运行中/ }))
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')

    await user.click(await screen.findByRole('button', { name: '关闭 Chat A' }))
    expect(screen.queryByTestId('conversation:/books/a:session-a')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Chat A.*运行中/ })).toBeInTheDocument()
  })

  it('opens one local draft without creating a backend conversation', async () => {
    const user = userEvent.setup()
    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={() => null}
      />,
    )

    const createButton = await screen.findByRole('button', { name: '在 Project A 中新建对话' })
    await user.click(createButton)
    await user.click(createButton)

    expect(createAgentChatSession).not.toHaveBeenCalled()
    expect(screen.getAllByTestId('draft-conversation')).toHaveLength(1)
  })

  it('uses the full split separator as one visible resize target', async () => {
    persistWorkbenchState({
      activeProjectPath: '/books/a',
      projects: {
        '/books/a': {
          tabs: [
            { kind: 'agent', id: 'primary-tab', workspace: '/books/a', group: 'primary', sessionId: 'session-a' },
            { kind: 'agent', id: 'secondary-tab', workspace: '/books/a', group: 'secondary', sessionId: 'session-secondary' },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: 'secondary-tab' },
          focusedGroup: 'secondary',
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={() => null}
      />,
    )

    const separator = await screen.findByRole('separator', { name: '调整分栏宽度' })
    expect(separator).toHaveClass('nova-resize-handle', 'nova-resize-divider', 'nova-resize-divider-vertical', 'w-2')
  })

  it('releases only detached terminal sessions that no persisted tab owns', async () => {
    persistWorkbenchState({
      activeProjectPath: '/books/a',
      projects: {
        '/books/a': {
          tabs: [
            { kind: 'terminal', id: 'owned-by-tab', workspace: '/books/a', profileId: 'shell', title: '' },
            {
              kind: 'terminal',
              id: 'legacy-tab',
              workspace: '/books/a',
              profileId: 'shell',
              title: '',
              terminalSessionId: 'legacy-session',
            },
          ],
          activeTabIds: { primary: 'owned-by-tab', secondary: null },
          focusedGroup: 'primary',
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

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={() => null}
      />,
    )

    await waitFor(() => expect(closeTerminalSession).toHaveBeenCalledWith('orphan-session'))
    expect(closeTerminalSession).toHaveBeenCalledTimes(1)
  })

  it('opens a document comment in the matching shared project page beside its conversation', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectPath: '/books/a',
      projects: {
        '/books/a': {
          tabs: [{ kind: 'agent', id: 'agent-tab', workspace: '/books/a', group: 'primary', sessionId: 'session-a' }],
          activeTabIds: { primary: 'agent-tab', secondary: null },
          focusedGroup: 'primary',
        },
      },
    })
    const feedback = {
      source: 'document' as const,
      reviewThreadId: 'document-thread',
      comments: [{ id: 'lore-comment', body: '补足人物动机', target: { kind: 'lore_item' as const, id: 'lin-chuan' } }],
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

    await user.click(await screen.findByRole('button', { name: 'open pending document feedback' }))
    await waitFor(() => expect(screen.getByTestId('review-target-page')).toHaveTextContent('lore|lin-chuan|lore-comment|1'))
    expect(screen.getByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'open pending document feedback' }))
    await waitFor(() => expect(screen.getByTestId('review-target-page')).toHaveTextContent('lore|lin-chuan|lore-comment|2'))
  })
})
