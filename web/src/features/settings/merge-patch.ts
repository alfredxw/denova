import type { Settings } from './types'

type JSONMergePatch<T> = T extends readonly unknown[]
  ? T | null
  : T extends object
    ? { [K in keyof T]?: JSONMergePatch<NonNullable<T[K]>> | null }
    : T | null

export type SettingsPatch = JSONMergePatch<Settings>

/** Builds a settings patch, retaining complete atomic engine model selections. */
export function createSettingsMergePatch(baseline: Settings, draft: Settings): SettingsPatch {
  const patch = createMergePatchValue(baseline, draft)
  if (patch === unchanged || !isPlainObject(patch)) return {}
  const settingsPatch = patch as SettingsPatch
  // The API replaces each Codex model/effort branch as one selection. A recursive
  // diff would omit the unchanged model during an effort edit (or lose effort).
  for (const role of ['ide', 'general', 'interactive_story'] as const) {
    const runtimePatch = settingsPatch.agent_runtimes?.[role]
    for (const engine of ['codex', 'claude'] as const) {
      const settings = draft.agent_runtimes?.[role]?.[engine]
      if (runtimePatch?.[engine] && settings) runtimePatch[engine] = { ...settings }
    }
  }
  return settingsPatch
}

const unchanged = Symbol('unchanged')

function createMergePatchValue(baseline: unknown, draft: unknown): unknown | typeof unchanged {
  if (Object.is(baseline, draft)) return unchanged
  if (Array.isArray(baseline) || Array.isArray(draft)) {
    return JSON.stringify(baseline) === JSON.stringify(draft) ? unchanged : draft
  }
  if (!isPlainObject(baseline) || !isPlainObject(draft)) return draft
  const result: Record<string, unknown> = {}
  const keys = new Set([...Object.keys(baseline), ...Object.keys(draft)])
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(draft, key) || draft[key] === undefined) {
      result[key] = null
      continue
    }
    if (!Object.prototype.hasOwnProperty.call(baseline, key)) {
      result[key] = draft[key]
      continue
    }
    const child = createMergePatchValue(baseline[key], draft[key])
    if (child !== unchanged) result[key] = child
  }
  return Object.keys(result).length === 0 ? unchanged : result
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
