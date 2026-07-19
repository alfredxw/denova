import { beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_STORY_STATE_DISPLAY,
  readStoryStateDisplayPreference,
  STORY_STATE_DISPLAY_STORAGE_KEY,
  writeStoryStateDisplayPreference,
} from './display-preference'

describe('story state display preference', () => {
  beforeEach(() => window.localStorage.clear())

  it('defaults to preview and ignores unknown persisted values', () => {
    expect(readStoryStateDisplayPreference()).toBe(DEFAULT_STORY_STATE_DISPLAY)
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, 'legacy-unknown')
    expect(readStoryStateDisplayPreference()).toBe('preview')
  })

  it('folds the short-lived visible value into expanded', () => {
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, 'visible')
    expect(readStoryStateDisplayPreference()).toBe('expanded')
  })

  it('persists the explicit user choice', () => {
    writeStoryStateDisplayPreference('director-only')
    expect(readStoryStateDisplayPreference()).toBe('director-only')
    writeStoryStateDisplayPreference('collapsed')
    expect(readStoryStateDisplayPreference()).toBe('collapsed')
    writeStoryStateDisplayPreference('preview')
    expect(readStoryStateDisplayPreference()).toBe('preview')
  })
})
