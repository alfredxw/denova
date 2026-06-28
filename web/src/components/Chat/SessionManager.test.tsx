import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import { MessageList as RawMessageList } from './MessageList'
import { SessionManager } from './SessionManager'
import type { ChatMessage } from '@/lib/api'
import type { SessionSummary } from '@/lib/api'

const sessions: SessionSummary[] = [
  { id: 'session-a', title: '设定讨论', active: true, message_count: 2, created_at: '', updated_at: '' },
  { id: 'session-b', title: '正文续写', active: false, message_count: 1, created_at: '', updated_at: '' },
]

function MessageList(props: ComponentProps<typeof RawMessageList>) {
  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 52 }}>
      <RawMessageList {...props} />
    </VirtuosoMockContext.Provider>
  )
}

function mockScrollMetrics(element: HTMLElement, initial = { scrollHeight: 1200, clientHeight: 320, scrollTop: 0 }) {
  let scrollHeight = initial.scrollHeight
  let clientHeight = initial.clientHeight
  let scrollTop = initial.scrollTop
  Object.defineProperty(element, 'scrollHeight', { configurable: true, get: () => scrollHeight })
  Object.defineProperty(element, 'clientHeight', { configurable: true, get: () => clientHeight })
  Object.defineProperty(element, 'scrollTop', {
    configurable: true,
    get: () => scrollTop,
    set: (value) => {
      scrollTop = value
    },
  })
  return {
    setScrollHeight: (value: number) => { scrollHeight = value },
    setClientHeight: (value: number) => { clientHeight = value },
    setScrollTop: (value: number) => { scrollTop = value },
    maxScrollTop: () => Math.max(0, scrollHeight - clientHeight),
  }
}

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
    const scrollMetrics = mockScrollMetrics(scroller)

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

    await waitFor(() => expect(scroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
  })

  it('用真实列表底部 spacer 避让浮动输入区，并禁止对话容器横向滚动', () => {
    const { container } = render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[{ type: 'message', role: 'assistant', content: '最后一行内容' }]}
        bottomPaddingClassName="pb-36"
        bottomPaddingPx={240}
      />,
    )

    const scroller = container.querySelector('.nova-chat-canvas')
    expect(scroller).toHaveClass('overflow-x-hidden')
    expect(scroller).not.toHaveStyle({ paddingBottom: '240px' })
    expect(container.querySelector('[data-nova-chat-bottom-spacer]')).toHaveStyle({ height: '240px' })
  })

  it('用户向上浏览时消息更新不会自动拉回底部', async () => {
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
    const scrollMetrics = mockScrollMetrics(scroller)

    await waitFor(() => expect(scroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
    fireEvent.wheel(scroller, { deltaY: -120 })
    fireEvent.keyDown(scroller, { key: 'ArrowUp' })
    scroller.scrollTop = 200
    fireEvent.scroll(scroller)

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
  })

  it('流式内容增长时保持锁定在底部', async () => {
    const { container, rerender } = render(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '继续写' },
          { type: 'message', role: 'assistant', content: '第一段', streaming: true },
        ]}
        scrollResetKey="session-a"
      />,
    )
    const scroller = container.firstElementChild as HTMLDivElement
    const scrollMetrics = mockScrollMetrics(scroller)
    scroller.scrollTop = scrollMetrics.maxScrollTop()
    fireEvent.scroll(scroller)

    scrollMetrics.setScrollHeight(1500)
    fireEvent.scroll(scroller)
    rerender(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '继续写' },
          { type: 'message', role: 'assistant', content: '第一段\n\n第二段\n\n第三段', streaming: true },
        ]}
        scrollResetKey="session-a"
      />,
    )

    await waitFor(() => expect(scroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
  })

  it('用户重新滚到底部后恢复流式锁定', async () => {
    const { container, rerender } = render(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '继续写' },
          { type: 'message', role: 'assistant', content: '第一段', streaming: true },
        ]}
        scrollResetKey="session-a"
      />,
    )
    const scroller = container.firstElementChild as HTMLDivElement
    const scrollMetrics = mockScrollMetrics(scroller)

    scroller.scrollTop = scrollMetrics.maxScrollTop()
    fireEvent.scroll(scroller)
    scroller.scrollTop = 120
    fireEvent.scroll(scroller)
    scrollMetrics.setScrollHeight(1500)
    rerender(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '继续写' },
          { type: 'message', role: 'assistant', content: '第一段\n\n用户还在看历史', streaming: true },
        ]}
        scrollResetKey="session-a"
      />,
    )
    expect(scroller.scrollTop).toBe(120)

    scroller.scrollTop = scrollMetrics.maxScrollTop()
    fireEvent.scroll(scroller)
    scrollMetrics.setScrollHeight(1800)
    rerender(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '继续写' },
          { type: 'message', role: 'assistant', content: '第一段\n\n用户还在看历史\n\n回到底部后继续跟随', streaming: true },
        ]}
        scrollResetKey="session-a"
      />,
    )

    await waitFor(() => expect(scroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
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
    expect(screen.getAllByRole('button', { name: '复制消息' })).toHaveLength(2)
  })

  it('消息悬浮复制按钮只复制用户和 Agent 正文消息，并显示成功反馈后恢复', async () => {
    vi.useFakeTimers()
    try {
      const writeText = vi.fn().mockResolvedValue(undefined)
      const handleEdit = vi.fn()
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { writeText },
      })

      render(
        <MessageList
          isStreaming={false}
          activityContent=""
          onEditMessage={handleEdit}
          messages={[
            { id: 'user-copy', type: 'message', role: 'user', content: '用户正文', turn_id: 'turn-user', created_at: '2026-06-26T09:00:00Z' },
            { id: 'assistant-copy', type: 'message', role: 'assistant', content: 'Agent 正文', created_at: '2026-06-26T09:00:01Z' },
            { id: 'subagent-copy', type: 'message', role: 'assistant', content: 'SubAgent 正文', agent_name: 'researcher', subagent: true, created_at: '2026-06-26T09:00:02Z' },
            { id: 'tool-copy', type: 'message', role: 'tool_call', content: 'execute\n{}', name: 'execute', created_at: '2026-06-26T09:00:03Z' },
          ]}
        />,
      )

      const copyButtons = screen.getAllByRole('button', { name: '复制消息' })
      expect(copyButtons).toHaveLength(2)

      fireEvent.click(copyButtons[0])
      await act(async () => {
        await Promise.resolve()
      })
      expect(screen.getByRole('button', { name: '已复制' })).toBeInTheDocument()
      act(() => {
        vi.advanceTimersByTime(1200)
      })
      expect(screen.getAllByRole('button', { name: '复制消息' })).toHaveLength(2)

      fireEvent.click(copyButtons[1])

      expect(writeText).toHaveBeenNthCalledWith(1, '用户正文')
      expect(writeText).toHaveBeenNthCalledWith(2, 'Agent 正文')

      fireEvent.click(screen.getByRole('button', { name: '编辑这轮输入' }))
      expect(handleEdit).toHaveBeenCalledWith(expect.objectContaining({ turn_id: 'turn-user' }))
    } finally {
      vi.useRealTimers()
    }
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

  it('长消息列表只挂载可视窗口附近的消息行', async () => {
    const messages: ChatMessage[] = Array.from({ length: 1000 }, (_, index) => ({
      id: `message-${index}`,
      type: 'message',
      role: index % 2 === 0 ? 'user' : 'assistant',
      content: `长列表消息 ${index}`,
    }))

    const { container } = render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={messages}
        scrollResetKey="long-session"
      />,
    )

    await waitFor(() => {
      expect(container.querySelectorAll('[data-nova-chat-item]').length).toBeGreaterThan(0)
    })
    expect(container.querySelectorAll('[data-nova-chat-item]').length).toBeLessThan(120)
    expect(screen.queryByText('长列表消息 500')).not.toBeInTheDocument()
  })
})
