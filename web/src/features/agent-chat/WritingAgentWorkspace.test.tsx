import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  createAgentChatSession,
  getAgentChatProjects,
  type AgentChatProject,
} from './api'
import { WritingAgentWorkspace, type WritingAgentWorkspaceProps } from './WritingAgentWorkspace'

vi.mock('./api', () => ({
  AGENT_CHAT_PROJECT_UPDATED_EVENT: 'nova:agent-chat-project-updated',
  createAgentChatSession: vi.fn(),
  deleteAgentChatSession: vi.fn(),
  getAgentChatProjects: vi.fn(),
  renameAgentChatSession: vi.fn(),
}))

vi.mock('./AgentChatConversationTab', () => ({
  AgentChatConversationTab: ({ sessionId, draft, active, host, onRunningChange, onDraftCommitted, projectId }: {
    sessionId: string
    draft?: boolean
    active: boolean
    projectId: string
    onRunningChange?: (projectId: string, sessionId: string, running: boolean) => void
    onDraftCommitted?: (message: string) => void
    host?: {
      sessionRailVisible: boolean
      onSessionRailVisibleChange: (visible: boolean) => void
      onCreateSession: (title?: string, customAgentId?: string) => void | Promise<void>
    }
  }) => (
    <div
      data-testid={draft ? 'conversation:draft' : `conversation:${sessionId}`}
      data-draft={draft ? 'true' : 'false'}
    >
      {active ? 'active' : 'hidden'}
      <button type="button" onClick={() => onRunningChange?.(projectId, sessionId, true)}>
        mark {sessionId} running
      </button>
      {active && host ? (
        <>
          {draft ? (
            <button type="button" onClick={() => onDraftCommitted?.('Configure this resource')}>commit draft</button>
          ) : null}
          {!host.sessionRailVisible ? (
            <button
              type="button"
              aria-pressed="false"
              onClick={() => host.onSessionRailVisibleChange(true)}
            >
              显示会话侧栏
            </button>
          ) : null}
          <button type="button" onClick={() => void host.onCreateSession()}>新建会话</button>
          <button type="button" onClick={() => void host.onCreateSession(undefined, 'focused-editor')}>切换自定义 Agent</button>
        </>
      ) : null}
    </div>
  ),
}))

function project(): AgentChatProject {
  return {
    id: 'book-a',
    type: 'book',
    path: '/books/a',
    name: 'Book A',
    status: 'available',
    current: true,
    total: 2,
    sessions: [
      {
        id: 'session-a',
        title: 'Running draft',
        created_at: '2026-08-26T10:00:00Z',
        updated_at: '2026-08-26T10:02:00Z',
        message_count: 4,
        running: true,
        active: true,
      },
      {
        id: 'session-b',
        title: 'Character notes',
        created_at: '2026-08-26T09:00:00Z',
        updated_at: '2026-08-26T09:03:00Z',
        message_count: 2,
        running: false,
        active: false,
      },
    ],
  }
}

function renderWorkspace(overrides: Partial<WritingAgentWorkspaceProps> = {}) {
  const props = {
    projectId: 'book-a',
    workspace: '/books/a',
    active: true,
    activeSessionId: 'session-a',
    composerSettings: {},
    tellers: [],
    imagePresets: [],
    references: [],
    loreReferences: [],
    loreReferenceLabels: {},
    loreSuggestions: [],
    styleScenes: [],
    textSelections: [],
    fileSuggestions: [],
    onReferenceRemove: vi.fn(),
    onLoreReferenceRemove: vi.fn(),
    onStyleSceneRemove: vi.fn(),
    onTextSelectionRemove: vi.fn(),
    ...overrides,
  } as unknown as WritingAgentWorkspaceProps

  return render(
    <TooltipProvider delayDuration={0}>
      <WritingAgentWorkspace {...props} />
    </TooltipProvider>,
  )
}

describe('WritingAgentWorkspace', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.mocked(getAgentChatProjects).mockReset().mockResolvedValue([project()])
    vi.mocked(createAgentChatSession).mockReset()
  })

  it('switches sessions without unmounting or stopping a running conversation', async () => {
    const user = userEvent.setup()
    const legacySwitch = vi.fn()
    renderWorkspace({ onSwitchSession: legacySwitch })

    expect(await screen.findByTestId('conversation:session-a')).toHaveTextContent('active')
    await user.click(screen.getByRole('button', { name: '切换到会话 Character notes，空闲' }))

    expect(screen.getByTestId('conversation:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:session-b')).toHaveTextContent('active')
    expect(legacySwitch).not.toHaveBeenCalled()
  })

  it('keeps a new Writing conversation local while the current one is running', async () => {
    const user = userEvent.setup()
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: '新建会话' }))

    expect(await screen.findByTestId('conversation:draft')).toHaveTextContent('active')
    expect(createAgentChatSession).not.toHaveBeenCalled()
    expect(screen.getByTestId('conversation:session-a')).toHaveTextContent('hidden')
  })

  it('persists a new conversation immediately when selecting a custom Agent', async () => {
    const user = userEvent.setup()
    vi.mocked(createAgentChatSession).mockResolvedValue({
      id: 'session-c',
      custom_agent_id: 'focused-editor',
      title: 'Focused editor',
      created_at: '2026-08-26T11:00:00Z',
      updated_at: '2026-08-26T11:00:00Z',
      message_count: 0,
      running: false,
      active: false,
    })
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: '切换自定义 Agent' }))

    await waitFor(() => expect(createAgentChatSession).toHaveBeenCalledWith('book-a', '', 'focused-editor'))
    expect(screen.getByTestId('conversation:session-c')).toHaveTextContent('active')
  })

  it('persists the optional quick-session rail preference', async () => {
    const user = userEvent.setup()
    renderWorkspace()

    const rail = await screen.findByRole('navigation', { name: '快捷会话切换' })
    const hideRailButton = await screen.findByRole('button', { name: '隐藏会话侧栏' })
    expect(screen.getAllByRole('button', { name: '隐藏会话侧栏' })).toHaveLength(1)
    expect(hideRailButton.closest('nav')).toBe(rail)
    await user.click(hideRailButton)

    expect(window.localStorage.getItem('nova.writingAgent.sessionRailVisible.v1')).toBe('false')
    expect(screen.queryByRole('navigation', { name: '快捷会话切换' })).not.toBeInTheDocument()
    const showRailButton = screen.getByRole('button', { name: '显示会话侧栏' })
    expect(showRailButton).toHaveAttribute('aria-pressed', 'false')
    expect(showRailButton.closest('nav')).toBeNull()
    await user.click(showRailButton)

    expect(window.localStorage.getItem('nova.writingAgent.sessionRailVisible.v1')).toBe('true')
    const reopenedRail = screen.getByRole('navigation', { name: '快捷会话切换' })
    const reopenedHideButton = screen.getByRole('button', { name: '隐藏会话侧栏' })
    expect(reopenedHideButton).toHaveAttribute('aria-pressed', 'true')
    expect(reopenedHideButton.closest('nav')).toBe(reopenedRail)
  })

  it('projects independent running state for multiple conversations in the same Book', async () => {
    const user = userEvent.setup()
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: '切换到会话 Character notes，空闲' }))
    await user.click(screen.getByRole('button', { name: 'mark session-b running' }))

    expect(screen.getByRole('button', { name: '切换到会话 Running draft，运行中' })).toHaveAttribute('data-running', 'true')
    expect(screen.getByRole('button', { name: '切换到会话 Character notes，运行中' })).toHaveAttribute('data-running', 'true')
  })

  it('opens a local draft when a Book has no conversations yet', async () => {
    const empty = project()
    empty.total = 0
    empty.sessions = []
    vi.mocked(getAgentChatProjects).mockResolvedValue([empty])

    renderWorkspace()

    expect(await screen.findByTestId('conversation:draft')).toHaveAttribute('data-draft', 'true')
    expect(screen.queryByText('暂无会话')).not.toBeInTheDocument()
    expect(createAgentChatSession).not.toHaveBeenCalled()
  })

  it('shares Project conversations while keeping a surface-local active selection', async () => {
    window.localStorage.setItem('nova.writingAgent.activeSession.v1:book-a', 'session-a')
    window.localStorage.setItem('nova.writingAgent.activeSession.v1:configuration:book-a', 'session-b')
    renderWorkspace({
      activeSessionPreferenceScope: 'configuration',
      sessionRailVisible: false,
    })

    expect(await screen.findByTestId('conversation:session-b')).toHaveTextContent('active')
    expect(screen.getByTestId('conversation:session-a')).toHaveTextContent('hidden')
    expect(getAgentChatProjects).toHaveBeenCalledWith()
    expect(window.localStorage.getItem('nova.writingAgent.activeSession.v1:book-a')).toBe('session-a')
    expect(window.localStorage.getItem('nova.writingAgent.activeSession.v1:configuration:book-a')).toBe('session-b')
  })
})
