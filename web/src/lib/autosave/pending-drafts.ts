export interface PendingDraft {
  id: string
  workspace: string
  path: string
  content: string
  baseContent: string
  baseRevision: string
  mode: 'auto' | 'manual'
  queuedAt: string
}

export type PendingDraftInput = Omit<PendingDraft, 'id' | 'queuedAt'>

const STORAGE_KEY = 'nova:pending-drafts:v1'
const MAX_PENDING_DRAFTS = 20
const MAX_PENDING_DRAFT_BYTES = 256 * 1024

export function queuePendingDraft(input: PendingDraftInput): PendingDraft[] {
  if (new TextEncoder().encode(input.content).byteLength > MAX_PENDING_DRAFT_BYTES) {
    return readAll()
  }
  const drafts = readAll().filter((draft) => !(draft.workspace === input.workspace && draft.path === input.path))
  drafts.push({
    ...input,
    id: newPendingDraftID(),
    queuedAt: new Date().toISOString(),
  })
  while (drafts.length > MAX_PENDING_DRAFTS) {
    drafts.shift()
  }
  writeAll(drafts)
  return drafts
}

export function listPendingDrafts(): PendingDraft[] {
  return readAll()
}

export function pendingDraftsForWorkspace(workspace: string): PendingDraft[] {
  return readAll().filter((draft) => draft.workspace === workspace)
}

export function removePendingDraft(id: string): PendingDraft[] {
  const drafts = readAll().filter((draft) => draft.id !== id)
  writeAll(drafts)
  return drafts
}

export function removePendingDraftsForFile(workspace: string, path: string): PendingDraft[] {
  const drafts = readAll().filter((draft) => !(draft.workspace === workspace && draft.path === path))
  writeAll(drafts)
  return drafts
}

function readAll(): PendingDraft[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isPendingDraft).sort(byQueuedAt)
  } catch {
    return []
  }
}

function writeAll(drafts: PendingDraft[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(drafts))
  } catch {
    // Storage may be unavailable (privacy mode or quota); the save lane keeps retrying in memory.
  }
}

function isPendingDraft(value: unknown): value is PendingDraft {
  if (!value || typeof value !== 'object') return false
  const draft = value as Record<string, unknown>
  return typeof draft.id === 'string' && draft.id.length > 0
    && typeof draft.workspace === 'string' && draft.workspace.length > 0
    && typeof draft.path === 'string' && draft.path.length > 0
    && typeof draft.content === 'string'
    && typeof draft.baseContent === 'string'
    && typeof draft.baseRevision === 'string'
    && (draft.mode === 'auto' || draft.mode === 'manual')
    && typeof draft.queuedAt === 'string' && draft.queuedAt.length > 0
}

function byQueuedAt(left: PendingDraft, right: PendingDraft) {
  return left.queuedAt.localeCompare(right.queuedAt)
}

function newPendingDraftID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`
}
