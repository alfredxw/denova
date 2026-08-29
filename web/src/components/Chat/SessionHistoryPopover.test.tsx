import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { SessionSummary } from '@/lib/api'
import { SessionHistoryPopover } from './SessionHistoryPopover'

function sessions(): SessionSummary[] {
  const now = Date.now()
  return [
    {
      id: 'session-a',
      title: 'Running draft',
      created_at: new Date(now - 12 * 60_000).toISOString(),
      updated_at: new Date(now - 11 * 60_000).toISOString(),
      message_count: 4,
      running: true,
      active: true,
    },
    {
      id: 'session-b',
      title: 'Character notes',
      created_at: new Date(now - 4 * 60 * 60_000).toISOString(),
      updated_at: new Date(now - 3 * 60 * 60_000).toISOString(),
      message_count: 2,
      running: false,
      active: false,
    },
  ]
}

describe('SessionHistoryPopover', () => {
  it('searches recent sessions, shows compact times and switches from the result list', async () => {
    const user = userEvent.setup()
    const onSwitch = vi.fn()
    render(
      <SessionHistoryPopover
        sessions={sessions()}
        activeSessionId="session-a"
        onSwitch={onSwitch}
        onManage={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '会话历史' }))

    expect(screen.getByText('Running draft')).toBeInTheDocument()
    expect(screen.getByText('11m')).toBeInTheDocument()
    await user.type(screen.getByRole('combobox', { name: '搜索会话' }), 'Character')
    expect(screen.queryByText('Running draft')).not.toBeInTheDocument()

    await user.click(screen.getByRole('option', { name: '切换到会话 Character notes' }))
    expect(onSwitch).toHaveBeenCalledWith('session-b')
  })

  it('keeps full session management available from the popover footer', async () => {
    const user = userEvent.setup()
    const onManage = vi.fn()
    render(
      <SessionHistoryPopover
        sessions={sessions()}
        activeSessionId="session-a"
        onSwitch={vi.fn()}
        onManage={onManage}
      />,
    )

    await user.click(screen.getByRole('button', { name: '会话历史' }))
    await user.click(screen.getByRole('button', { name: '管理全部会话' }))

    expect(onManage).toHaveBeenCalledOnce()
  })

  it('shows the existing empty result state when no sessions are available', async () => {
    const user = userEvent.setup()
    render(
      <SessionHistoryPopover
        sessions={[]}
        activeSessionId=""
        onSwitch={vi.fn()}
        onManage={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '会话历史' }))

    expect(screen.getByText('没有匹配的会话')).toBeInTheDocument()
  })
})
