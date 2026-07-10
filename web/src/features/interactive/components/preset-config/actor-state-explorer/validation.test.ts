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
  initial_state_ops: [],
}

describe('isActorStateExplorerValueValid', () => {
  it('keeps a newly added empty initial state op invalid until the path is filled', () => {
    expect(isActorStateExplorerValueValid({
      ...baseValue,
      initial_state_ops: [{ op: 'set', path: '', value: '' }],
    })).toBe(false)

    expect(isActorStateExplorerValueValid({
      ...baseValue,
      initial_state_ops: [{ op: 'set', path: 'actors.protagonist.state.resources.hp', value: 5 }],
    })).toBe(true)
  })

  it('validates trait state ops with the same path rules', () => {
    expect(isActorStateExplorerValueValid({
      ...baseValue,
      trait_pools: [{
        id: 'pool',
        name: '词条池',
        traits: [{ id: 'trait', name: '词条', ops: [{ op: 'inc', path: '..bad', value: 1 }] }],
      }],
    })).toBe(false)
  })
})
