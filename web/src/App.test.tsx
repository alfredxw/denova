import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { TooltipProvider } from './components/ui/tooltip'
import { useWorkspaceStore } from './stores/workspace-store'

const defaultPayloads: Record<string, unknown> = {
  '/api/workspace/current': { workspace: '/books/demo', has_state: true },
  '/api/workspace/tree': [],
  '/api/workspace/summary': { title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] },
  '/api/styles': { styles: [] },
  '/api/books': { books: [] },
  '/api/settings': {
    user: {},
    workspace: {},
    effective: { max_open_tabs: 5 },
    paths: { nova_dir: '/nova/user', user_config: '', workspace_config: '' },
  },
  '/api/books/create': { workspace: '/nova/user/新书', book_meta: { title: '新书', author: '', description: '' } },
  '/api/workspace/switch': { workspace: '/books/from-picker', message: '已切换到: /books/from-picker' },
  '/api/sessions': { sessions: [] },
  '/api/session/messages': [],
  '/api/chat/active': { active: false },
  '/api/lore/items': { items: [] },
  '/api/lore/versions': { versions: [] },
  '/api/lore/agent/messages': [],
  '/api/interactive/tellers': { tellers: [] },
  '/api/automations': { tasks: [] },
}

describe('App', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useWorkspaceStore.setState({ mode: 'ide', rightPanel: 'ai', commandOpen: false })
    mockApiFetch()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('opens book management first without workspace and avoids workspace-bound APIs', async () => {
    const user = userEvent.setup()
    mockApiFetch({
      '/api/workspace/current': { workspace: '', has_state: false },
      '/api/books': { books: [] },
    })

    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    expect(await screen.findByText('当前 Nova 数据目录下还没有书籍')).toBeInTheDocument()
    expect(screen.getByText('请先新建一本书，或导入现有小说/角色卡创建书籍。创建后再进入写作、互动、Agent 和自动化工作流。')).toBeInTheDocument()

    await waitFor(() => expect(fetchCallPaths()).toContain('/api/workspace/current'))
    const paths = fetchCallPaths()
    expect(paths).toContain('/api/books')
    expect(paths).toContain('/api/settings')
    expect(paths).not.toContain('/api/workspace/tree')
    expect(paths).not.toContain('/api/workspace/summary')
    expect(paths).not.toContain('/api/styles')
    expect(paths).not.toContain('/api/sessions')
    expect(paths).not.toContain('/api/session/messages')
    expect(paths).not.toContain('/api/chat/active')
    expectOnlyActivePrimaryMenu('书籍管理')

    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    await user.click(within(header as HTMLElement).getByRole('button', { name: 'IDE 模式' }))
    expect(await screen.findByText('当前 Nova 数据目录下还没有书籍')).toBeInTheDocument()
    expectOnlyActivePrimaryMenu('书籍管理')
    expect(fetchCallPaths()).not.toContain('/api/chat/active')
  })

  it('renders the IDE and interactive mode switch in the main header', async () => {
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    expect(within(header as HTMLElement).getByRole('button', { name: 'IDE 模式' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toBeInTheDocument()
  })

  it('applies the persisted primary menu order', async () => {
    const user = userEvent.setup()
    window.localStorage.setItem('nova.activity.order.v1', JSON.stringify(['automations', 'agents', 'books', 'writing', 'story']))
    window.localStorage.setItem('nova.activity.order.ide.v1', JSON.stringify(['automations', 'agents', 'books', 'writing']))
    window.localStorage.setItem('nova.activity.order.interactive.v1', JSON.stringify(['agents', 'books', 'story']))
    window.localStorage.setItem('nova.activity.order.ide.v2', JSON.stringify(['books', 'agents', 'writing', 'lore', 'creator', 'teller', 'versions', 'skills', 'automations']))
    window.localStorage.setItem('nova.activity.order.interactive.v2', JSON.stringify(['automations', 'story', 'timeline', 'lore', 'creator', 'teller', 'books', 'skills', 'agents']))

    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    expect(primaryMenuLabels().slice(0, 4)).toEqual(['书籍管理', 'Agents', '写作', '资料库'])

    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    await user.click(within(header as HTMLElement).getByRole('button', { name: '互动模式' }))
    expect(primaryMenuLabels().slice(0, 4)).toEqual(['自动化', '剧情', '剧情路线图', '资料库'])
  })

  it('does not render the removed task panel UI', async () => {
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    expect(screen.queryByLabelText('显示/隐藏任务面板')).not.toBeInTheDocument()
    expect(screen.queryByText('任务')).not.toBeInTheDocument()
    expect(screen.queryByText('写作流')).not.toBeInTheDocument()
  })

  it('opens book management as a global workspace page outside editor tabs', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    expect(screen.queryByRole('button', { name: '导入酒馆角色卡' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '书籍管理' }))

    expect(await screen.findByText('书架')).toBeInTheDocument()
    expect(screen.queryByText('打开其他目录')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '关闭书籍管理' })).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('输入工作区目录路径...')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '导入酒馆角色卡' }))
    expect(screen.getByText('选择 PNG 或 JSON 角色卡，并决定写入当前书还是创建一本新书。')).toBeInTheDocument()
  })

  it('creates new books in the Nova data directory', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    await user.click(screen.getByRole('button', { name: '书籍管理' }))
    await screen.findByText('书架')
    await user.click(screen.getByRole('button', { name: '新建书籍' }))
    expect(screen.getByText('新书将创建在')).toBeInTheDocument()
    expect(screen.getByText('/nova/user')).toBeInTheDocument()
    await user.type(screen.getByPlaceholderText('书名（必填）'), '新书')
    await user.click(screen.getByRole('button', { name: '创建' }))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith('/api/books/create', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ title: '新书', author: '', description: '' }),
      }))
    })
  })

  it('opens settings as a workspace page outside editor tabs', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    await user.click(screen.getByRole('button', { name: '设置' }))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect((await screen.findAllByText('IDE 模式')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('互动模式').length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: '编辑器' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '故事舞台' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '关闭设置' })).toBeInTheDocument()
    expect(screen.queryByText('Agent 模型分配')).not.toBeInTheDocument()
  })

  it('opens Agents as a global management page and toggles back', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    await user.click(screen.getByRole('button', { name: 'Agents' }))

    expect(screen.queryByText('全局默认配置')).not.toBeInTheDocument()
    expect(screen.queryByText('默认 Agent')).not.toBeInTheDocument()
    expect((await screen.findAllByText('IDE 创作 Agent')).length).toBeGreaterThan(0)
    expect(within(screen.getByRole('navigation')).getAllByText('互动')).toHaveLength(1)
    expect(screen.getByText('模型与思考')).toBeInTheDocument()
    expect(screen.getByText('系统提示')).toBeInTheDocument()
    expect(screen.getByLabelText('自定义 System Prompt')).toBeInTheDocument()
    expect(screen.getByText('自定义提示在行为、创作偏好、策略和风格上优先于 Nova 内置提示，但不会覆盖工具权限、输出协议、互动禁写文件、结构化 JSON 要求和后端校验边界。')).toBeInTheDocument()
    expect(screen.getByText('工具能力')).toBeInTheDocument()
    expect(screen.queryByText('Agent 模型分配')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '关闭 Agents' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '资料库 Agent资料库维护、初始化设定与 CREATOR.md 写入' }))
    expect(screen.getByText('工具能力')).toBeInTheDocument()
    expect(screen.getByText('修改文件')).toBeInTheDocument()
    expect(screen.getByText('Skills')).toBeInTheDocument()
    expect(screen.getByText('读取资料库')).toBeInTheDocument()
    expect(screen.getByText('写入资料库')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '版本说明 Agent自动版本摘要' }))
    expect(screen.getByText('这个 Agent 当前是纯模型调用，不修改文件、资料库或叙事编排；这里只配置模型与思考参数。')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '工具 Agent小说导入时识别章节分割正则' }))
    expect(screen.getAllByText('小说导入时识别章节分割正则').length).toBeGreaterThan(0)
    expect(screen.getByText('这个 Agent 当前是纯模型调用，不修改文件、资料库或叙事编排；这里只配置模型与思考参数。')).toBeInTheDocument()

    const agentsButton = screen.getByRole('button', { name: 'Agents' })
    expect(agentsButton).toHaveClass('is-active')
    expectOnlyActivePrimaryMenu('Agents')
    const settingsButton = screen.getByRole('button', { name: '设置' })
    await user.click(settingsButton)
    expect(await screen.findByRole('button', { name: '关闭设置' })).toBeInTheDocument()
    expect(agentsButton).not.toHaveClass('is-active')
    expect(settingsButton).toHaveClass('is-active')
    expectOnlyActivePrimaryMenu('设置')

    await user.click(settingsButton)
    await user.click(screen.getByRole('button', { name: 'Agents' }))
    expect(screen.queryByRole('button', { name: '关闭 Agents' })).not.toBeInTheDocument()
    expect(screen.queryByText('模型与思考')).not.toBeInTheDocument()
  })

  it('opens shared Agents page without leaving interactive shell', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    await user.click(within(header as HTMLElement).getByRole('button', { name: '互动模式' }))
    await user.click(screen.getByRole('button', { name: 'Agents' }))

    expect(await screen.findByRole('button', { name: '关闭 Agents' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expectOnlyActivePrimaryMenu('Agents')
    expect(screen.getByRole('button', { name: '剧情' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '剧情路线图' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '写作' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Agents' }))
    expect(screen.queryByRole('button', { name: '关闭 Agents' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '剧情' })).toHaveClass('is-active')
    expectOnlyActivePrimaryMenu('剧情')
  })

  it('opens Automations as a shared page without switching saved mode', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    await user.click(within(header as HTMLElement).getByRole('button', { name: '互动模式' }))
    await user.click(screen.getByRole('button', { name: '自动化' }))

    expect(await screen.findByRole('button', { name: '关闭自动化' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expectOnlyActivePrimaryMenu('自动化')
    expect(screen.getByText('续写章节')).toBeInTheDocument()
    expect(screen.getByText('定时规则')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '自动化' }))
    expect(screen.queryByRole('button', { name: '关闭自动化' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '剧情' })).toHaveClass('is-active')
    expectOnlyActivePrimaryMenu('剧情')
  })

  it('guides an empty IDE lore store into the Writing Agent ideation flow', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    expect(await screen.findByText('开始构思新书')).toBeInTheDocument()
    expect(screen.getByText('先和创作 Agent 讨论灵感、题材、核心冲突、世界观、人设、叙事风格、大纲和写作规则；阶段性结论会整理到 ideas.md（灵感），再决定是否沉淀资料库或创建章节。')).toBeInTheDocument()
    expect(screen.queryByText('资料库还是空的')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '和创作 Agent 聊灵感' }))

    expect(await screen.findByText('创作Agent')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByDisplayValue(/专业小说创作 Agent/)).toBeInTheDocument()
    })
    expect(screen.queryByDisplayValue(/lore-init/)).not.toBeInTheDocument()
    expect(screen.queryByText('用自然语言批量整理、补充和修改资料库')).not.toBeInTheDocument()
  })

  it('keeps one active shared menu while switching shared pages from interactive mode', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    await user.click(within(header as HTMLElement).getByRole('button', { name: '互动模式' }))

    await user.click(screen.getByRole('button', { name: 'Agents' }))
    expect(await screen.findByRole('button', { name: '关闭 Agents' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expectOnlyActivePrimaryMenu('Agents')

    await user.click(screen.getByRole('button', { name: '书籍管理' }))
    expect(await screen.findByRole('button', { name: '关闭书籍管理' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.getByRole('button', { name: '剧情' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '写作' })).not.toBeInTheDocument()
    expectOnlyActivePrimaryMenu('书籍管理')

    await user.click(screen.getByRole('button', { name: '设置' }))
    expect(await screen.findByRole('button', { name: '关闭设置' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expectOnlyActivePrimaryMenu('设置')

    await user.click(screen.getByRole('button', { name: '自动化' }))
    expect(await screen.findByRole('button', { name: '关闭自动化' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expectOnlyActivePrimaryMenu('自动化')

    await user.click(screen.getByRole('button', { name: 'Agents' }))
    expect(await screen.findByRole('button', { name: '关闭 Agents' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动模式' })).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.getByRole('button', { name: '剧情' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '写作' })).not.toBeInTheDocument()
    expectOnlyActivePrimaryMenu('Agents')
  })
})

function expectOnlyActivePrimaryMenu(label: string) {
  const agentsButton = screen.getByRole('button', { name: 'Agents' })
  const activityBar = agentsButton.closest('aside')
  if (!activityBar) throw new Error('activity bar not found')
  const activeLabels = within(activityBar)
    .getAllByRole('button')
    .filter((button) => button.className.includes('is-active'))
    .map((button) => button.getAttribute('aria-label') || button.textContent || '')
  expect(activeLabels).toEqual([label])
}

function primaryMenuLabels() {
  const agentsButton = screen.getByRole('button', { name: 'Agents' })
  const activityBar = agentsButton.closest('aside')
  if (!activityBar) throw new Error('activity bar not found')
  return within(activityBar)
    .getAllByRole('button')
    .map((button) => button.getAttribute('aria-label') || button.textContent || '')
}

function mockApiFetch(overrides: Record<string, unknown> = {}) {
  const payloads = { ...defaultPayloads, ...overrides }
  globalThis.fetch = vi.fn(async (input) => {
    const rawUrl = readFetchUrl(input)
    const path = new URL(rawUrl, 'http://localhost').pathname
    return new Response(JSON.stringify(payloads[path] ?? {}), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as typeof fetch
}

function fetchCallPaths() {
  return vi.mocked(globalThis.fetch).mock.calls.map(([input]) => {
    const rawUrl = readFetchUrl(input)
    return new URL(rawUrl, 'http://localhost').pathname
  })
}

function readFetchUrl(input: RequestInfo | URL) {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}
