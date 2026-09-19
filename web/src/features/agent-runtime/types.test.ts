import { describe, expect, it } from 'vitest'
import { resolveRuntimePreferences, runtimeSelection, runtimeModel, runtimeModelKey, runtimeModelFromKey, type RuntimePreferences } from './types'

describe('runtime configuration inheritance', () => {
  it('keeps inactive engine settings out of a Claude execution', () => {
    const parent: RuntimePreferences = { selected: 'codex', codex: { model: 'old-model', effort: 'high' }, claude: { model: 'opus', effort: 'max' } }
    const effective = resolveRuntimePreferences(parent, { selected: 'claude', claude: { model: 'sonnet' } })
    expect(effective.codex).toEqual(parent.codex)
    expect(runtimeSelection(effective)).toEqual({ kind: 'claude', claude: { model: 'sonnet' } })
    expect(runtimeModel(runtimeSelection(effective)!)).toEqual({ model: 'sonnet' })
    expect(runtimeSelection({ selected: 'claude', codex: parent.codex })).toBeNull()
    expect(parent.claude).toEqual({ model: 'opus', effort: 'max' })
  })
  it('replaces a complete model branch independently of the selected engine', () => {
    const parent: RuntimePreferences = { selected: 'codex', codex: { model: 'parent-model', effort: 'high' } }
    const draft: RuntimePreferences = { selected: 'native', codex: { model: 'child-model' } }
    expect(resolveRuntimePreferences(parent, draft)).toEqual(draft)
    delete draft.selected
    expect(resolveRuntimePreferences(parent, draft)).toEqual({ selected: 'codex', codex: { model: 'child-model' } })
    delete draft.codex
    expect(resolveRuntimePreferences(parent, draft)).toEqual(parent)
    expect(parent.codex?.effort).toBe('high')
  })
})

it('replaces CLI effort with an API profile and keeps model IDs distinct from profile IDs', () => {
  const parent: RuntimePreferences = { selected: 'codex', codex: { model: 'same', effort: 'high' } }
  const selected = resolveRuntimePreferences(parent, { codex: runtimeModelFromKey('profile:same') })
  expect(runtimeSelection(selected)).toEqual({ kind: 'codex', codex: { profile_id: 'same' } })
  expect(runtimeModelKey(selected.codex)).toBe('profile:same')
  expect(runtimeModelKey(parent.codex)).toBe('cli:same')
  expect(runtimeSelection(resolveRuntimePreferences(selected, { codex: runtimeModelFromKey('cli:same') }))).toEqual({ kind: 'codex', codex: { model: 'same' } })
})
