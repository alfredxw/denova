import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAgentChatSession, getAgentChatProjects, type AgentChatProject } from './api'
import { persistWorkbenchState, readStoredWorkbenchState } from './tab-state'
import { closeTerminalSession, getTerminalRuntimeStatus, type TerminalSessionInfo } from './terminal/api'
import { AgentChatView } from './AgentChatView'

vi.mock('./api', () => ({
  createAgentChatSession: vi.fn(),
  deleteAgentChatSession: vi.fn(),
  getAgentChatProjects: vi.fn(),
  renameAgentChatSession: vi.fn(),
}))

vi.mock('./AgentChatConversationTab', () => ({
  AgentChatConversationTab: ({ workspace, sessionId, active, draft }: {
    workspace: string
    sessionId: string
    active: boolean
    draft?: boolean
  }) => <div data-testid={draft ? 'draft-conversation' : `conversation:${workspace}:${sessionId}`}>{active ? 'active' : 'hidden'}</div>,
}))

vi.mock('./terminal/api', () => ({
  closeTerminalSession: vi.fn(),
  getTerminalRuntimeStatus: vi.fn(),
}))

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
    vi.mocked(closeTerminalSession).mockReset().mockResolvedValue()
    vi.mocked(getTerminalRuntimeStatus).mockReset().mockResolvedValue({
      enabled: true,
      shell: '/bin/sh',
      default_cwd: '/books/a',
      max_sessions: 8,
      scrollback_kb: 256,
      sessions: [],
    })
  })

  it('keeps a separate tab set and mounted conversation for every selected project', async () => {
    const user = userEvent.setup()
    render(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={() => null}
      />,
    )

    await user.click(await screen.findByRole('button', { name: 'Chat A' }))
    expect(within(screen.getAllByRole('tablist')[0]).getByRole('tab', { name: /Chat A/ })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: 'Chat A' }).parentElement).not.toHaveClass('bg-[var(--nova-active)]')

    await user.click(screen.getByRole('button', { name: 'Project B' }))
    await user.click(screen.getByRole('button', { name: 'Chat B' }))
    expect(within(screen.getAllByRole('tablist')[0]).getByRole('tab', { name: /Chat B/ })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('active')

    await user.click(screen.getByRole('button', { name: 'Project A' }))
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

  it('opens one local draft without creating a backend conversation', async () => {
    const user = userEvent.setup()
    render(
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

    render(
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

    render(
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
})
