import { describe, expect, it } from 'vitest'
import { createProjectAgentChatClient } from './agent-chat-client'

describe('createProjectAgentChatClient', () => {
  it('binds only the immutable Project session identity into transport scope', () => {
    const client = createProjectAgentChatClient('project-a', 'session-a')

    expect(client.transportOptions?.scope).toEqual({
      session_id: 'session-a',
    })
  })
})
