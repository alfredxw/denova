import { describe, expect, it } from 'vitest'
import { presetResourceVisibleInMode } from './preset-ownership'

describe('preset resource visibility', () => {
  it('shows shared resources in both creative destinations', () => {
    expect(presetResourceVisibleInMode('teller', 'writing')).toBe(true)
    expect(presetResourceVisibleInMode('image', 'game')).toBe(true)
  })

  it('keeps dedicated resources in their owning destination', () => {
    expect(presetResourceVisibleInMode('director', 'writing')).toBe(false)
    expect(presetResourceVisibleInMode('director', 'game')).toBe(true)
  })
})
