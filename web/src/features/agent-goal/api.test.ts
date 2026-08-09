import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchConversationGoal, mutateConversationGoal } from './api'

const apiClientMocks = vi.hoisted(() => ({ requestJSON: vi.fn() }))

vi.mock('@/lib/api-client', () => ({
  jsonHeaders: { 'Content-Type': 'application/json' },
  requestJSON: apiClientMocks.requestJSON,
}))

describe('conversation goal API', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('loads the goal through the project-scoped conversation binding', async () => {
    apiClientMocks.requestJSON.mockResolvedValueOnce({ goal: null })

    await fetchConversationGoal({ mode: 'agent_chat', project_id: 'project-1', session_id: 'session-1' })

    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith(
      '/api/projects/project-1/conversation-goal?mode=agent_chat&session_id=session-1',
    )
  })

  it('sends goal mutations with the current revision fence', async () => {
    apiClientMocks.requestJSON.mockResolvedValueOnce({ goal: { id: 'goal-1', status: 'active', revision: 8 } })

    await mutateConversationGoal(
      { mode: 'writing', project_id: 'project-1', session_id: 'session-1' },
      'set',
      7,
      'Finish and verify the complete feature',
    )

    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith('/api/projects/project-1/conversation-goal', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        binding: { mode: 'writing', session_id: 'session-1' },
        action: 'set',
        expected_revision: 7,
        objective: 'Finish and verify the complete feature',
      }),
    })
  })
})
