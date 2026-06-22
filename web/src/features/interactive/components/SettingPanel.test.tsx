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
        '/api/config-manager/messages': [],
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

  it('opens the embedded config manager from the lore directory', async () => {
    render(<SettingPanel mode="lore" workspace="/books/demo" />)

    expect((await screen.findAllByText('配置管理 Agent')).length).toBeGreaterThan(0)
    expect(screen.getByPlaceholderText('让配置管理 Agent 调整当前模块...')).toBeInTheDocument()
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
        body: JSON.stringify({ path: 'CREATOR.md', content: '新的最高规则', base_revision: '' }),
      }))
    })
  })

  it('streams config manager responses from the lore module', async () => {
    const user = userEvent.setup()
    const encoder = new TextEncoder()
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input instanceof Request ? input.url : input.toString()
      const path = new URL(rawUrl, 'http://localhost').pathname
      if (path === '/api/config-manager/stream') {
        return new Response(
          new ReadableStream({
            start(controller) {
              controller.enqueue(encoder.encode(`event: chunk\ndata: ${JSON.stringify({ content: '已更新资料库' })}\n\n`))
              controller.close()
            },
          }),
          { status: 200 },
        )
      }
      const payloads: Record<string, unknown> = {
        '/api/lore/items': { items: [] },
        '/api/config-manager/messages': [],
      }
      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })

    render(<SettingPanel mode="lore" workspace="/books/demo-cache" />)
    const input = await screen.findByPlaceholderText('让配置管理 Agent 调整当前模块...')
    await user.type(input, '补充设定{Enter}')

    expect(await screen.findByText('补充设定')).toBeInTheDocument()
    expect(await screen.findByText('已更新资料库')).toBeInTheDocument()
  })

  it('loads the shared config manager history from lore and teller modules', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input instanceof Request ? input.url : input.toString()
      const path = new URL(rawUrl, 'http://localhost').pathname
      const payloads: Record<string, unknown> = {
        '/api/lore/items': { items: [] },
        '/api/config-manager/messages': [{ role: 'user', content: '统一配置历史消息' }],
      }
      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })

    const { rerender } = render(<SettingPanel mode="lore" workspace="/books/scroll-check" />)
    expect(await screen.findByText('统一配置历史消息')).toBeInTheDocument()
    expect(document.querySelector('.nova-chat-canvas')?.parentElement).toHaveClass('flex', 'min-h-0', 'flex-col')

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
            },
            slots: [],
            custom: false,
          },
        ]}
      />,
    )
    await userEvent.click(await screen.findByRole('button', { name: '配置管理 Agent' }))
    expect(await screen.findByText('统一配置历史消息')).toBeInTheDocument()
    expect(document.querySelector('.nova-chat-canvas')?.parentElement).toHaveClass('flex', 'min-h-0', 'flex-col')
  })
})
