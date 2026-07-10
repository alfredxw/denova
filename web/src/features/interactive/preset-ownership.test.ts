import { describe, expect, it } from 'vitest'
import { GAME_ONLY_PRESET_RESOURCE_KINDS, SHARED_PRESET_RESOURCE_KINDS, presetModuleOwnership, presetResourceVisibleInMode } from './preset-ownership'

describe('preset module ownership', () => {
  it('keeps narrative styles and image presets shared while game orchestration modules stay game-only', () => {
    expect(SHARED_PRESET_RESOURCE_KINDS).toEqual(['teller', 'image'])
    expect(GAME_ONLY_PRESET_RESOURCE_KINDS).toEqual(['director', 'event', 'rule', 'actor-state', 'memory-structure'])
    expect(presetModuleOwnership('teller')).toBe('shared')
    expect(presetModuleOwnership('image')).toBe('shared')
    expect(presetModuleOwnership('director')).toBe('gameOnly')
    expect(presetModuleOwnership('event')).toBe('gameOnly')
    expect(presetModuleOwnership('rule')).toBe('gameOnly')
    expect(presetModuleOwnership('actor-state')).toBe('gameOnly')
    expect(presetModuleOwnership('memory-structure')).toBe('gameOnly')
    expect(presetResourceVisibleInMode('teller', 'writing')).toBe(true)
    expect(presetResourceVisibleInMode('image', 'writing')).toBe(true)
    expect(presetResourceVisibleInMode('event', 'writing')).toBe(false)
    expect(presetResourceVisibleInMode('event', 'game')).toBe(true)
    expect(presetResourceVisibleInMode('memory-structure', 'writing')).toBe(false)
    expect(presetResourceVisibleInMode('memory-structure', 'game')).toBe(true)
  })
})
