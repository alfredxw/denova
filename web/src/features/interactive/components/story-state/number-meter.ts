import type { ActorStateField } from '../../types'

export interface NumberMeterGeometry {
  min: number
  max: number
  startPercent: number
  widthPercent: number
  zeroPercent?: number
  tone: 'standard' | 'negative' | 'positive' | 'neutral'
}

/** Resolves ordinary and zero-centered bounded numbers for every state view. */
export function resolveNumberMeter(field: ActorStateField, value: unknown): NumberMeterGeometry | null {
  if (typeof value !== 'number' || !Number.isFinite(value)
    || typeof field.min !== 'number' || !Number.isFinite(field.min)
    || typeof field.max !== 'number' || !Number.isFinite(field.max)
    || field.max <= field.min) return null

  const position = numberPosition(value, field.min, field.max)
  if (field.min < 0 && field.max > 0) {
    const zero = numberPosition(0, field.min, field.max)
    if (value < 0) {
      return { min: field.min, max: field.max, startPercent: position, widthPercent: zero - position, zeroPercent: zero, tone: 'negative' }
    }
    if (value > 0) {
      return { min: field.min, max: field.max, startPercent: zero, widthPercent: position - zero, zeroPercent: zero, tone: 'positive' }
    }
    return { min: field.min, max: field.max, startPercent: zero, widthPercent: 0, zeroPercent: zero, tone: 'neutral' }
  }
  return { min: field.min, max: field.max, startPercent: 0, widthPercent: position, tone: 'standard' }
}

function numberPosition(value: number, min: number, max: number) {
  return Math.min(100, Math.max(0, ((value - min) / (max - min)) * 100))
}
