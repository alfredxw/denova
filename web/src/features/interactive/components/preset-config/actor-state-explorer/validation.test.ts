import { describe, expect, it } from 'vitest'
import { isActorStateExplorerValueValid } from './validation'
import type { ExplorerProps } from './types'

const baseValue: ExplorerProps['value'] = {
  templates: [{
    id: 'protagonist',
    name: '主角',
    fields: [{ id: 'hp', name: '生命', path: 'resources.hp', type: 'number' }],
  }],
  initial_actors: [{ id: 'protagonist', name: '主角', template_id: 'protagonist' }],
  trait_pools: [],
}

describe('isActorStateExplorerValueValid', () => {
  it('rejects missing pools and invalid draw counts in template trait rules', () => {
    expect(isActorStateExplorerValueValid({
      ...baseValue,
      templates: [{ ...baseValue.templates![0], trait_rules: [{ pool_id: 'missing', draw_count: 1 }] }],
    })).toBe(false)

    expect(isActorStateExplorerValueValid({
      ...baseValue,
      templates: [{ ...baseValue.templates![0], trait_rules: [{ pool_id: 'nature', draw_count: 2 }] }],
      trait_pools: [{ id: 'nature', name: '性格', traits: [{ id: 'patient', name: '耐心', weight: 1 }] }],
    })).toBe(false)

    expect(isActorStateExplorerValueValid({
      ...baseValue,
      templates: [{ ...baseValue.templates![0], trait_rules: [{ pool_id: 'nature', draw_count: 1 }] }],
      trait_pools: [{ id: 'nature', name: '性格', traits: [{ id: 'patient', name: '耐心', weight: 1 }] }],
    })).toBe(true)
  })

  it('validates trait identity, weight, and Actor template references', () => {
    expect(isActorStateExplorerValueValid({
      ...baseValue,
      trait_pools: [{
        id: 'pool',
        name: '词条池',
        traits: [{ id: 'trait', name: '词条', weight: 0 }],
      }],
    })).toBe(false)

    expect(isActorStateExplorerValueValid({
      ...baseValue,
      initial_actors: [{ id: 'protagonist', name: '主角', template_id: 'missing' }],
    })).toBe(false)
  })

  it('rejects path separators after Unicode normalization', () => {
    for (const name of ['精神/意志状态', '精神／意志状态']) {
      expect(isActorStateExplorerValueValid({
        ...baseValue,
        templates: [{ ...baseValue.templates![0], fields: [{ name, type: 'string' }] }],
      })).toBe(false)
    }
  })
})
