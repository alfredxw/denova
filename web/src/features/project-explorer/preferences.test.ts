import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import {
  readProjectExplorerPreferences,
  useProjectExplorerPreferences,
} from './preferences'

describe('project explorer preferences', () => {
  beforeEach(() => window.localStorage.clear())

  it('isolates state by stable Project identity and seeds Writing defaults only once', () => {
    const { result, unmount } = renderHook(() => useProjectExplorerPreferences('one', ['setting', 'chapters']))
    expect(result.current.preferences.expandedPaths).toEqual(['setting', 'chapters'])

    act(() => result.current.collapseAll())
    unmount()

    expect(readProjectExplorerPreferences('one', ['setting', 'chapters']).expandedPaths).toEqual([])
    expect(readProjectExplorerPreferences('two', ['setting', 'chapters']).expandedPaths).toEqual(['setting', 'chapters'])
  })

  it('keeps expansion paths aligned with directory mutations', () => {
    const { result } = renderHook(() => useProjectExplorerPreferences('one', ['src', 'src/components', 'docs']))

    act(() => result.current.relocateBranch('src', 'app'))
    expect(result.current.preferences.expandedPaths).toEqual(['app', 'app/components', 'docs'])

    act(() => result.current.removeBranch('app'))
    expect(result.current.preferences.expandedPaths).toEqual(['docs'])
  })
})
