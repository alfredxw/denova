import { describe, expect, it } from 'vitest'
import type { Snapshot } from '../../types'
import { buildStoryStateModel } from './model'

describe('buildStoryStateModel', () => {
  it('does not expose legacy empty containers or an empty story context as world facts', () => {
    const snapshot: Snapshot = {
      story_id: 'story',
      branch_id: 'main',
      turns: [],
      state: {
        actors: {
          protagonist: {
            name: '林风',
            role: 'protagonist',
            template_id: 'protagonist',
            state: { 生命: 10 },
          },
          story: {
            name: '故事上下文',
            role: 'story_context',
            template_id: 'story_context',
            state: {
              当前详细地点: '',
              当前事件: '   ',
              当前规则标记: {},
              可承接钩子: [],
            },
          },
        },
        characters: {},
        events: [],
        on_stage: [],
        scene: {},
      },
    }

    const model = buildStoryStateModel(snapshot)

    expect(model.actors.map(([actorId]) => actorId)).toEqual(['protagonist'])
    expect(model.worldFacts).toEqual([])
    expect(model.hasState).toBe(true)
  })

  it('keeps meaningful zero and false values while pruning empty nested world state', () => {
    const snapshot: Snapshot = {
      story_id: 'story',
      branch_id: 'main',
      turns: [],
      state: {
        actors: {
          story: {
            name: '故事上下文',
            role: 'story_context',
            template_id: 'story_context',
            state: {
              当前详细地点: '黄泉酒馆',
              当前事件: '主角观察堂内局势',
              当前场景压力: 0,
              当前规则标记: { 已封锁出口: false, 备注: '' },
              可承接钩子: [],
            },
          },
        },
        scene: { weather: '', visibility: false },
      },
    }

    expect(buildStoryStateModel(snapshot).worldFacts).toEqual([
      ['scene', { visibility: false }],
      ['故事上下文', {
        当前详细地点: '黄泉酒馆',
        当前事件: '主角观察堂内局势',
        当前场景压力: 0,
        当前规则标记: { 已封锁出口: false },
      }],
    ])
  })
})
