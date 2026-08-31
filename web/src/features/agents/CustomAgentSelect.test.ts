import { describe, expect, it } from 'vitest'
import { customAgentsForRuntime } from './CustomAgentSelect'

describe('customAgentsForRuntime', () => {
  it('returns only enabled complete instances for the requested runtime contract', () => {
    const agents = customAgentsForRuntime([
      { id: 'editor', name: 'Editor', contract: 'writing.primary.v1' },
      { id: 'archived', name: 'Archived', contract: 'writing.primary.v1', enabled: false },
      { id: 'game', name: 'Game master', contract: 'game.narrator.v1' },
      { id: 'missing-name', contract: 'writing.primary.v1' },
    ], 'ide')

    expect(agents.map((agent) => agent.id)).toEqual(['editor'])
  })
})
