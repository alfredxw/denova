import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { SessionManagementPanel } from './SessionManagementPanel'

it('生成中仍可单击会话整行并立即显示目标会话', async () => {
  const user = userEvent.setup()
  let finishSwitch!: () => void
  const switchRequest = new Promise<void>((resolve) => { finishSwitch = resolve })
  const onSwitch = vi.fn(() => switchRequest)

  render(
    <SessionManagementPanel
      sessions={[
        { id: 'current', title: '当前会话', active: true, message_count: 3, created_at: '2026-07-01T10:00:00Z', updated_at: '2026-07-01T10:00:00Z' },
        { id: 'target', title: 'just say hello', active: false, message_count: 17, created_at: '2026-07-02T13:26:00Z', updated_at: '2026-07-02T13:26:00Z' },
      ]}
      activeSessionId="current"
      disabled
      onCreate={vi.fn()}
      onSwitch={onSwitch}
      onRename={vi.fn()}
      onDelete={vi.fn()}
      onEnterChat={vi.fn()}
    />,
  )

  const targetSession = screen.getByRole('button', { name: /just say hello.*17 条消息/ })
  expect(targetSession).toBeEnabled()
  await user.click(targetSession)

  expect(onSwitch).toHaveBeenCalledWith('target')
  expect(targetSession).toHaveAttribute('aria-current', 'true')

  await act(async () => finishSwitch())
})

it('长会话标题只占剩余空间且不会挤压会话计数', () => {
  const title = '请和我一起启动一本新书：先读取 ideas.md 和 CREATOR.md，通过对话梳理灵感、题材、冲突、世界观和人设'
  render(
    <SessionManagementPanel
      sessions={[
        { id: 'current', title, active: true, message_count: 18, created_at: '2026-06-30T14:16:00Z', updated_at: '2026-06-30T14:16:00Z' },
      ]}
      activeSessionId="current"
      onCreate={vi.fn()}
      onSwitch={vi.fn()}
      onRename={vi.fn()}
      onDelete={vi.fn()}
      onEnterChat={vi.fn()}
    />,
  )

  expect(screen.getByText('1 / 1 个会话')).toHaveClass('shrink-0', 'whitespace-nowrap')
  expect(screen.getByText(`当前：${title}`)).toHaveClass('min-w-0', 'flex-1', 'truncate', 'text-right')
})
