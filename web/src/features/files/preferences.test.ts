import { beforeEach, describe, expect, it } from 'vitest'
import {
  persistProjectFileEditorPreferences,
  persistProjectFilesPreferences,
  readProjectFileEditorPreferences,
  readProjectFilesPreferences,
  relocateExpandedBranch,
  removeExpandedBranch,
} from './preferences'

describe('project files preferences', () => {
  beforeEach(() => window.localStorage.clear())

  it('isolates layout preferences by stable project identity', () => {
    persistProjectFilesPreferences('one', {
      expandedPaths: ['src', 'src/components'],
      showIgnored: true,
      treeVisible: false,
    })

    expect(readProjectFilesPreferences('one')).toEqual({
      expandedPaths: ['src', 'src/components'],
      showIgnored: true,
      treeVisible: false,
    })
    expect(readProjectFilesPreferences('two')).toEqual({
      expandedPaths: [],
      showIgnored: false,
      treeVisible: true,
    })
  })

  it('keeps editor behavior as a user-level preference with word wrap enabled by default', () => {
    expect(readProjectFileEditorPreferences()).toEqual({ wordWrap: true })
    persistProjectFileEditorPreferences({ wordWrap: false })
    expect(readProjectFileEditorPreferences()).toEqual({ wordWrap: false })
  })

  it('keeps restored expansion paths aligned with directory mutations', () => {
    const paths = ['src', 'src/components', 'docs']
    expect(relocateExpandedBranch(paths, 'src', 'app')).toEqual(['app', 'app/components', 'docs'])
    expect(removeExpandedBranch(paths, 'src')).toEqual(['docs'])
  })
})
