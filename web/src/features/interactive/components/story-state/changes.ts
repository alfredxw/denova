import type { StoryStateChange } from './model'

export type StateChangeTone = 'positive' | 'negative' | 'neutral'

export type StateChangeKind = 'delta' | 'added' | 'removed' | 'updated' | 'cleared' | 'archived' | 'restored'

/**
 * ClassifiedStateChange is the presentation-ready form of one raw state op.
 * Components localize labels from kind; delta carries signed numeric changes.
 */
export interface ClassifiedStateChange {
  kind: StateChangeKind
  /** Signed numeric delta for inc/decrement ops on numeric fields. */
  delta: number | null
  /** Short preview of an added/removed value, when printable. */
  text: string
  reason?: string
  tone: StateChangeTone
}

/** classifyStateChange folds a raw state op into its presentation shape. */
export function classifyStateChange(change: StoryStateChange, numeric: boolean): ClassifiedStateChange {
  const op = change.op.trim().toLowerCase()
  const delta = numericChangeDelta(change, numeric)
  if (delta !== null) {
    return { kind: 'delta', delta, text: '', reason: change.reason, tone: delta > 0 ? 'positive' : delta < 0 ? 'negative' : 'neutral' }
  }
  const text = printableChangeValue(change.value)
  if (op === 'archive') {
    return { kind: 'archived', delta: null, text: '', reason: change.reason, tone: 'negative' }
  }
  if (op === 'restore') {
    return { kind: 'restored', delta: null, text: '', reason: change.reason, tone: 'positive' }
  }
  if (['push', 'append', 'add'].includes(op)) {
    return { kind: 'added', delta: null, text, reason: change.reason, tone: 'positive' }
  }
  if (['pull', 'remove', 'delete'].includes(op)) {
    return { kind: 'removed', delta: null, text, reason: change.reason, tone: 'negative' }
  }
  if (op === 'unset') {
    return { kind: 'cleared', delta: null, text: '', reason: change.reason, tone: 'neutral' }
  }
  return { kind: 'updated', delta: null, text: '', reason: change.reason, tone: 'neutral' }
}

/** mergeFieldChanges picks the most informative classification for one field. */
export function mergeFieldChanges(changes: StoryStateChange[], numeric: boolean): ClassifiedStateChange | null {
  if (changes.length === 0) return null
  const classified = changes.map((change) => classifyStateChange(change, numeric))
  return classified.find((change) => change.kind === 'delta')
    || classified.find((change) => change.kind === 'added' || change.kind === 'removed')
    || classified[0]
}

function numericChangeDelta(change: StoryStateChange, numeric: boolean) {
  const op = change.op.trim().toLowerCase()
  if (!numeric || (op !== 'inc' && op !== 'increment' && op !== 'decrement') || typeof change.value !== 'number') return null
  return op === 'decrement' ? -Math.abs(change.value) : change.value
}

function printableChangeValue(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value) && value.every((item) => item === null || ['string', 'number', 'boolean'].includes(typeof item))) {
    return value.map((item) => (item === null ? '' : String(item))).filter(Boolean).join('、')
  }
  return ''
}
