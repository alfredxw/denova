import { describe, expect, it } from 'vitest'
import type { ActorStateField } from '../../types'
import { resolveNumberMeter } from './number-meter'

function field(min: number, max: number): ActorStateField {
  return { name: '数值', type: 'number', min, max }
}

describe('resolveNumberMeter', () => {
  it('fills an ordinary bounded meter from its minimum', () => {
    expect(resolveNumberMeter(field(0, 100), 25)).toEqual({
      min: 0, max: 100, startPercent: 0, widthPercent: 25, tone: 'standard',
    })
  })

  it('draws negative and positive values away from zero', () => {
    expect(resolveNumberMeter(field(-100, 100), -40)).toEqual({
      min: -100, max: 100, startPercent: 30, widthPercent: 20, zeroPercent: 50, tone: 'negative',
    })
    expect(resolveNumberMeter(field(-100, 100), 60)).toEqual({
      min: -100, max: 100, startPercent: 50, widthPercent: 30, zeroPercent: 50, tone: 'positive',
    })
    expect(resolveNumberMeter(field(-100, 100), 0)).toEqual({
      min: -100, max: 100, startPercent: 50, widthPercent: 0, zeroPercent: 50, tone: 'neutral',
    })
  })

  it('rejects unbounded, invalid, and non-numeric values', () => {
    expect(resolveNumberMeter({ name: '等级', type: 'number', min: 1 }, 10)).toBeNull()
    expect(resolveNumberMeter(field(10, 10), 10)).toBeNull()
    expect(resolveNumberMeter(field(0, 100), '10')).toBeNull()
  })
})
