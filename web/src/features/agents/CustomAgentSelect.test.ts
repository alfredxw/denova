import { describe, expect, it } from 'vitest'
import { customAgentsForBase } from './CustomAgentSelect'

describe('customAgentsForBase', () => {
  it('returns only enabled complete instances for the requested fixed base', () => {
    const agents = customAgentsForBase([
      { id: 'editor', name: 'Editor', base_kind: 'ide' },
      { id: 'archived', name: 'Archived', base_kind: 'ide', enabled: false },
      { id: 'game', name: 'Game master', base_kind: 'interactive_story' },
      { id: 'missing-name', base_kind: 'ide' },
    ], 'ide')

    expect(agents.map((agent) => agent.id)).toEqual(['editor'])
  })
})
