import { describe, expect, it } from 'vitest'
import type { ActorStateField } from '../../types'
import { resolveStateFieldLayout } from './field-layout'

function field(partial: Partial<ActorStateField> & Pick<ActorStateField, 'name' | 'type'>): ActorStateField {
  return partial
}

function historicalField(value: Record<string, unknown>): ActorStateField {
  return value as unknown as ActorStateField
}

describe('resolveStateFieldLayout', () => {
  it('routes bounded numbers to the stat renderer in the overview group', () => {
    const layout = resolveStateFieldLayout(field({ name: '生命', type: 'number', min: 0, max: 10 }), 7)
    expect(layout).toEqual({ renderer: 'stat', group: 'overview', customGroup: false })
  })

  it('routes unbounded numbers and short strings to inline overview fields', () => {
    expect(resolveStateFieldLayout(field({ name: '年龄', type: 'number' }), 23).renderer).toBe('inline')
    expect(resolveStateFieldLayout(field({ name: '宗门', type: 'string' }), '散修').renderer).toBe('inline')
    expect(resolveStateFieldLayout(field({ name: '已觉醒', type: 'bool' }), true).renderer).toBe('inline')
    expect(resolveStateFieldLayout(field({ name: '宗门', type: 'string' }), '散修').group).toBe('overview')
  })

  it('routes long strings to the block renderer in the details group', () => {
    const layout = resolveStateFieldLayout(field({ name: '当前处境', type: 'string' }), '左臂骨裂需要七天时间休养，与六天之后的采药任务直接冲突，单手采药效率至少减半。')
    expect(layout).toEqual({ renderer: 'block', group: 'details', customGroup: false })
  })

  it('routes primitive lists to list and object-bearing values to object in holdings', () => {
    expect(resolveStateFieldLayout(field({ name: '功法', type: 'list' }), ['敛息诀'])).toEqual({ renderer: 'list', group: 'holdings', customGroup: false })
    expect(resolveStateFieldLayout(field({ name: '图鉴', type: 'list' }), [{ 名称: '敛息诀' }])).toEqual({ renderer: 'object', group: 'holdings', customGroup: false })
    expect(resolveStateFieldLayout(field({ name: '储物袋', type: 'object' }), { 灵石: 9 })).toEqual({ renderer: 'object', group: 'holdings', customGroup: false })
  })

  it('ignores historical spoiler metadata while preserving custom group strings', () => {
    const legacySpoiler = resolveStateFieldLayout(historicalField({ name: '隐藏风险', type: 'list', visibility: 'spoiler' }), ['被追踪'])
    expect(legacySpoiler).toEqual({ renderer: 'list', group: 'holdings', customGroup: false })
    const declared = resolveStateFieldLayout(historicalField({ name: '隐藏风险', type: 'list', visibility: 'spoiler', group: 'spoiler' }), ['被追踪'])
    expect(declared).toEqual({ renderer: 'list', group: 'spoiler', customGroup: true })
    const localized = resolveStateFieldLayout(field({ name: '隐藏风险', type: 'list', group: '暗线' }), ['被追踪'])
    expect(localized).toEqual({ renderer: 'list', group: '暗线', customGroup: true })
  })

  it('honors display hints with graceful fallback', () => {
    expect(resolveStateFieldLayout(field({ name: '当前处境', type: 'string', display: 'block' }), '短').renderer).toBe('block')
    expect(resolveStateFieldLayout(field({ name: '生命', type: 'number', min: 0, max: 10, display: 'stat' }), 7).renderer).toBe('stat')
    expect(resolveStateFieldLayout(field({ name: '年龄', type: 'number', display: 'stat' }), 23).renderer).toBe('inline')
    expect(resolveStateFieldLayout(field({ name: '当前处境', type: 'string', display: 'inline' }), '这是一段明确超过二十四字符阈值的长文本内容，用于测试展示提示覆盖。').renderer).toBe('inline')
  })

  it('infers layout from value shape when no schema field exists', () => {
    expect(resolveStateFieldLayout(undefined, 9).renderer).toBe('inline')
    expect(resolveStateFieldLayout(undefined, '暴雨将至，城外山路被洪水冲断无法通行，南北商队全部滞留在驿站之中。').renderer).toBe('block')
    expect(resolveStateFieldLayout(undefined, ['a', 'b']).renderer).toBe('list')
    expect(resolveStateFieldLayout(undefined, { weather: '雨' }).renderer).toBe('object')
  })
})
