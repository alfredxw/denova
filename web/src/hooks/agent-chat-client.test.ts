import { beforeEach, describe, expect, it, vi } from 'vitest'
import { requestJSON } from '@/lib/api-client/client'
import { createProjectAgentChatClient } from './agent-chat-client'

vi.mock('@/lib/api-client/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api-client/client')>()
  return { ...actual, requestJSON: vi.fn() }
})

describe('project AgentChat client', () => {
  beforeEach(() => {
    vi.mocked(requestJSON).mockReset()
  })

  it('binds both new turns and reconnects to one immutable project conversation', () => {
    const client = createProjectAgentChatClient('/books/alpha', 'session-a')

    expect(client.fixedSessionId).toBe('session-a')
    expect(client.transportOptions).toEqual({
      api: '/api/agent-chat/chat',
      streamApi: '/api/agent-chat/chat/stream',
      scope: { workspace: '/books/alpha', session_id: 'session-a' },
    })
  })

  it('echoes the exact binding on commands, history and Ask resolutions', async () => {
    vi.mocked(requestJSON)
      .mockResolvedValueOnce({ command_id: 'control', operation_id: 'operation', cursor: 3 })
      .mockResolvedValueOnce({ messages: [], page: { next_before: '4', has_more: true, total: 9 } })
      .mockResolvedValueOnce({ schema: 'ask.result.v1', id: 'ask-a', status: 'cancelled' })
    const client = createProjectAgentChatClient('/books/alpha', 'session-a')

    await client.submitChatCommand('abort', 'control', 'operation', undefined, 'user_requested')
    await client.getMessagesPage(undefined, { limit: 5, before: '7' })
    await client.cancelSessionAsk('foreign-session', 'ask-a')

    expect(requestJSON).toHaveBeenNthCalledWith(1, '/api/agent-chat/chat/commands', expect.objectContaining({
      body: JSON.stringify({
        workspace: '/books/alpha',
        session_id: 'session-a',
        type: 'abort',
        command_id: 'control',
        target_operation_id: 'operation',
        reason: 'user_requested',
      }),
    }))
    expect(requestJSON).toHaveBeenNthCalledWith(
      2,
      '/api/agent-chat/session/messages?workspace=%2Fbooks%2Falpha&session_id=session-a&limit=5&before=7',
    )
    expect(requestJSON).toHaveBeenNthCalledWith(3, '/api/agent-chat/session/asks/ask-a/cancel', expect.objectContaining({
      body: JSON.stringify({
        workspace: '/books/alpha',
        session_id: 'session-a',
        reason: 'user_cancelled',
      }),
    }))
  })
})
