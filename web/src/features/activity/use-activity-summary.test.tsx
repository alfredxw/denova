import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { queryClient } from '@/lib/query-client'
import { activitySummaryQueryKey, setActivityMessageUnreadCount, useActivitySummary } from './use-activity-summary'

const apiClientMocks = vi.hoisted(() => ({ requestJSON: vi.fn() }))

vi.mock('@/lib/api-client', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api-client')>(),
  requestJSON: apiClientMocks.requestJSON,
}))

describe('useActivitySummary', () => {
  beforeEach(() => {
    queryClient.clear()
    apiClientMocks.requestJSON.mockReset().mockResolvedValue({
      message_unread_count: 5,
      automation_inbox_unread_count: 2,
      automation_running_count: 1,
    })
  })

  it('shares one summary request across StrictMode consumers', async () => {
    render(
      <StrictMode>
        <SummaryConsumer label="first" />
        <SummaryConsumer label="second" />
      </StrictMode>,
    )

    await waitFor(() => expect(apiClientMocks.requestJSON).toHaveBeenCalledTimes(1))
    expect(apiClientMocks.requestJSON).toHaveBeenCalledWith('/api/activity/summary')
    expect(await screen.findByTestId('first')).toHaveTextContent('5')
    expect(screen.getByTestId('second')).toHaveTextContent('5')
  })

  it('updates the cached message count without replacing automation counts', () => {
    queryClient.setQueryData(activitySummaryQueryKey, {
      message_unread_count: 5,
      automation_inbox_unread_count: 2,
      automation_running_count: 1,
    })

    setActivityMessageUnreadCount(3)

    expect(queryClient.getQueryData(activitySummaryQueryKey)).toEqual({
      message_unread_count: 3,
      automation_inbox_unread_count: 2,
      automation_running_count: 1,
    })
  })
})

function SummaryConsumer({ label }: { label: string }) {
  const summary = useActivitySummary().data
  return <span data-testid={label}>{summary?.message_unread_count ?? 'loading'}</span>
}
