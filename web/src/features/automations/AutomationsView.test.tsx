import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
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

    await user.click(screen.getByRole('button', { name: '创建' }))
    await waitFor(() => expect(createdTask).not.toBeNull())
    expect(createdTask).toMatchObject({
      enabled: false,
      name: '自动 Review',
      target: { kind: 'workspace', workspace: '/books/a' },
      triggers: [{ type: 'chapter_batch', chapter_batch_size: 5 }],
    })
  })

  it('keeps edits made while a new task creation request is in flight', async () => {
    const user = userEvent.setup()
    const createGate = deferred<void>()
    let submitted: Record<string, unknown> | null = null
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
      http.post('/api/automations', async ({ request }) => {
        submitted = await request.json() as Record<string, unknown>
        await createGate.promise
        return HttpResponse.json({ ...submitted, id: 'created-race', catalog_id: 'workspace-a:created-race', revision: 'rev-1' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)
    expect(await screen.findByText('还没有自动化任务')).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: '新建自动化' })[0])
    await user.click(await screen.findByRole('button', { name: /自动 Review/ }))
    await user.click(screen.getByRole('button', { name: '创建' }))
    await waitFor(() => expect(submitted).not.toBeNull())

    const name = screen.getByDisplayValue('自动 Review')
    await user.clear(name)
    await user.type(name, 'Edited while creating')
    await act(async () => {
      createGate.resolve()
      await createGate.promise
    })

    expect(await screen.findByDisplayValue('Edited while creating')).toBeInTheDocument()
  })

  it('autosaves existing task configuration without sending runtime state', async () => {
    const user = userEvent.setup()
    let updateBody: Record<string, unknown> | null = null
    const existing = {
      ...taskBase,
      id: 'review',
      catalog_id: 'workspace-a:review',
      revision: 'rev-1',
      scope: 'workspace',
      name: 'Review',
      target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
      trigger_state: { schedule: { last_checked_at: 'today' } },
      last_run: { id: 'run-1' },
      recent_runs: [{ id: 'run-1' }],
    }
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [existing] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
      http.patch('/api/automations/:id', async ({ request }) => {
        updateBody = await request.json() as Record<string, unknown>
        return HttpResponse.json({ ...existing, ...updateBody, revision: 'rev-2', updated_at: '2026-07-18T12:00:00Z' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)

    const name = await screen.findByDisplayValue('Review')
    await user.clear(name)
    await user.type(name, 'Review latest chapters')
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
    fireEvent.keyDown(screen.getByRole('heading', { level: 2, name: '自动化' }), { key: 's', ctrlKey: true })

    await waitFor(() => expect(updateBody).not.toBeNull())
    expect(updateBody).toMatchObject({ name: 'Review latest chapters' })
    expect(updateBody).toHaveProperty('base_revision', 'rev-1')
    expect(updateBody).not.toHaveProperty('trigger_state')
    expect(updateBody).not.toHaveProperty('last_run')
    expect(updateBody).not.toHaveProperty('recent_runs')
  })

  it('rebases a stale save over the latest task, archives overlaps, and retries with local preference', async () => {
    const user = userEvent.setup()
    const baseline = {
      ...taskBase,
      id: 'review',
      catalog_id: 'workspace-a:review',
      revision: 'rev-1',
      scope: 'workspace',
      name: 'Review',
      prompt: 'original prompt',
      target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
    }
    const external = { ...baseline, revision: 'rev-2', name: 'Agent review', prompt: 'agent prompt' }
    let listRequests = 0
    const patchBodies: Record<string, unknown>[] = []
    let archived: Record<string, unknown> | null = null
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => {
        listRequests += 1
        return HttpResponse.json({ tasks: [listRequests === 1 ? baseline : external] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
      http.post('/api/autosave-conflicts', async ({ request }) => {
        archived = await request.json() as Record<string, unknown>
        return HttpResponse.json({ id: 'conflict-1', path: '/conflicts/conflict-1.json' }, { status: 201 })
      }),
      http.patch('/api/automations/:id', async ({ request }) => {
        const body = await request.json() as Record<string, unknown>
        patchBodies.push(body)
        if (patchBodies.length === 1) {
          return HttpResponse.json({ error: 'stale revision', code: 'revision_conflict' }, { status: 409 })
        }
        return HttpResponse.json({ ...external, ...body, revision: 'rev-3' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)

    const name = await screen.findByDisplayValue('Review')
    await user.clear(name)
    await user.type(name, 'Local review')
    fireEvent.keyDown(screen.getByRole('heading', { level: 2, name: '自动化' }), { key: 's', ctrlKey: true })

    await waitFor(() => expect(patchBodies).toHaveLength(2))
    expect(patchBodies[0]).toMatchObject({ name: 'Local review', prompt: 'original prompt', base_revision: 'rev-1' })
    expect(patchBodies[1]).toMatchObject({ name: 'Local review', prompt: 'agent prompt', base_revision: 'rev-2' })
    expect(archived).toMatchObject({
      resource: 'automation',
      id: 'workspace-a:review',
      strategy: 'merge_non_overlap_prefer_local',
      conflict_paths: [['name']],
    })
    expect(screen.getByDisplayValue('Local review')).toBeInTheDocument()
  })

  it('flushes a pending task edit before opening the delete confirmation', async () => {
    const user = userEvent.setup()
    const saveGate = deferred<void>()
    let patchStarted = false
    const existing = {
      ...taskBase,
      id: 'review',
      catalog_id: 'workspace-a:review',
      scope: 'workspace',
      name: 'Review',
      target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
    }
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [existing] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
      http.patch('/api/automations/:id', async ({ request }) => {
        patchStarted = true
        const update = await request.json() as Record<string, unknown>
        await saveGate.promise
        return HttpResponse.json({ ...existing, ...update, updated_at: '2026-07-18T12:00:00Z' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)

    const name = await screen.findByDisplayValue('Review')
    await user.clear(name)
    await user.type(name, 'Review before delete')
    await user.click(screen.getByRole('button', { name: '删除任务' }))

    expect(screen.queryByRole('heading', { name: '删除自动化任务' })).not.toBeInTheDocument()
    await waitFor(() => expect(patchStarted).toBe(true))

    await act(async () => {
      saveGate.resolve()
      await saveGate.promise
    })
    expect(await screen.findByRole('heading', { name: '删除自动化任务' })).toBeInTheDocument()
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
          prompt: automationRequests === 1 ? 'Initial server prompt' : 'Externally updated prompt',
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
      await waitFor(() => expect(screen.getByRole('textbox', { name: 'Prompt' })).toHaveValue('Externally updated prompt'))
    } finally {
      await act(async () => { await i18n.changeLanguage(previousLanguage) })
    }
  })

  it('reloads an externally changed automation file and keeps non-overlapping local edits', async () => {
    const user = userEvent.setup()
    let automationRequests = 0
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => {
        automationRequests += 1
        return HttpResponse.json({ tasks: [{
          ...taskBase,
          id: 'external-reload',
          catalog_id: 'workspace-a:external-reload',
          revision: automationRequests === 1 ? 'rev-1' : 'rev-2',
          scope: 'workspace',
          name: 'Server name',
          prompt: automationRequests === 1 ? 'Initial prompt' : 'Agent prompt',
          target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
    )

    render(<AutomationsView workspace="/books/a" />)
    const name = await screen.findByDisplayValue('Server name')
    await user.clear(name)
    await user.type(name, 'Local name')

    act(() => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', {
        detail: { workspace: '/books/a', paths: ['.nova/automations/tasks.json'] },
      }))
    })

    await waitFor(() => expect(automationRequests).toBeGreaterThanOrEqual(2))
    expect(screen.getByDisplayValue('Local name')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('textbox', { name: /提示词|Prompt/ })).toHaveValue('Agent prompt'))
  })

  it('does not overwrite edits made while an overlapping reload is being archived', async () => {
    const user = userEvent.setup()
    const previousLanguage = i18n.language
    const archiveGate = deferred<void>()
    let archiveStarted = false
    let automationRequests = 0
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [
        { name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', () => {
        automationRequests += 1
        return HttpResponse.json({ tasks: [{
          ...taskBase,
          id: 'archive-race',
          catalog_id: 'workspace-a:archive-race',
          revision: automationRequests === 1 ? 'rev-1' : 'rev-2',
          scope: 'workspace',
          name: automationRequests === 1 ? 'Server name' : 'Agent name',
          prompt: automationRequests === 1 ? 'Initial prompt' : 'Agent prompt',
          target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.get('/api/automations/runs/active', () => HttpResponse.json({ runs: [] })),
      http.post('/api/autosave-conflicts', async () => {
        archiveStarted = true
        await archiveGate.promise
        return HttpResponse.json({ id: 'conflict-race', path: '/conflicts/conflict-race.json' }, { status: 201 })
      }),
      http.patch('/api/automations/:id', async ({ request }) => {
        const body = await request.json() as Record<string, unknown>
        return HttpResponse.json({ ...body, id: 'archive-race', catalog_id: 'workspace-a:archive-race', revision: 'rev-3', scope: 'workspace', target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' }, recent_runs: [] })
      }),
    )

    try {
      render(<AutomationsView workspace="/books/a" />)
      const name = await screen.findByDisplayValue('Server name')
      await user.clear(name)
      await user.type(name, 'Local name')

      await act(async () => {
        await i18n.changeLanguage(previousLanguage === 'en-US' ? 'zh-CN' : 'en-US')
      })
      await waitFor(() => expect(archiveStarted).toBe(true))

      const prompt = screen.getByRole('textbox', { name: /提示词|Prompt/ })
      await user.clear(prompt)
      await user.type(prompt, 'Edited while archiving')
      await act(async () => {
        archiveGate.resolve()
        await archiveGate.promise
      })

      await waitFor(() => expect(screen.getByDisplayValue('Local name')).toBeInTheDocument())
      expect(screen.getByRole('textbox', { name: /提示词|Prompt/ })).toHaveValue('Edited while archiving')
    } finally {
      archiveGate.resolve()
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
