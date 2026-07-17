import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw/server'
import { MessageCenterButton } from './MessageCenter'

describe('MessageCenterButton', () => {
  it('marks the visible message as read when the center opens', async () => {
    const markRead = vi.fn()
    server.use(
      http.get('/api/messages', () =>
        HttpResponse.json({
          unread_count: 1,
          items: [{
            id: 'changelog:unreleased',
            type: 'changelog',
            title: 'Unreleased',
            summary: '消息中心。',
            body: '### Added\n\n- 消息中心。',
          }],
        }),
      ),
      http.post('/api/messages/:id/read', ({ params }) => {
        markRead(params.id)
        return HttpResponse.json({
          id: 'changelog:unreleased',
          type: 'changelog',
          title: 'Unreleased',
          summary: '消息中心。',
          body: '### Added\n\n- 消息中心。',
          read_at: '2026-06-30T00:00:00Z',
        })
      }),
    )

    render(<MessageCenterButton className="h-8 w-8" />)

    expect(await screen.findByText('1')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '打开消息中心' }))

    expect(await screen.findAllByText('Denova 未发布更新')).toHaveLength(2)
    expect(await screen.findAllByText('消息中心。')).toHaveLength(2)
    expect(await screen.findByText('给 Denova 充点 token')).toBeInTheDocument()
    expect(screen.getByText('如果 Denova 项目有帮到你，可以给它也充点 token，帮助 Denova 持续开源、持续迭代。非常感谢！')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Denova 赞助二维码' })).toHaveAttribute('src', '/donate.png')
    expect(screen.getByText('给 Denova 点个 Star')).toBeInTheDocument()
    expect(screen.getByText('如果 Denova 项目有帮到你，欢迎去 GitHub 点个 Star，这是对 Denova 持续开源、持续迭代最大的支持。')).toBeInTheDocument()
    const starLink = screen.getByRole('link', { name: '去 GitHub 点 Star' })
    expect(starLink).toHaveAttribute('href', 'https://github.com/alfredxw/denova')
    expect(starLink).toHaveAttribute('target', '_blank')
    await waitFor(() => expect(markRead).toHaveBeenCalledWith('changelog:unreleased'))
    await waitFor(() => expect(screen.queryByText('1')).not.toBeInTheDocument())
  })

  it('does not show the donation prompt for non-changelog messages', async () => {
    server.use(
      http.get('/api/messages', () =>
        HttpResponse.json({
          unread_count: 0,
          items: [{
            id: 'notice:1',
            type: 'notice',
            title: '系统通知',
            summary: '普通消息。',
            body: '普通正文。',
            read_at: '2026-06-30T00:00:00Z',
          }],
        }),
      ),
    )

    render(<MessageCenterButton className="h-8 w-8" />)

    await userEvent.click(screen.getByRole('button', { name: '打开消息中心' }))

    expect(await screen.findAllByText('系统通知')).toHaveLength(2)
    expect(screen.queryByText('给 Denova 充点 token')).not.toBeInTheDocument()
    expect(screen.queryByRole('img', { name: 'Denova 赞助二维码' })).not.toBeInTheDocument()
    expect(screen.queryByText('给 Denova 点个 Star')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '去 GitHub 点 Star' })).not.toBeInTheDocument()
  })

  it('marks all messages as read from the center header', async () => {
    const markAllRead = vi.fn()
    const items = [
      {
        id: 'changelog:unreleased',
        type: 'changelog',
        title: 'Unreleased',
        summary: '第一条消息。',
        body: '### Added\n\n- 第一条消息。',
      },
      {
        id: 'changelog:v0.1.17',
        type: 'changelog',
        title: 'v0.1.17',
        summary: '第二条消息。',
        body: '### Fixed\n\n- 第二条消息。',
      },
    ]
    server.use(
      http.get('/api/messages', () =>
        HttpResponse.json({
          unread_count: 2,
          items,
        }),
      ),
      http.post('/api/messages/:id/read', ({ params }) =>
        HttpResponse.json({
          ...items.find((item) => item.id === params.id),
          read_at: '2026-06-30T00:00:00Z',
        }),
      ),
      http.post('/api/messages/read-all', () => {
        markAllRead()
        return HttpResponse.json({
          unread_count: 0,
          items: items.map((item) => ({ ...item, read_at: '2026-06-30T00:00:00Z' })),
        })
      }),
    )

    render(<MessageCenterButton className="h-8 w-8" />)

    expect(await screen.findByText('2')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '打开消息中心' }))
    await userEvent.click(await screen.findByRole('button', { name: '全部已读' }))

    await waitFor(() => expect(markAllRead).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.queryByText('2')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.queryByText('1')).not.toBeInTheDocument())
  })
})
