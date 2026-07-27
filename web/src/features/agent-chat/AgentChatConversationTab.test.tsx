import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentChat } from '@/hooks/useAgentChat'
import { AgentChatConversationTab } from './AgentChatConversationTab'

const chat = vi.hoisted(() => ({
  send: vi.fn(),
  loadSessions: vi.fn(),
  loadHistory: vi.fn(),
  resumeActiveChat: vi.fn(),
}))

vi.mock('@/hooks/useAgentChat', () => ({ useAgentChat: vi.fn() }))
vi.mock('@/components/Chat/AgentPanel', () => ({
  AgentPanel: ({ onSend, sessionDraft }: { onSend: (message: string, options: { onSubmissionStart: () => void }) => Promise<boolean>; sessionDraft?: boolean }) => (
    <button type="button" onClick={() => void onSend('First useful message', { onSubmissionStart: vi.fn() })}>
      {sessionDraft ? 'send draft' : 'send session'}
    </button>
  ),
}))

describe('AgentChatConversationTab draft lifecycle', () => {
  beforeEach(() => {
    Object.values(chat).forEach((mock) => mock.mockReset())
    chat.loadSessions.mockResolvedValue(undefined)
    chat.loadHistory.mockResolvedValue(undefined)
    chat.resumeActiveChat.mockResolvedValue(undefined)
    chat.send.mockImplementation(async (_message, options) => {
      options?.onSubmissionStart?.()
      return true
    })
    vi.mocked(useAgentChat).mockReturnValue({
      ...chat,
      messages: [], sessions: [], activeSessionId: '', isStreaming: false,
      runtimeProjection: null, abortPending: false, commandSubmitting: false,
      queueActionPendingCommandID: '', activityContent: '', references: [], loreReferences: [],
      styleScenes: [], textSelections: [], planMode: false, hasEarlierMessages: false,
      isLoadingEarlierHistory: false, createChatSession: vi.fn(), switchChatSession: vi.fn(),
      renameChatSession: vi.fn(), deleteChatSession: vi.fn(), loadEarlierHistory: vi.fn(),
      analyzeContext: vi.fn(), stop: vi.fn(), steerQueuedCommand: vi.fn(), deleteQueuedCommand: vi.fn(),
      editQueuedCommand: vi.fn(), removeReference: vi.fn(), addLoreReference: vi.fn(),
      removeLoreReference: vi.fn(), addStyleScene: vi.fn(), removeStyleScene: vi.fn(),
      removeTextSelection: vi.fn(), setPlanMode: vi.fn(), togglePlanMode: vi.fn(),
      submitPlanQuestion: vi.fn(), approveProposedPlan: vi.fn(), exitPlanMode: vi.fn(),
    } as never)
  })

  it('skips backend reads until the first submission commits the draft', async () => {
    const user = userEvent.setup()
    const onDraftCommitted = vi.fn()
    const props = {
      workspace: '/books/a', sessionId: 's-local-draft', active: true, draft: true,
      composerSettings: {} as never, tellers: [], imagePresets: [], onDraftCommitted,
    }
    const { rerender } = render(<AgentChatConversationTab {...props} />)

    expect(chat.loadHistory).not.toHaveBeenCalled()
    expect(chat.resumeActiveChat).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'send draft' }))
    expect(onDraftCommitted).toHaveBeenCalledWith('First useful message')

    rerender(<AgentChatConversationTab {...props} draft={false} />)
    expect(chat.loadHistory).not.toHaveBeenCalled()
    expect(chat.resumeActiveChat).not.toHaveBeenCalled()
  })
})
