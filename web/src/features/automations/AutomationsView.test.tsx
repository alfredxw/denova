import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

const reviewTemplate = {
  id: 'review',
  version: 1,
  description: '每 5 个新章节检查连续性、设定、节奏与语言质量。',
  target_kinds: ['workspace'],
  defaults: {
    ...taskBase,
    enabled: false,
    name: '自动 Review',
    template: 'review',
    prompt: '评审新增章节',
    triggers: [{ id: 'chapter_batch_review', type: 'chapter_batch', enabled: true, notify_policy: 'inbox', chapter_batch_size: 5 }],
  },
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
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
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

  it('creates no task until a chosen template draft is saved', async () => {
    const user = userEvent.setup()
    let createdTask: Record<string, unknown> | null = null
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [
        { name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
      http.post('/api/automations', async ({ request }) => {
        createdTask = await request.json() as Record<string, unknown>
        return HttpResponse.json({ ...createdTask, id: 'auto-review', catalog_id: 'workspace-a:auto-review' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)

    expect(await screen.findByText('还没有自动化任务')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
    await user.click(screen.getAllByRole('button', { name: '新建自动化' })[0])
    expect(await screen.findByText('选择起始模板')).toBeInTheDocument()
    expect(createdTask).toBeNull()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('选择起始模板')).not.toBeInTheDocument()
    expect(createdTask).toBeNull()

    await user.click(screen.getAllByRole('button', { name: '新建自动化' })[0])
    await user.click(await screen.findByRole('button', { name: /自动 Review/ }))
    expect(createdTask).toBeNull()
    expect(screen.getByDisplayValue('自动 Review')).toBeInTheDocument()
    expect(screen.getByLabelText('状态')).toHaveValue('false')

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(createdTask).not.toBeNull())
    expect(createdTask).toMatchObject({
      enabled: false,
      name: '自动 Review',
      target: { kind: 'workspace', workspace: '/books/a' },
      triggers: [{ type: 'chapter_batch', chapter_batch_size: 5 }],
    })
  })
})
