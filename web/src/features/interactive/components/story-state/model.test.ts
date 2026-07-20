import { describe, expect, it } from 'vitest'
import type { Snapshot } from '../../types'
import { buildLedgerGroups, buildStoryStateModel, splitLedgerGroupsForPreview } from './model'

describe('buildStoryStateModel', () => {
  it('takes the first two ordered groups for the preview', () => {
    const groups = buildLedgerGroups([
      { id: 'profile', label: '基本身份', field: { name: '基本身份', type: 'string', group: '人物设定' }, value: '游侠' },
      { id: 'panel', label: '面板', field: { name: '面板', type: 'object', group: '面板' }, value: { 力量: 12 } },
      { id: 'state', label: '状态', field: { name: '状态', type: 'object', group: '状态' }, value: { 生命: 10 } },
    ], [])

    const preview = splitLedgerGroupsForPreview(groups)

    expect(preview.preview.map((group) => group.key)).toEqual(['人物设定', '面板'])
    expect(preview.hidden.map((group) => group.key)).toEqual(['状态'])
  })

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
