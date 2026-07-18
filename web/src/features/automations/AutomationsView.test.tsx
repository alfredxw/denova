import { render, screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { AutomationsView } from './AutomationsView'

const taskBase = {
  enabled: true,
  template: 'custom_prompt',
  prompt: '',
  schedule: { kind: 'manual', hour: 9, minute: 0 },
  triggers: [],
  default_action_policy: 'auto_run',
  write_mode: 'read_only',
  write_scope: 'none',
  output_policy: 'run_record_only',
  output_path: '',
  recent_runs: [],
}

describe('AutomationsView', () => {
  it('shows one user catalog grouped by global and every workspace', async () => {
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [
        { name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
        { name: 'Book B', path: '/books/b', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [
        { ...taskBase, id: 'same', catalog_id: 'workspace-a:same', scope: 'workspace', name: 'Review A', target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' } },
        { ...taskBase, id: 'same', catalog_id: 'workspace-b:same', scope: 'workspace', name: 'Review B', target: { kind: 'workspace', workspace: '/books/b', workspace_id: 'workspace-b' } },
        { ...taskBase, id: 'global', catalog_id: 'global', scope: 'user', name: 'Global research', target: { kind: 'user' } },
      ] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
    )

    render(<AutomationsView workspace="/books/a" />)

    expect(await screen.findByText('Global research')).toBeInTheDocument()
    expect(screen.getByText('Book A')).toBeInTheDocument()
    expect(screen.getByText('Book B')).toBeInTheDocument()
    expect(screen.getAllByText('Review A').length).toBeGreaterThan(0)
    expect(screen.getByText('Review B')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '工作区' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '用户' })).not.toBeInTheDocument()
  })
})
