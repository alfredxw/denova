import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { MessageItem } from './MessageItem'

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

  it('流式 assistant 消息即时渲染常见 Markdown 结构', () => {
    render(<MessageItem message={{ role: 'assistant', content: '# 实时标题\n- 实时条目\n`cmd`', streaming: true }} />)

    expect(screen.getByRole('heading', { name: '实时标题' })).toBeInTheDocument()
    expect(screen.getByText('实时标题')).toBeInTheDocument()
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

  it('流式和持久化 assistant 消息使用一致的 Markdown DOM 结构', () => {
    const content = '# 标题\n\n第一段。\n\n- 条目 A\n- 条目 B\n\n> 引用'
    const { container, rerender } = render(<MessageItem message={{ role: 'assistant', content, streaming: true }} />)
    const streamedTags = Array.from(container.querySelector('.chat-agent-message')?.children || []).map((node) => node.tagName)

    rerender(<MessageItem message={{ role: 'assistant', content, streaming: false }} />)
    const persistedTags = Array.from(container.querySelector('.chat-agent-message')?.children || []).map((node) => node.tagName)

    expect(streamedTags).toEqual(['H1', 'P', 'UL', 'BLOCKQUOTE'])
    expect(persistedTags).toEqual(streamedTags)
  })

  it('流式和持久化 assistant 消息安全打开外部 Markdown 引用且保留站内链接行为', () => {
    const content = '结论。[SearXNG Search API](https://docs.searxng.org/dev/search_api.html)\n\n[站内文档](/docs/web-access)'
    const { rerender } = render(<MessageItem message={{ role: 'assistant', content, streaming: true }} />)

    const assertLinks = () => {
      expect(screen.getByRole('link', { name: 'SearXNG Search API' })).toHaveAttribute('href', 'https://docs.searxng.org/dev/search_api.html')
      expect(screen.getByRole('link', { name: 'SearXNG Search API' })).toHaveAttribute('target', '_blank')
      expect(screen.getByRole('link', { name: 'SearXNG Search API' })).toHaveAttribute('rel', 'noopener noreferrer')
      expect(screen.getByRole('link', { name: '站内文档' })).not.toHaveAttribute('target')
      expect(screen.getByRole('link', { name: '站内文档' })).not.toHaveAttribute('rel')
    }

    assertLinks()
    rerender(<MessageItem message={{ role: 'assistant', content, streaming: false }} />)
    assertLinks()
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
    render(
      <MessageItem
        message={{ role: 'error', content: '[NodeRunError] 400 Bad Request', streaming: false }}
        onRegenerate={vi.fn()}
      />,
    )

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

  it('历史思考过程的首帧直接以折叠态渲染', () => {
    const markup = renderToStaticMarkup(
      <MessageItem message={{ role: 'thinking', content: '历史思考内容', streaming: false }} />,
    )

    expect(markup).toContain('aria-expanded="false"')
    expect(markup).not.toContain('历史思考内容')
  })

  it('直接增长的流式 thinking 立即复用单棵文本树显示最新内容', () => {
    const { container, rerender } = render(<MessageItem message={{ role: 'thinking', content: '正在分析', streaming: true }} />)

    rerender(<MessageItem message={{ role: 'thinking', content: '正在分析下一条线索', streaming: true }} />)

    expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()
    expect(screen.getByText('正在分析下一条线索')).toBeInTheDocument()
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

    expect(screen.getByText('调用工具')).toBeInTheDocument()
    expect(screen.getByText('write')).toBeInTheDocument()
    expect(screen.getByText('写入完成')).toBeInTheDocument()
  })

  it('失败的工具卡片在折叠态直接展示错误原因', () => {
    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'web_fetch\n{"url":"https://example.com"}',
          name: 'web_fetch',
          args: '{"url":"https://example.com"}',
          status: 'error',
          result: 'web_fetch failed: target URL is invalid',
        }}
      />,
    )

    expect(screen.getByText('web_fetch failed: target URL is invalid')).toBeInTheDocument()
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

    expect(screen.getByText('已返回当前批次 · 后续内容可从 offset=80 读取')).toHaveClass('text-[var(--nova-text-faint)]')
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
    render(
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

    expect(screen.getByText('访问被阻止 · 改用其他公开来源')).toHaveClass('basis-full', 'whitespace-normal')
    await user.click(screen.getByRole('button', { name: '详情' }))
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

    expect(screen.getByText('调用工具')).toBeInTheDocument()
    expect(screen.getByText('skill')).toBeInTheDocument()
    expect(screen.getByText('# Skill: alpha ALPHA_BODY')).toBeInTheDocument()
  })

  it('edit 单次精确替换时流式展示 new_string', () => {
    const args = JSON.stringify({
      path: 'chapters/ch01.md',
      old_string: '旧开场',
      new_string: '新的开场正文。'.repeat(8),
    })
    const { container } = render(
      <MessageItem
        message={{
          id: 'tool-batch-edit',
          role: 'tool_call',
          content: 'edit',
          name: 'edit',
          args,
          status: 'running',
        }}
      />,
    )

    const preview = container.querySelector('[data-nova-scroll-lock="tool-stream-preview"]')
    expect(preview).toBeInTheDocument()
    expect(preview).toHaveTextContent('新的开场正文')
  })

  it('隐藏章节正文的工具卡片展示写入状态和说明详情', async () => {
    const user = userEvent.setup()
    const path = '/Users/me/nova/.nova/测试/chapters/ch01.md'

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: `write\n{"path":"${path}"}`,
          name: 'write',
          args: `{"path":"${path}"}`,
          status: 'running',
          sse_hidden_fields: ['content'],
          sse_hidden_reason: 'novel_chapter_body',
          sse_display_notice: 'chapter_body_hidden',
          sse_generated_chars: 123,
        }}
      />,
    )

    expect(screen.getByText('正在写入章节 · 已生成 123 字')).toBeInTheDocument()
    expect(screen.queryByText('准备执行工具请求')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '详情' }))
    expect(screen.getByText('路径：')).toBeInTheDocument()
    expect(screen.getByText(path)).toBeInTheDocument()
    expect(screen.getByText('已生成：123 字')).toBeInTheDocument()
    expect(screen.getByText('章节正文已在实时输出中隐藏，文件仍会正常写入。')).toBeInTheDocument()
    expect(screen.queryByText(/content/)).not.toBeInTheDocument()
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
          illustration,
        }}
        onInsertIllustration={handleInsert}
      />,
    )

    expect(screen.getByText('章节插画')).toBeInTheDocument()
    expect(screen.getByText('assets/illustrations/ch01/run/image.png')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: '雨夜' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Fillustrations%2Fch01%2Frun%2Fimage.png')

    await user.click(screen.getByRole('button', { name: '放大查看章节插画' }))
    expect(within(screen.getByRole('dialog')).getByRole('img', { name: '雨夜' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Fillustrations%2Fch01%2Frun%2Fimage.png')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /插入正文/ }))
    expect(handleInsert).toHaveBeenCalledWith(illustration)
  })

  it('assistant Markdown 图像支持 workspace 路径展示和点击放大', async () => {
    const user = userEvent.setup()
    render(<MessageItem message={{ role: 'assistant', content: '![封面](assets/image/generated/cover.png)' }} />)

    expect(screen.getByRole('img', { name: '封面' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Fimage%2Fgenerated%2Fcover.png')

    await user.click(screen.getByRole('button', { name: '放大查看图像' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('img', { name: '封面' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Fimage%2Fgenerated%2Fcover.png')
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
    expect(screen.getByRole('img', { name: '第二张互动图像' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-b%2Fimage.png')

    await user.click(screen.getByRole('button', { name: '上一张互动图像' }))
    expect(screen.getByRole('img', { name: '第一张互动图像' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png')
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

    expect(screen.getByRole('img', { name: '第三张互动图像' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-c%2Fimage.png')
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

  it('txt 章节插画卡片不允许一键插入 Markdown 图像', () => {
    const handleInsert = vi.fn()

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'generate_image',
          name: 'generate_image',
          status: 'success',
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
        }}
      />,
    )
    const preview = container.querySelector('[data-nova-scroll-lock="tool-stream-preview"]') as HTMLDivElement
    expect(preview).toBeInTheDocument()
    expect(preview.textContent).toBe(initialContent)
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
        }}
      />,
    )

    await waitFor(() => expect(preview.scrollTop).toBe(scrollMetrics.maxScrollTop()))
    expect(preview.textContent).toBe(nextContent)
  })

  it('todo 工具卡片以成功结果为真源，并显示进度', () => {
    const args = JSON.stringify({
      plan: [
        { step: '过时步骤', status: 'pending' },
      ],
    })
    const result = JSON.stringify({
      schema: 'todo.plan.v1',
      plan: [
        { step: '梳理需求', status: 'completed' },
        { step: '实现接口', status: 'in_progress' },
        { step: '补充测试', status: 'pending' },
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
    const partial = '{"plan":[{"step":"第一项","status":"completed"},{"step":"第二项","stat'

    render(
      <MessageItem
        message={{
          role: 'tool_call',
          content: 'todo',
          name: 'todo',
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

  it('Plan 问题卡选择选项时只更新卡片内部状态', async () => {
    const user = userEvent.setup()

    render(
      <MessageItem
        message={{
          role: 'plan_question',
          content: JSON.stringify({
            questions: [{
              id: 'scope',
              type: 'single',
              question: '这次要先确认什么？',
              options: [
                { id: 'recommended', label: '采用推荐方案', recommended: true },
                { id: 'manual', label: '手动确认' },
              ],
            }],
          }),
        }}
      />,
    )

    const manualOption = screen.getByRole('button', { name: /手动确认/ })
    expect(manualOption).not.toHaveClass('border-[var(--nova-accent)]')

    await user.click(manualOption)

    expect(manualOption).toHaveClass('border-[var(--nova-accent)]')
  })

  it('Plan 问题卡生成中时展示占位状态', () => {
    render(<MessageItem message={{ role: 'plan_question', status: 'running', streaming: true, content: '', thinking_preview: '准备输出问题卡' }} />)

    expect(screen.getByText('规划问题')).toBeInTheDocument()
    expect(screen.getByText('生成中')).toBeInTheDocument()
    expect(screen.getByText('正在生成规划问题卡…')).toBeInTheDocument()
    expect(screen.getByText('正在规划：')).toBeInTheDocument()
    expect(screen.getByText('准备输出问题卡')).toBeInTheDocument()
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

  it('Plan 问题卡提交后隐藏按钮并展示完成态', async () => {
    const user = userEvent.setup()
    const handleSubmit = vi.fn()

    render(
      <MessageItem
        message={{
          role: 'plan_question',
          content: JSON.stringify({
            questions: [{
              id: 'scope',
              type: 'single',
              question: '这次要先确认什么？',
              options: [
                { id: 'recommended', label: '采用推荐方案', recommended: true },
                { id: 'manual', label: '手动确认' },
              ],
            }],
          }),
        }}
        onSubmitPlanQuestion={handleSubmit}
      />,
    )

    await user.click(screen.getByRole('button', { name: /提交全部回答/ }))

    expect(handleSubmit).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: /提交全部回答/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /采用推荐项/ })).not.toBeInTheDocument()
    expect(screen.getByText('已提交回答，正在继续规划。')).toBeInTheDocument()
  })

  it.each([
    ['确认并执行', '已确认并开始执行。', 'approve'],
    ['继续讨论', '已选择继续讨论。', 'continue'],
    ['退出 Plan Mode', '已退出 Plan Mode。', 'exit'],
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
    expect(screen.queryByRole('button', { name: '退出 Plan Mode' })).not.toBeInTheDocument()
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

  it('工具审批使用专用卡片且只提交 Allow once 或 Deny', async () => {
    const user = userEvent.setup()
    const onResolve = vi.fn().mockResolvedValue({
      schema: 'ask.result.v1', id: 'approval-1', status: 'answered',
      answers: [{
        question_id: 'tool-approval', question: 'Allow once?',
        selected_options: [{ id: 'allow-once', label: 'Allow once' }],
      }],
    })
    render(
      <MessageItem
        message={{
          id: 'approval-1', role: 'ask', ask: {
            schema: 'ask.pending.v1', id: 'approval-1', kind: 'tool_approval',
            tool_call_id: 'tool-1', agent_kind: 'ide', status: 'pending',
            questions: [{
              id: 'tool-approval', question: 'Allow once?',
              options: [{ id: 'allow-once', label: 'Allow once' }, { id: 'deny', label: 'Deny' }],
            }],
            approval: {
              mode: 'ask', tool_name: 'bash', command: 'npm test', cwd: '.',
              details: '{"action":"run"}',
              risk: 'high', reason: '不在白名单', rule_id: 'bash_unlisted_command', args_hash: 'abc',
            },
          },
        }}
        onResolveAsk={onResolve}
      />,
    )

    expect(screen.getByText('npm test')).toBeInTheDocument()
    expect(screen.getByText('{"action":"run"}')).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: /其他/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '仅允许本次' }))
    await waitFor(() => expect(onResolve).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'approval-1' }),
      { status: 'answered', answers: [{ question_id: 'tool-approval', selected_option_ids: ['allow-once'] }] },
    ))
    expect(screen.getByText('已允许一次')).toBeInTheDocument()
  })
})
