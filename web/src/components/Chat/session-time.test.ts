import { describe, expect, it } from 'vitest'
import { formatCompactSessionTime } from './session-time'

describe('formatCompactSessionTime', () => {
  const now = Date.UTC(2026, 7, 29, 12, 0, 0)

  it('uses compact seconds, minutes, hours and days for recent sessions', () => {
    expect(formatCompactSessionTime(now - 1_000, now)).toBe('1s')
    expect(formatCompactSessionTime(now - 11 * 60_000, now)).toBe('11m')
    expect(formatCompactSessionTime(now - 22 * 60 * 60_000, now)).toBe('22h')
    expect(formatCompactSessionTime(now - 3 * 24 * 60 * 60_000, now)).toBe('3d')
  })

  it('uses a localized short date for older sessions and rejects invalid values', () => {
    expect(formatCompactSessionTime(now - 8 * 24 * 60 * 60_000, now)).not.toMatch(/^\d+[smhd]$/)
    expect(formatCompactSessionTime('invalid', now)).toBe('')
  })
})
