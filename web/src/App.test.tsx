import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { TooltipProvider } from './components/ui/tooltip'
import { useWorkspaceStore } from './stores/workspace-store'

describe('App', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useWorkspaceStore.setState({ mode: 'ide', rightPanel: 'ai', commandOpen: false })
    globalThis.fetch = vi.fn(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input.url
      const path = new URL(rawUrl, 'http://localhost').pathname
      const payloads: Record<string, unknown> = {
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
      }

      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }) as typeof fetch
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
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

    expect(await screen.findByText('最近书籍')).toBeInTheDocument()
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
    await screen.findByText('最近书籍')
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

  it('guides an empty IDE lore store into the Lore Agent initialization flow', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    expect(await screen.findByText('资料库还是空的')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '去资料库 Agent' }))

    expect((await screen.findAllByText('资料库 Agent')).length).toBeGreaterThan(0)
    await waitFor(() => {
      expect(screen.getByDisplayValue(/lore-init/)).toBeInTheDocument()
    })
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
