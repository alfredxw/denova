import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { MessageItem } from './MessageItem'

describe('MessageItem', () => {
  it('稳定 assistant 消息使用完整 Markdown 渲染', () => {
    render(<MessageItem message={{ role: 'assistant', content: '# 标题\n\n- 条目' }} />)

    expect(screen.getByRole('heading', { name: '标题' })).toBeInTheDocument()
    expect(screen.getByText('条目')).toBeInTheDocument()
  })

  it('assistant 消息不展示 Nova 标题和气泡容器', () => {
    const { container } = render(<MessageItem message={{ role: 'assistant', content: '直接展示正文' }} />)

    expect(screen.queryByText('Nova')).not.toBeInTheDocument()
    expect(container.querySelector('.nova-assistant-message')).toBeNull()
    expect(container.querySelector('.chat-agent-message')).toHaveTextContent('直接展示正文')
  })

  it('流式 assistant 消息即时渲染常见 Markdown 结构', () => {
    render(<MessageItem message={{ role: 'assistant', content: '# 实时标题\n- 实时条目\n`cmd`', streaming: true }} />)

    expect(screen.getByRole('heading', { name: '实时标题' })).toBeInTheDocument()
    expect(screen.getByText('实时标题')).toBeInTheDocument()
    expect(screen.getByText('实时条目')).toBeInTheDocument()
    expect(screen.getByText('cmd')).toBeInTheDocument()
  })

  it('流式和持久化 assistant 消息使用一致的 Markdown DOM 结构', () => {
    const content = '# 标题\n\n第一段。\n\n- 条目 A\n- 条目 B\n\n> 引用'
    const { container, rerender } = render(<MessageItem message={{ role: 'assistant', content, streaming: true }} />)
    const streamedTags = Array.from(container.querySelector('.chat-agent-message')?.children || []).map((node) => node.tagName)

    rerender(<MessageItem message={{ role: 'assistant', content, streaming: false }} />)
    const persistedTags = Array.from(container.querySelector('.chat-agent-message')?.children || []).map((node) => node.tagName)

    expect(streamedTags).toEqual(['H1', 'P', 'UL', 'BLOCKQUOTE'])
    expect(persistedTags).toEqual(streamedTags)
  })

  it('互动模式 assistant 消息高亮常见对白引号', () => {
    const { container } = render(
      <MessageItem
        highlightDialogue
        message={{ role: 'assistant', content: '他说：“走吧。”\n\n她答：「等等。」\n\n旁白写道 "now".' }}
      />,
    )

    const highlights = container.querySelectorAll('.nova-dialogue-highlight')
    expect(highlights).toHaveLength(3)
    expect(highlights[0]).toHaveTextContent('“走吧。”')
    expect(highlights[1]).toHaveTextContent('「等等。」')
    expect(highlights[2]).toHaveTextContent('"now"')
  })

  it('普通 assistant 消息默认不高亮对白', () => {
    const { container } = render(<MessageItem message={{ role: 'assistant', content: '他说：“走吧。”' }} />)

    expect(container.querySelector('.nova-dialogue-highlight')).toBeNull()
  })

  it('流式互动消息同样高亮对白', () => {
    const { container } = render(
      <MessageItem
        highlightDialogue
        message={{ role: 'assistant', content: '他说：“走吧。”\n她答：「等等。」', streaming: true }}
      />,
    )

    const highlights = container.querySelectorAll('.nova-dialogue-highlight')
    expect(highlights).toHaveLength(2)
  })

  it('互动消息在最早版本缺少版本索引时仍显示下一版切换按钮', async () => {
    const user = userEvent.setup()
    const handleSwitch = vi.fn()

    render(
      <MessageItem
        message={{
          role: 'assistant',
          content: '最早版本',
          turn_id: 'turn-1',
          turn_versions: [
            { turn_id: 'turn-1', ts: '2026-05-31T00:00:00Z', current: true },
            { turn_id: 'turn-2', ts: '2026-05-31T00:01:00Z' },
          ],
        }}
        onRegenerate={vi.fn()}
        onSwitchVersion={handleSwitch}
      />,
    )

    expect(screen.getByText('1/2')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '切换到上一版' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: '切换到下一版' }))
    expect(handleSwitch).toHaveBeenCalledWith(expect.objectContaining({ turn_id: 'turn-1' }), 1)
  })

  it('思考过程流式时默认展开，结束后默认折叠但可手动展开', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<MessageItem message={{ role: 'thinking', content: '正在分析', streaming: true }} />)

    expect(screen.getByText('正在分析')).toBeInTheDocument()

    rerender(<MessageItem message={{ role: 'thinking', content: '已经分析完', streaming: false }} />)
    expect(screen.queryByText('已经分析完')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /思考过程/ }))
    expect(screen.getByText('已经分析完')).toBeInTheDocument()
  })

  it('工具调用卡片展示工具名、摘要和成功结果', () => {
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'write_file\n{"path":"chapters/ch01.md"}',
          name: 'write_file',
          args: '{"path":"chapters/ch01.md"}',
          status: 'success',
          result: '写入完成',
        }}
      />,
    )

    expect(screen.getByText('调用工具')).toBeInTheDocument()
    expect(screen.getByText('write_file')).toBeInTheDocument()
    expect(screen.getByText('写入完成')).toBeInTheDocument()
  })

  it('write_todos 工具卡片渲染为待办列表，并显示进度', () => {
    const args = JSON.stringify({
      todos: [
        { content: '梳理需求', activeForm: '梳理需求中', status: 'completed' },
        { content: '实现接口', activeForm: '实现接口中', status: 'in_progress' },
        { content: '补充测试', activeForm: '补充测试中', status: 'pending' },
      ],
    })

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'write_todos',
          name: 'write_todos',
          args,
          status: 'running',
        }}
      />,
    )

    expect(screen.getByText('待办列表')).toBeInTheDocument()
    expect(screen.getByText('1/3')).toBeInTheDocument()
    expect(screen.getByText('梳理需求')).toBeInTheDocument()
    expect(screen.getAllByText('实现接口中').length).toBeGreaterThan(0)
    expect(screen.getByText('补充测试')).toBeInTheDocument()
  })

  it('write_todos 工具卡片在流式不完整 JSON 时仍能渲染已完整的 todo 项', () => {
    const partial = '{"todos":[{"content":"第一项","activeForm":"做第一项","status":"completed"},{"content":"第二项","activeForm":"做第二项","stat'

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'write_todos',
          name: 'write_todos',
          args: partial,
          status: 'running',
        }}
      />,
    )

    expect(screen.getByText('待办列表')).toBeInTheDocument()
    expect(screen.getByText('第一项')).toBeInTheDocument()
  })

  it('task 工具卡片展示委派目标和结果', async () => {
    const user = userEvent.setup()
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'task',
          name: 'task',
          args: '{"subagent_type":"researcher","description":"查找线索"}',
          status: 'success',
          result: '找到三条线索',
        }}
      />,
    )

    expect(screen.getByText('委派任务')).toBeInTheDocument()
    expect(screen.queryByText('task')).not.toBeInTheDocument()
    expect(screen.getByText('委派给 researcher')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '详情' }))
    expect(screen.getByText('委派结果')).toBeInTheDocument()
    expect(screen.getAllByText('找到三条线索').length).toBeGreaterThan(0)
  })

  it('SubAgent assistant 输出默认显示紧凑小窗并可行内展开收起', async () => {
    const user = userEvent.setup()
    const longContent = `# 调研结果\n\n${'这是用于折叠预览的前置内容。'.repeat(20)}\n\n最终隐藏结论`

    render(
      <MessageItem
        message={{
          role: 'assistant',
          content: longContent,
          agent_name: 'researcher',
          subagent: true,
        }}
      />,
    )

    expect(screen.getByText('researcher 输出')).toBeInTheDocument()
    expect(screen.getByText('输出完成')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /researcher 输出/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('最终隐藏结论')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /researcher 输出/ }))
    expect(screen.getByRole('button', { name: /researcher 输出/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('最终隐藏结论')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /researcher 输出/ }))
    expect(screen.getByRole('button', { name: /researcher 输出/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('最终隐藏结论')).not.toBeInTheDocument()
  })

  it('SubAgent assistant 流式输出显示运行状态和预览', () => {
    render(
      <MessageItem
        message={{
          role: 'assistant',
          content: '实时片段',
          agent_name: 'researcher',
          subagent: true,
          streaming: true,
        }}
      />,
    )

    expect(screen.getByText('researcher 输出')).toBeInTheDocument()
    expect(screen.getByText('正在流式输出')).toBeInTheDocument()
    expect(screen.getByText('实时片段')).toBeInTheDocument()
  })

  it('SubAgent assistant 有详情回调时只打开子会话详情', async () => {
    const user = userEvent.setup()
    const handleOpen = vi.fn()

    const longContent = `${'详情预览。'.repeat(80)}\n\n隐藏的完整结论`
    render(
      <MessageItem
        message={{
          role: 'assistant',
          content: longContent,
          agent_name: 'researcher',
          subagent: true,
          subagent_session_id: 'run-1-subagent-01-researcher',
        }}
        onOpenSubAgentSession={handleOpen}
      />,
    )

    expect(screen.getByText('打开详情')).toBeInTheDocument()
    expect(screen.queryByText('隐藏的完整结论')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /researcher 输出/ }))
    expect(handleOpen).toHaveBeenCalledWith(expect.objectContaining({ subagent_session_id: 'run-1-subagent-01-researcher' }))
    expect(screen.queryByText('隐藏的完整结论')).not.toBeInTheDocument()
  })

  it('上下文压缩消息渲染为单个带 Loading 的简洁小窗', () => {
    render(
      <MessageItem
        message={{
          role: 'context_compaction',
          status: 'running',
          phase: 'pre_run',
          attempt: 2,
          tokens_before: 900,
          context_window_tokens: 1000,
          threshold: 0.9,
          source_message_count: 12,
          content: '压缩摘要流式片段',
          streaming: true,
        }}
      />,
    )

    expect(screen.getByText('上下文压缩')).toBeInTheDocument()
    expect(screen.getByText('压缩中')).toBeInTheDocument()
    expect(screen.getByLabelText('压缩中')).toBeInTheDocument()
    expect(screen.getByText('第 2 次')).toBeInTheDocument()
    expect(screen.getByText('压缩摘要流式片段')).toBeInTheDocument()
    expect(screen.queryByText('90%')).not.toBeInTheDocument()
    expect(screen.queryByText('阈值 90%')).not.toBeInTheDocument()
  })
})
