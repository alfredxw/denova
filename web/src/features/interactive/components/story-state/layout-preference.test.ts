import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerFieldGroup } from './model'
import {
  applyStoryStateLayout,
  defaultStoryStateLayout,
  moveStoryStateLayoutField,
  readStoryStateLayouts,
  writeStoryStateTemplateLayout,
} from './layout-preference'

function group(key: string, fieldIds: string[], custom = false): LedgerFieldGroup {
  return {
    key,
    custom,
    fields: fieldIds.map((id) => ({ id, label: id, value: id, renderer: 'inline', change: null })),
  }
}

describe('story state template layout preference', () => {
  beforeEach(() => window.localStorage.clear())

  it('reconciles saved order with added and removed schema fields', () => {
    const defaults = [
      group('overview', ['name', 'level', 'new-field']),
      group('holdings', ['items']),
      group('details', ['situation']),
    ]
    const saved = {
      groups: [
        { key: 'holdings', field_ids: ['level', 'items', 'removed-field'] },
        { key: 'overview', field_ids: ['name'] },
        { key: 'removed-group', field_ids: ['ghost'] },
      ],
    }

    const applied = applyStoryStateLayout(defaults, saved)

    expect(applied.map((item) => item.key)).toEqual(['holdings', 'overview', 'details'])
    expect(applied[0].fields.map((field) => field.id)).toEqual(['level', 'items'])
    expect(applied[1].fields.map((field) => field.id)).toEqual(['name', 'new-field'])
    expect(applied[2].fields.map((field) => field.id)).toEqual(['situation'])
  })

  it('moves a field across groups without mutating the defaults', () => {
    const defaults = [group('overview', ['level', 'strength']), group('holdings', ['items'])]
    const initial = defaultStoryStateLayout(defaults)

    const moved = moveStoryStateLayoutField(initial, 'strength', 'holdings', 0)

    expect(moved.groups).toEqual([
      { key: 'overview', field_ids: ['level'] },
      { key: 'holdings', field_ids: ['strength', 'items'] },
    ])
    expect(defaults[0].fields.map((field) => field.id)).toEqual(['level', 'strength'])
  })

  it('keeps the layout intact when a stale drag target group no longer exists', () => {
    const initial = defaultStoryStateLayout([
      group('overview', ['level', 'strength']),
      group('holdings', ['items']),
    ])

    expect(moveStoryStateLayoutField(initial, 'strength', 'removed-group', 0)).toEqual(initial)
  })

  it('persists layouts by story and template and removes only the reset template', () => {
    const hero = { groups: [{ key: 'overview', field_ids: ['strength'] }] }
    const npc = { groups: [{ key: 'overview', field_ids: ['favorability'] }] }

    writeStoryStateTemplateLayout('story-a', 'protagonist', hero)
    writeStoryStateTemplateLayout('story-a', 'important_character', npc)
    writeStoryStateTemplateLayout('story-b', 'protagonist', npc)

    expect(readStoryStateLayouts('story-a')).toEqual({ protagonist: hero, important_character: npc })
    expect(readStoryStateLayouts('story-b')).toEqual({ protagonist: npc })

    writeStoryStateTemplateLayout('story-a', 'protagonist', null)
    expect(readStoryStateLayouts('story-a')).toEqual({ important_character: npc })
    expect(readStoryStateLayouts('story-b')).toEqual({ protagonist: npc })
  })
})
