import { beforeEach, describe, expect, it } from 'vitest'
import { readStoredActivityBarWidth } from './WorkbenchShell'

describe('WorkbenchShell activity bar width', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('uses a wider default so expanded primary menu labels fit', () => {
    expect(readStoredActivityBarWidth()).toBe(180)
  })

  it('migrates the old narrow default width without overwriting custom widths', () => {
    window.localStorage.setItem('nova.layout.activityBarWidth', '152')
    expect(readStoredActivityBarWidth()).toBe(180)

    window.localStorage.setItem('nova.layout.activityBarWidth', '240')
    expect(readStoredActivityBarWidth()).toBe(240)
  })
})
