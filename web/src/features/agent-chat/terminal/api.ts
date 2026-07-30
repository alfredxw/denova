import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import type { TerminalCommandProfile } from '../types'

/** Backend snapshot of one live terminal session. */
export interface TerminalSessionInfo {
  id: string
  /** Stable workbench tab that owns this process; absent only on sessions created by older clients. */
  owner_tab_id?: string
  profile_id: string
  title: string
  command: string
  args: string[]
  cwd: string
  project_id?: string
  workspace: string
  cols: number
  rows: number
  created_at: string
  attached: number
  exited: boolean
  exit_code: number
  exit_error?: string
  /** Attach token for the WebSocket handshake; only handed out over authenticated endpoints. */
  token: string
}

export interface TerminalRuntimeStatus {
  enabled: boolean
  shell: string
  commands: TerminalCommandProfile[]
  default_cwd: string
  max_sessions: number
  scrollback_kb: number
  sessions: TerminalSessionInfo[]
}

export interface CreateTerminalSessionRequest {
  /** Stable tab identity used by the backend to make creation idempotent. */
  owner_tab_id: string
  /** Project that owns the tab; independent from the foreground Writing book. */
  project_id: string
  workspace?: string
  profile_id: string
  title?: string
  /** Only used by legacy `custom` tabs; built-in profiles resolve from backend settings. */
  command?: string
  args?: string[]
  cwd?: string
  cols: number
  rows: number
}

export async function getTerminalRuntimeStatus(): Promise<TerminalRuntimeStatus> {
  return requestJSON<TerminalRuntimeStatus>('/api/terminal/sessions')
}

export async function createTerminalSession(request: CreateTerminalSessionRequest): Promise<TerminalSessionInfo> {
  return requestJSON<TerminalSessionInfo>('/api/terminal/sessions', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  })
}

export async function closeTerminalSession(id: string): Promise<void> {
  await requestJSON(`/api/terminal/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

/** Build the attach WebSocket URL, following the page protocol so HTTPS deployments use wss. */
export function terminalAttachURL(id: string, token: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/terminal/sessions/${encodeURIComponent(id)}/attach?token=${encodeURIComponent(token)}`
}
