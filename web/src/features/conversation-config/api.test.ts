import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchConversationConfig, patchConversationConfig } from './api'

const apiClientMocks = vi.hoisted(() => ({ requestJSON: vi.fn() }))

vi.mock('@/lib/api-client', () => ({
  jsonHeaders: { 'Content-Type': 'application/json' },
  requestJSON: apiClientMocks.requestJSON,
}))

describe('conversation config API', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('encodes the stable conversation binding in GET', async () => {
    apiClientMocks.requestJSON.mockResolvedValueOnce({ revision: 1 })

    await fetchConversationConfig({
      mode: 'automation',
      project_id: 'project-1',
      session_id: 'automation-run-1',
      run_id: 'run-1',
    })

    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith(
      '/api/conversation-config?mode=automation&project_id=project-1&session_id=automation-run-1&run_id=run-1',
    )
  })

  it('sends a compare-and-swap partial patch', async () => {
    apiClientMocks.requestJSON.mockResolvedValueOnce({ revision: 8 })
    const binding = { mode: 'writing' as const, session_id: 'session-1' }

    await patchConversationConfig(binding, { approval_mode: 'full_access' }, 7)

    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith('/api/conversation-config', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        binding,
        base_revision: 7,
        changes: { approval_mode: 'full_access' },
      }),
    })
  })
})
