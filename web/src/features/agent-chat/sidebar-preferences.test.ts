import { describe, expect, it } from 'vitest'
import type { AgentChatProject } from './api'
import {
  orderAgentChatProjects,
  orderAgentChatSessions,
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
    pinnedSessions: {},
    manualProjectOrder: [],
    manualSessionOrder: {},
    projectOpenedAt: {},
    sessionOpenedAt: {},
    ...overrides,
  }
}

describe('AgentChat sidebar ordering preferences', () => {
  it('sorts both levels by server updates while keeping pinned entries first', () => {
    const value = preferences({
      pinnedProjects: ['/books/a'],
      pinnedSessions: { '/books/a': ['a-old'] },
    })

    expect(orderAgentChatProjects(projects, value).map((project) => project.path)).toEqual(['/books/a', '/books/b'])
    expect(orderAgentChatSessions(projects[0], value).map((session) => session.id)).toEqual(['a-old', 'a-new'])
  })

  it('uses persisted open recency with update time as a deterministic fallback', () => {
    const value = preferences({
      sortMode: 'opened',
      projectOpenedAt: { '/books/a': 200, '/books/b': 100 },
      sessionOpenedAt: { '/books/a': { 'a-old': 300 } },
    })

    expect(orderAgentChatProjects(projects, value).map((project) => project.path)).toEqual(['/books/a', '/books/b'])
    expect(orderAgentChatSessions(projects[0], value).map((session) => session.id)).toEqual(['a-old', 'a-new'])
  })

  it('honours manual order and preserves ids outside the current server window when reordering', () => {
    const value = preferences({
      sortMode: 'manual',
      manualProjectOrder: ['/books/b', '/books/a'],
      manualSessionOrder: { '/books/a': ['a-new', 'a-old'] },
    })

    expect(orderAgentChatProjects(projects, value).map((project) => project.path)).toEqual(['/books/b', '/books/a'])
    expect(orderAgentChatSessions(projects[0], value).map((session) => session.id)).toEqual(['a-new', 'a-old'])
    expect(reorderKnownItems(['hidden', 'a-new', 'a-old'], ['a-new', 'a-old'], 'a-old', 'a-new'))
      .toEqual(['hidden', 'a-old', 'a-new'])
  })
})
