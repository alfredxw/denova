import type { ActorStateField } from '../../types'

/** Renderer kinds understood by the stage state ledger. */
export type StateFieldRenderer = 'stat' | 'inline' | 'block' | 'list' | 'object'

/** Built-in ledger groups used when a field declares no explicit group. */
export type StateFieldGroupKey = 'overview' | 'details' | 'holdings'

export const BUILTIN_STATE_FIELD_GROUPS: StateFieldGroupKey[] = ['overview', 'details', 'holdings']

export interface StateFieldLayout {
  renderer: StateFieldRenderer
  /** Builtin group key, or the template-declared group name. */
  group: string
  /** True when the group name came from ActorStateField.group. */
  customGroup: boolean
}

/** Strings at or above this length render as block paragraphs by default. */
const LONG_TEXT_THRESHOLD = 24

export function resolveStateFieldLayout(field: ActorStateField | undefined, value: unknown): StateFieldLayout {
  const renderer = resolveStateFieldRenderer(field, value)
  const declaredGroup = typeof field?.group === 'string' ? field.group.trim() : ''
  if (declaredGroup) {
    return { renderer, group: declaredGroup, customGroup: true }
  }
  switch (renderer) {
    case 'stat':
    case 'inline':
      return { renderer, group: 'overview', customGroup: false }
    case 'block':
      return { renderer, group: 'details', customGroup: false }
    default:
      return { renderer, group: 'holdings', customGroup: false }
  }
}

function resolveStateFieldRenderer(field: ActorStateField | undefined, value: unknown): StateFieldRenderer {
  const display = field?.display
  if (display === 'inline' || display === 'block' || display === 'list') return display
  if (display === 'stat') return isBoundedNumberField(field, value) ? 'stat' : 'inline'

  if (isBoundedNumberField(field, value)) return 'stat'

  const type = field?.type || inferredType(value)
  switch (type) {
    case 'list':
      return containsObjectItem(value) ? 'object' : 'list'
    case 'object':
      return 'object'
    case 'number':
    case 'bool':
    case 'enum':
      return 'inline'
    default:
      return isLongText(value) ? 'block' : 'inline'
  }
}

export function isBoundedNumberField(field: ActorStateField | undefined, value: unknown): boolean {
  return field?.type === 'number'
    && typeof value === 'number'
    && Number.isFinite(value)
    && typeof field.min === 'number'
    && Number.isFinite(field.min)
    && typeof field.max === 'number'
    && Number.isFinite(field.max)
    && field.max > field.min
}

function isLongText(value: unknown) {
  return typeof value === 'string' && value.trim().length >= LONG_TEXT_THRESHOLD
}

function containsObjectItem(value: unknown) {
  return Array.isArray(value) && value.some((item) => typeof item === 'object' && item !== null)
}

function inferredType(value: unknown): string {
  if (Array.isArray(value)) return 'list'
  if (value === null) return 'object'
  return typeof value
}
