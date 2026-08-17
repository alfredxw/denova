import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderToStaticMarkup } from 'react-dom/server'
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { MessageItem as ProjectMessageItem } from './MessageItem'

function MessageItem(props: ComponentProps<typeof ProjectMessageItem>) {
  return <ProjectMessageItem {...props} projectId={props.projectId || 'project-message'} />
}

function mockScrollMetrics(element: HTMLElement, initial = { scrollHeight: 520, clientHeight: 128, scrollTop: 0 }) {
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
    maxScrollTop: () => Math.max(0, scrollHeight - clientHeight),
  }
}

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

  it('流式 assistant 以纯文本更新，完成后再解析 Markdown', () => {
    const content = '# 实时标题\n- 实时条目\n`cmd`'
    const { container, rerender } = render(<MessageItem message={{ role: 'assistant', content, streaming: true }} />)

    expect(screen.queryByRole('heading', { name: '实时标题' })).toBeNull()
    expect(container.querySelector('.chat-agent-message')).toHaveTextContent('# 实时标题 - 实时条目 `cmd`')

    rerender(<MessageItem message={{ role: 'assistant', content, streaming: false }} />)
    expect(screen.getByRole('heading', { name: '实时标题' })).toBeInTheDocument()
    expect(screen.getByText('实时条目')).toBeInTheDocument()
    expect(screen.getByText('cmd')).toBeInTheDocument()
  })

  it('流式 assistant 尚无正文时以 Shimmer 显示思考状态', () => {
    render(<MessageItem message={{ role: 'assistant', content: '', streaming: true }} />)

    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('思考中...')
    expect(status.querySelector('.bg-clip-text')).toBeInTheDocument()
  })

  it('流式 assistant 只渲染最新目标正文的一棵 Markdown 树', () => {
    const { container, rerender } = render(<MessageItem message={{ role: 'assistant', content: '第一行内容', streaming: true }} />)

    expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()

    rerender(<MessageItem message={{ role: 'assistant', content: '第一行内容', streaming_target_content: '第一行内容\n第二行内容', streaming: true }} />)

    expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()
    expect(container.querySelector('.chat-agent-message')).toHaveTextContent('第二行内容')

    rerender(<MessageItem message={{ role: 'assistant', content: '第一行内容\n第二行内容', streaming: true }} />)

    expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()
    expect(container.querySelector('.chat-agent-message')).toHaveTextContent('第二行内容')
  })

  it('流式正文使用与持久化 Markdown 一致的段落结构，完成后再解析块级语法', () => {
    const content = '# 标题\n\n第一段。\n\n- 条目 A\n- 条目 B\n\n> 引用'
    const { container, rerender } = render(<MessageItem message={{ role: 'assistant', content, streaming: true }} />)
    const streamedTags = Array.from(container.querySelector('.chat-agent-message')?.children || []).map((node) => node.tagName)

    rerender(<MessageItem message={{ role: 'assistant', content, streaming: false }} />)
    const persistedTags = Array.from(container.querySelector('.chat-agent-message')?.children || []).map((node) => node.tagName)

    expect(streamedTags).toEqual(['P', 'P', 'P', 'P'])
    expect(persistedTags).toEqual(['H1', 'P', 'UL', 'BLOCKQUOTE'])
  })

  it('assistant 只在完成后激活 Markdown 链接行为', () => {
    const content = '结论。[SearXNG Search API](https://docs.searxng.org/dev/search_api.html)\n\n[站内文档](/docs/web-access)'
    const { rerender } = render(<MessageItem message={{ role: 'assistant', content, streaming: true }} />)
    expect(screen.queryByRole('link')).toBeNull()

    const assertLinks = () => {
      expect(screen.getByRole('link', { name: 'SearXNG Search API' })).toHaveAttribute('href', 'https://docs.searxng.org/dev/search_api.html')
      expect(screen.getByRole('link', { name: 'SearXNG Search API' })).toHaveAttribute('target', '_blank')
      expect(screen.getByRole('link', { name: 'SearXNG Search API' })).toHaveAttribute('rel', 'noopener noreferrer')
      expect(screen.getByRole('link', { name: '站内文档' })).not.toHaveAttribute('target')
      expect(screen.getByRole('link', { name: '站内文档' })).not.toHaveAttribute('rel')
    }

    rerender(<MessageItem message={{ role: 'assistant', content, streaming: false }} />)
    assertLinks()
  })

  it('持久化 assistant 的 GFM 自动链接仍使用完整 Markdown 渲染', () => {
    render(<MessageItem message={{ role: 'assistant', content: '参考 https://example.com/docs' }} />)

    expect(screen.getByRole('link', { name: 'https://example.com/docs' })).toHaveAttribute('href', 'https://example.com/docs')
  })

  it('流式 assistant 不展示操作按钮但预留底部操作区，完成后再展示复制', () => {
    const { container, rerender } = render(
      <MessageItem
        message={{ role: 'assistant', content: '故事继续。', streaming: true }}
      />,
    )

    expect(container.querySelector('.nova-message-meta-spacer')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '复制消息' })).not.toBeInTheDocument()

    rerender(
      <MessageItem
        message={{ role: 'assistant', content: '故事继续。', turn_id: 'turn-1', streaming: false }}
      />,
    )

    expect(container.querySelector('.nova-message-meta-spacer')).toBeNull()
    expect(container.querySelector('.nova-message-meta')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '复制消息' })).toBeInTheDocument()
  })

  it('流式 assistant 即使存在完成后操作，也要等输出结束才显示按钮', () => {
    render(
      <MessageItem
        message={{ role: 'assistant', content: '故事继续。', turn_id: 'turn-1', streaming: true }}
        onEditAssistantReply={vi.fn()}
        onGenerateInteractiveImage={vi.fn()}
        onRegenerate={vi.fn()}
      />,
    )

    expect(screen.queryByRole('button', { name: '复制消息' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '编辑 AI 回复' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '生成互动图像' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重新生成这一轮' })).not.toBeInTheDocument()
  })

  it('游戏模式持久化 AI 回复把编辑按钮放在复制与图像生成同一操作行', async () => {
    const user = userEvent.setup()
    const handleEdit = vi.fn()
    const handleCreateBranch = vi.fn()
    const { container } = render(
      <MessageItem
        message={{ role: 'assistant', content: '朋友住在 3 楼 403 室。', turn_id: 'turn-1' }}
        onEditAssistantReply={handleEdit}
        onCreateBranch={handleCreateBranch}
        onGenerateInteractiveImage={vi.fn()}
      />,
    )

    const actionRow = container.querySelector('.nova-message-meta') as HTMLElement
    expect(within(actionRow).getAllByRole('button').map((button) => button.getAttribute('aria-label'))).toEqual([
      '复制消息',
      '编辑 AI 回复',
      '从此处创建分支',
      '生成互动图像',
    ])

    await user.click(screen.getByRole('button', { name: '编辑 AI 回复' }))
    expect(handleEdit).toHaveBeenCalledWith(expect.objectContaining({ turn_id: 'turn-1' }))
    await user.click(screen.getByRole('button', { name: '从此处创建分支' }))
    expect(handleCreateBranch).toHaveBeenCalledWith(expect.objectContaining({ turn_id: 'turn-1' }))
  })

  it('错误消息结束后展示复制和重试操作', () => {
    const { container } = render(
      <MessageItem
        message={{ role: 'error', content: '[NodeRunError] 400 Bad Request', streaming: false }}
        onRegenerate={vi.fn()}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('[NodeRunError] 400 Bad Request')
    expect(container.querySelector('[role="alert"] span')).toHaveClass('whitespace-pre-wrap', 'break-words')
    expect(container.querySelector('[role="alert"] span')).not.toHaveClass('truncate')
    expect(screen.getByRole('button', { name: '复制消息' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重新生成这一轮' })).toBeInTheDocument()
  })

  it('流式 user 消息没有编辑权限时也能复制并保留与后续内容的间隔', () => {
    const { container } = render(<MessageItem message={{ role: 'user', content: '继续', streaming: false }} />)

    expect(container.querySelector('.nova-message-meta-spacer')).toBeNull()
    expect(container.querySelector('.nova-message-meta')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '复制消息' })).toBeInTheDocument()
  })

  it('游戏模式 assistant 消息高亮常见对白引号', () => {
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

  it('游戏模式 assistant 消息不按角色名冒号高亮，避免误判叙述句', () => {
    const { container } = render(
      <MessageItem
        highlightDialogue
        message={{ role: 'assistant', content: '林晚：我们走。\n\nJohn: wait here.\n\n他说：“走吧。”' }}
      />,
    )

    const highlights = container.querySelectorAll('.nova-dialogue-highlight')
    expect(highlights).toHaveLength(1)
    expect(highlights[0]).toHaveTextContent('“走吧。”')
  })

  it('普通 assistant 消息默认不高亮对白', () => {
    const { container } = render(<MessageItem message={{ role: 'assistant', content: '他说：“走吧。”' }} />)

    expect(container.querySelector('.nova-dialogue-highlight')).toBeNull()
  })

  it('流式互动消息完成后再高亮对白', () => {
    const content = '他说：“走吧。”\n她答：「等等。」'
    const { container, rerender } = render(
      <MessageItem
        highlightDialogue
        message={{ role: 'assistant', content, streaming: true }}
      />,
    )

    expect(container.querySelectorAll('.nova-dialogue-highlight')).toHaveLength(0)
    rerender(<MessageItem highlightDialogue message={{ role: 'assistant', content, streaming: false }} />)
    expect(container.querySelectorAll('.nova-dialogue-highlight')).toHaveLength(2)
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

  it('思考流式时默认展开，结束后保留首行摘要并默认折叠正文', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<MessageItem message={{ role: 'thinking', content: '正在分析', streaming: true }} />)

    expect(screen.getByRole('region', { name: '思考内容' })).toHaveTextContent('正在分析')

    rerender(<MessageItem message={{ role: 'thinking', content: '已经分析完', streaming: false }} />)
    expect(screen.queryByRole('region', { name: '思考内容' })).not.toBeInTheDocument()
    expect(screen.getByText('已经分析完')).toHaveAttribute('data-thinking-preview')

    await user.click(screen.getByRole('button', { name: '展开思考' }))
    expect(screen.getByRole('region', { name: '思考内容' })).toHaveTextContent('已经分析完')
  })

  it('历史思考首帧直接显示摘要并保持正文折叠', () => {
    const markup = renderToStaticMarkup(
      <MessageItem message={{ role: 'thinking', content: '历史思考内容', streaming: false }} />,
    )

    expect(markup).toContain('aria-expanded="false"')
    expect(markup).toContain('data-thinking-preview="true"')
    expect(markup).toContain('历史思考内容')
    expect(markup).not.toContain('aria-label="思考内容"')
  })

  it('思考标题仅展示首个非空行并允许窄屏省略', () => {
    render(<MessageItem message={{ role: 'thinking', content: '\n  Now I need to update the recent events.  \nSecond line' }} />)

    const trigger = screen.getByRole('button', { name: '展开思考' })
    expect(trigger).toHaveTextContent('思考·Now I need to update the recent events.')
    const preview = within(trigger).getByText('Now I need to update the recent events.')
    expect(preview).toHaveClass('min-w-0', 'flex-1', 'truncate')
    expect(within(trigger).queryByText('Second line')).not.toBeInTheDocument()
  })

  it('直接增长的流式 thinking 立即复用单棵文本树显示最新内容', () => {
    const { container, rerender } = render(<MessageItem message={{ role: 'thinking', content: '正在分析', streaming: true }} />)

    rerender(<MessageItem message={{ role: 'thinking', content: '正在分析下一条线索', streaming: true }} />)

    expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()
    expect(container.querySelector('[data-thinking-preview]')).toHaveTextContent('正在分析下一条线索')
    expect(screen.getByRole('region', { name: '思考内容' })).toHaveTextContent('正在分析下一条线索')
  })

  it('thinking 使用带浅边框和 scroll fade 的 320px 上限窗口', () => {
    render(<MessageItem message={{ role: 'thinking', content: '逐行分析。\n'.repeat(24), streaming: true }} />)

    const content = screen.getByRole('region', { name: '思考内容' })
    expect(content).toHaveClass('scroll-fade-y', 'scroll-fade-8', 'max-h-80', 'overflow-y-auto')
    expect(content).not.toHaveClass('h-40', 'border-l', 'px-3')
    expect(content.parentElement).toHaveAttribute('data-thinking-scroll-frame')
    expect(content.parentElement).toHaveClass('border', 'border-border/60')
  })

  it('流式 thinking 默认追尾，用户上滚后暂停并在回到底部后恢复', async () => {
    const renderThinking = (content: string) => (
      <MessageItem message={{ id: 'thinking-scroll', role: 'thinking', content, streaming: true }} />
    )
    const { rerender } = render(renderThinking('第一段。'.repeat(160)))
    const content = screen.getByRole('region', { name: '思考内容' })
    const metrics = mockScrollMetrics(content, { scrollHeight: 520, clientHeight: 320, scrollTop: 0 })

    await waitFor(() => expect(content.scrollTop).toBe(metrics.maxScrollTop()))

    content.scrollTop = 240
    fireEvent.wheel(content, { deltaY: -12 })
    metrics.setScrollHeight(760)
    rerender(renderThinking('第一段。'.repeat(160) + '第二段。'.repeat(80)))
    await act(async () => { await new Promise(resolve => requestAnimationFrame(resolve)) })
    expect(content.scrollTop).toBe(240)

    content.scrollTop = metrics.maxScrollTop()
    fireEvent.scroll(content)
    metrics.setScrollHeight(900)
    rerender(renderThinking('第一段。'.repeat(160) + '第二段。'.repeat(80) + '第三段。'.repeat(80)))
    await waitFor(() => expect(content.scrollTop).toBe(metrics.maxScrollTop()))
  })

  it('工具调用卡片展示工具名、摘要和成功结果', () => {
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'write\n{"path":"chapters/ch01.md"}',
          name: 'write',
          args: '{"path":"chapters/ch01.md"}',
          status: 'success',
          result: '写入完成',
        }}
      />,
    )

    expect(screen.queryByText('调用工具')).not.toBeInTheDocument()
    expect(screen.getByText('写入')).toHaveAttribute('title', 'write')
    expect(screen.getByText('ch01.md')).toBeInTheDocument()
  })

  it('工具调用卡片支持英文名称并原样保留自定义工具名', () => {
    act(() => setConfiguredLocale('en-US'))
    try {
      const { rerender } = render(
        <MessageItem
          message={{
            role: 'tool_call',
            content: 'write',
            name: 'write',
            status: 'success',
          }}
        />,
      )

      expect(screen.queryByText('Calling Tool')).not.toBeInTheDocument()
      expect(screen.getByText('Write')).toHaveAttribute('title', 'write')

      rerender(
        <MessageItem
          message={{
            role: 'tool_call',
            content: 'grep',
            name: 'grep',
            status: 'success',
          }}
        />,
      )

      expect(screen.getByText('Search')).toHaveAttribute('title', 'grep')

      rerender(
        <MessageItem
          message={{
            role: 'tool_call',
            content: 'publish_manuscript',
            name: 'publish_manuscript',
            status: 'success',
          }}
        />,
      )

      expect(screen.getByText('publish_manuscript')).not.toHaveAttribute('title')
    } finally {
      act(() => setConfiguredLocale('zh-CN'))
    }
  })

  it('grep 和 shell 工具卡片优先展示并保留调用描述', () => {
    const { rerender } = render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'bash',
          name: 'bash',
          args: '{"command":"go test ./agent/tools","description":"运行工具包测试"}',
          status: 'running',
          tool_presentation: { call: 'terminal', result: 'terminal' },
        }}
      />,
    )

    expect(screen.getByText('命令')).toHaveAttribute('title', 'bash')
    expect(screen.getByText('运行工具包测试')).toBeInTheDocument()

    rerender(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'grep',
          name: 'grep',
          args: '{"command":"rg -n TODO .","description":"检索待办项"}',
          status: 'running',
          tool_presentation: { call: 'search', result: 'search' },
        }}
      />,
    )

    expect(screen.getByText('检索')).toHaveAttribute('title', 'grep')
    expect(screen.getByText('检索待办项')).toBeInTheDocument()

    rerender(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'bash',
          name: 'bash',
          args: '{"command":"go test ./agent/tools","description":"运行工具包测试"}',
          status: 'success',
          result: 'ok',
          tool_presentation: { call: 'terminal', result: 'terminal' },
        }}
      />,
    )

    expect(screen.getByText('运行工具包测试')).toBeInTheDocument()
  })

  it('工具卡详情随卡片宽度换行且仅保留纵向滚动', async () => {
    const user = userEvent.setup()
    const longToken = 'a'.repeat(240)
    const { container } = render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'read',
          name: 'read',
          args: JSON.stringify({ path: `chapters/${longToken}.md` }),
          status: 'success',
          result: longToken,
        }}
      />,
    )

    await user.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)

    const detail = container.querySelector('[data-slot="collapsible-content"]')
    expect(detail).toHaveClass('min-w-0', 'max-w-full', 'overflow-x-hidden', 'overflow-y-auto')
    expect(detail).not.toHaveClass('overflow-auto')
    detail?.querySelectorAll('pre').forEach((element) => {
      expect(element).toHaveClass('min-w-0', 'max-w-full', 'whitespace-pre-wrap', '[overflow-wrap:anywhere]')
    })
  })

  it('失败的工具卡片在工具名后单行省略错误原因，并可展开查看完整详情', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'edit\n{"path":"chapter.md"}',
          name: 'edit',
          args: '{"path":"chapter.md"}',
          status: 'error',
          result: 'invoke tool "edit": old_string was not found in chapter.md',
        }}
      />,
    )

    const header = container.querySelector('[data-nova-tool-header]') as HTMLElement
    const summary = container.querySelector('[data-nova-tool-summary]') as HTMLElement
    expect(header).toHaveClass('grid', 'grid-cols-[auto_minmax(0,1fr)]')
    expect(summary.parentElement).toHaveTextContent('编辑invoke tool "edit": old_string was not found in chapter.md')
    expect(summary).toHaveClass('truncate', 'text-[var(--nova-danger)]')

    await user.click(header)
    expect(header).toHaveAttribute('aria-expanded', 'true')
    expect(container.querySelector('[data-slot="collapsible-content"]')).toHaveTextContent('invoke tool "edit": old_string was not found in chapter.md')
  })

  it('shell 生命周期成功且子进程非零退出时显示为注意结果而非工具失败', () => {
    const result = `${JSON.stringify({
      schema: 'process.result.v1',
      status: 'failed',
      exit_code: 2,
      output_truncated: false,
      recovery: { retryable: true, suggestion: '检查输出并修正命令。' },
    })}\ncommand not found`
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'bash\n{"command":"missing-command"}',
          name: 'bash',
          args: '{"command":"missing-command"}',
          status: 'success',
          result,
        }}
      />,
    )

    expect(screen.getByText('命令已结束（退出码 2）')).toHaveClass('text-[var(--nova-warning)]')
    expect(screen.queryByText(/检查输出并修正命令/)).not.toBeInTheDocument()
  })

  it('read 按页返回时以正常完成态展示可选 continuation', () => {
    const result = `${JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      limits: { returned: 80, truncated: true, next_offset: 80 },
      recovery: { retryable: true, suggestion: '使用 offset 继续读取。' },
    })}\npartial content`
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'read\n{"path":"chapters/long.md"}',
          name: 'read',
          args: '{"path":"chapters/long.md"}',
          status: 'success',
          result,
        }}
      />,
    )

    expect(screen.getByText('long.md · 已返回当前批次 · 后续内容可从 offset=80 读取')).toHaveClass('text-[var(--nova-text-faint)]')
    expect(screen.queryByText(/使用 offset 继续读取/)).not.toBeInTheDocument()
  })

  it('独立工具结果卡把非零进程退出呈现为需留意的结果', () => {
    render(
      <MessageItem
        message={{
          role: 'tool_result',
          content: `${JSON.stringify({ schema: 'process.result.v1', status: 'failed', exit_code: 1 })}\nstderr`,
        }}
      />,
    )

    expect(screen.getByText('工具结果需留意')).toBeInTheDocument()
    expect(screen.getByText('warning')).toBeInTheDocument()
    expect(screen.getByText('命令已结束（退出码 1）')).toBeInTheDocument()
    expect(screen.queryByText('工具执行失败')).not.toBeInTheDocument()
  })

  it('独立工具结果详情不会被超长无空格内容撑宽', async () => {
    const user = userEvent.setup()
    const longToken = 'b'.repeat(240)
    const { container } = render(
      <MessageItem message={{ role: 'tool_result', content: longToken }} />,
    )

    await user.click(screen.getByRole('button', { name: '展开' }))

    const detail = container.querySelector('pre')
    expect(detail).toHaveClass('max-w-full', 'overflow-x-hidden', 'overflow-y-auto', 'whitespace-pre-wrap', '[overflow-wrap:anywhere]')
    expect(detail).not.toHaveClass('overflow-auto')
  })

  it('网页工具卡片把结构化恢复状态显示为可读摘要', async () => {
    const user = userEvent.setup()
    const resultBody = JSON.stringify({
      schema: 'web_fetch.v1',
      status: 'blocked',
      attempts: [
        { method: 'direct_http', outcome: 'access_denied', http_status: 403 },
        { method: 'jina_reader', outcome: 'access_denied', http_status: 403 },
        { method: 'browser', outcome: 'access_denied', http_status: 403 },
      ],
      retry_strategy: 'use_alternate_source',
      suggested_action: 'Use another public source. 改用其他公开来源。',
      url: 'https://example.com/article',
      final_url: 'https://example.com/article',
    })
    const result = `${resultBody}\n\n[Denova tool result metadata]\nschema: tool_result.v1\nsource: web`
    const { container } = render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'web_fetch\n{"url":"https://example.com/article"}',
          name: 'web_fetch',
          args: '{"url":"https://example.com/article"}',
          status: 'success',
          result,
        }}
      />,
    )

    expect(screen.getByText('访问被阻止 · 改用其他公开来源')).toHaveClass('col-start-2', 'col-end-3', 'whitespace-normal')
    await user.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
    expect(screen.getByText(/"status": "blocked"/)).toBeInTheDocument()
    expect(screen.getAllByText(/改用其他公开来源/)).toHaveLength(2)
  })

  it('显式 Skill 预加载复用现有工具卡展示名称和结果', () => {
    render(
      <MessageItem
        message={{
          id: 'explicit-skill-01',
          role: 'tool_call',
          content: 'skill\n{"name":"alpha"}',
          name: 'skill',
          args: '{"name":"alpha"}',
          status: 'success',
          result: '# Skill: alpha\n\nALPHA_BODY',
        }}
      />,
    )

    expect(screen.queryByText('调用工具')).not.toBeInTheDocument()
    expect(screen.getByText('加载 Skill')).toHaveAttribute('title', 'skill')
    expect(screen.getByText('# Skill: alpha ALPHA_BODY')).toBeInTheDocument()
  })

  it('所有工具都按原始参数流展示，不依赖工具名或字段', () => {
    const args = '{"path":"ignored.md","description":"ignored","nested":[{"anything":"完整但保持紧凑"}]}'
    const { container } = render(
      <MessageItem
        message={{
          id: 'tool-generic-stream',
          role: 'tool_call',
          content: 'custom_transform',
          name: 'custom_transform',
          args,
          status: 'running',
          streaming: true,
        }}
      />,
    )

    const preview = container.querySelector('[data-nova-scroll-lock="tool-input-stream"]')
    expect(preview).toBeInTheDocument()
    expect(preview?.textContent).toBe(args)
    expect(container.querySelector('[data-nova-tool-header]')).not.toHaveTextContent('ignored.md')
    expect(container.querySelector('[data-nova-tool-header]')).not.toHaveTextContent('ignored')
  })

  it('流式阶段绕过工具专用展示', () => {
    const args = '{"action":"update","mutations":[{"id":"first","text":"第一项","status":"pending"}]}'
    const { container } = render(
      <MessageItem
        message={{
          id: 'tool-opaque-stream',
          role: 'tool_call',
          content: 'todo',
          name: 'todo',
          args,
          status: 'running',
          streaming: true,
          tool_presentation: { call: 'todo', result: 'todo' },
        }}
      />,
    )

    expect(container.querySelector('[data-nova-scroll-lock="tool-input-stream"]')?.textContent).toBe(args)
    expect(screen.queryByText('待办列表')).not.toBeInTheDocument()
    expect(screen.queryByText('章节正文已在实时输出中隐藏，文件仍会正常写入。')).not.toBeInTheDocument()
  })

  it.each([
    ['grep', '{"description":"搜索章节标题","pattern":"第'],
    ['bash', '{"description":"检查构建结果","command":"pnpm'],
  ])('%s 流式卡片显示 description，同时保留原始输入', (name, args) => {
    const { container } = render(
      <MessageItem
        message={{
          id: `tool-stream-${name}`,
          role: 'tool_call',
          content: name,
          name,
          args,
          status: 'running',
          streaming: true,
        }}
      />,
    )

    expect(container.querySelector('[data-nova-tool-header]')).toHaveTextContent(name === 'grep' ? '搜索章节标题' : '检查构建结果')
    expect(container.querySelector('[data-nova-scroll-lock="tool-input-stream"]')?.textContent).toBe(args)
  })

  it.each(['read', 'write', 'edit'])('%s 流式卡片显示文件名，同时保留原始输入', (name) => {
    const args = `{"path":"chapters/${name}.md","payload":"still streaming`
    const { container } = render(
      <MessageItem
        message={{
          id: `tool-stream-file-${name}`,
          role: 'tool_call',
          content: name,
          name,
          args,
          status: 'running',
          streaming: true,
        }}
      />,
    )

    expect(container.querySelector('[data-nova-tool-header]')).toHaveTextContent(`${name}.md`)
    expect(container.querySelector('[data-nova-scroll-lock="tool-input-stream"]')?.textContent).toBe(args)
  })

  it('edit 从首个参数增量开始展示，不等待 new_string', () => {
    const baseMessage = {
      id: 'tool-incremental-edit',
      role: 'tool_call' as const,
      content: 'edit',
      name: 'edit',
      status: 'running' as const,
      streaming: true,
    }
    const { container, rerender } = render(
      <MessageItem
        message={{
          ...baseMessage,
          args: `{"path":"chapters/ch01.md","edits":[{"old_string":"${'旧正文。'.repeat(30)}`,
        }}
      />,
    )

    const initialHeader = container.querySelector('[data-nova-tool-header]')
    expect(initialHeader).toHaveAttribute('aria-expanded', 'false')
    expect(initialHeader).toHaveTextContent('编辑')
    expect(initialHeader).toHaveTextContent('ch01.md')
    expect(container.querySelector('[data-nova-scroll-lock="tool-input-stream"]')).toHaveTextContent('旧正文')

    rerender(
      <MessageItem
        message={{
          ...baseMessage,
          args: '{"path":"chapters/ch01.md","edits":[{"old_string":"旧正文","new_string":"新正文正在生成',
        }}
      />,
    )

    expect(container.querySelector('[data-nova-scroll-lock="tool-input-stream"]')).toHaveTextContent('新正文正在生成')
  })

  it('文件工具摘要只显示文件名，完整路径保留在展开详情中', async () => {
    const user = userEvent.setup()
    const path = '/Users/me/nova/chapters/ch01.md'
    const message = {
      id: 'tool-write-path-summary',
      role: 'tool_call' as const,
      content: 'write',
      name: 'write',
      args: JSON.stringify({ path, content: '新正文' }),
    }
    const { container, rerender } = render(
      <MessageItem message={{ ...message, status: 'running' }} />,
    )

    const liveHeader = container.querySelector('[data-nova-tool-header]')
    expect(liveHeader).toHaveTextContent('ch01.md')
    expect(liveHeader).not.toHaveTextContent(path)

    rerender(<MessageItem message={{ ...message, status: 'success', result: 'updated workspace file' }} />)
    const completedHeader = container.querySelector('[data-nova-tool-header]')
    expect(completedHeader).toHaveTextContent('ch01.md')
    expect(completedHeader).not.toHaveTextContent(path)

    await user.click(completedHeader as HTMLElement)
    expect(completedHeader).toHaveAttribute('aria-expanded', 'true')
    expect(Array.from(container.querySelectorAll('pre')).some((element) => element.textContent?.includes(path))).toBe(true)
  })

  it('read 完成后在卡片外层展示文件名', () => {
    const path = '/Users/me/nova/chapters/ch02.md'
    const { container } = render(
      <MessageItem
        message={{
          id: 'tool-read-path-summary',
          role: 'tool_call',
          content: 'read',
          name: 'read',
          args: JSON.stringify({ path }),
          status: 'success',
          result: 'chapter content',
        }}
      />,
    )

    const header = container.querySelector('[data-nova-tool-header]')
    expect(header).toHaveTextContent('读取')
    expect(header).toHaveTextContent('ch02.md')
    expect(header).not.toHaveTextContent(path)
  })

  it('章节插画工具卡片展示预览并触发插入', async () => {
    const user = userEvent.setup()
    const handleInsert = vi.fn()
    const illustration = {
      schema: 'chapter_illustration.v1',
      chapter_path: 'chapters/ch01.md',
      image_path: 'assets/illustrations/ch01/run/image.png',
      meta_path: 'assets/illustrations/ch01/run/meta.json',
      markdown: '![雨夜](assets/illustrations/ch01/run/image.png)',
      alt_text: '雨夜',
      profile_id: 'default',
      provider: 'openai',
      model: 'gpt-image-1',
      size: '4096x2304',
      quality: 'high',
      output_format: 'png',
    } as const

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'generate_image',
          name: 'generate_image',
          status: 'success',
          tool_presentation: { call: 'image', result: 'image' },
          illustration,
        }}
        onInsertIllustration={handleInsert}
      />,
    )

    expect(screen.getByText('章节插画')).toBeInTheDocument()
    expect(screen.getByText('assets/illustrations/ch01/run/image.png')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: '雨夜' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Fillustrations%2Fch01%2Frun%2Fimage.png')

    await user.click(screen.getByRole('button', { name: '放大查看章节插画' }))
    expect(within(screen.getByRole('dialog')).getByRole('img', { name: '雨夜' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Fillustrations%2Fch01%2Frun%2Fimage.png')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /插入正文/ }))
    expect(handleInsert).toHaveBeenCalledWith(illustration)
  })

  it('assistant Markdown 图像支持 workspace 路径展示和点击放大', async () => {
    const user = userEvent.setup()
    render(<MessageItem message={{ role: 'assistant', content: '![封面](assets/image/generated/cover.png)' }} />)

    expect(screen.getByRole('img', { name: '封面' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Fimage%2Fgenerated%2Fcover.png')

    await user.click(screen.getByRole('button', { name: '放大查看图像' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('img', { name: '封面' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Fimage%2Fgenerated%2Fcover.png')
    expect(within(dialog).queryByTitle('assets/image/generated/cover.png')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '放大' })).toBeInTheDocument()
  })

  it('assistant 回合正文下方内联展示互动图像版本', async () => {
    const user = userEvent.setup()
    render(
      <MessageItem
        message={{
          id: 'assistant-turn-1',
          role: 'assistant',
          content: '这一轮剧情。',
          turn_id: 'turn-1',
          interactive_images: [
            {
              schema: 'interactive_image.v1',
              story_id: 'story-1',
              branch_id: 'main',
              turn_id: 'turn-1',
              image_path: 'assets/interactive/images/story-1/main/turn-1/run-a/image.png',
              meta_path: 'assets/interactive/images/story-1/main/turn-1/run-a/meta.json',
              alt_text: '第一张互动图像',
            },
            {
              schema: 'interactive_image.v1',
              story_id: 'story-1',
              branch_id: 'main',
              turn_id: 'turn-1',
              image_path: 'assets/interactive/images/story-1/main/turn-1/run-b/image.png',
              meta_path: 'assets/interactive/images/story-1/main/turn-1/run-b/meta.json',
              alt_text: '第二张互动图像',
            },
          ],
        }}
      />,
    )

    expect(screen.getByText('这一轮剧情。')).toBeInTheDocument()
    expect(screen.queryByText('互动图像')).not.toBeInTheDocument()
    expect(screen.queryByText((text) => text.includes('assets/interactive/images'))).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重新生成' })).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: '第二张互动图像' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-b%2Fimage.png')

    await user.click(screen.getByRole('button', { name: '上一张互动图像' }))
    expect(screen.getByRole('img', { name: '第一张互动图像' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png')
  })

  it('互动媒体 contract 不依赖工具名渲染互动图像', () => {
    render(
      <MessageItem
        message={{
          role: 'tool_result',
          content: '',
          name: 'renamed_media_generator',
          status: 'success',
          tool_presentation: { call: 'interactive_media', result: 'interactive_media' },
          interactive_image_status: 'success',
          interactive_image: {
            schema: 'interactive_image.v1',
            story_id: 'story-1',
            branch_id: 'main',
            turn_id: 'turn-1',
            image_path: 'assets/interactive/images/story-1/main/turn-1/run-a/image.png',
            meta_path: 'assets/interactive/images/story-1/main/turn-1/run-a/meta.json',
            alt_text: '契约驱动的互动图像',
          },
        }}
      />,
    )

    expect(screen.getByRole('img', { name: '契约驱动的互动图像' })).toHaveAttribute(
      'src',
      '/api/projects/project-message/files/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png',
    )
  })

  it('已完成工具按 result contract 恢复 Writing 互动图像', () => {
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: '',
          name: 'renamed_media_generator',
          status: 'success',
          result: JSON.stringify({
            schema: 'interactive_image.v1',
            story_id: 'story-1',
            branch_id: 'main',
            turn_id: 'turn-1',
            image_path: 'assets/interactive/images/story-1/main/turn-1/reopened/image.png',
            meta_path: 'assets/interactive/images/story-1/main/turn-1/reopened/meta.json',
            alt_text: '冷重载恢复的互动图像',
          }),
          tool_presentation: { call: 'image', result: 'interactive_media' },
        }}
      />,
    )

    expect(screen.getByRole('img', { name: '冷重载恢复的互动图像' })).toHaveAttribute(
      'src',
      '/api/projects/project-message/files/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Freopened%2Fimage.png',
    )
    expect(screen.queryByText('renamed_media_generator')).not.toBeInTheDocument()
  })

  it('assistant 回合互动图像新增版本后自动切到最新图像', async () => {
    const user = userEvent.setup()
    const baseMessage = {
      id: 'assistant-turn-1',
      role: 'assistant' as const,
      content: '这一轮剧情。',
      turn_id: 'turn-1',
      interactive_images: [
        {
          schema: 'interactive_image.v1',
          story_id: 'story-1',
          branch_id: 'main',
          turn_id: 'turn-1',
          image_path: 'assets/interactive/images/story-1/main/turn-1/run-a/image.png',
          meta_path: 'assets/interactive/images/story-1/main/turn-1/run-a/meta.json',
          alt_text: '第一张互动图像',
        },
        {
          schema: 'interactive_image.v1',
          story_id: 'story-1',
          branch_id: 'main',
          turn_id: 'turn-1',
          image_path: 'assets/interactive/images/story-1/main/turn-1/run-b/image.png',
          meta_path: 'assets/interactive/images/story-1/main/turn-1/run-b/meta.json',
          alt_text: '第二张互动图像',
        },
      ],
    }

    const { rerender } = render(<MessageItem message={baseMessage} />)

    await user.click(screen.getByRole('button', { name: '上一张互动图像' }))
    expect(screen.getByRole('img', { name: '第一张互动图像' })).toBeInTheDocument()

    rerender(
      <MessageItem
        message={{
          ...baseMessage,
          interactive_images: [
            ...baseMessage.interactive_images,
            {
              schema: 'interactive_image.v1',
              story_id: 'story-1',
              branch_id: 'main',
              turn_id: 'turn-1',
              image_path: 'assets/interactive/images/story-1/main/turn-1/run-c/image.png',
              meta_path: 'assets/interactive/images/story-1/main/turn-1/run-c/meta.json',
              alt_text: '第三张互动图像',
            },
          ],
        }}
      />,
    )

    expect(screen.getByRole('img', { name: '第三张互动图像' })).toHaveAttribute('src', '/api/projects/project-message/files/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-c%2Fimage.png')
  })

  it('assistant 回合元信息显示手动生成互动图像按钮', async () => {
    const user = userEvent.setup()
    const handleGenerate = vi.fn()
    render(
      <MessageItem
        message={{ role: 'assistant', content: '这一轮剧情。', turn_id: 'turn-1' }}
        onGenerateInteractiveImage={handleGenerate}
      />,
    )

    await user.click(screen.getByRole('button', { name: '生成互动图像' }))
    expect(handleGenerate).toHaveBeenCalledWith(expect.objectContaining({ turn_id: 'turn-1' }))
  })

  it('生成互动图像期间只在目标回合显示 loading，并禁用其他回合的重复请求', () => {
    const { rerender } = render(
      <MessageItem
        message={{ role: 'assistant', content: '目标回合。', turn_id: 'turn-1' }}
        onGenerateInteractiveImage={vi.fn()}
        generatingInteractiveImageTurnId="turn-1"
      />,
    )

    expect(screen.getByRole('button', { name: '生成互动图像' })).toBeDisabled()
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()

    rerender(
      <MessageItem
        message={{ role: 'assistant', content: '其他回合。', turn_id: 'turn-2' }}
        onGenerateInteractiveImage={vi.fn()}
        generatingInteractiveImageTurnId="turn-1"
      />,
    )

    expect(screen.getByRole('button', { name: '生成互动图像' })).toBeDisabled()
    expect(document.querySelector('.animate-spin')).not.toBeInTheDocument()
  })

  it('txt 章节插画卡片不允许一键插入 Markdown 图像', () => {
    const handleInsert = vi.fn()

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'generate_image',
          name: 'generate_image',
          status: 'success',
          tool_presentation: { call: 'image', result: 'image' },
          illustration: {
            schema: 'chapter_illustration.v1',
            chapter_path: 'chapters/ch01.txt',
            image_path: 'assets/illustrations/ch01/run/image.png',
            meta_path: 'assets/illustrations/ch01/run/meta.json',
            markdown: '![雨夜](assets/illustrations/ch01/run/image.png)',
            alt_text: '雨夜',
            profile_id: 'default',
            provider: 'openai',
            model: 'gpt-image-1',
          },
        }}
        onInsertIllustration={handleInsert}
      />,
    )

    expect(screen.getByText('当前章节不是 Markdown，不能一键插入')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /插入正文/ })).toBeDisabled()
  })

  it('工具调用流式预览默认锁定到底部', async () => {
    const initialContent = `完整起点。${'开头。'.repeat(250)}完整终点。`
    const nextContent = `${initialContent}${'继续。'.repeat(160)}新的完整终点。`
    const initialArgs = JSON.stringify({ path: 'chapters/ch01.md', content: initialContent })
    const nextArgs = JSON.stringify({ path: 'chapters/ch01.md', content: nextContent })
    const { container, rerender } = render(
      <MessageItem
        message={{
          id: 'tool-write',
          role: 'tool_call',
          content: 'write',
          name: 'write',
          args: initialArgs,
          status: 'running',
          streaming: true,
        }}
      />,
    )
    const preview = container.querySelector('[data-nova-scroll-lock="tool-input-stream"]') as HTMLDivElement
    expect(preview).toBeInTheDocument()
    expect(preview.textContent).toBe(initialArgs)
    const scrollMetrics = mockScrollMetrics(preview)
    preview.scrollTop = scrollMetrics.maxScrollTop()
    fireEvent.scroll(preview)

    scrollMetrics.setScrollHeight(760)
    fireEvent.scroll(preview)
    rerender(
      <MessageItem
        message={{
          id: 'tool-write',
          role: 'tool_call',
          content: 'write',
          name: 'write',
          args: nextArgs,
          status: 'running',
          streaming: true,
        }}
      />,
    )

    await waitFor(() => expect(preview.scrollTop).toBe(scrollMetrics.maxScrollTop()))
    expect(preview.textContent).toBe(nextArgs)
  })

  it('todo 工具卡片以成功结果为真源，并显示进度', () => {
    const args = JSON.stringify({
      action: 'update', expected_revision: 0, mutations: [
        { id: 'old', text: '过时步骤', status: 'pending' },
      ],
    })
    const result = JSON.stringify({
      schema: 'agent.todo.v1', revision: 1,
      items: [
        { id: 'requirements', text: '梳理需求', status: 'completed' },
        { id: 'implementation', text: '实现接口', status: 'in_progress' },
        { id: 'tests', text: '补充测试', status: 'pending' },
      ],
    })

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'todo',
          name: 'todo',
          args,
          status: 'success',
          result,
          tool_presentation: { call: 'todo', result: 'todo' },
        }}
      />,
    )

    expect(screen.getByText('待办列表')).toBeInTheDocument()
    expect(screen.getByText('1/3')).toBeInTheDocument()
    expect(screen.getByText('梳理需求')).toBeInTheDocument()
    expect(screen.getAllByText('实现接口').length).toBeGreaterThan(0)
    expect(screen.getByText('补充测试')).toBeInTheDocument()
    expect(screen.queryByText('过时步骤')).not.toBeInTheDocument()
  })

  it('todo 工具卡片在流式不完整 JSON 时仍能渲染已完整的步骤', () => {
    const partial = '{"action":"update","mutations":[{"id":"first","text":"第一项","status":"completed"},{"id":"second","text":"第二项","stat'

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'todo',
          name: 'todo',
          args: partial,
          status: 'running',
          tool_presentation: { call: 'todo', result: 'todo' },
        }}
      />,
    )

    expect(screen.getByText('待办列表')).toBeInTheDocument()
    expect(screen.getByText('第一项')).toBeInTheDocument()
  })

  it('todo 完整替换在流式 items 中逐项展示双语内容', () => {
    const partial = '{"action":"replace","items":[{"id":"first","text":"Draft outline / 起草大纲","status":"completed"},{"id":"second","text":"Write scene / 撰写场景","stat'

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'todo',
          name: 'todo',
          args: partial,
          status: 'running',
          tool_presentation: { call: 'todo', result: 'todo' },
        }}
      />,
    )

    expect(screen.getByText('Draft outline / 起草大纲')).toBeInTheDocument()
    expect(screen.queryByText('Write scene / 撰写场景')).not.toBeInTheDocument()
  })

  it('task 工具卡片展示委派目标和结果', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'task',
          name: 'task',
          args: '{"subagent_type":"researcher","description":"查找线索"}',
          status: 'success',
          result: '找到三条线索',
          tool_presentation: { call: 'delegation', result: 'delegation' },
        }}
      />,
    )

    expect(screen.getByText('委派任务')).toBeInTheDocument()
    expect(screen.queryByText('task')).not.toBeInTheDocument()
    expect(screen.getByText('委派给 researcher')).toBeInTheDocument()
    await user.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
    expect(screen.getByText('委派结果')).toBeInTheDocument()
    expect(screen.getAllByText('找到三条线索').length).toBeGreaterThan(0)
  })

  it('超长工具来源会在整行折叠触发器内正常收缩', () => {
    const longToolName = 'workspace-tool-with-a-name-that-is-longer-than-the-available-ide-card-width'
    const longAgentName = 'general-purpose-subagent-with-an-unusually-long-display-name'
    const { container } = render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: longToolName,
          name: longToolName,
          args: '{"path":"chapters/long-file-name.md"}',
          status: 'success',
          result: 'done',
          agent_name: longAgentName,
          subagent: true,
        }}
      />,
    )

    const header = container.querySelector('[data-nova-tool-header]')
    expect(header).toHaveClass('grid', 'min-h-9', 'grid-cols-[auto_minmax(0,1fr)]', 'px-2.5', 'text-left')
    expect(header?.parentElement).toHaveClass('text-[11px]')
    expect(header).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText(longToolName)).toHaveClass('min-w-0', 'truncate')
    expect(screen.getByText(longAgentName)).toHaveClass('truncate')
    expect(screen.queryByText('详情')).not.toBeInTheDocument()
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
    const { container } = render(
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
    expect(container.querySelector('[data-nova-scroll-lock="context-compaction-summary"]')).toHaveClass(
      'max-w-full', 'overflow-x-hidden', 'overflow-y-auto', '[overflow-wrap:anywhere]',
    )
  })

  it('Plan 生成中预览长时间不变化时自动隐藏', () => {
    vi.useFakeTimers()
    try {
      render(<MessageItem message={{ role: 'proposed_plan', status: 'running', streaming: true, content: '', thinking_preview: '正在整理最终计划' }} />)

      expect(screen.getByText('正在规划：')).toBeInTheDocument()
      expect(screen.getByText('正在整理最终计划')).toBeInTheDocument()

      act(() => {
        vi.advanceTimersByTime(3600)
      })

      expect(screen.queryByText('正在规划：')).not.toBeInTheDocument()
      expect(screen.queryByText('正在整理最终计划')).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it.each([
    ['确认并执行', '已确认并开始执行。', 'approve'],
    ['继续讨论', '已选择继续讨论。', 'continue'],
    ['退出计划模式', '已退出计划模式。', 'exit'],
  ])('最终计划卡点击“%s”后隐藏按钮并展示完成态', async (buttonName, statusText, action) => {
    const user = userEvent.setup()
    const handleApprove = vi.fn()
    const handleContinue = vi.fn()
    const handleExit = vi.fn()

    render(
      <MessageItem
        message={{
          role: 'proposed_plan',
          content: '# 计划标题\n\n## Summary\n\n- 先确认方向\n\n## Key Changes\n\n- 输出结构化方案',
        }}
        onApprovePlan={action === 'approve' ? handleApprove : undefined}
        onContinuePlan={action === 'continue' ? handleContinue : undefined}
        onExitPlanMode={action === 'exit' ? handleExit : undefined}
      />,
    )

    expect(screen.getByRole('heading', { name: 'Summary' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Key Changes' })).toBeInTheDocument()
    expect(document.querySelector('.chat-agent-message h1')).toHaveTextContent('计划标题')
    expect(document.querySelector('.chat-agent-message ul')).toHaveTextContent('先确认方向')
    await user.click(screen.getByRole('button', { name: buttonName }))

    expect(screen.getByText(statusText)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '确认并执行' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '继续讨论' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '退出计划模式' })).not.toBeInTheDocument()
    if (action === 'approve') expect(handleApprove).toHaveBeenCalledTimes(1)
    if (action === 'continue') expect(handleContinue).toHaveBeenCalledTimes(1)
    if (action === 'exit') expect(handleExit).toHaveBeenCalledTimes(1)
  })

  it('Ask 单选题展示推荐项、自动 Other，并提交结构化答案', async () => {
    const user = userEvent.setup()
    const onResolve = vi.fn().mockResolvedValue({
      schema: 'ask.result.v1', id: 'ask-1', status: 'answered',
      answers: [{ question_id: 'strategy', question: '采用哪种方案？', selected_options: [{ id: 'safe', label: '稳妥方案' }] }],
    })
    render(
      <MessageItem
        message={{
          id: 'ask-1', role: 'ask', ask: {
            schema: 'ask.pending.v1', id: 'ask-1', tool_call_id: 'ask-1', agent_kind: 'ide', status: 'pending',
            questions: [{
              id: 'strategy', question: '采用哪种方案？', recommended_option_id: 'safe',
              options: [{ id: 'safe', label: '稳妥方案', description: '风险较低' }, { id: 'fast', label: '快速方案' }],
            }],
          },
        }}
        onResolveAsk={onResolve}
      />,
    )

    expect(screen.getByText('推荐')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /其他/ })).toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: /稳妥方案/ }))
    await user.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'ask-1' }),
      { status: 'answered', answers: [{ question_id: 'strategy', selected_option_ids: ['safe'] }] },
    ))
    expect(screen.getByText('已回答')).toBeInTheDocument()
  })

  it('Ask 原始工具帧按用户输入语言映射单字符串文案', () => {
    render(
      <MessageItem
        message={{
          id: 'ask-raw', role: 'tool_call', name: 'ask', status: 'running',
          tool_presentation: { call: 'interaction', result: 'interaction' },
          args: JSON.stringify({ questions: [{
            id: 'direction', prompt: '选择下一步？',
            options: [
              { value: 'continue', label: '继续', description: '沿当前方向推进。', recommended: true },
              { value: 'change', label: '调整', description: '切换到新的方向。' },
            ],
          }] }),
        }}
      />,
    )

    expect(screen.getByText('选择下一步？')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /继续/ })).toBeInTheDocument()
    expect(screen.getByText('沿当前方向推进。')).toBeInTheDocument()
    expect(screen.queryByText(/Choose|Continue/)).not.toBeInTheDocument()
  })

  it('历史中的已回答 Ask 默认折叠，并可点击整行查看原问题和答案', async () => {
    const user = userEvent.setup()
    render(
      <MessageItem
        message={{
          id: 'ask-history', role: 'ask', ask: {
            schema: 'ask.pending.v1', id: 'ask-history', tool_call_id: 'ask-history', agent_kind: 'ide', status: 'answered',
            questions: [{
              id: 'direction', question: '选择方向？',
              options: [{ id: 'continue', label: '继续' }, { id: 'change', label: '调整' }],
            }],
            answers: [{
              question_id: 'direction', question: '选择方向？',
              selected_options: [{ id: 'continue', label: '继续' }],
            }],
          },
        }}
      />,
    )

    const header = screen.getByRole('button', { name: /需要你的回答.*已回答/ })
    expect(header).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('选择方向？')).not.toBeInTheDocument()

    await user.click(header)
    expect(header).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('选择方向？')).toBeInTheDocument()
    expect(screen.getByText('继续')).toBeInTheDocument()
  })

  it('Ask 接口错误不直接展示英文服务端文案，而是按用户语言本地化', async () => {
    const user = userEvent.setup()
    const onResolve = vi.fn().mockRejectedValue(new Error('Invalid ask answer'))
    render(
      <MessageItem
        message={{
          id: 'ask-error', role: 'ask', ask: {
            schema: 'ask.pending.v1', id: 'ask-error', tool_call_id: 'ask-error', agent_kind: 'ide', status: 'pending',
            questions: [{
              id: 'strategy', question: '采用哪种方案？',
              options: [{ id: 'safe', label: '稳妥方案' }, { id: 'fast', label: '快速方案' }],
            }],
          },
        }}
        onResolveAsk={onResolve}
      />,
    )

    await user.click(screen.getByRole('radio', { name: '稳妥方案' }))
    await user.click(screen.getByRole('button', { name: '提交' }))

    expect(await screen.findByText('提交回答失败，请重试。')).toBeInTheDocument()
    expect(screen.queryByText('Invalid ask answer')).not.toBeInTheDocument()
  })

  it('Ask 多题保留返回导航状态，支持多选与 Other 自由输入', async () => {
    const user = userEvent.setup()
    const onResolve = vi.fn().mockResolvedValue({ schema: 'ask.result.v1', id: 'ask-2', status: 'answered', answers: [] })
    render(
      <MessageItem
        message={{
          id: 'ask-2', role: 'ask', ask: {
            schema: 'ask.pending.v1', id: 'ask-2', tool_call_id: 'ask-2', agent_kind: 'config_manager', status: 'pending',
            questions: [
              { id: 'scope', question: '修改范围？', options: [{ id: 'one', label: '当前项' }, { id: 'all', label: '全部' }] },
              { id: 'checks', question: '需要哪些检查？', multi_select: true, options: [{ id: 'schema', label: 'Schema' }, { id: 'links', label: '引用' }] },
            ],
          },
        }}
        onResolveAsk={onResolve}
      />,
    )

    await user.click(screen.getByRole('radio', { name: '当前项' }))
    await user.click(screen.getByRole('button', { name: '下一步' }))
    await user.click(screen.getByRole('checkbox', { name: 'Schema' }))
    await user.click(screen.getByRole('checkbox', { name: '其他' }))
    await user.type(screen.getByRole('textbox', { name: '其他' }), '并发回归')
    await user.click(screen.getByRole('button', { name: '上一步' }))
    expect(screen.getByRole('radio', { name: '当前项' })).toBeChecked()
    await user.click(screen.getByRole('button', { name: '下一步' }))
    await user.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(
      expect.anything(),
      { status: 'answered', answers: [
        { question_id: 'scope', selected_option_ids: ['one'] },
        { question_id: 'checks', selected_option_ids: ['schema', 'other'], custom_input: '并发回归' },
      ] },
    ))
  })

  it('Ask 可结构化取消，并可直接从刷新后的 pending 状态重新渲染', async () => {
    const user = userEvent.setup()
    const onResolve = vi.fn().mockResolvedValue({ schema: 'ask.result.v1', id: 'ask-refresh', status: 'cancelled', cancel_reason: 'user_cancelled' })
    const message = {
      id: 'ask-refresh', role: 'ask' as const, ask: {
        schema: 'ask.pending.v1', id: 'ask-refresh', tool_call_id: 'ask-refresh', agent_kind: 'ide', status: 'pending' as const,
        questions: [{ id: 'detail', question: '补充说明？' }],
      },
    }
    const { unmount } = render(<MessageItem message={message} onResolveAsk={onResolve} />)
    expect(screen.getByRole('textbox', { name: '补充说明？' })).toBeInTheDocument()
    unmount()

    render(<MessageItem message={message} onResolveAsk={onResolve} />)
    await user.click(screen.getByRole('button', { name: '取消' }))
    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(expect.anything(), { status: 'cancelled' }))
    expect(screen.getByText('已取消')).toBeInTheDocument()
  })

  it('工具审批内联到原工具卡并可保存工作区命令规则', async () => {
    const user = userEvent.setup()
    const settingsUpdated = vi.fn()
    window.addEventListener('nova:settings-updated', settingsUpdated)
    const onResolve = vi.fn().mockResolvedValue({
      schema: 'ask.result.v1', id: 'approval-1', status: 'answered',
      answers: [{
        question_id: 'tool-approval', question: 'Allow once?',
        selected_options: [{ id: 'allow-workspace', label: 'Always allow' }],
      }],
    })
    const { container } = render(
      <MessageItem
        message={{
          id: 'tool-1', role: 'tool_call', name: 'bash', args: '{"command":"npm test"}', status: 'running', ask: {
            schema: 'ask.pending.v1', id: 'approval-1', kind: 'tool_approval',
            tool_call_id: 'tool-1', agent_kind: 'ide', status: 'pending',
            questions: [{
              id: 'tool-approval', question: 'Allow once?',
              options: [
                { id: 'allow-once', label: 'Allow once' },
                { id: 'allow-workspace', label: 'Always allow' },
                { id: 'deny', label: 'Deny' },
              ],
            }],
            approval: {
              mode: 'ask', tool_name: 'bash', command: 'npm test', cwd: '.',
              risk: 'high', rule_id: 'bash_unlisted_command', args_hash: 'abc',
        can_remember: true, rule_matcher_version: 1,
        rule_command_key: '["npm","test"]', rule_command_pattern: 'npm test ...',
            },
          },
        }}
        onResolveAsk={onResolve}
      />,
    )

    expect(screen.getByText('npm test')).toBeInTheDocument()
    expect(container.querySelector('pre')).toHaveClass(
      'max-w-full', 'overflow-x-hidden', 'overflow-y-auto', '[overflow-wrap:anywhere]',
    )
  expect(screen.getByText('此命令不在当前安全模式的自动允许范围内。')).toBeInTheDocument()
  expect(screen.getByText(/工作区规则：npm test/)).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: /其他/ })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '在此工作区始终允许' }))
    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(
      expect.objectContaining({ ask: expect.objectContaining({ id: 'approval-1' }) }),
      { status: 'answered', answers: [{ question_id: 'tool-approval', selected_option_ids: ['allow-workspace'] }] },
    ))
    expect(screen.getByText('已在此工作区持续允许')).toBeInTheDocument()
    expect(settingsUpdated).toHaveBeenCalledTimes(1)
    window.removeEventListener('nova:settings-updated', settingsUpdated)
  })
})
