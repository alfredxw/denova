import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import i18n from '@/i18n'
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
    const user = userEvent.setup()
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

    const bookBGroup = screen.getByRole('button', { name: /Book B/ })
    expect(bookBGroup).toHaveAttribute('aria-expanded', 'true')
    await user.click(bookBGroup)
    expect(bookBGroup).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Review B')).not.toBeInTheDocument()
    expect(screen.getAllByText('Review A').length).toBeGreaterThan(0)

    await user.click(bookBGroup)
    expect(bookBGroup).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Review B')).toBeInTheDocument()
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
    expect(screen.getByRole('switch', { name: '状态' })).toHaveAttribute('data-state', 'unchecked')

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(createdTask).not.toBeNull())
    expect(createdTask).toMatchObject({
      enabled: false,
      name: '自动 Review',
      target: { kind: 'workspace', workspace: '/books/a' },
      triggers: [{ type: 'chapter_batch', chapter_batch_size: 5 }],
    })
  })

  it('keeps the newest workspace result when an earlier load resolves last', async () => {
    const firstLoad = deferred<Array<Record<string, unknown>>>()
    let automationRequests = 0
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [
        { name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
        { name: 'Book B', path: '/books/b', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', async () => {
        automationRequests += 1
        if (automationRequests === 1) {
          return HttpResponse.json({ tasks: await firstLoad.promise })
        }
        return HttpResponse.json({ tasks: [{
          ...taskBase,
          id: 'workspace-b',
          catalog_id: 'workspace-b:workspace-b',
          scope: 'workspace',
          name: 'Newest workspace task',
          target: { kind: 'workspace', workspace: '/books/b', workspace_id: 'workspace-b' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
    )

    const view = render(<AutomationsView workspace="/books/a" />)
    await waitFor(() => expect(automationRequests).toBe(1))
    view.rerender(<AutomationsView workspace="/books/b" />)

    expect((await screen.findAllByText('Newest workspace task')).length).toBeGreaterThan(0)
    await act(async () => {
      firstLoad.resolve([{
        ...taskBase,
        id: 'workspace-a',
        catalog_id: 'workspace-a:workspace-a',
        scope: 'workspace',
        name: 'Stale workspace task',
        target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
      }])
      await firstLoad.promise
      await Promise.resolve()
    })

    expect(screen.getAllByText('Newest workspace task').length).toBeGreaterThan(0)
    expect(screen.queryByText('Stale workspace task')).not.toBeInTheDocument()
  })

  it('keeps an unsaved task draft when a background reload completes', async () => {
    const user = userEvent.setup()
    const previousLanguage = i18n.language
    let automationRequests = 0
    let activeRunResponses = 0
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [
        { name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', () => {
        automationRequests += 1
        return HttpResponse.json({ tasks: [{
          ...taskBase,
          id: 'draft-protection',
          catalog_id: 'workspace-a:draft-protection',
          scope: 'workspace',
          name: 'Server task name',
          target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => {
        if (activeRunResponses <= 0) return HttpResponse.json({ runs: [] })
        activeRunResponses -= 1
        return HttpResponse.json({ runs: [{
          task_id: 'draft-protection',
          run: {
            id: 'background-run',
            task_id: 'draft-protection',
            scope: 'workspace',
            workspace: '/books/a',
            trigger: 'schedule',
            status: 'running',
            started_at: '2026-07-18T12:00:00Z',
            summary: '',
            tool_manifest: [],
          },
        }] })
      }),
      http.get('/api/automations/runs/background-run/stream', () => HttpResponse.text('', {
        headers: { 'Content-Type': 'text/event-stream' },
      })),
    )

    try {
      render(<AutomationsView workspace="/books/a" />)
      const nameInput = await screen.findByDisplayValue('Server task name')
      await user.clear(nameInput)
      await user.type(nameInput, 'Unsaved local name')

      // One response is consumed by load(); the other exercises active-run resume.
      activeRunResponses = 2
      await act(async () => {
        await i18n.changeLanguage(previousLanguage === 'en-US' ? 'zh-CN' : 'en-US')
      })
      await waitFor(() => expect(automationRequests).toBeGreaterThanOrEqual(2))
      await user.click(screen.getByRole('button', { name: /任务配置|Task Config/ }))

      expect(screen.getByDisplayValue('Unsaved local name')).toBeInTheDocument()
      expect(screen.queryByDisplayValue('Server task name')).not.toBeInTheDocument()
    } finally {
      await act(async () => { await i18n.changeLanguage(previousLanguage) })
    }
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
