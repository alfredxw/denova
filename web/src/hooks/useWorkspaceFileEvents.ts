import { useEffect, useRef } from 'react'

import type { WorkspaceChangeEvent, WorkspaceFileChange, WorkspaceFileChangeType } from '@/features/changes/types'
import { isWorkspaceChangeForWorkspace } from '@/features/changes/types'
import { streamWorkspaceEvents, type SSEEvent } from '@/lib/api'

const INITIAL_RECONNECT_DELAY_MS = 250
const MAX_RECONNECT_DELAY_MS = 5_000
const WORKSPACE_CHANGE_TYPES = new Set<WorkspaceFileChangeType>(['added', 'updated', 'deleted'])

/** Maintains the one app-level workspace event stream used by every work mode. */
export function useWorkspaceFileEvents(
  workspace: string,
  onChange: (event: WorkspaceChangeEvent) => void | Promise<void>,
) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  useEffect(() => {
    if (!workspace) return
    const abortController = new AbortController()
    let disposed = false
    let activeReader: ReadableStreamDefaultReader<SSEEvent> | null = null

    const observe = async () => {
      let reconnectDelay = INITIAL_RECONNECT_DELAY_MS
      while (!disposed && !abortController.signal.aborted) {
        try {
          const stream = await streamWorkspaceEvents(abortController.signal)
          reconnectDelay = INITIAL_RECONNECT_DELAY_MS
          const reader = stream.getReader()
          activeReader = reader
          try {
            while (!disposed && !abortController.signal.aborted) {
              const { done, value } = await reader.read()
              if (done) break
              const event = parseWorkspaceChangeSSE(value)
              if (!event || !isWorkspaceChangeForWorkspace(event, workspace)) continue
              window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: event }))
              await onChangeRef.current(event)
            }
          } finally {
            if (activeReader === reader) activeReader = null
            reader.releaseLock()
          }
        } catch (error) {
          if (disposed || abortController.signal.aborted) return
          console.warn('[useWorkspaceFileEvents.ts] workspace event stream disconnected; retrying', {
            workspace,
            error,
          })
        }
        if (disposed || abortController.signal.aborted) return
        await waitForReconnect(reconnectDelay, abortController.signal)
        reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY_MS)
      }
    }

    void observe()
    return () => {
      disposed = true
      abortController.abort()
      void activeReader?.cancel()
    }
  }, [workspace])
}

export function parseWorkspaceChangeSSE(event: SSEEvent): WorkspaceChangeEvent | null {
  if (event.event !== 'workspace-change') return null
  let value: unknown
  try {
    value = JSON.parse(event.data)
  } catch (error) {
    console.warn('[useWorkspaceFileEvents.ts] ignored malformed workspace event JSON', { error })
    return null
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const raw = value as Record<string, unknown>
  if (typeof raw.workspace !== 'string') return null

  const changes = Array.isArray(raw.changes)
    ? raw.changes.map(parseWorkspaceFileChange).filter((change): change is WorkspaceFileChange => change !== null)
    : []
  const paths = Array.from(new Set([
    ...readStringArray(raw.paths),
    ...changes.map(change => change.path),
  ]))
  const normalized: WorkspaceChangeEvent = {
    workspace: raw.workspace,
    source: typeof raw.source === 'string' ? raw.source : undefined,
    resync: raw.resync === true,
    changes,
  }
  // An absent path list means "refresh any relevant open resource" to the
  // existing workspace-change consumers. Preserve that contract for resync.
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

function waitForReconnect(delay: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise(resolve => {
    const finish = () => {
      window.clearTimeout(timer)
      signal.removeEventListener('abort', finish)
      resolve()
    }
    const timer = window.setTimeout(finish, delay)
    signal.addEventListener('abort', finish, { once: true })
  })
}
