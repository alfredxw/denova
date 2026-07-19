import { diff3Merge } from 'node-diff3'

export interface RebaseConflict {
  path: string[]
  baseline: unknown
  local: unknown
  external: unknown
}

export interface RebaseResult<T> {
  value: T
  conflicts: RebaseConflict[]
}

/** Replays local text edits over a newer external snapshot and exposes true overlaps to the caller. */
export function rebaseText(previousSaved: string, currentDraft: string, nextSaved: string): string {
  return rebaseTextWithConflicts(previousSaved, currentDraft, nextSaved).value
}

/**
 * Merges text changes. True overlaps prefer the active local editor in the
 * canonical value, while returning all three versions for durable recovery.
 */
export function rebaseTextWithConflicts(
  previousSaved: string,
  currentDraft: string,
  nextSaved: string,
): RebaseResult<string> {
  if (currentDraft === previousSaved) return { value: nextSaved, conflicts: [] }
  if (nextSaved === previousSaved || currentDraft === nextSaved) return { value: currentDraft, conflicts: [] }

  const conflicts: RebaseConflict[] = []
  const regions = diff3Merge(
    splitTextLines(currentDraft),
    splitTextLines(previousSaved),
    splitTextLines(nextSaved),
    { excludeFalseConflicts: true },
  )
  const value = regions.flatMap((region) => {
    if (region.ok) return region.ok
    if (!region.conflict) return []
    const local = region.conflict.a.join('')
    const baseline = region.conflict.o.join('')
    const external = region.conflict.b.join('')
    conflicts.push({ path: [], baseline, local, external })
    return region.conflict.a
  }).join('')

  return { value, conflicts }
}

/** Replays locally changed JSON fields over a newer snapshot while retaining untouched external fields. */
export function rebaseJSONValue<T>(previousSaved: T, currentDraft: T, nextSaved: T): T {
  return rebaseJSONValueWithConflicts(previousSaved, currentDraft, nextSaved).value
}

/** Replays JSON changes and reports every leaf where local and external edits overlap. */
export function rebaseJSONValueWithConflicts<T>(previousSaved: T, currentDraft: T, nextSaved: T): RebaseResult<T> {
  const conflicts: RebaseConflict[] = []
  const value = rebaseJSONBranch(previousSaved, currentDraft, nextSaved, [], conflicts)
  return { value: value as T, conflicts }
}

function rebaseJSONBranch(
  previousSaved: unknown,
  currentDraft: unknown,
  nextSaved: unknown,
  path: string[],
  conflicts: RebaseConflict[],
): unknown {
  if (jsonValueEqual(previousSaved, currentDraft)) return nextSaved
  if (jsonValueEqual(previousSaved, nextSaved) || jsonValueEqual(currentDraft, nextSaved)) return currentDraft
  if (!isJSONRecord(previousSaved) || !isJSONRecord(currentDraft) || !isJSONRecord(nextSaved)) {
    conflicts.push({ path, baseline: previousSaved, local: currentDraft, external: nextSaved })
    return currentDraft
  }

  const rebased: Record<string, unknown> = { ...nextSaved }
  const keys = new Set([...Object.keys(previousSaved), ...Object.keys(currentDraft)])
  for (const key of keys) {
    const previousHasKey = Object.prototype.hasOwnProperty.call(previousSaved, key)
    const currentHasKey = Object.prototype.hasOwnProperty.call(currentDraft, key)
    const previousValue = previousSaved[key]
    const currentValue = currentDraft[key]
    if (previousHasKey === currentHasKey && jsonValueEqual(previousValue, currentValue)) continue
    if (!currentHasKey) {
      const externalHasKey = Object.prototype.hasOwnProperty.call(nextSaved, key)
      const externalValue = nextSaved[key]
      if (externalHasKey && !jsonValueEqual(previousValue, externalValue)) {
        conflicts.push({
          path: [...path, key],
          baseline: previousValue,
          local: undefined,
          external: externalValue,
        })
      }
      delete rebased[key]
      continue
    }
    rebased[key] = rebaseJSONBranch(previousValue, currentValue, nextSaved[key], [...path, key], conflicts)
  }
  return rebased
}

function splitTextLines(value: string): string[] {
  return value.match(/[^\r\n]*(?:\r\n|\r|\n)|[^\r\n]+$/g) ?? []
}

function isJSONRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

function jsonValueEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  if (Array.isArray(left) || Array.isArray(right)) {
    return Array.isArray(left)
      && Array.isArray(right)
      && left.length === right.length
      && left.every((value, index) => jsonValueEqual(value, right[index]))
  }
  if (!isJSONRecord(left) || !isJSONRecord(right)) return false
  const leftKeys = Object.keys(left)
  const rightKeys = Object.keys(right)
  return leftKeys.length === rightKeys.length
    && leftKeys.every((key) => Object.prototype.hasOwnProperty.call(right, key) && jsonValueEqual(left[key], right[key]))
}
