import { createElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChangeSummaryCard, canUndoAgentChange, summarizeGroupFiles } from './AgentChangeSummaryCard'
import type { WorkspaceChangeGroup, WorkspaceChangeGroupSummary } from '../types'

const apiMocks = vi.hoisted(() => ({
  getWorkspaceChangeGroup: vi.fn(),
  getWorkspaceChangeReviewThread: vi.fn(),
  listWorkspaceChangeGroups: vi.fn(),
  undoWorkspaceChangeGroup: vi.fn(),
}))
const preloadReviewDiffEditorMock = vi.hoisted(() => vi.fn())

vi.mock('../api', () => apiMocks)
vi.mock('../review/review-editor-loader', () => ({
  preloadReviewDiffEditor: preloadReviewDiffEditorMock,
}))

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.getWorkspaceChangeGroup.mockResolvedValue({
    id: 'group-1',
    created_at: '2026-07-16T00:00:00Z',
    review_status: 'pending',
    apply_state: 'applied',
    change_sets: [],
  })
  apiMocks.getWorkspaceChangeReviewThread.mockResolvedValue({
    id: 'thread-1',
    latest_group_id: 'group-1',
    groups: [],
    comments: [],
    files: [],
  })
  preloadReviewDiffEditorMock.mockResolvedValue(undefined)
})

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

describe('AgentChangeSummaryCard review preload', () => {
  it('warms the newest review thread and editor as soon as its card mounts', async () => {
    renderSummaryCard(true)

    await waitFor(() => expect(apiMocks.getWorkspaceChangeReviewThread).toHaveBeenCalledWith('/book', 'thread-1'))
    expect(preloadReviewDiffEditorMock).toHaveBeenCalledTimes(1)
  })

  it('waits for user intent before warming an older review card', async () => {
    const { container } = renderSummaryCard(false)
    await waitFor(() => expect(apiMocks.getWorkspaceChangeGroup).toHaveBeenCalled())
    expect(apiMocks.getWorkspaceChangeReviewThread).not.toHaveBeenCalled()
    expect(preloadReviewDiffEditorMock).not.toHaveBeenCalled()

    fireEvent.pointerEnter(container.querySelector('[data-change-summary-card="group-1"]')!)

    await waitFor(() => expect(apiMocks.getWorkspaceChangeReviewThread).toHaveBeenCalledWith('/book', 'thread-1'))
    expect(preloadReviewDiffEditorMock).toHaveBeenCalledTimes(1)
  })
})

function renderSummaryCard(eagerPreload: boolean) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const summary = {
    id: 'group-1',
    review_thread_id: 'thread-1',
    run_id: 'run-1',
    created_at: '2026-07-16T00:00:00Z',
    review_status: 'pending',
    apply_state: 'applied',
    change_set_count: 1,
    paths: ['draft.md'],
  } satisfies WorkspaceChangeGroupSummary
  return render(createElement(
    QueryClientProvider,
    { client: queryClient },
    createElement(AgentChangeSummaryCard, {
      workspace: '/book',
      summary,
      eagerPreload,
      onReview: vi.fn(),
    }),
  ))
}

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
