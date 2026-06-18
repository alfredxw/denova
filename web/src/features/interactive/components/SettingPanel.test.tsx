import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SettingPanel } from './SettingPanel'

describe('SettingPanel', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input.url
      const path = new URL(rawUrl, 'http://localhost').pathname
      const payloads: Record<string, unknown> = {
        '/api/lore/items': { items: [] },
        '/api/lore/agent/messages': [],
        '/api/workspace/file': { path: 'CREATOR.md', content: '最高优先级规则' },
      }
      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }) as typeof fetch
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('prefills the lore-init instruction from the empty Lore Agent shortcut', async () => {
    const user = userEvent.setup()
    render(<SettingPanel mode="lore" workspace="/books/demo" />)

    expect(await screen.findByText('和资料库 Agent 对话')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '初始化故事设定' }))

    await waitFor(() => {
      expect(screen.getByDisplayValue(/lore-init/)).toBeInTheDocument()
    })
  })

  it('edits CREATOR.md from the lore directory', async () => {
    const user = userEvent.setup()
    render(<SettingPanel mode="lore" workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: 'CREATOR.md' }))
    expect(await screen.findByDisplayValue('最高优先级规则')).toBeInTheDocument()

    await user.clear(screen.getByDisplayValue('最高优先级规则'))
    await user.type(screen.getByPlaceholderText('写下本书最高优先级的创作规则...'), '新的最高规则')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith('/api/workspace/file', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ path: 'CREATOR.md', content: '新的最高规则' }),
      }))
    })
  })

  it('keeps Lore Agent chat visible after switching pages before history reloads', async () => {
    const user = userEvent.setup()
    const encoder = new TextEncoder()
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input instanceof Request ? input.url : input.toString()
      const path = new URL(rawUrl, 'http://localhost').pathname
      if (path === '/api/lore/agent/stream') {
        const result = {
          message: '已更新资料库',
          items: [],
          created: [],
          updated: [],
          deleted_ids: [],
        }
        return new Response(
          new ReadableStream({
            start(controller) {
              controller.enqueue(encoder.encode(`event: lore_result\ndata: ${JSON.stringify(result)}\n\n`))
              controller.close()
            },
          }),
          { status: 200 },
        )
      }
      const payloads: Record<string, unknown> = {
        '/api/lore/items': { items: [] },
        '/api/lore/agent/messages': [],
      }
      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })

    const { rerender } = render(<SettingPanel mode="lore" workspace="/books/demo-cache" />)
    const input = await screen.findByPlaceholderText('输入资料库修改指令，Enter 发送，Shift+Enter 换行')
    await user.type(input, '补充设定{Enter}')

    expect(await screen.findByText('补充设定')).toBeInTheDocument()
    expect(await screen.findByText('已更新资料库')).toBeInTheDocument()

    rerender(<SettingPanel mode="creator" workspace="/books/demo-cache" />)
    rerender(<SettingPanel mode="lore" workspace="/books/demo-cache" />)

    expect(await screen.findByText('补充设定')).toBeInTheDocument()
    expect(screen.getByText('已更新资料库')).toBeInTheDocument()
  })

  it('keeps agent message lists inside their own scroll containers', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input instanceof Request ? input.url : input.toString()
      const path = new URL(rawUrl, 'http://localhost').pathname
      const payloads: Record<string, unknown> = {
        '/api/lore/items': { items: [] },
        '/api/lore/agent/messages': [{ role: 'user', content: '资料库历史消息' }],
        '/api/interactive/tellers/agent/messages': [{ role: 'user', content: '编排历史消息' }],
      }
      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })

    const { rerender } = render(<SettingPanel mode="lore" workspace="/books/scroll-check" />)
    expect(await screen.findByText('资料库历史消息')).toBeInTheDocument()
    expect(document.querySelector('.nova-chat-canvas')?.parentElement).toHaveClass('flex', 'min-h-0', 'flex-1', 'flex-col', 'overflow-hidden')

    rerender(
      <SettingPanel
        mode="teller"
        workspace="/books/scroll-check"
        tellers={[
          {
            version: 1,
            id: 'classic',
            name: '经典叙事',
            description: '默认方案',
            random_event_rate: 0,
            style_rules: [],
            tags: [],
            context_policy: {
              creator: 'summary',
              lore: 'summary',
              runtime_state: 'summary',
              recent_turns: 6,
            },
            slots: [],
            custom: false,
          },
        ]}
      />,
    )
    await userEvent.click(await screen.findByRole('button', { name: '叙事编排 Agent' }))
    expect(await screen.findByText('编排历史消息')).toBeInTheDocument()
    expect(document.querySelector('.nova-chat-canvas')?.parentElement).toHaveClass('flex', 'min-h-0', 'flex-1', 'flex-col', 'overflow-hidden')
  })
})
