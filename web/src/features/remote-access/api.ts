import { queryOptions } from '@tanstack/react-query'
import { jsonHeaders, requestJSON } from '@/lib/api-client/client'

export interface RemoteAccessStatus {
  local: boolean
  authenticated: boolean
  lan_url?: string
}

// Authentication completes before the workbench mounts. Query coalescing also
// prevents React StrictMode from consuming a one-use connection link twice.
export const remoteAccessQuery = queryOptions({
  queryKey: ['remote-access'],
  staleTime: Infinity,
  retry: false,
  queryFn: async ({ client }): Promise<RemoteAccessStatus> => {
    const hash = new URLSearchParams(window.location.hash.slice(1))
    const token = hash.get('pair')
    if (token !== null) {
      hash.delete('pair')
      const remaining = hash.toString()
      window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}${remaining ? `#${remaining}` : ''}`)
    }
    // Reuse the normal query retry policy for the read only. The outer query
    // keeps the one-use token in this invocation and never replays a pairing POST.
    const status = await client.fetchQuery({
      queryKey: ['remote-access-status'],
      staleTime: 0,
      gcTime: 0,
      queryFn: () => requestJSON<RemoteAccessStatus>('/api/auth/status', { cache: 'no-store' }),
    })
    // Reopening a previously used link must not interrupt a valid browser login.
    if (status.authenticated || token === null) return status
    return requestJSON('/api/auth/pair', { method: 'POST', headers: jsonHeaders, body: JSON.stringify({ token }) })
  },
})

export function loginRemoteAccess(username: string, password: string): Promise<RemoteAccessStatus> {
  return requestJSON('/api/auth/login', { method: 'POST', headers: jsonHeaders, body: JSON.stringify({ username, password }) })
}

export function createConnectionLink(): Promise<{ url: string; expires_in: number }> {
  return requestJSON('/api/auth/link', { method: 'POST' })
}

export async function logoutRemoteAccess(): Promise<void> {
  await requestJSON('/api/auth/logout', { method: 'POST' })
  // Reload retires all current streams and removes private in-memory query data.
  window.location.reload()
}
