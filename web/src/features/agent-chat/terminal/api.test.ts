import { beforeEach, describe, expect, it, vi } from 'vitest'
import { requestJSON } from '@/lib/api-client/client'
import { createTerminalSession } from './api'

vi.mock('@/lib/api-client/client', () => ({
  jsonHeaders: {},
  requestJSON: vi.fn(),
}))

describe('AgentChat terminal API', () => {
  beforeEach(() => vi.mocked(requestJSON).mockReset())

  it('takes Project identity from the route and sends no workspace authority', async () => {
    vi.mocked(requestJSON).mockResolvedValue({ id: 'terminal-one' })

    await createTerminalSession('project-one', {
      owner_tab_id: 'tab-one',
      profile_id: 'shell',
      cols: 100,
      rows: 30,
    })

    expect(requestJSON).toHaveBeenCalledWith('/api/projects/project-one/terminal/sessions', {
      method: 'POST',
      headers: {},
      body: JSON.stringify({ owner_tab_id: 'tab-one', profile_id: 'shell', cols: 100, rows: 30 }),
    })
  })
})
