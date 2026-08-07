import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { prefetchProjectChangeReviewThread, useProjectChangeGroup, useProjectChangeGroups, useProjectChangeReviewThread } from './use-change-review'

const apiMocks = vi.hoisted(() => ({
  listProjectChangeGroups: vi.fn(),
  getProjectChangeGroup: vi.fn(),
  getProjectChangeReviewThread: vi.fn(),
}))

vi.mock('./api', () => ({
  ...apiMocks,
}))

describe('useProjectChangeGroups', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listProjectChangeGroups.mockResolvedValue([
      { id: 'group-1', created_at: '', review_status: 'pending', apply_state: 'applied', paths: ['chapters/ch01.md'] },
    ])
    apiMocks.getProjectChangeGroup.mockResolvedValue({
      id: 'group-1',
      created_at: '',
      review_status: 'pending',
      apply_state: 'applied',
      change_sets: [{ path: 'chapters/ch01.md' }],
    })
    apiMocks.getProjectChangeReviewThread.mockResolvedValue({
      id: 'thread-1',
      latest_group_id: 'group-1',
      groups: [],
      comments: [],
      files: [{ path: 'chapters/ch01.md' }],
    })
  })

  it('refreshes only active Project queries affected by watcher paths', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <Harness projectId="project-current" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.listProjectChangeGroups).toHaveBeenCalledTimes(1))
    expect(apiMocks.listProjectChangeGroups).toHaveBeenLastCalledWith('project-current', {})

    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { paths: ['chapters/ch01.md'] } }))
    window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: { project_id: 'project-old', paths: ['chapters/ch01.md'] } }))
    await Promise.resolve()
    expect(apiMocks.listProjectChangeGroups).toHaveBeenCalledTimes(1)

    window.dispatchEvent(new CustomEvent('nova:workspace-change', {
      detail: { project_id: 'project-current', source: 'watcher', paths: ['interactive/story/story-1.jsonl'] },
    }))
    await Promise.resolve()
    expect(apiMocks.listProjectChangeGroups).toHaveBeenCalledTimes(1)

    window.dispatchEvent(new CustomEvent('nova:workspace-change', {
      detail: { project_id: 'project-current', source: 'watcher', paths: ['chapters/ch01.md'] },
    }))
    await waitFor(() => expect(apiMocks.listProjectChangeGroups).toHaveBeenCalledTimes(2))
  })

  it('shares one global event invalidation across multiple hook consumers', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    render(
      <QueryClientProvider client={queryClient}>
        <Harness projectId="project-current" />
        <Harness projectId="project-current" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.listProjectChangeGroups).toHaveBeenCalledTimes(1))

    window.dispatchEvent(new CustomEvent('nova:workspace-change', {
      detail: { project_id: 'project-current', change_group_id: 'group-1' },
    }))

    await waitFor(() => expect(apiMocks.listProjectChangeGroups).toHaveBeenCalledTimes(2))
    expect(invalidateSpy).toHaveBeenCalledTimes(1)
  })

  it('loads a review thread and shares the workspace-scoped event subscription', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <ThreadHarness projectId="project-current" threadID="thread-1" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.getProjectChangeReviewThread).toHaveBeenCalledWith('project-current', 'thread-1'))

    window.dispatchEvent(new CustomEvent('nova:workspace-change', {
      detail: { project_id: 'project-current', change_group_id: 'group-1' },
    }))
    await waitFor(() => expect(apiMocks.getProjectChangeReviewThread).toHaveBeenCalledTimes(2))
  })

  it('reuses a prefetched review thread when the review surface mounts', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await prefetchProjectChangeReviewThread(queryClient, 'project-current', 'thread-1')

    render(
      <QueryClientProvider client={queryClient}>
        <ThreadHarness projectId="project-current" threadID="thread-1" />
      </QueryClientProvider>,
    )

    await waitFor(() => expect(apiMocks.getProjectChangeReviewThread).toHaveBeenCalledTimes(1))
    expect(apiMocks.getProjectChangeReviewThread).toHaveBeenCalledWith('project-current', 'thread-1')
  })

  it('loads one historical review group and refreshes it from the shared workspace event', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <GroupHarness projectId="project-current" groupID="group-1" />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(apiMocks.getProjectChangeGroup).toHaveBeenCalledWith('project-current', 'group-1'))

    window.dispatchEvent(new CustomEvent('nova:workspace-change', {
      detail: { project_id: 'project-current', change_group_id: 'group-1' },
    }))
    await waitFor(() => expect(apiMocks.getProjectChangeGroup).toHaveBeenCalledTimes(2))
  })
})

function Harness({ projectId }: { projectId: string }) {
  useProjectChangeGroups(projectId)
  return null
}

function ThreadHarness({ projectId, threadID }: { projectId: string; threadID: string }) {
  useProjectChangeReviewThread(projectId, threadID)
  return null
}

function GroupHarness({ projectId, groupID }: { projectId: string; groupID: string }) {
  useProjectChangeGroup(projectId, groupID)
  return null
}
