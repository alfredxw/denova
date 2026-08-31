import { describe, expect, it } from 'vitest'
import { agentChatPageIdsForProjectType } from './types'

describe('agentChatPageIdsForProjectType', () => {
  it('exposes version management only for the Agents Project', () => {
    expect(agentChatPageIdsForProjectType('agents')).toEqual(['versions'])
    expect(agentChatPageIdsForProjectType('book')).toEqual(['reader', 'lore'])
    expect(agentChatPageIdsForProjectType('general')).toEqual([])
  })
})
