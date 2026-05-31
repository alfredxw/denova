import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
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

    expect(screen.getByText('实时标题')).toBeInTheDocument()
    expect(screen.getByText('实时条目')).toBeInTheDocument()
    expect(screen.getByText('cmd')).toBeInTheDocument()
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
})
