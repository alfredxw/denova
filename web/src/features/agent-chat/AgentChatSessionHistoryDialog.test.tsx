import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { getAgentChatHistory, type AgentChatHistoryItem, type AgentChatProject } from './api'
import { AgentChatSessionHistoryDialog } from './AgentChatSessionHistoryDialog'

vi.mock('./api', async (importOriginal) => ({
  ...await importOriginal<typeof import('./api')>(),
  getAgentChatHistory: vi.fn(),
}))

const historyItem: AgentChatHistoryItem = {
  workspace: '/books/alpha',
  project_name: 'Alpha',
  session: {
    id: 'historical',
    title: 'Historical chat',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-02-01T00:00:00Z',
    message_count: 12,
    running: false,
    active: false,
  },
}

const otherProjectItem: AgentChatHistoryItem = {
  ...historyItem,
  workspace: '/books/beta',
  project_name: 'Beta',
  session: { ...historyItem.session, id: 'beta-history', title: 'Beta chat' },
}

const projects: AgentChatProject[] = [
  { path: otherProjectItem.workspace, name: 'Beta', current: false, total: 1, sessions: [] },
  { path: historyItem.workspace, name: 'Alpha', current: true, total: 1, sessions: [] },
]

describe('AgentChatSessionHistoryDialog', () => {
  beforeEach(() => {
    vi.mocked(getAgentChatHistory).mockReset().mockResolvedValue({
      items: [historyItem], total: 1, offset: 0, has_more: false,
    })
  })

  it('searches durable history and opens the selected conversation', async () => {
    const user = userEvent.setup()
    const onOpenSession = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <TooltipProvider>
        <AgentChatSessionHistoryDialog
          open
          projects={projects}
          currentProjectPath={historyItem.workspace}
          onOpenChange={onOpenChange}
          onOpenSession={onOpenSession}
          onRenameSession={vi.fn()}
          onDeleteSession={vi.fn()}
        />
      </TooltipProvider>,
    )

    await user.type(screen.getByRole('textbox', { name: '搜索对话历史' }), 'plot')
    await waitFor(() => expect(getAgentChatHistory).toHaveBeenLastCalledWith(expect.objectContaining({
      query: 'plot', workspace: historyItem.workspace,
    })))
    await user.click(await screen.findByRole('button', { name: '打开 Historical chat' }))
    expect(onOpenSession).toHaveBeenCalledWith(historyItem)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('uses a collapsible current-project-first master pane to switch session lists', async () => {
    const user = userEvent.setup()
    vi.mocked(getAgentChatHistory).mockImplementation(async (options = {}) => ({
      items: options.workspace === otherProjectItem.workspace ? [otherProjectItem] : [historyItem],
      total: 1,
      offset: 0,
      has_more: false,
    }))

    render(
      <TooltipProvider>
        <AgentChatSessionHistoryDialog
          open
          projects={projects}
          currentProjectPath={historyItem.workspace}
          onOpenChange={vi.fn()}
          onOpenSession={vi.fn()}
          onRenameSession={vi.fn()}
          onDeleteSession={vi.fn()}
        />
      </TooltipProvider>,
    )

    await screen.findByRole('button', { name: '打开 Historical chat' })
    const projectNav = screen.getByRole('navigation', { name: '项目' })
    const projectButtons = within(projectNav).getAllByRole('button')
    expect(projectButtons.map((button) => button.getAttribute('aria-label'))).toEqual([
      'Alpha，1 个对话', 'Beta，1 个对话',
    ])
    expect(projectButtons[0]).toHaveAttribute('aria-current', 'true')
    expect(screen.getByText('当前项目')).toBeInTheDocument()

    await user.click(within(projectNav).getByRole('button', { name: 'Beta，1 个对话' }))
    await waitFor(() => expect(getAgentChatHistory).toHaveBeenLastCalledWith(expect.objectContaining({
      workspace: otherProjectItem.workspace,
    })))
    expect(await screen.findByRole('button', { name: '打开 Beta chat' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '收起项目列表' }))
    expect(screen.queryByRole('navigation', { name: '项目' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开项目列表' }))
    expect(screen.getByRole('navigation', { name: '项目' })).toBeInTheDocument()
  })
})
