import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { MessageList } from './MessageList'
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

describe('MessageList', () => {
  it('历史消息首次加载后默认滚动到底部', async () => {
    const { container, rerender } = render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[]}
        scrollResetKey="session-a"
      />,
    )
    const scroller = container.firstElementChild as HTMLDivElement
    let scrollTop = 0
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 1200 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 320 })
    Object.defineProperty(scroller, 'scrollTop', {
      configurable: true,
      get: () => scrollTop,
      set: (value) => {
        scrollTop = value
      },
    })

    rerender(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '第一条消息' },
          { type: 'message', role: 'assistant', content: '最新回复' },
        ]}
        scrollResetKey="session-a"
      />,
    )

    await waitFor(() => expect(scroller.scrollTop).toBe(1200))
  })

  it('用户向上浏览时消息更新不会自动拉回底部', () => {
    const originalScrollIntoView = HTMLElement.prototype.scrollIntoView
    const scrollIntoView = vi.fn()
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView,
    })
    try {
      const { container, rerender } = render(
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '第一条消息' },
            { type: 'message', role: 'assistant', content: '历史回复' },
          ]}
          scrollResetKey="session-a"
        />,
      )
      const scroller = container.firstElementChild as HTMLDivElement
      let scrollTop = 0
      Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 1200 })
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 320 })
      Object.defineProperty(scroller, 'scrollTop', {
        configurable: true,
        get: () => scrollTop,
        set: (value) => {
          scrollTop = value
        },
      })

      scroller.scrollTop = 200
      fireEvent.scroll(scroller)
      scrollIntoView.mockClear()

      rerender(
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '第一条消息' },
            { type: 'message', role: 'assistant', content: '历史回复' },
            { type: 'message', role: 'assistant', content: '新增回复' },
          ]}
          scrollResetKey="session-a"
        />,
      )

      expect(scroller.scrollTop).toBe(200)
      expect(scrollIntoView).not.toHaveBeenCalled()
    } finally {
      Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
        configurable: true,
        value: originalScrollIntoView,
      })
    }
  })

  it('展示 /clear 产生的上下文清理分界且保留前后消息', () => {
    render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '清理前问题' },
          { type: 'clear', created_at: '2026-05-17T08:00:00Z' },
          { type: 'message', role: 'assistant', content: '清理后回答' },
        ]}
      />,
    )

    expect(screen.getByText('清理前问题')).toBeInTheDocument()
    expect(screen.getByRole('separator', { name: '上下文已清理' })).toBeInTheDocument()
    expect(screen.getByText(/之前消息不再参与创作Agent上下文/)).toBeInTheDocument()
    expect(screen.getByText('清理后回答')).toBeInTheDocument()
  })

  it('消息 hover 时间按当天和历史日期格式渲染', () => {
    const today = new Date()
    today.setHours(9, 5, 0, 0)
    const oldDay = new Date(2020, 0, 1, 20, 30, 0, 0)
    const traceTime = new Date(2020, 0, 1, 21, 45, 0, 0)

    render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '当天消息', created_at: today.toISOString() },
          { type: 'message', role: 'assistant', content: '历史消息', created_at: oldDay.toISOString() },
          { type: 'message', role: 'thinking', content: '思考过程', created_at: traceTime.toISOString() },
          { type: 'message', role: 'tool_call', content: 'execute\n{}', name: 'execute', created_at: traceTime.toISOString() },
        ]}
      />,
    )

    expect(screen.getByText('09:05')).toBeInTheDocument()
    expect(screen.getByText('2020-01-01 20:30')).toBeInTheDocument()
    expect(screen.queryByText('2020-01-01 21:45')).not.toBeInTheDocument()
  })

  it('运行中的上下文压缩卡片存在时不再渲染第二个 activity 卡片', () => {
    render(
      <MessageList
        isStreaming
        activityContent="正在压缩上下文…"
        messages={[
          {
            role: 'context_compaction',
            status: 'running',
            content: '',
            streaming: true,
          },
        ]}
      />,
    )

    expect(screen.getByText('上下文压缩')).toBeInTheDocument()
    expect(screen.getByLabelText('压缩中')).toBeInTheDocument()
    expect(screen.queryByText('正在压缩上下文…')).not.toBeInTheDocument()
  })

  it('折叠执行过程时仍直接展示 SubAgent assistant 小窗', () => {
    render(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceBeforeAssistant
        messages={[
          { type: 'message', role: 'thinking', content: '根 Agent 思考' },
          { type: 'message', role: 'assistant', content: 'SubAgent 可见输出', agent_name: 'researcher', subagent: true },
          { type: 'message', role: 'assistant', content: '根 Agent 回复' },
        ]}
      />,
    )

    expect(screen.getByRole('button', { name: /思考过程/ })).toBeInTheDocument()
    expect(screen.getByText('researcher 输出')).toBeInTheDocument()
    expect(screen.getByText('SubAgent 可见输出')).toBeInTheDocument()
    expect(screen.getByText('根 Agent 回复')).toBeInTheDocument()
  })

  it('有子会话详情回调时将同一 SubAgent 时间线收敛为一个卡片', async () => {
    const user = userEvent.setup()
    const handleOpen = vi.fn()
    render(
      <MessageList
        isStreaming={false}
        activityContent=""
        onOpenSubAgentSession={handleOpen}
        messages={[
          { type: 'message', role: 'thinking', content: 'SubAgent 思考', agent_name: 'researcher', subagent: true, subagent_session_id: 'run-1-subagent-01-researcher' },
          { type: 'message', role: 'tool_call', name: 'read_file', content: 'read_file', agent_name: 'researcher', subagent: true, subagent_session_id: 'run-1-subagent-01-researcher' },
          { type: 'message', role: 'assistant', content: 'SubAgent 可见输出', agent_name: 'researcher', subagent: true, subagent_session_id: 'run-1-subagent-01-researcher' },
          { type: 'message', role: 'assistant', content: '根 Agent 回复' },
        ]}
      />,
    )

    expect(screen.queryByText('SubAgent 思考')).not.toBeInTheDocument()
    expect(screen.queryByText('read_file')).not.toBeInTheDocument()
    expect(screen.getByText('researcher 输出')).toBeInTheDocument()
    expect(screen.getByText('SubAgent 可见输出')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /researcher 输出/ }))
    expect(handleOpen).toHaveBeenCalledWith(expect.objectContaining({ subagent_session_id: 'run-1-subagent-01-researcher' }))
  })
})
