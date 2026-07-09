import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { SessionManager } from './SessionManager'
import type { SessionSummary } from '@/lib/api'

const sessions: SessionSummary[] = [
  { id: 'session-a', title: '设定讨论', active: true, message_count: 2, created_at: '', updated_at: '' },
  { id: 'session-b', title: '正文续写', active: false, message_count: 1, created_at: '', updated_at: '' },
]

describe('SessionManager', () => {
  it('支持重命名和删除会话入口', async () => {
    const user = userEvent.setup()
    const handleRename = vi.fn()
    const handleDelete = vi.fn()

    render(
      <SessionManager
        sessions={sessions}
        activeSessionId="session-b"
        onCreate={vi.fn()}
        onSwitch={vi.fn()}
        onRename={handleRename}
        onDelete={handleDelete}
      />,
    )

    await user.click(screen.getByRole('button', { name: '重命名会话 正文续写' }))
    const input = screen.getByRole('textbox', { name: '会话标题' })
    await user.clear(input)
    await user.type(input, '新标题{Enter}')
    await user.click(screen.getByRole('button', { name: '删除会话 正文续写' }))

    expect(handleRename).toHaveBeenCalledWith('session-b', '新标题')
    expect(handleDelete).toHaveBeenCalledWith('session-b')
  })
})
