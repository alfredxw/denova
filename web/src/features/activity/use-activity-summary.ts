import { useQuery } from '@tanstack/react-query'
import { requestJSON } from '@/lib/api-client'
import { queryClient } from '@/lib/query-client'

export interface ActivitySummary {
  message_unread_count: number
  automation_inbox_unread_count: number
  automation_running_count: number
}

export const activitySummaryQueryKey = ['activity-summary'] as const
const ACTIVITY_SUMMARY_REFRESH_INTERVAL_MS = 30_000

/** Polls the compact global badge projection instead of three full record lists. */
export function useActivitySummary() {
  return useQuery({
    queryKey: activitySummaryQueryKey,
    queryFn: () => requestJSON<ActivitySummary>('/api/activity/summary'),
    refetchInterval: ACTIVITY_SUMMARY_REFRESH_INTERVAL_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  }, queryClient)
}

/** Keeps both desktop and mobile message badges in sync after an optimistic read. */
export function setActivityMessageUnreadCount(count: number) {
  queryClient.setQueryData<ActivitySummary>(activitySummaryQueryKey, (current) => current ? {
    ...current,
    message_unread_count: Math.max(0, count),
  } : current)
}
