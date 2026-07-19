import { beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_STORY_STATE_DISPLAY,
  readStoryStateDisplayPreference,
  STORY_STATE_DISPLAY_STORAGE_KEY,
  writeStoryStateDisplayPreference,
} from './display-preference'

describe('story state display preference', () => {
  beforeEach(() => window.localStorage.clear())

  it('defaults to visible and ignores unknown persisted values', () => {
    expect(readStoryStateDisplayPreference()).toBe(DEFAULT_STORY_STATE_DISPLAY)
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, 'legacy-unknown')
    expect(readStoryStateDisplayPreference()).toBe('visible')
  })

  it('folds legacy preview/expanded values into visible', () => {
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, 'preview')
    expect(readStoryStateDisplayPreference()).toBe('visible')
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, 'expanded')
    expect(readStoryStateDisplayPreference()).toBe('visible')
  })

  it('persists the explicit user choice', () => {
    writeStoryStateDisplayPreference('director-only')
    expect(readStoryStateDisplayPreference()).toBe('director-only')
    writeStoryStateDisplayPreference('collapsed')
    expect(readStoryStateDisplayPreference()).toBe('collapsed')
  })
})
