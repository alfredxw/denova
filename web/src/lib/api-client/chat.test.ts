import { beforeEach, describe, expect, it, vi } from 'vitest'
import { requestJSON } from './client'
import { getGlobalAgentRunTraces } from './chat'

vi.mock('./client', () => ({
  fetchAPI: vi.fn(),
  jsonHeaders: vi.fn(),
  parseUIMessageStream: vi.fn(),
  readErrorMessage: vi.fn(),
  requestJSON: vi.fn(),
}))

describe('Agent Run trace API', () => {
  beforeEach(() => {
    vi.mocked(requestJSON).mockReset()
    vi.mocked(requestJSON).mockResolvedValue({ runs: [], issues: [] })
  })

  it('asks the bounded global catalog to include the navigation target', async () => {
    await getGlobalAgentRunTraces(25, { projectId: 'project-a', runId: 'run-42' })

    expect(requestJSON).toHaveBeenCalledWith('/api/agent-runs?limit=25&project_id=project-a&run_id=run-42')
  })
})
