import type { WorkspaceChangeEvent, WorkspaceFileChange, WorkspaceFileChangeType } from '@/features/changes/types'
import type { SSEEvent } from '@/lib/api-client/types'

const WORKSPACE_CHANGE_TYPES = new Set<WorkspaceFileChangeType>(['added', 'updated', 'deleted'])

export type WorkspaceEventClientMessage =
  | { type: 'subscribe'; projectId: string; authorization?: string }
  | { type: 'authorization'; authorization?: string }
  | { type: 'unsubscribe' }

export type WorkspaceEventWorkerMessage =
  | { type: 'workspace-change'; event: WorkspaceChangeEvent }
  | { type: 'remote-access-required' }

/** Narrow MessagePort surface used by both the browser runtime and unit tests. */
export interface WorkspaceEventPort {
  onmessage: ((event: MessageEvent<WorkspaceEventClientMessage>) => void) | null
  onmessageerror: ((event: MessageEvent<unknown>) => void) | null
  postMessage(message: WorkspaceEventWorkerMessage): void
  start(): void
  close(): void
}

export function isWorkspaceEventClientMessage(value: unknown): value is WorkspaceEventClientMessage {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const message = value as Record<string, unknown>
  if (message.type === 'unsubscribe') return true
  if (message.type === 'authorization') {
    return message.authorization === undefined || typeof message.authorization === 'string'
  }
  return message.type === 'subscribe' &&
    typeof message.projectId === 'string' &&
    message.projectId.length > 0 &&
    (message.authorization === undefined || typeof message.authorization === 'string')
}

export function isWorkspaceEventWorkerMessage(value: unknown): value is WorkspaceEventWorkerMessage {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const message = value as Record<string, unknown>
  if (message.type === 'remote-access-required') return true
  return message.type === 'workspace-change' && Boolean(message.event) && typeof message.event === 'object'
}

export function parseWorkspaceChangeSSE(event: SSEEvent): WorkspaceChangeEvent | null {
  if (event.event !== 'workspace-change') return null
  let value: unknown
  try {
    value = JSON.parse(event.data)
  } catch (error) {
    console.warn('[workspace-events/protocol.ts] ignored malformed workspace event JSON', { error })
    return null
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const raw = value as Record<string, unknown>
  if (typeof raw.project_id !== 'string' || !raw.project_id || typeof raw.workspace !== 'string') return null

  const changes = Array.isArray(raw.changes)
    ? raw.changes.map(parseWorkspaceFileChange).filter((change): change is WorkspaceFileChange => change !== null)
    : []
  const paths = Array.from(new Set([
    ...readStringArray(raw.paths),
    ...changes.map(change => change.path),
  ]))
  const normalized: WorkspaceChangeEvent = {
    project_id: raw.project_id,
    workspace: raw.workspace,
    source: typeof raw.source === 'string' ? raw.source : undefined,
    resync: raw.resync === true,
    changes,
  }
  // A missing path list tells existing consumers to refresh all relevant
  // resources. Preserve that contract for watcher repair and reconnect resync.
  if (paths.length > 0) normalized.paths = paths
  return normalized
}

function parseWorkspaceFileChange(value: unknown): WorkspaceFileChange | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const raw = value as Record<string, unknown>
  if (typeof raw.path !== 'string' || !raw.path || typeof raw.type !== 'string') return null
  if (!WORKSPACE_CHANGE_TYPES.has(raw.type as WorkspaceFileChangeType)) return null
  return { path: raw.path, type: raw.type as WorkspaceFileChangeType }
}

function readStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string' && item.length > 0)
}
