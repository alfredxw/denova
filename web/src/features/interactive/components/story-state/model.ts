import type { ActorStateField, ActorStateSchemaSnapshot, ActorTraitInstance, Snapshot, TurnEvent } from '../../types'
import type { ClassifiedStateChange } from './changes'
import { mergeFieldChanges } from './changes'
import { resolveStateFieldLayout, type StateFieldRenderer } from './field-layout'

export type ActorStateEntry = [string, Record<string, unknown>]

export interface StoryStateChange {
  id: string
  actorId?: string
  path: string
  op: string
  value?: unknown
  reason?: string
}

export interface StoryStateModel {
  actors: ActorStateEntry[]
  worldFacts: Array<[string, unknown]>
  changes: StoryStateChange[]
  hasState: boolean
}

export function buildStoryStateModel(snapshot: Snapshot | null): StoryStateModel {
  const stateFacts = snapshot
    ? Object.entries(snapshot.state).filter(([, value]) => value !== undefined && value !== null)
    : []
  const { actors, worldFacts } = splitStoryStateFacts(stateFacts)
  return {
    actors,
    worldFacts,
    changes: stateChanges(snapshot?.current_turn?.state_delta),
    hasState: actors.length > 0 || worldFacts.length > 0,
  }
}

export function splitStoryStateFacts(stateFacts: Array<[string, unknown]>) {
  const stateObjects = actorEntries(stateFacts)
  const actors = stateObjects.filter(([actorId, actor]) => isActorLike(actorId, actor))
  const otherFacts = stateFacts.filter(([key]) => key !== 'actors')
  const worldFacts = ([
    ...otherFacts,
    ...stateObjects
      .filter(([actorId, actor]) => !isActorLike(actorId, actor))
      .map(([actorId, actor]): [string, unknown] => [actorName(actorId, actor), stateObjectValue(actor)]),
  ] satisfies Array<[string, unknown]>)
    .map(([key, value]): [string, unknown] => [key, compactStateValue(value)])
    .filter(([, value]) => value !== undefined)
  return { actors, worldFacts, stateObjects, otherFacts }
}

export function actorEntries(stateFacts: Array<[string, unknown]>): ActorStateEntry[] {
  const actors = stateFacts.find(([key]) => key === 'actors')?.[1]
  if (!isRecord(actors)) return []
  return Object.entries(actors)
    .filter((entry): entry is ActorStateEntry => isRecord(entry[1]))
    .sort(([leftId, left], [rightId, right]) => actorPriority(leftId, left) - actorPriority(rightId, right))
}

export function actorName(actorId: string, actor: Record<string, unknown>) {
  return stringValue(actor.name) || actorId
}

export function actorTemplate(actor: Record<string, unknown>, schema?: ActorStateSchemaSnapshot) {
  const templateId = stringValue(actor.template_id)
  return schema?.system.templates?.find((item) => item.id === templateId)
}

export function visibleActorTraits(actor: Record<string, unknown>) {
  if (!Array.isArray(actor.traits)) return []
  return actor.traits.filter(isActorTrait).filter((trait) => trait.visibility !== 'hidden')
}

export function actorFieldEntries(
  actor: Record<string, unknown>,
  schemaFields: ActorStateField[] | undefined,
): Array<{ field: ActorStateField; value: unknown }> {
  const state = isRecord(actor.state) ? actor.state : {}
  const visibleFields = (schemaFields || [])
    .filter((field) => field.visibility !== 'hidden')
  if (schemaFields !== undefined) {
    return visibleFields.map((field) => ({
      field,
      value: state[field.name] ?? (field.id ? state[field.id] : undefined) ?? (field.path ? state[field.path] : undefined),
    }))
  }

  const directState = Object.fromEntries(Object.entries(actor).filter(([key]) => !['name', 'role', 'template_id', 'state', 'traits'].includes(key)))
  return Object.entries({ ...directState, ...state }).map(([name, value]) => ({
    field: { name: humanizeStateKey(name), type: inferredFieldType(value) } satisfies ActorStateField,
    value,
  }))
}

export function stateChanges(delta: TurnEvent['state_delta']): StoryStateChange[] {
  if (!delta) return []
  const actorChanges: StoryStateChange[] = (delta.actor_ops || []).map((op, index) => ({
    id: `actor:${op.actor_id}:${op.field_id}:${index}`,
    actorId: op.actor_id,
    path: op.field_id,
    op: op.op,
    value: op.value,
    reason: op.reason,
  }))
  const sharedChanges: StoryStateChange[] = (delta.ops || []).map((op, index) => ({
    id: `state:${op.path}:${index}`,
    path: op.path,
    op: op.op,
    value: op.value,
    reason: op.reason,
  }))
  return [...actorChanges, ...sharedChanges]
}

export function statePathLabel(path: string) {
  const parts = path.split('.').filter(Boolean)
  const useful = parts[0] === 'actors' && parts.length > 3 ? parts.slice(3) : parts
  return useful.map(humanizeStateKey).join(' / ')
}

export function humanizeStateKey(value: string) {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/^\w/, (letter) => letter.toUpperCase())
}

function actorPriority(actorId: string, actor: Record<string, unknown>) {
  return isLeadActor(actorId, actor) ? 0 : 1
}

function isLeadActor(actorId: string, actor: Record<string, unknown>) {
  const candidates = [actorId, stringValue(actor.role), stringValue(actor.template_id)].map((value) => value.toLowerCase())
  return candidates.some((value) => value === 'protagonist' || value === 'lead' || value === 'player' || value === '主角')
}

function isActorLike(actorId: string, actor: Record<string, unknown>) {
  const role = stringValue(actor.role).toLowerCase()
  const templateId = stringValue(actor.template_id).toLowerCase()
  if (['story_context', 'world', 'scene', 'global', 'faction', 'base', 'instance', 'location', 'clock', 'quest'].some((marker) => role === marker || templateId === marker)) return false
  if (role) return true
  if (isLeadActor(actorId, actor)) return true
  const identity = `${actorId} ${templateId}`.toLowerCase()
  return ['important_character', 'opponent', 'supporting', 'character', 'npc', 'enemy', 'monster'].some((marker) => identity.includes(marker))
}

function stateObjectValue(actor: Record<string, unknown>) {
  if (isRecord(actor.state)) return actor.state
  return Object.fromEntries(Object.entries(actor).filter(([key]) => !['name', 'role', 'template_id', 'traits'].includes(key)))
}

function compactStateValue(value: unknown): unknown {
  if (value === undefined || value === null) return undefined
  if (typeof value === 'string') return value.trim() ? value : undefined
  if (Array.isArray(value)) {
    const items = value.map(compactStateValue).filter((item) => item !== undefined)
    return items.length > 0 ? items : undefined
  }
  if (isRecord(value)) {
    const entries = Object.entries(value)
      .map(([key, item]): [string, unknown] => [key, compactStateValue(item)])
      .filter(([, item]) => item !== undefined)
    return entries.length > 0 ? Object.fromEntries(entries) : undefined
  }
  return value
}

function inferredFieldType(value: unknown) {
  if (Array.isArray(value)) return 'list'
  if (value === null) return 'object'
  if (typeof value === 'boolean') return 'bool'
  return typeof value
}

function isActorTrait(value: unknown): value is ActorTraitInstance {
  return isRecord(value)
    && typeof value.pool_id === 'string'
    && typeof value.trait_id === 'string'
    && typeof value.name === 'string'
}

// --- Ledger field grouping -------------------------------------------------

export interface LedgerFieldEntry {
  id: string
  label: string
  field?: ActorStateField
  value: unknown
}

export interface LedgerFieldItem extends LedgerFieldEntry {
  renderer: StateFieldRenderer
  change: ClassifiedStateChange | null
}

export interface LedgerFieldGroup {
  /** Builtin group key or the template-declared group name. */
  key: string
  custom: boolean
  fields: LedgerFieldItem[]
}

/**
 * buildLedgerGroups resolves each field's renderer + group (template hints
 * first, shape heuristics as fallback) and attaches the classified turn
 * change. The schema's field array is the fallback order; any user ordering
 * is applied later as a UI-only preference.
 */
export function buildLedgerGroups(entries: LedgerFieldEntry[], changes: StoryStateChange[]): LedgerFieldGroup[] {
  const groups: LedgerFieldGroup[] = []
  const byKey = new Map<string, LedgerFieldGroup>()
  for (const entry of entries) {
    const layout = resolveStateFieldLayout(entry.field, entry.value)
    let group = byKey.get(layout.group)
    if (!group) {
      group = { key: layout.group, custom: layout.customGroup, fields: [] }
      byKey.set(layout.group, group)
      groups.push(group)
    }
    const fieldChanges = matchFieldChanges(changes, entry)
    group.fields.push({
      ...entry,
      renderer: layout.renderer,
      change: mergeFieldChanges(fieldChanges, typeof entry.value === 'number'),
    })
  }
  return groups.sort((left, right) => (
    ledgerGroupPriority(left) - ledgerGroupPriority(right)
  ))
}

const BUILTIN_GROUP_PRIORITY: Record<string, number> = {
  overview: 0,
  holdings: 1,
  details: 3,
  spoiler: 4,
}

/** Custom groups sit between holdings and details and keep first-seen field order. */
function ledgerGroupPriority(group: LedgerFieldGroup) {
  if (group.custom) return 2
  return BUILTIN_GROUP_PRIORITY[group.key] ?? 2
}

/** Splits the current ordered layout into its first two preview groups and the remainder. */
export function splitLedgerGroupsForPreview(groups: LedgerFieldGroup[]): { preview: LedgerFieldGroup[]; hidden: LedgerFieldGroup[] } {
  return { preview: groups.slice(0, 2), hidden: groups.slice(2) }
}

function matchFieldChanges(changes: StoryStateChange[], entry: LedgerFieldEntry) {
  const candidates = entry.field ? actorFieldPaths(entry.field) : [entry.id]
  return changes.filter((change) => candidates.some((path) => sameFieldPath(change.path, path) || change.path.startsWith(`${path}.`)))
}

export function actorFieldPaths(field: ActorStateField) {
  return [field.id, field.path, field.name]
    .filter((value): value is string => typeof value === 'string' && value.trim() !== '')
}

export function sameFieldPath(left: string, right: string) {
  const normalizedLeft = humanizeStateKey(left.trim()).toLocaleLowerCase()
  const normalizedRight = humanizeStateKey(right.trim()).toLocaleLowerCase()
  return normalizedLeft === normalizedRight
}

/**
 * changeFieldLabel resolves a change to its template field name so the
 * summary row shows localized names instead of raw field ids.
 */
export function changeFieldLabel(change: StoryStateChange, actors: ActorStateEntry[], schema?: ActorStateSchemaSnapshot): string {
  if (change.actorId) {
    const actor = actors.find(([actorId]) => actorId === change.actorId)?.[1]
    const template = actor ? actorTemplate(actor, schema) : undefined
    const field = template?.fields?.find((candidate) => actorFieldPaths(candidate).some((path) => sameFieldPath(path, change.path)))
    if (field) return field.name
    return humanizeStateKey(change.path)
  }
  const segments = change.path.split('.').filter(Boolean)
  return humanizeStateKey(segments[segments.length - 1] || change.path)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}
