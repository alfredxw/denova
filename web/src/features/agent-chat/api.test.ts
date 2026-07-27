import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getAgentChatProjects, type AgentChatProject } from './api'

const apiClientMocks = vi.hoisted(() => ({ requestJSON: vi.fn() }))

vi.mock('@/lib/api-client/client', () => ({
  jsonHeaders: {},
  requestJSON: apiClientMocks.requestJSON,
}))

describe('AgentChat project API request coalescing', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('merges Strict Mode startup reads and permits a later refresh', async () => {
    const project = {
      path: '/books/one', name: 'One', current: false, total: 1,
      sessions: [],
    } satisfies AgentChatProject
    apiClientMocks.requestJSON.mockResolvedValueOnce({ projects: [project] })

    const first = getAgentChatProjects()
    const replayed = getAgentChatProjects()

    expect(replayed).toBe(first)
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(1)
    await expect(Promise.all([first, replayed])).resolves.toEqual([[project], [project]])

    apiClientMocks.requestJSON.mockResolvedValueOnce({ projects: [] })
    await expect(getAgentChatProjects()).resolves.toEqual([])
    expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(2)
  })
})
