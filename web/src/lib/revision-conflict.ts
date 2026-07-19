import { APIError } from '@/lib/api-client'

const MAX_REVISION_SAVE_ATTEMPTS = 3

interface RevisionSnapshot<Value> {
  value: Value
  revision?: string
}

interface SaveWithRevisionRecoveryOptions<Value, Saved> {
  baseline: Value
  draft: Value
  revision?: string
  save: (draft: Value, revision?: string) => Promise<Saved>
  loadLatest: () => Promise<RevisionSnapshot<Value>>
  rebase: (baseline: Value, draft: Value, latest: Value) => Value | Promise<Value>
}

/** Revision mismatches are internal concurrency signals that callers may reload and rebase. */
export function isRevisionConflict(error: unknown): error is APIError {
  if (!(error instanceof APIError)) return false
  if (error.code) return error.code === 'revision_conflict'
  return error.status === 409 || error.status === 412
}

/** Retries an explicit revisioned write after replaying its local change over the latest snapshot. */
export async function saveWithRevisionRecovery<Value, Saved>({
  baseline,
  draft,
  revision,
  save,
  loadLatest,
  rebase,
}: SaveWithRevisionRecoveryOptions<Value, Saved>): Promise<Saved> {
  let currentBaseline = baseline
  let currentDraft = draft
  let currentRevision = revision

  for (let attempt = 1; ; attempt += 1) {
    try {
      return await save(currentDraft, currentRevision)
    } catch (error) {
      if (!isRevisionConflict(error) || attempt >= MAX_REVISION_SAVE_ATTEMPTS) throw error
      const latest = await loadLatest()
      currentDraft = await rebase(currentBaseline, currentDraft, latest.value)
      currentBaseline = latest.value
      currentRevision = latest.revision
    }
  }
}
