import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { TooltipProvider } from './components/ui/tooltip'

describe('App', () => {
  beforeEach(() => {
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

  it('renders the mode switch in the main header', async () => {
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    const header = screen.getByText('Nova').closest('header')
    expect(header).not.toBeNull()
    expect(within(header as HTMLElement).getByRole('button', { name: '写作' })).toBeInTheDocument()
    expect(within(header as HTMLElement).getByRole('button', { name: '互动' })).toBeInTheDocument()
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

  it('opens book management as a global dialog outside editor tabs', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    await user.click(screen.getByRole('button', { name: '书籍管理' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('最近书籍')).toBeInTheDocument()
    expect(within(dialog).queryByText('打开其他目录')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '关闭书籍管理' })).toBeInTheDocument()
    expect(within(dialog).queryByPlaceholderText('输入工作区目录路径...')).not.toBeInTheDocument()
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
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: '新建书籍' }))
    expect(within(dialog).getByText(/新书将创建在：/)).toHaveTextContent('/nova/user')
    await user.type(within(dialog).getByPlaceholderText('书名（必填）'), '新书')
    await user.click(within(dialog).getByRole('button', { name: '创建' }))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith('/api/books/create', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ title: '新书', author: '', description: '' }),
      }))
    })
  })

  it('opens settings as a global dialog outside editor tabs', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <App />
      </TooltipProvider>,
    )

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/chat/active', undefined))
    await user.click(screen.getByRole('button', { name: '设置' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('IDE 模式')).toBeInTheDocument()
    expect(within(dialog).getByText('互动模式')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '编辑器' })).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '故事舞台' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '关闭 设置' })).not.toBeInTheDocument()
  })
})
