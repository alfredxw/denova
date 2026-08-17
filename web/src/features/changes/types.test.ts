import { describe, expect, it } from 'vitest'
import {
  isWorkspaceChangeForWorkspace,
  workspaceChangeImpact,
  workspaceChangePaths,
} from './types'

describe('workspace change event identity', () => {
  it('accepts only events with the active canonical workspace', () => {
    expect(isWorkspaceChangeForWorkspace({ workspace: '/books/current' }, '/books/current')).toBe(true)
    expect(isWorkspaceChangeForWorkspace({ workspace: '/books/old' }, '/books/current')).toBe(false)
    expect(isWorkspaceChangeForWorkspace({}, '/books/current')).toBe(false)
    expect(isWorkspaceChangeForWorkspace(undefined, '/books/current')).toBe(false)
  })

  it('derives the narrowest safe invalidation from canonical file changes', () => {
    expect(workspaceChangeImpact({ changes: [{ path: 'chapters/one.md', type: 'updated' }] })).toBe('content')
    expect(workspaceChangeImpact({ changes: [{ path: 'chapters/two.md', type: 'added' }] })).toBe('structure')
    expect(workspaceChangeImpact({ resync: true, changes: [] })).toBe('structure')
    expect(workspaceChangeImpact({ paths: ['chapters/unknown.md'] })).toBe('structure')
  })

  it('deduplicates every supported workspace-change path field', () => {
    expect(workspaceChangePaths({
      affected_paths: ['chapters/one.md'],
      paths: ['chapters/two.md', 'chapters/one.md'],
      changes: [{ path: 'chapters/three.md', type: 'updated' }],
      path: 'chapters/two.md',
    })).toEqual(['chapters/one.md', 'chapters/two.md', 'chapters/three.md'])
  })
})
