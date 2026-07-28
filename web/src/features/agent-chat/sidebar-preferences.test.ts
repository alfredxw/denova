import { describe, expect, it } from 'vitest'
import type { AgentChatProject } from './api'
import {
  orderAgentChatProjects,
  reorderKnownItems,
  type AgentChatSidebarPreferences,
} from './sidebar-preferences'

const projects: AgentChatProject[] = [
  {
    path: '/books/a', name: 'A', current: true, total: 2, error: '',
    sessions: [
      { id: 'a-old', title: 'Old A', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-02T00:00:00Z', message_count: 1, running: false, active: false },
      { id: 'a-new', title: 'New A', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-03-02T00:00:00Z', message_count: 1, running: false, active: false },
    ],
  },
  {
    path: '/books/b', name: 'B', current: false, total: 1, error: '',
    sessions: [
      { id: 'b-new', title: 'New B', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-04-02T00:00:00Z', message_count: 1, running: false, active: false },
    ],
  },
]

function preferences(overrides: Partial<AgentChatSidebarPreferences> = {}): AgentChatSidebarPreferences {
  return {
    sortMode: 'updated',
    pinnedProjects: [],
    manualProjectOrder: [],
    projectOpenedAt: {},
    ...overrides,
  }
}

describe('AgentChat sidebar ordering preferences', () => {
  it('sorts projects by server updates while keeping pinned entries first', () => {
    const value = preferences({
      pinnedProjects: ['/books/a'],
    })

    expect(orderAgentChatProjects(projects, value).map((project) => project.path)).toEqual(['/books/a', '/books/b'])
  })

  it('uses persisted open recency with update time as a deterministic fallback', () => {
    const value = preferences({
      sortMode: 'opened',
      projectOpenedAt: { '/books/a': 200, '/books/b': 100 },
    })

    expect(orderAgentChatProjects(projects, value).map((project) => project.path)).toEqual(['/books/a', '/books/b'])
  })

  it('honours manual order and preserves ids outside the current server window when reordering', () => {
    const value = preferences({
      sortMode: 'manual',
      manualProjectOrder: ['/books/b', '/books/a'],
    })

    expect(orderAgentChatProjects(projects, value).map((project) => project.path)).toEqual(['/books/b', '/books/a'])
    expect(reorderKnownItems(['/books/b', '/books/a'], ['/books/b', '/books/a'], '/books/a', '/books/b'))
      .toEqual(['/books/a', '/books/b'])
  })
})
