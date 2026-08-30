import { describe, expect, it } from 'vitest'
import { createProjectAgentChatClient } from './agent-chat-client'

describe('createProjectAgentChatClient', () => {
  it('binds the configuration channel into the immutable transport scope', () => {
    const client = createProjectAgentChatClient('project-a', 'session-a', 'configuration')

    expect(client.transportOptions?.scope).toEqual({
      session_id: 'session-a',
      channel: 'configuration',
    })
  })

  it('preserves the legacy ordinary Agent transport shape', () => {
    const client = createProjectAgentChatClient('project-a', 'session-a')

    expect(client.transportOptions?.scope).toEqual({ session_id: 'session-a' })
  })
})
