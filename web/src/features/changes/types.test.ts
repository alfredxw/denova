import { describe, expect, it } from 'vitest'
import { isWorkspaceChangeForWorkspace } from './types'

describe('workspace change event identity', () => {
  it('accepts only events with the active canonical workspace', () => {
    expect(isWorkspaceChangeForWorkspace({ workspace: '/books/current' }, '/books/current')).toBe(true)
    expect(isWorkspaceChangeForWorkspace({ workspace: '/books/old' }, '/books/current')).toBe(false)
    expect(isWorkspaceChangeForWorkspace({}, '/books/current')).toBe(false)
    expect(isWorkspaceChangeForWorkspace(undefined, '/books/current')).toBe(false)
  })
})
