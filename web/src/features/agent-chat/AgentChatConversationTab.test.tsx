import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { AgentChatConversationTab, type AgentChatConversationTabProps } from './AgentChatConversationTab'

const chat = vi.hoisted(() => ({
  isStreaming: true,
  isExecutionActive: false,
  loadSessions: vi.fn().mockResolvedValue([]),
  loadHistory: vi.fn().mockResolvedValue(undefined),
  resumeActiveChat: vi.fn().mockResolvedValue(undefined),
  send: vi.fn().mockResolvedValue(true),
}))

vi.mock('@/hooks/useAgentChat', () => ({ useAgentChat: () => chat }))
vi.mock('@/hooks/agent-chat-client', () => ({ createProjectAgentChatClient: () => ({}) }))
vi.mock('@/components/Chat/AgentPanel', () => ({ AgentPanel: () => null }))

afterEach(cleanup)

it('waits for the startup recovery probe before sending an initial instruction exactly once', async () => {
  const consumed = vi.fn()
  const props: AgentChatConversationTabProps = {
    projectId: 'project', projectType: 'general', workspace: '/project', sessionId: 'session', active: true,
    composerSettings: {} as AgentChatConversationTabProps['composerSettings'], tellers: [], imagePresets: [],
    pendingAction: { id: 'initial', message: 'Develop this extension', displayMessage: 'Develop this extension' },
    onPendingActionConsumed: consumed,
  }
  const { rerender } = render(<AgentChatConversationTab {...props} />)
  await waitFor(() => expect(chat.resumeActiveChat).toHaveBeenCalledWith('session'))
  expect(chat.send).not.toHaveBeenCalled()
  expect(consumed).not.toHaveBeenCalled()

  chat.isStreaming = false
  rerender(<AgentChatConversationTab {...props} pendingAction={{ ...props.pendingAction! }} />)
  await waitFor(() => expect(consumed).toHaveBeenCalledWith('initial'))
  expect(chat.send).toHaveBeenCalledWith('Develop this extension', expect.objectContaining({ displayMessage: 'Develop this extension' }))
  rerender(<AgentChatConversationTab {...props} pendingAction={{ ...props.pendingAction! }} />)
  expect(chat.send).toHaveBeenCalledTimes(1)
})
