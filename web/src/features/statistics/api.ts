import { requestJSON } from '@/lib/api'

export interface UsageTotals {
  requests: number
  external_turns: number
  failed_requests: number
  unknown_usage: number
  prompt: number
  cached: number
  output: number
  tokens: number
  duration_ms: number
  ttft_ms: number
  ttft_requests: number
}

export interface StatisticsReport {
  projects: Array<{ id: string; name: string }>
  game: {
    stories: Array<{ id: string; project_id: string; name: string; turns: number }>
    days: Array<{ date: string; turns: number }>
    new_branches: number
  }
  writing: {
    projects: Array<{ id: string; name: string; words: number; chapters: number; confirmed: number }>
    days: Array<{ date: string; delta: number | null }>
  }
  usage: {
    totals: UsageTotals
    days: Array<UsageTotals & { date: string }>
    models: Array<UsageTotals & { name: string; runtime: string }>
  }
  issues: number
}

export function getStatistics(projectID: string, days: number, signal?: AbortSignal) {
  const query = new URLSearchParams({ days: String(days), timezone: Intl.DateTimeFormat().resolvedOptions().timeZone })
  if (projectID) query.set('project_id', projectID)
  return requestJSON<StatisticsReport>(`/api/statistics?${query}`, { signal })
}
