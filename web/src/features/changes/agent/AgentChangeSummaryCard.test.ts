import { describe, expect, it } from 'vitest'
import { canUndoAgentChange, summarizeGroupFiles } from './AgentChangeSummaryCard'
import type { WorkspaceChangeGroup, WorkspaceChangeGroupSummary } from '../types'

describe('summarizeGroupFiles', () => {
  it('uses the first before and last after for repeated edits in one run', () => {
    const group = {
      id: 'group-1',
      created_at: '2026-07-16T00:00:00Z',
      review_status: 'pending',
      apply_state: 'applied',
      change_sets: [
        changeSet('change-1', 1, 'draft.md', 'one\ntwo\n', 'one\nTWO\n'),
        changeSet('change-2', 2, 'draft.md', 'one\nTWO\n', 'one\nTWO\nthree\n'),
      ],
    } as WorkspaceChangeGroup

    expect(summarizeGroupFiles(group)).toEqual([{ path: 'draft.md', additions: 2, deletions: 1 }])
  })

  it('ignores housekeeping change sets', () => {
    const group = {
      id: 'group-1',
      created_at: '2026-07-16T00:00:00Z',
      review_status: 'pending',
      apply_state: 'applied',
      change_sets: [
        changeSet('change-1', 1, 'draft.md', 'before\n', 'after\n'),
        { ...changeSet('undo-1', 2, 'draft.md', 'after\n', 'before\n'), origin: 'undo' },
      ],
    } as WorkspaceChangeGroup

    expect(summarizeGroupFiles(group)).toEqual([{ path: 'draft.md', additions: 1, deletions: 1 }])
  })
})

describe('canUndoAgentChange', () => {
  it('blocks conversation-card undo while an Agent run is active', () => {
    const summary = { can_undo: true } as WorkspaceChangeGroupSummary
    expect(canUndoAgentChange(summary, true)).toBe(false)
    expect(canUndoAgentChange(summary, false)).toBe(true)
  })
})

function changeSet(id: string, sequence: number, path: string, before: string, after: string) {
  return {
    id,
    sequence,
    group_id: 'group-1',
    path,
    before_content: before,
    after_content: after,
    review_status: 'pending' as const,
    apply_state: 'applied' as const,
    created_at: '2026-07-16T00:00:00Z',
    origin: 'agent',
  }
}
