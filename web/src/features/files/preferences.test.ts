import { beforeEach, describe, expect, it } from 'vitest'
import {
  persistProjectFileEditorPreferences,
  readProjectFileEditorPreferences,
} from './preferences'

describe('project files preferences', () => {
  beforeEach(() => window.localStorage.clear())

  it('keeps editor behavior as a user-level preference with word wrap enabled by default', () => {
    expect(readProjectFileEditorPreferences()).toEqual({ wordWrap: true })
    persistProjectFileEditorPreferences({ wordWrap: false })
    expect(readProjectFileEditorPreferences()).toEqual({ wordWrap: false })
  })
})
