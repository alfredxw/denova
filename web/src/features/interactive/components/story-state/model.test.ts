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

  it('separates archived Actors from active tabs and classifies lifecycle deltas', () => {
    const snapshot: Snapshot = {
      story_id: 'story',
      branch_id: 'main',
      turns: [],
      state: {
        actors: {
          protagonist: { name: '林风', role: 'protagonist', template_id: 'hero', state: { 状态: '警惕' } },
          wolf: { name: '赤瞳狼王', role: 'opponent', template_id: 'enemy', state: { 遗言: '完整归档状态不应进入活动页' } },
        },
        actor_archives: { wolf: { reason: '本回合已确认死亡', source_turn_id: 'turn-death' } },
      },
      current_turn: {
        id: 'turn-death', parent_id: null, branch_id: 'main', ts: '2026-07-21T00:00:00Z', user: '结束战斗', narrative: '狼王倒下。',
        state_delta: { ops: [{ op: 'set', path: 'actor_archives.wolf', value: { reason: '本回合已确认死亡', source_turn_id: 'turn-death' }, reason: '本回合已确认死亡' }] },
      },
    }

    const model = buildStoryStateModel(snapshot)

    expect(model.actors.map(([actorId]) => actorId)).toEqual(['protagonist'])
    expect(model.archivedActors).toMatchObject([{ actorId: 'wolf', name: '赤瞳狼王', reason: '本回合已确认死亡', sourceTurnId: 'turn-death' }])
    expect(model.worldFacts).toEqual([])
    expect(model.changes).toMatchObject([{ actorId: 'wolf', op: 'archive', path: '', reason: '本回合已确认死亡' }])
    expect(model.hasState).toBe(true)
  })
})
