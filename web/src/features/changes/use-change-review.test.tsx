import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useWorkspaceChangeGroup, useWorkspaceChangeGroups, useWorkspaceChangeReviewThread } from './use-change-review'

const apiMocks = vi.hoisted(() => ({
  listWorkspaceChangeGroups: vi.fn(),
  getWorkspaceChangeGroup: vi.fn(),
  getWorkspaceChangeReviewThread: vi.fn(),
}))

vi.mock('./api', () => ({
  ...apiMocks,
}))

describe('useWorkspaceChangeGroups', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listWorkspaceChangeGroups.mockResolvedValue([])
    apiMocks.getWorkspaceChangeGroup.mockResolvedValue({ id: 'group-1', created_at: '', review_status: 'pending', apply_state: 'applied', change_sets: [] })
    apiMocks.getWorkspaceChangeReviewThread.mockResolvedValue({ id: 'thread-1', latest_group_id: '', groups: [], comments: [], files: [] })
  })

  it('refreshes only for events carrying the active canonical workspace', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <Harness workspace="/books/current" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.listWorkspaceChangeGroups).toHaveBeenCalledTimes(1))
    expect(apiMocks.listWorkspaceChangeGroups).toHaveBeenLastCalledWith('/books/current', {})

    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { paths: ['chapters/ch01.md'] } }))
    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { workspace: '/books/old', paths: ['chapters/ch01.md'] } }))
    await Promise.resolve()
    expect(apiMocks.listWorkspaceChangeGroups).toHaveBeenCalledTimes(1)

    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { workspace: '/books/current', paths: ['chapters/ch01.md'] } }))
    await waitFor(() => expect(apiMocks.listWorkspaceChangeGroups).toHaveBeenCalledTimes(2))
  })

  it('shares one global event invalidation across multiple hook consumers', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    render(
      <QueryClientProvider client={queryClient}>
        <Harness workspace="/books/current" />
        <Harness workspace="/books/current" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.listWorkspaceChangeGroups).toHaveBeenCalledTimes(1))

    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { workspace: '/books/current' } }))

    await waitFor(() => expect(apiMocks.listWorkspaceChangeGroups).toHaveBeenCalledTimes(2))
    expect(invalidateSpy).toHaveBeenCalledTimes(1)
  })

  it('loads a review thread and shares the workspace-scoped event subscription', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <ThreadHarness workspace="/books/current" threadID="thread-1" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.getWorkspaceChangeReviewThread).toHaveBeenCalledWith('/books/current', 'thread-1'))

    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { workspace: '/books/current' } }))
    await waitFor(() => expect(apiMocks.getWorkspaceChangeReviewThread).toHaveBeenCalledTimes(2))
  })

  it('loads one historical review group and refreshes it from the shared workspace event', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <GroupHarness workspace="/books/current" groupID="group-1" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.getWorkspaceChangeGroup).toHaveBeenCalledWith('/books/current', 'group-1'))

    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { workspace: '/books/current' } }))
    await waitFor(() => expect(apiMocks.getWorkspaceChangeGroup).toHaveBeenCalledTimes(2))
  })
})

function Harness({ workspace }: { workspace: string }) {
  useWorkspaceChangeGroups(workspace)
  return null
}

function ThreadHarness({ workspace, threadID }: { workspace: string; threadID: string }) {
  useWorkspaceChangeReviewThread(workspace, threadID)
  return null
}

function GroupHarness({ workspace, groupID }: { workspace: string; groupID: string }) {
  useWorkspaceChangeGroup(workspace, groupID)
  return null
}
