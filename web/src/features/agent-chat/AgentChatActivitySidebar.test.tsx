import { useLayoutEffect } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { AgentChatActivitySidebar, type AgentChatActivitySidebarProps } from './AgentChatActivitySidebar'
import type { AgentChatProject } from './api'

const commits = vi.hoisted(() => [] as { id: string; expanded: boolean }[])

vi.mock('./AgentChatSidebarProject', () => ({
  projectSortableID: (id: string) => id,
  AgentChatSidebarProject: ({ project, expanded, onToggle }: {
    project: AgentChatProject
    expanded: boolean
    onToggle: () => void
  }) => {
    // Observe every committed state, including the commit before parent layout effects.
    useLayoutEffect(() => { commits.push({ id: project.id, expanded }) })
    return <button aria-expanded={expanded} onClick={onToggle}>{project.id}</button>
  },
}))

function project(id: string): AgentChatProject {
  return { id, name: id, path: id, type: 'book', status: 'available', current: false, total: 0, sessions: [] }
}

function props(projects: AgentChatProject[] = []): AgentChatActivitySidebarProps {
  return {
    projects, activitiesByProject: new Map(), loading: false, error: '', activeProjectId: 'a',
    onSelectProject: vi.fn(), onOpenActivity: vi.fn(), onOpenSession: vi.fn(),
    onRenameSession: vi.fn(), onCreateSession: vi.fn(), onOpenHistory: vi.fn(),
    onAddProject: vi.fn(), projectDirectoryBusy: false, onRenameProject: vi.fn(),
    onRelinkProject: vi.fn(), onArchiveProject: vi.fn(),
  }
}

beforeEach(() => { commits.length = 0; localStorage.clear() })

it.each([false, true])('commits the intended expansion immediately (asynchronous projects: %s)', (asyncProjects) => {
  const initial = props(asyncProjects ? [] : [project('a'), project('b')])
  const view = render(<AgentChatActivitySidebar {...initial} />)
  if (asyncProjects) view.rerender(<AgentChatActivitySidebar {...initial} projects={[project('a'), project('b')]} />)
  expect(commits.filter(({ id }) => id === 'a').every(({ expanded }) => expanded)).toBe(true)
  expect(commits.filter(({ id }) => id === 'b')).not.toHaveLength(0)
  expect(commits.filter(({ id }) => id === 'b').every(({ expanded }) => !expanded)).toBe(true)
})

it('preserves row toggles across selection and refresh, while external selection expands the project', () => {
  const initial = props([project('a'), project('b')])
  const view = render(<AgentChatActivitySidebar {...initial} />)
  fireEvent.click(screen.getByRole('button', { name: 'b' }))
  expect(initial.onSelectProject).toHaveBeenCalledWith(initial.projects[1])
  view.rerender(<AgentChatActivitySidebar {...initial} activeProjectId="b" />)
  expect(screen.getByRole('button', { name: 'b' })).toHaveAttribute('aria-expanded', 'true')
  fireEvent.click(screen.getByRole('button', { name: 'b' }))
  commits.length = 0
  view.rerender(<AgentChatActivitySidebar {...initial} activeProjectId="b" projects={[project('a'), project('b'), project('c')]} />)
  expect(screen.getByRole('button', { name: 'b' })).toHaveAttribute('aria-expanded', 'false')
  expect(commits.filter(({ id }) => id === 'c').every(({ expanded }) => !expanded)).toBe(true)
  view.rerender(<AgentChatActivitySidebar {...initial} activeProjectId="a" />)
  view.rerender(<AgentChatActivitySidebar {...initial} activeProjectId="b" />)
  expect(screen.getByRole('button', { name: 'b' })).toHaveAttribute('aria-expanded', 'true')
})
