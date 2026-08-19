import { describe, expect, it } from 'vitest'
import type { Snapshot, TurnEvent } from '../../types'
import { branchCreationSourceFromTurn, plotNodesFromSnapshot } from './model'

describe('interactive branching model', () => {
  it('uses narrative text instead of model-only continuation text in player-facing labels', () => {
    const turn = {
      id: 'turn-goal',
      branch_id: 'main',
      parent_id: null,
      ts: '2026-08-20T00:00:00Z',
      user: 'Continue implementing and verify the goal.',
      user_context_only: true,
      narrative: 'The hidden door opens onto a moonlit courtyard.',
    } as TurnEvent

    const branchSource = branchCreationSourceFromTurn(turn, 'Turn')
    const plotNode = plotNodesFromSnapshot({ turns: [turn] } as Snapshot, () => 'Turn')[0]
    expect(branchSource.title).toContain('The hidden door')
    expect(plotNode?.title).toContain('The hidden door')
    expect(branchSource.title).not.toContain('Continue implementing')
    expect(plotNode?.title).not.toContain('Continue implementing')
  })
})
