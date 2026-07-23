import type { PublicRuleRoll } from '@/lib/api'
import type { RuleResolution, StoryDirector, StorySummary } from '../../types'

export function storyRuleVisibilityMode(story: StorySummary | undefined, directors: StoryDirector[]) {
  const directorID = story?.story_director_id || 'default'
  const director = directors.find((item) => item.id === directorID) || directors.find((item) => item.id === 'default')
  return director?.strategy?.rule_visibility_mode || 'audit_only'
}

export function publicRuleRollFromResolution(resolution?: RuleResolution): PublicRuleRoll | null {
  if (!resolution?.result) return null
  const result = resolution.result
  return {
    resolution_id: resolution.id,
    label: result.label || resolution.request?.rule?.label || resolution.request?.challenge || resolution.request?.action,
    difficulty: resolution.request?.difficulty,
    dice: result.dice,
    roll_mode: result.roll_mode || resolution.request?.rule?.roll_mode,
    rolls: result.rolls,
    kept_roll: result.kept_roll,
    base_target: result.base_target,
    target: result.target,
    bonus_total: result.bonus_total,
    total: result.total,
    outcome: result.outcome,
    result: result.result,
    cost: resolution.request?.cost,
    stakes: resolution.request?.adjudication?.stakes,
    state_changes: result.state_changes,
  }
}

export function publicRuleRollFromToolOutput(content: string): PublicRuleRoll | null {
  const parsed = parseJSONRecord(content)
  if (!parsed) return null
  const rolls = Array.isArray(parsed.rolls) ? parsed.rolls.map(Number).filter(Number.isFinite) : undefined
  const stateChanges = Array.isArray(parsed.state_changes)
    ? parsed.state_changes
      .map((item) => isPlainRecord(item) ? {
        actor_id: String(item.actor_id || '').trim(),
        field_id: String(item.field_id || '').trim(),
        change: Number(item.change),
        reason: typeof item.reason === 'string' ? item.reason : undefined,
      } : null)
      .filter((item): item is NonNullable<typeof item> => Boolean(item && item.actor_id && item.field_id && Number.isFinite(item.change)))
    : undefined
  return {
    resolution_id: stringFromRecord(parsed, 'resolution_id'),
    label: stringFromRecord(parsed, 'label') || stringFromRecord(parsed, 'challenge'),
    difficulty: stringFromRecord(parsed, 'difficulty'),
    dice: stringFromRecord(parsed, 'dice'),
    roll_mode: stringFromRecord(parsed, 'roll_mode'),
    rolls,
    kept_roll: numberFromRecord(parsed, 'kept_roll'),
    base_target: numberFromRecord(parsed, 'base_target'),
    target: numberFromRecord(parsed, 'target'),
    bonus_total: numberFromRecord(parsed, 'bonus_total'),
    total: numberFromRecord(parsed, 'total'),
    outcome: stringFromRecord(parsed, 'outcome'),
    result: stringFromRecord(parsed, 'result'),
    cost: stringFromRecord(parsed, 'cost'),
    stakes: stringFromRecord(parsed, 'stakes'),
    state_changes: stateChanges,
  }
}

function parseJSONRecord(content: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(content)
    return isPlainRecord(parsed) ? parsed : null
  } catch {
    return null
  }
}

function stringFromRecord(record: Record<string, unknown>, key: string) {
  const value = record[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function numberFromRecord(record: Record<string, unknown>, key: string) {
  const value = Number(record[key])
  return Number.isFinite(value) ? value : undefined
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}
