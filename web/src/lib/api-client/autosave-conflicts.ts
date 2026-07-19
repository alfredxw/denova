import { jsonHeaders, requestJSON } from './client'

export interface AutosaveConflictSnapshot {
  revision?: string
  value: unknown
}

export interface AutosaveConflictRecord {
  resource: string
  scope: string
  id: string
  base: AutosaveConflictSnapshot
  local: AutosaveConflictSnapshot
  external: AutosaveConflictSnapshot
  merged: AutosaveConflictSnapshot
  strategy: string
  conflict_paths: string[][]
}

export interface PreservedAutosaveConflict {
  id: string
  path: string
  storage: 'server' | 'browser'
}

/** Writes the full conflict to the server journal, with a per-record browser fallback for offline edits. */
export async function preserveAutosaveConflict(record: AutosaveConflictRecord): Promise<PreservedAutosaveConflict> {
  try {
    const saved = await requestJSON<{ id: string; path: string }>('/api/autosave-conflicts', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify(record),
    })
    return { ...saved, storage: 'server' }
  } catch (error) {
    const id = browserConflictID()
    const key = `nova:autosave-conflict:${id}`
    window.localStorage.setItem(key, JSON.stringify({ ...record, archived_at: new Date().toISOString() }))
    console.warn('[autosave-conflicts.ts] server conflict journal unavailable; retained browser recovery copy', {
      id,
      resource: record.resource,
      scope: record.scope,
      recordID: record.id,
      error,
    })
    return { id, path: key, storage: 'browser' }
  }
}

function browserConflictID(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}
