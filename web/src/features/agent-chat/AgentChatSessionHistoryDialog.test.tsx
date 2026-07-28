import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { getAgentChatHistory, type AgentChatHistoryItem } from './api'
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
          onOpenChange={onOpenChange}
          onOpenSession={onOpenSession}
          onRenameSession={vi.fn()}
          onDeleteSession={vi.fn()}
        />
      </TooltipProvider>,
    )

    await user.type(screen.getByRole('textbox', { name: '搜索对话历史' }), 'plot')
    await waitFor(() => expect(getAgentChatHistory).toHaveBeenLastCalledWith(expect.objectContaining({ query: 'plot' })))
    await user.click(await screen.findByRole('button', { name: '打开 Historical chat' }))
    expect(onOpenSession).toHaveBeenCalledWith(historyItem)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
