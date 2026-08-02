import { beforeEach, describe, expect, it } from 'vitest'
import { persistProjectFilesPreferences, readProjectFilesPreferences } from './preferences'

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
})
