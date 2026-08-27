import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { AgentPanelProps } from '@/components/Chat/AgentPanel'
import {
  createAgentChatSession,
  getAgentChatProjects,
  type AgentChatProject,
} from './api'
import { WritingAgentWorkspace } from './WritingAgentWorkspace'

vi.mock('./api', () => ({
  createAgentChatSession: vi.fn(),
  deleteAgentChatSession: vi.fn(),
  getAgentChatProjects: vi.fn(),
  renameAgentChatSession: vi.fn(),
}))

vi.mock('./AgentChatConversationTab', () => ({
  AgentChatConversationTab: ({ sessionId, active, host, onRunningChange, projectId }: {
    sessionId: string
    active: boolean
    projectId: string
    onRunningChange?: (projectId: string, sessionId: string, running: boolean) => void
    host?: { sessionRailVisible: boolean; onSessionRailVisibleChange: (visible: boolean) => void }
  }) => (
    <div data-testid={`conversation:${sessionId}`}>
      {active ? 'active' : 'hidden'}
      <button type="button" onClick={() => onRunningChange?.(projectId, sessionId, true)}>
        mark {sessionId} running
      </button>
      {active && host ? (
        <button type="button" onClick={() => host.onSessionRailVisibleChange(!host.sessionRailVisible)}>
          {host.sessionRailVisible ? '隐藏快捷会话栏' : '显示快捷会话栏'}
        </button>
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

function renderWorkspace(overrides: Partial<AgentPanelProps> = {}) {
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
  } as unknown as AgentPanelProps

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
    await user.click(screen.getByRole('button', { name: '切换到会话 Character notes' }))

    expect(screen.getByTestId('conversation:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:session-b')).toHaveTextContent('active')
    expect(legacySwitch).not.toHaveBeenCalled()
  })

  it('can create and enter another conversation while the current one is running', async () => {
    const user = userEvent.setup()
    vi.mocked(createAgentChatSession).mockResolvedValue({
      id: 'session-c',
      title: 'New branch',
      created_at: '2026-08-26T11:00:00Z',
      updated_at: '2026-08-26T11:00:00Z',
      message_count: 0,
      running: false,
      active: false,
    })
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: '新建会话' }))

    await waitFor(() => expect(createAgentChatSession).toHaveBeenCalledWith('book-a'))
    expect(screen.getByTestId('conversation:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:session-c')).toHaveTextContent('active')
  })

  it('persists the optional quick-session rail preference', async () => {
    const user = userEvent.setup()
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: '隐藏快捷会话栏' }))

    expect(window.localStorage.getItem('nova.writingAgent.sessionRailVisible.v1')).toBe('false')
    expect(screen.getByRole('button', { name: '显示快捷会话栏' })).toBeInTheDocument()
  })

  it('projects independent running state for multiple conversations in the same Book', async () => {
    const user = userEvent.setup()
    renderWorkspace()

    await user.click(await screen.findByRole('button', { name: '切换到会话 Character notes' }))
    await user.click(screen.getByRole('button', { name: 'mark session-b running' }))

    expect(screen.getByRole('button', { name: '切换到会话 Running draft' })).toHaveAttribute('data-running', 'true')
    expect(screen.getByRole('button', { name: '切换到会话 Character notes' })).toHaveAttribute('data-running', 'true')
  })

  it('shows a recoverable empty state when a Book has no conversations yet', async () => {
    const empty = project()
    empty.total = 0
    empty.sessions = []
    vi.mocked(getAgentChatProjects).mockResolvedValue([empty])

    renderWorkspace()

    expect(await screen.findByText('暂无会话')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '新建会话' }).length).toBeGreaterThan(0)
  })
})
