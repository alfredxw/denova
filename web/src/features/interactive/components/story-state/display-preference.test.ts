import { beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_STORY_STATE_DISPLAY,
  readStoryStateDisplayPreference,
  STORY_STATE_DISPLAY_STORAGE_KEY,
  writeStoryStateDisplayPreference,
} from './display-preference'

describe('story state display preference', () => {
  beforeEach(() => window.localStorage.clear())

  it('defaults to an adaptive preview and ignores unknown persisted values', () => {
    expect(readStoryStateDisplayPreference()).toBe(DEFAULT_STORY_STATE_DISPLAY)
    window.localStorage.setItem(STORY_STATE_DISPLAY_STORAGE_KEY, 'legacy-expanded')
    expect(readStoryStateDisplayPreference()).toBe('preview')
  })

  it('persists the explicit user choice', () => {
    writeStoryStateDisplayPreference('director-only')
    expect(readStoryStateDisplayPreference()).toBe('director-only')
  })
})
