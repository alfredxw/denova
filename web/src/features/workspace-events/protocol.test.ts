import { describe, expect, it, vi } from 'vitest'

import { parseWorkspaceChangeSSE } from '@/features/workspace-events/protocol'

describe('Project workspace event protocol', () => {
  it('normalizes watcher changes and derives unique paths', () => {
    expect(parseWorkspaceChangeSSE({
      event: 'workspace-change',
      data: JSON.stringify({
        project_id: 'project-demo',
        workspace: '/books/demo',
        source: 'watcher',
        changes: [
          { path: 'chapters/ch01.md', type: 'updated' },
          { path: 'chapters/ch02.md', type: 'added' },
        ],
        paths: ['chapters/ch01.md'],
      }),
    })).toEqual({
      project_id: 'project-demo',
      workspace: '/books/demo',
      source: 'watcher',
      resync: false,
      changes: [
        { path: 'chapters/ch01.md', type: 'updated' },
        { path: 'chapters/ch02.md', type: 'added' },
      ],
      paths: ['chapters/ch01.md', 'chapters/ch02.md'],
    })
  })

  it('ignores malformed and unrelated SSE frames', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(parseWorkspaceChangeSSE({ event: 'heartbeat', data: '{}' })).toBeNull()
    expect(parseWorkspaceChangeSSE({ event: 'workspace-change', data: '{' })).toBeNull()
    expect(parseWorkspaceChangeSSE({ event: 'workspace-change', data: '{}' })).toBeNull()
    expect(warn).toHaveBeenCalledTimes(1)
  })

  it('keeps resync pathless so Project consumers reload canonical state', () => {
    expect(parseWorkspaceChangeSSE({
      event: 'workspace-change',
      data: JSON.stringify({ project_id: 'project-demo', workspace: '/books/demo', source: 'watcher', resync: true }),
    })).toEqual({
      project_id: 'project-demo',
      workspace: '/books/demo',
      source: 'watcher',
      resync: true,
      changes: [],
    })
  })
})
