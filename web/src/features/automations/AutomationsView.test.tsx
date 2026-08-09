import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { server } from '@/test/msw/server'
import { consumeAgentChatSessionNavigation } from '@/features/agent-chat/session-navigation'
import { AutomationsView as ProjectAutomationsView } from './AutomationsView'

function AutomationsView({ workspace, onOpenAgentChat = () => undefined }: { workspace: string; onOpenAgentChat?: () => void }) {
  const name = workspace.split('/').filter(Boolean).at(-1) || 'current'
  return <ProjectAutomationsView projectId={`workspace-${name}`} workspace={workspace} onOpenAgentChat={onOpenAgentChat} />
}

const taskBase = {
  enabled: true,
  template: 'custom_prompt',
  prompt: '',
  schedule: { kind: 'manual', hour: 9, minute: 0 },
  triggers: [],
  default_action_policy: 'auto_run',
  recent_runs: [],
}

const projectA = {
  id: 'workspace-a', type: 'book', path: '/books/a', name: 'Book A', status: 'available',
  current: true, total: 0, sessions: [],
}

const projectB = {
  id: 'workspace-b', type: 'book', path: '/books/b', name: 'Book B', status: 'available',
  current: false, total: 0, sessions: [],
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
  beforeEach(() => {
    server.use(http.get('/api/agent-chat/projects', () => HttpResponse.json({ projects: [projectA] })))
  })
  it('opens the admitted run in its Project AgentChat conversation', async () => {
    const user = userEvent.setup()
    const onOpenAgentChat = vi.fn()
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [{
        ...taskBase,
        id: 'run-in-agent-chat',
        catalog_id: 'workspace-a:run-in-agent-chat',
        scope: 'workspace',
        name: 'Run in AgentChat',
        target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
      }] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.post('/api/automations/:id/run', () => HttpResponse.json({ run: {
        id: 'run-1',
        task_id: 'run-in-agent-chat',
        project_id: 'workspace-a',
        session_id: 'automation-run-1',
        status: 'running',
      } })),
    )

    try {
      render(<AutomationsView workspace="/books/a" onOpenAgentChat={onOpenAgentChat} />)
      await user.click(await screen.findByRole('button', { name: '立即运行' }))

      await waitFor(() => expect(onOpenAgentChat).toHaveBeenCalledOnce())
      expect(consumeAgentChatSessionNavigation()).toEqual({
        projectId: 'workspace-a',
        sessionId: 'automation-run-1',
      })
    } finally {
      consumeAgentChatSessionNavigation()
    }
  })

  it('collapses and restores the automation sidebar from the page header', async () => {
    const user = userEvent.setup()
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
    )

    render(<AutomationsView workspace="/books/a" />)

    const collapse = await screen.findByRole('button', { name: '收起侧边栏' })
    const separator = screen.getByRole('separator', { name: '调整侧边栏宽度' })
    await user.click(collapse)

    expect(screen.getByRole('button', { name: '展开侧边栏' })).toHaveAttribute('aria-pressed', 'false')
    expect(separator).toHaveAttribute('aria-hidden', 'true')

    await user.click(screen.getByRole('button', { name: '展开侧边栏' }))
    expect(screen.getByRole('button', { name: '收起侧边栏' })).toHaveAttribute('aria-pressed', 'true')
    expect(separator).toHaveAttribute('aria-hidden', 'false')
  })

  it('lists every Project catalog and toggles a Project from the full header row', async () => {
    const user = userEvent.setup()
    server.use(
      http.get('/api/agent-chat/projects', () => HttpResponse.json({ projects: [projectA, projectB] })),
      http.get('/api/books', () => HttpResponse.json({ books: [
        { project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
        { project_id: 'workspace-b', name: 'Book B', path: '/books/b', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', ({ request }) => {
        const query = new URL(request.url).searchParams
        const projectId = query.get('project_id')
        return HttpResponse.json({ tasks: [
          { ...taskBase, id: 'same', catalog_id: `${projectId}:same`, scope: 'workspace', name: projectId === 'workspace-a' ? 'Review A' : 'Review B', target: { kind: 'workspace', workspace: projectId === 'workspace-a' ? '/books/a' : '/books/b', project_id: projectId } },
        ] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
    )

    render(<AutomationsView workspace="/books/a" />)

    expect(await screen.findByRole('button', { name: '折叠Book A' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '折叠Book B' })).toBeInTheDocument()
    expect(screen.getAllByText('Review A').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Review B').length).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: '折叠Book A' }))
    expect(screen.getByRole('button', { name: '展开Book A' })).toHaveAttribute('aria-expanded', 'false')
    await user.click(screen.getByRole('button', { name: '展开Book A' }))
    expect(screen.getByRole('button', { name: '折叠Book A' })).toHaveAttribute('aria-expanded', 'true')
  })

  it('keeps healthy Project catalogs available when another Project fails to load', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    server.use(
      http.get('/api/agent-chat/projects', () => HttpResponse.json({ projects: [projectA, projectB] })),
      http.get('/api/automations', ({ request }) => {
        const projectId = new URL(request.url).searchParams.get('project_id')
        if (projectId === 'workspace-b') return HttpResponse.json({ error: 'Project unavailable' }, { status: 503 })
        return HttpResponse.json({ tasks: [{
          ...taskBase,
          id: 'healthy',
          catalog_id: 'workspace-a:healthy',
          scope: 'workspace',
          name: 'Healthy Project task',
          target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
    )

    try {
      render(<AutomationsView workspace="/books/a" />)
      expect((await screen.findAllByText('Healthy Project task')).length).toBeGreaterThan(0)
      expect(screen.getByRole('button', { name: '折叠Book A' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '展开Book B' })).toBeInTheDocument()
      expect(screen.getByText('1 个项目的自动化目录加载失败，其他项目仍可正常使用。')).toBeInTheDocument()
    } finally {
      consoleError.mockRestore()
    }
  })

  it('starts a new draft for the Project chosen from its catalog shortcut', async () => {
    const user = userEvent.setup()
    let created = false
    server.use(
      http.get('/api/agent-chat/projects', () => HttpResponse.json({ projects: [projectA, projectB] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.post('/api/automations', () => {
        created = true
        return HttpResponse.json({})
      }),
    )

    render(<AutomationsView workspace="/books/a" />)

    await user.click(await screen.findByRole('button', { name: '在 Book B 中新建自动化' }))
    expect(screen.getByDisplayValue('未命名自动化')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '所属项目' })).toHaveTextContent('Book B')
    expect(screen.getByRole('combobox', { name: '模板' })).toHaveTextContent('空白自动化')
    expect(screen.getByRole('button', { name: '展开Book B' })).toHaveAttribute('aria-expanded', 'false')
    expect(created).toBe(false)
  })

  it('opens an inline draft on the current Project and creates only after the user saves it', async () => {
    const user = userEvent.setup()
    let createdTask: Record<string, unknown> | null = null
    server.use(
      http.get('/api/agent-chat/projects', () => HttpResponse.json({ projects: [projectA, projectB] })),
      http.get('/api/books', () => HttpResponse.json({ books: [
        { project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
        { project_id: 'workspace-b', name: 'Book B', path: '/books/b', author: '', last_opened_at: '' },
      ] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.post('/api/automations', async ({ request }) => {
        createdTask = await request.json() as Record<string, unknown>
        return HttpResponse.json({ ...createdTask, id: 'auto-review', catalog_id: 'workspace-a:auto-review' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)

    expect(await screen.findByText('还没有自动化任务')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '展开Book A' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: '新建自动化' })[0])
    expect(await screen.findByDisplayValue('未命名自动化')).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '所属项目' })).toHaveTextContent('Book A')
    expect(screen.getByRole('combobox', { name: '模板' })).toHaveTextContent('空白自动化')
    expect(createdTask).toBeNull()

    await chooseProject(user, 'Book B')
    await chooseTemplate(user, '自动 Review')
    expect(createdTask).toBeNull()
    expect(screen.getByDisplayValue('自动 Review')).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: '状态' })).toHaveAttribute('data-state', 'unchecked')

    await user.click(screen.getByRole('button', { name: '创建' }))
    await waitFor(() => expect(createdTask).not.toBeNull())
    expect(createdTask).toMatchObject({
      enabled: false,
      name: '自动 Review',
      target: { kind: 'workspace', project_id: 'workspace-b', workspace: '/books/b' },
      triggers: [{ type: 'chapter_batch', chapter_batch_size: 5 }],
    })
  })

  it('keeps edits made while a new task creation request is in flight', async () => {
    const user = userEvent.setup()
    const createGate = deferred<void>()
    let submitted: Record<string, unknown> | null = null
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [reviewTemplate] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.post('/api/automations', async ({ request }) => {
        submitted = await request.json() as Record<string, unknown>
        await createGate.promise
        return HttpResponse.json({ ...submitted, id: 'created-race', catalog_id: 'workspace-a:created-race', revision: 'rev-1' })
      }),
    )

    render(<AutomationsView workspace="/books/a" />)
    expect(await screen.findByText('还没有自动化任务')).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: '新建自动化' })[0])
    await chooseTemplate(user, '自动 Review')
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
      target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
      trigger_state: { schedule: { last_checked_at: 'today' } },
      last_run: { id: 'run-1' },
      recent_runs: [{ id: 'run-1' }],
    }
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [existing] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
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
    expect(updateBody).not.toHaveProperty('write_mode')
    expect(updateBody).not.toHaveProperty('write_scope')
    expect(updateBody).not.toHaveProperty('output_policy')
    expect(updateBody).not.toHaveProperty('output_path')
    expect(screen.getByRole('combobox', { name: '所属项目' })).toBeDisabled()
    expect(screen.queryByText('执行模式')).not.toBeInTheDocument()
    expect(screen.queryByText('写入范围')).not.toBeInTheDocument()
    expect(screen.queryByText('输出路径')).not.toBeInTheDocument()
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
      target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
    }
    const external = { ...baseline, revision: 'rev-2', name: 'Agent review', prompt: 'agent prompt' }
    let listRequests = 0
    const patchBodies: Record<string, unknown>[] = []
    let archived: Record<string, unknown> | null = null
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => {
        listRequests += 1
        return HttpResponse.json({ tasks: [listRequests === 1 ? baseline : external] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
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
      target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
    }
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [{ project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
      http.get('/api/automations', () => HttpResponse.json({ tasks: [existing] })),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
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

  it('keeps an unsaved task draft when a background reload completes', async () => {
    const user = userEvent.setup()
    const previousLanguage = i18n.language
    let automationRequests = 0
    server.use(
      http.get('/api/books', () => HttpResponse.json({ books: [
        { project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
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
          target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
    )

    try {
      render(<AutomationsView workspace="/books/a" />)
      const nameInput = await screen.findByDisplayValue('Server task name')
      await user.clear(nameInput)
      await user.type(nameInput, 'Unsaved local name')

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
      http.get('/api/books', () => HttpResponse.json({ books: [{ project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' }] })),
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
          target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
    )

    render(<AutomationsView workspace="/books/a" />)
    const name = await screen.findByDisplayValue('Server name')
    await user.clear(name)
    await user.type(name, 'Local name')

    act(() => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', {
        detail: { project_id: 'workspace-a', workspace: '/books/a', paths: ['.nova/automations/tasks.json'] },
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
        { project_id: 'workspace-a', name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
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
          target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' },
        }] })
      }),
      http.get('/api/automations/templates', () => HttpResponse.json({ templates: [] })),
      http.get('/api/automations/inbox', () => HttpResponse.json({ items: [] })),
      http.post('/api/autosave-conflicts', async () => {
        archiveStarted = true
        await archiveGate.promise
        return HttpResponse.json({ id: 'conflict-race', path: '/conflicts/conflict-race.json' }, { status: 201 })
      }),
      http.patch('/api/automations/:id', async ({ request }) => {
        const body = await request.json() as Record<string, unknown>
        return HttpResponse.json({ ...body, id: 'archive-race', catalog_id: 'workspace-a:archive-race', revision: 'rev-3', scope: 'workspace', target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' }, recent_runs: [] })
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

async function chooseProject(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByRole('combobox', { name: '所属项目' }))
  await user.click(await screen.findByRole('option', { name }))
}

async function chooseTemplate(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByRole('combobox', { name: '模板' }))
  await user.click(await screen.findByRole('option', { name }))
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
