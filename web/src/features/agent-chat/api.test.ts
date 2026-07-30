import { beforeEach, describe, expect, it, vi } from 'vitest'
import { addAgentChatProject, getAgentChatHistory, getAgentChatProjects, selectAgentChatProjectDirectory, type AgentChatProject } from './api'

const apiClientMocks = vi.hoisted(() => ({ requestJSON: vi.fn() }))

vi.mock('@/lib/api-client/client', () => ({
  jsonHeaders: {},
  requestJSON: apiClientMocks.requestJSON,
}))

describe('AgentChat project API request coalescing', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('merges Strict Mode startup reads and permits a later refresh', async () => {
    const project = {
      id: 'project-one',
      type: 'book',
      status: 'available',
      path: '/books/one',
      name: 'One',
      current: false,
      total: 1,
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

describe('AgentChat history API', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('encodes search parameters and normalizes missing items', async () => {
    apiClientMocks.requestJSON.mockResolvedValueOnce({
      total: 0,
      offset: 80,
      has_more: false,
    })

    await expect(
      getAgentChatHistory({
        query: '  plot arc  ',
        projectId: '  project-one  ',
        offset: 80,
        limit: 40,
      }),
    ).resolves.toMatchObject({ items: [] })
    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith('/api/agent-chat/history?query=plot+arc&project_id=project-one&offset=80&limit=40', {
      signal: undefined,
    })
  })
})

describe('AgentChat project folder API', () => {
  beforeEach(() => apiClientMocks.requestJSON.mockReset())

  it('delegates directory selection to the host and creates with path only', async () => {
    apiClientMocks.requestJSON.mockResolvedValueOnce({ path: '/projects/story', canceled: false })
    await expect(selectAgentChatProjectDirectory('/projects/old')).resolves.toEqual({
      path: '/projects/story',
      canceled: false,
    })
    expect(apiClientMocks.requestJSON).toHaveBeenLastCalledWith('/api/host/dialogs/directory', {
      method: 'POST',
      headers: {},
      body: JSON.stringify({ initial_path: '/projects/old' }),
    })

    apiClientMocks.requestJSON.mockResolvedValueOnce({ id: 'project-story' })
    await addAgentChatProject('/projects/story')
    expect(apiClientMocks.requestJSON).toHaveBeenLastCalledWith('/api/agent-chat/projects', {
      method: 'POST',
      headers: {},
      body: JSON.stringify({ path: '/projects/story' }),
    })
  })
})
