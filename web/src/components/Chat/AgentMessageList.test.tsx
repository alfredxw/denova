import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from '@/lib/api'
import { chatMessagesToAgentUIMessages } from '@/lib/agent-legacy-message'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { MessageList } from './MessageList'

function renderMessageList(ui: ReactElement) {
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
      {ui}
    </VirtuosoMockContext.Provider>,
  )
}

describe('Agent MessageList', () => {
  it('在历史窗口顶部按需加载更早消息', () => {
    const loadEarlier = vi.fn()
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={agentTurnMessages()}
        hasEarlierMessages
        onLoadEarlierMessages={loadEarlier}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '加载更早消息' }))
    expect(loadEarlier).toHaveBeenCalledTimes(1)
  })

  it('前置更早消息时保持原首条消息的虚拟索引', () => {
    const current = { id: 'current-message', role: 'assistant', parts: [{ type: 'text', text: '当前窗口首条' }] } as AgentUIMessage
    const earlier = { id: 'earlier-message', role: 'user', parts: [{ type: 'text', text: '更早窗口消息' }] } as AgentUIMessage
    const list = (messages: AgentUIMessage[]) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList isStreaming={false} activityContent="" messages={messages} scrollResetKey="session-a" />
      </VirtuosoMockContext.Provider>
    )
    const { rerender } = render(list([current]))
    const indexBefore = screen.getByText('当前窗口首条').closest('[data-item-index]')?.getAttribute('data-item-index')

    rerender(list([earlier, current]))

    expect(screen.getByText('当前窗口首条').closest('[data-item-index]')).toHaveAttribute('data-item-index', indexBefore)
  })

  it('renders optional stage content after the latest message and before the composer spacer', () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={agentTurnMessages()}
        afterContent={<section data-testid="stage-state">当前状态台</section>}
        afterContentKey="turn-2:collapsed"
        bottomPaddingPx={120}
      />,
    )

    const prose = screen.getByText('第一轮剧情')
    const state = screen.getByTestId('stage-state')
    expect(prose.compareDocumentPosition(state) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(state.closest('[data-nova-chat-after-content]')?.nextElementSibling).toHaveAttribute('data-nova-chat-bottom-spacer')
  })

  it('keeps the streamed prose bottom fixed during the layout commit with a composer spacer', () => {
    let rowHeight = 40
    let scrollHeight = 500
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.hasAttribute('data-nova-chat-row-key')) {
        const scroller = this.closest<HTMLElement>('.nova-chat-canvas')
        return {
          bottom: 60 + rowHeight - ((scroller?.scrollTop || 0) - 400),
          height: rowHeight,
        } as DOMRect
      }
      return originalRect.call(this)
    })
    const renderList = (content: string) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent=""
          bottomPaddingPx={146}
          messages={[{
            id: 'streaming-assistant',
            role: 'assistant',
            parts: [{ type: 'text', text: content, state: 'streaming' }],
          }] as AgentUIMessage[]}
        />
      </VirtuosoMockContext.Provider>
    )

    try {
      const { container, rerender } = render(renderList('第一行'))
      const scroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
      if (!scroller) throw new Error('Expected message scroller')
      Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
      scroller.scrollTop = 400
      fireEvent.scroll(scroller)

      rowHeight = 70
      scrollHeight = 530
      rerender(renderList('第一行\n第二行'))

      expect(scroller.scrollTop).toBe(430)
    } finally {
      rectSpy.mockRestore()
    }
  })

  it('keeps the bottom fixed when the activity placeholder becomes multiline thinking', () => {
    let rowHeight = 28
    let rowExtent = 500
    let scrollHeight = 500
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.classList.contains('nova-chat-canvas')) return { top: 0 } as DOMRect
      if (this.hasAttribute('data-nova-chat-row-key')) {
        const scroller = this.closest<HTMLElement>('.nova-chat-canvas')
        return { bottom: rowExtent - (scroller?.scrollTop || 0), height: rowHeight } as DOMRect
      }
      return originalRect.call(this)
    })
    const renderList = (messages: AgentUIMessage[]) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent="正在思考…"
          collapseTraceGroups
          messages={messages}
        />
      </VirtuosoMockContext.Provider>
    )

    try {
      const { container, rerender } = render(renderList([]))
      const scroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
      if (!scroller) throw new Error('Expected message scroller')
      Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
      scroller.scrollTop = 400
      fireEvent.scroll(scroller)

      rowHeight = 98
      rowExtent = 570
      scrollHeight = 570
      rerender(renderList([{
        id: 'streaming-thinking',
        role: 'assistant',
        metadata: { run_id: 'run-thinking' },
        parts: [{
          type: 'reasoning',
          text: '第一行思考\n第二行思考\n第三行思考',
          state: 'streaming',
        }],
      }] as AgentUIMessage[]))

      expect(scroller.scrollTop).toBe(470)
    } finally {
      rectSpy.mockRestore()
    }
  })

  it('does not apply a compensating scroll after an idle stage interaction', () => {
    const renderList = (afterContentKey: string) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={agentTurnMessages()}
          afterContent={<button type="button">展开状态</button>}
          afterContentKey={afterContentKey}
        />
      </VirtuosoMockContext.Provider>
    )
    const { container, rerender } = render(renderList('collapsed'))
    const scroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
    if (!scroller) throw new Error('Expected message scroller')
    let scrollHeight = 500
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 100 })
    scroller.scrollTop = 400
    fireEvent.scroll(scroller)

    fireEvent.pointerDown(screen.getByRole('button', { name: '展开状态' }))
    scrollHeight = 700
    rerender(renderList('expanded'))

    // Idle footer following is disabled at the virtualizer boundary. If a
    // scroll event still occurs, the lock must not create a second visible
    // jump by writing the previously captured position back afterward.
    scroller.scrollTop = 600
    fireEvent.scroll(scroller)

    expect(scroller.scrollTop).toBe(600)
  })

  it('有可见流式 thinking 时不再追加会被动态内容推动的活动卡片', () => {
    renderMessageList(
      <MessageList
        isStreaming
        activityContent="正在思考…"
        collapseTraceGroups
        activeTraceDisplay="collapsed"
        messages={[
          {
            id: 'assistant-thinking',
            role: 'assistant',
            parts: [
              { type: 'reasoning', text: '正在分析当前剧情。', state: 'streaming' },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.queryByText('正在分析当前剧情。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /正在执行/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('正在思考…')).not.toBeInTheDocument()
  })

  it('审批作为工具状态留在同一个 execution fold 内', () => {
    const { container } = renderMessageList(
      <MessageList
        isStreaming
        isExecutionActive
        activityContent=""
        collapseTraceGroups
        activeTraceDisplay="expanded"
        messages={[
          {
            id: 'approval-tool', role: 'assistant', metadata: { run_id: 'run-approval' },
            parts: [{ type: 'dynamic-tool', toolName: 'bash', toolCallId: 'execution-approval', state: 'input-available', input: { command: 'npm test' } }],
          },
          {
            id: 'approval-state', role: 'assistant', metadata: { run_id: 'run-approval' },
            parts: [{
              type: 'data-agent-ask', id: 'approval-state', data: {
                schema: 'ask.pending.v1', id: 'approval-state', kind: 'tool_approval',
                tool_call_id: 'execution-approval', agent_kind: 'ide', status: 'pending',
                questions: [{ id: 'tool-approval', question: 'Approve?', options: [{ id: 'allow-once', label: 'Allow once' }, { id: 'deny', label: 'Deny' }] }],
                approval: { mode: 'ask', tool_name: 'bash', command: 'npm test', risk: 'high', rule_id: 'bash_unlisted_command', args_hash: 'a'.repeat(64) },
              },
            }],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(container.querySelectorAll('[data-agent-execution-process]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-tool-approval-panel]')).toHaveLength(1)
    expect(screen.getByText('npm test')).toBeInTheDocument()
  })

  it('审批展开后将操作区锚定到悬浮 composer 上方', async () => {
    const initialMessages = [{
      id: 'approval-tool', role: 'assistant', metadata: { run_id: 'run-approval' },
      parts: [{ type: 'dynamic-tool', toolName: 'bash', toolCallId: 'execution-approval', state: 'input-available', input: { command: 'npm test' } }],
    }] as AgentUIMessage[]
    const pendingMessages = [
      ...initialMessages,
      {
        id: 'approval-state', role: 'assistant', metadata: { run_id: 'run-approval' },
        parts: [{
          type: 'data-agent-ask', id: 'approval-state', data: {
            schema: 'ask.pending.v1', id: 'approval-state', kind: 'tool_approval',
            tool_call_id: 'execution-approval', agent_kind: 'ide', status: 'pending',
            questions: [{ id: 'tool-approval', question: 'Approve?', options: [{ id: 'allow-once', label: 'Allow once' }, { id: 'deny', label: 'Deny' }] }],
            approval: { mode: 'ask', tool_name: 'bash', command: 'npm test', risk: 'high', rule_id: 'bash_unlisted_command', args_hash: 'a'.repeat(64) },
          },
        }],
      },
    ] as AgentUIMessage[]
    let scrollTop = 400
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.classList.contains('nova-chat-canvas')) {
        return { top: 100, right: 320, bottom: 300, left: 0, width: 320, height: 200 } as DOMRect
      }
      if (this.classList.contains('nova-agent-composer')) {
        return { top: 220, right: 320, bottom: 300, left: 0, width: 320, height: 80 } as DOMRect
      }
      if (this.hasAttribute('data-nova-chat-row-key')) {
        return { top: 500 - scrollTop, right: 320, bottom: 820 - scrollTop, left: 0, width: 320, height: 320 } as DOMRect
      }
      return originalRect.call(this)
    })
    const frameCallbacks: FrameRequestCallback[] = []
    const animationFrameSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frameCallbacks.push(callback)
      return frameCallbacks.length
    })
    const renderList = (messages: AgentUIMessage[]) => (
      <div className="relative flex h-[300px] flex-col">
        <MessageList
          isStreaming={false}
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="expanded"
          messages={messages}
        />
        <div className="nova-chat-input-area"><div className="nova-agent-composer" /></div>
      </div>
    )

    try {
      const { container, rerender } = renderMessageList(renderList(initialMessages))
      frameCallbacks.length = 0

      rerender(renderList(pendingMessages))

      await waitFor(() => expect(container.querySelector('[data-tool-approval-panel]')).toBeInTheDocument())
      const pendingScroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
      if (!pendingScroller) throw new Error('Expected message scroller')
      Object.defineProperty(pendingScroller, 'scrollHeight', { configurable: true, value: 900 })
      Object.defineProperty(pendingScroller, 'clientHeight', { configurable: true, value: 200 })
      Object.defineProperty(pendingScroller, 'scrollTop', {
        configurable: true,
        get: () => scrollTop,
        set: value => { scrollTop = value },
      })
      fireEvent.scroll(pendingScroller)
      expect(frameCallbacks.length).toBeGreaterThan(0)
      act(() => {
        for (let frame = 0; frame < 20 && frameCallbacks.length > 0; frame += 1) {
          frameCallbacks.splice(0).forEach(callback => callback(frame * 16))
        }
      })
      expect(scrollTop).toBe(600)
    } finally {
      animationFrameSpy.mockRestore()
      rectSpy.mockRestore()
    }
  })

  it('尚无真实流式内容时直接以 Shimmer 显示思考状态', () => {
    renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { id: 'user-next-chapter', role: 'user', parts: [{ type: 'text', text: '继续下一章' }] },
          {
            id: 'assistant-cycle-started',
            role: 'assistant',
            parts: [{
              type: 'data-agent-activity',
              id: 'cycle-started',
              data: { event: 'agent_cycle_started', message: '继续下一章', operation_id: 'operation-1' },
            }],
          },
        ] as AgentUIMessage[]}
      />,
    )

    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('思考中')
    expect(status.querySelector('.bg-clip-text')).toBeInTheDocument()
    expect(screen.getAllByText('继续下一章')).toHaveLength(1)
  })

  it('真实助手内容开始流式输出后隐藏默认 Shimmer', () => {
    renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { id: 'user-continue', role: 'user', parts: [{ type: 'text', text: '继续' }] },
          { id: 'assistant-streaming', role: 'assistant', parts: [{ type: 'text', text: '新一章开头', state: 'streaming' }] },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByText('新一章开头')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('直接渲染 AgentUIMessage parts 并上报 turn anchor', async () => {
    const handleVisibleTurnAnchorChange = vi.fn()
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={agentTurnMessages()}
        onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
      />,
    )

    expect(screen.getByText('第一轮用户')).toBeInTheDocument()
    expect(screen.getByText('第一轮剧情')).toBeInTheDocument()
    await waitFor(() => expect(handleVisibleTurnAnchorChange).toHaveBeenCalledWith('turn-1'))
  })

  it('同一次历史 SubAgent 委派的逐轮会话 ID 会合并为一张更新中的卡片', () => {
    const subAgentMetadata = {
      run_id: 'run-review',
      agent_name: 'general-purpose',
      root_agent_name: 'DenovaAgent',
      run_path: ['DenovaAgent', 'general-purpose'],
      subagent: true as const,
    }
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        onOpenSubAgentSession={vi.fn()}
        messages={[
          {
            id: 'review-loop-1',
            role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
            parts: [{ type: 'text', text: '先读取待审章节。' }],
          },
          {
            id: 'review-tool',
            role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
            parts: [{
              type: 'dynamic-tool', toolName: 'bash', toolCallId: 'review-tool-call', state: 'output-available',
              input: { command: 'count words' }, output: 'done',
            }],
          },
          {
            id: 'review-approval',
            role: 'assistant',
            metadata: {},
            parts: [{
              type: 'data-agent-ask',
              data: {
                id: 'review-approval',
                agent_kind: 'ide',
                status: 'answered',
                tool_call_id: 'review-tool-call',
                questions: [{ id: 'tool-approval', question: '允许统计字数？', options: [] }],
              },
            }],
          },
          {
            id: 'review-loop-2',
            role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-03-general-purpose' },
            parts: [{ type: 'text', text: '继续核对上下文。' }],
          },
          {
            id: 'review-loop-3',
            role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-05-general-purpose' },
            parts: [{ type: 'text', text: '审稿完成，输出最终结论。' }],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getAllByRole('button', { name: /general-purpose 输出/ })).toHaveLength(1)
    expect(screen.getByText('审稿完成，输出最终结论。')).toBeInTheDocument()
    expect(screen.queryByText('先读取待审章节。')).not.toBeInTheDocument()
    expect(screen.queryByText('继续核对上下文。')).not.toBeInTheDocument()
  })

  it('根 Agent 的新委派边界不会合并同名 SubAgent 卡片', () => {
    const subAgentMetadata = {
      run_id: 'run-review',
      agent_name: 'general-purpose',
      root_agent_name: 'DenovaAgent',
      run_path: ['DenovaAgent', 'general-purpose'],
      subagent: true as const,
    }
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        onOpenSubAgentSession={vi.fn()}
        messages={[
          {
            id: 'first-delegation', role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
            parts: [{ type: 'text', text: '第一次审稿结论。' }],
          },
          {
            id: 'second-task', role: 'assistant', metadata: { run_id: 'run-review' },
            parts: [{
              type: 'dynamic-tool', toolName: 'task', toolCallId: 'second-task', state: 'output-available',
              input: { subagent_type: 'general-purpose', description: 'review again' }, output: 'done',
            }],
          },
          {
            id: 'second-delegation', role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-02-general-purpose' },
            parts: [{ type: 'text', text: '第二次审稿结论。' }],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getAllByRole('button', { name: /general-purpose 输出/ })).toHaveLength(2)
    expect(screen.getByText('第一次审稿结论。')).toBeInTheDocument()
    expect(screen.getByText('第二次审稿结论。')).toBeInTheDocument()
  })

  it('折叠根 Agent 执行过程时已完成的子工具审批不会拆开 SubAgent 卡片', () => {
    const rootMetadata = {
      run_id: 'run-review',
      agent_name: 'DenovaAgent',
      root_agent_name: 'DenovaAgent',
      run_path: ['DenovaAgent'],
    }
    const subAgentMetadata = {
      ...rootMetadata,
      agent_name: 'general-purpose',
      run_path: ['DenovaAgent', 'general-purpose'],
      subagent: true as const,
    }
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        onOpenSubAgentSession={vi.fn()}
        messages={[
          {
            id: 'root-thinking', role: 'assistant', metadata: rootMetadata,
            parts: [{ type: 'reasoning', text: '准备委派审稿。' }],
          },
          {
            id: 'root-progress', role: 'assistant', metadata: { ...rootMetadata, display_phase: 'progress' },
            parts: [{ type: 'text', text: '开始委派审稿。' }],
          },
          {
            id: 'review-loop-1', role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
            parts: [{ type: 'text', text: '先读取待审章节。' }],
          },
          {
            id: 'review-tool', role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
            parts: [{
              type: 'dynamic-tool', toolName: 'bash', toolCallId: 'review-tool-call', state: 'output-available',
              input: { command: 'count words' }, output: 'done',
            }],
          },
          {
            id: 'review-approval', role: 'assistant', metadata: {},
            parts: [{
              type: 'data-agent-ask',
              data: {
                id: 'review-approval',
                agent_kind: 'ide',
                status: 'answered',
                tool_call_id: 'review-tool-call',
                questions: [{ id: 'tool-approval', question: '允许统计字数？', options: [] }],
              },
            }],
          },
          {
            id: 'review-loop-2', role: 'assistant',
            metadata: { ...subAgentMetadata, subagent_session_id: 'run-review-subagent-03-general-purpose' },
            parts: [{ type: 'text', text: '审稿完成。' }],
          },
          {
            id: 'root-result', role: 'assistant', metadata: { ...rootMetadata, display_phase: 'final' },
            parts: [{ type: 'text', text: '主 Agent 已处理审稿意见。' }],
          },
        ] as AgentUIMessage[]}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /执行过程.*SubAgent/ }))
    expect(screen.getAllByRole('button', { name: /general-purpose 输出/ })).toHaveLength(1)
    expect(screen.getByText('审稿完成。')).toBeInTheDocument()
  })

  it('回合顶端定位时忽略刚好结束在视口上边界的前一回合', async () => {
    const handleVisibleTurnAnchorChange = vi.fn()
    const { container } = render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 52, itemHeight: 52 }}>
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={agentTurnMessages()}
          onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
        />
      </VirtuosoMockContext.Provider>,
    )
    const scroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
    if (!scroller) throw new Error('Expected message scroller')
    await waitFor(() => expect(handleVisibleTurnAnchorChange).toHaveBeenCalledWith('turn-1'))

    handleVisibleTurnAnchorChange.mockClear()
    scroller.scrollTop = 104
    fireEvent.scroll(scroller)

    await waitFor(() => expect(handleVisibleTurnAnchorChange).toHaveBeenCalledWith('turn-2'))
    expect(handleVisibleTurnAnchorChange).not.toHaveBeenCalledWith('turn-1')
  })

  it('把本轮引用显示在已发送的用户消息内', () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[{
          id: 'user-with-references',
          role: 'user',
          metadata: {
            user_references: [
              { kind: 'file', label: 'chapters/ch01.md' },
              { kind: 'selection', label: 'chapters/ch02.md', start_line: 8, end_line: 10, detail: '被引用的正文' },
              { kind: 'review_comment', id: 'comment-1', label: 'setting/progress.md', start_line: 24, detail: '需要增加爽点' },
            ],
          },
          parts: [{ type: 'text', text: '请统一修改' }],
        }] as AgentUIMessage[]}
      />,
    )

    const references = screen.getByTestId('sent-message-references')
    expect(references).toHaveTextContent('chapters/ch01.md')
    expect(references).toHaveTextContent('chapters/ch02.md')
    expect(references).toHaveTextContent('需要增加爽点')
    expect(screen.getByText('请统一修改')).toBeInTheDocument()
  })

  it('把持久化变更摘要插入对应 run 的最后一条消息后', () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[
          { id: 'assistant-a', role: 'assistant', metadata: { run_id: 'run-a' }, parts: [{ type: 'text', text: '第一轮完成' }] },
          { id: 'user-b', role: 'user', parts: [{ type: 'text', text: '继续调整' }] },
          { id: 'assistant-b', role: 'assistant', metadata: { run_id: 'run-b' }, parts: [{ type: 'text', text: '第二轮完成' }] },
        ] as AgentUIMessage[]}
        timelineAttachments={[
          { id: 'group-a', runId: 'run-a', content: <div data-testid="summary-a">第一轮变更</div> },
          { id: 'group-b', runId: 'run-b', content: <div data-testid="summary-b">第二轮变更</div> },
        ]}
      />,
    )

    const firstMessage = screen.getByText('第一轮完成')
    const firstSummary = screen.getByTestId('summary-a')
    const secondUser = screen.getByText('继续调整')
    const secondSummary = screen.getByTestId('summary-b')
    expect(firstMessage.compareDocumentPosition(firstSummary) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(firstSummary.compareDocumentPosition(secondUser) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.getByText('第二轮完成').compareDocumentPosition(secondSummary) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(firstSummary.closest('[data-nova-chat-item="attachment"]')).toHaveClass('pb-4')
    expect(firstSummary.closest('[data-nova-chat-item="attachment"]')).not.toHaveClass('last:pb-0')
    expect(secondSummary.closest('[data-nova-chat-item="attachment"]')).toHaveClass('pb-0')
  })

  it('按 parts 折叠 assistant 正文前的 trace', async () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          {
            id: 'assistant-1',
            role: 'assistant',
            parts: [
              { type: 'reasoning', id: 'reason-1', text: '内部思考' },
              { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
              { type: 'text', id: 'text-1', text: '可见正文' },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.queryByText('内部思考')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /执行过程/ }))
    expect(screen.getByText('内部思考')).toBeInTheDocument()
    expect(screen.getByText('可见正文')).toBeInTheDocument()
  })

  it('正文之后的 thinking 和工具调用统一折叠为一个分组', async () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          {
            id: 'assistant-1',
            role: 'assistant',
            parts: [
              { type: 'text', id: 'text-1', text: '可见正文' },
              { type: 'reasoning', id: 'reason-1', text: '提交前的检查' },
              { type: 'dynamic-tool', toolName: 'submit_choices', toolCallId: 'tool-1', state: 'output-available', input: {}, output: 'ok' },
              { type: 'dynamic-tool', toolName: 'submit_actor_state_patches', toolCallId: 'tool-2', state: 'output-available', input: {}, output: 'ok' },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByText('可见正文')).toBeInTheDocument()
    expect(screen.queryByText('提交前的检查')).not.toBeInTheDocument()
    expect(screen.queryByText('submit_choices')).not.toBeInTheDocument()
    const traceButtons = screen.getAllByRole('button', { name: /执行过程.*2 次工具调用/ })
    expect(traceButtons).toHaveLength(1)
    fireEvent.click(traceButtons[0])
    expect(screen.getByText('提交前的检查')).toBeInTheDocument()
    expect(screen.getByText('submit_choices')).toBeInTheDocument()
    expect(screen.getByText('submit_actor_state_patches')).toBeInTheDocument()
  })

  it('把同一次运行的中间正文、思考和工具统一折叠，只保留最终正文', () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          { id: 'reason-1', role: 'assistant', metadata: { run_id: 'run-diagnose' }, parts: [{ type: 'reasoning', id: 'reason-1', text: '先检查故事索引。' }] },
          { id: 'tool-1', role: 'assistant', metadata: { run_id: 'run-diagnose' }, parts: [{ type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-1', state: 'output-available', input: { path: 'index.json' }, output: 'invalid json' }] },
          { id: 'progress-1', role: 'assistant', metadata: { run_id: 'run-diagnose', display_phase: 'progress' }, parts: [{ type: 'text', id: 'progress-1', text: '找到问题了，继续确认错误位置。' }] },
          { id: 'reason-2', role: 'assistant', metadata: { run_id: 'run-diagnose' }, parts: [{ type: 'reasoning', id: 'reason-2', text: '核对修复结果。' }] },
          { id: 'tool-2', role: 'assistant', metadata: { run_id: 'run-diagnose' }, parts: [{ type: 'dynamic-tool', toolName: 'bash', toolCallId: 'tool-2', state: 'output-available', input: { command: 'jq empty index.json' }, output: 'ok' }] },
          { id: 'final-1', role: 'assistant', metadata: { run_id: 'run-diagnose', display_phase: 'final' }, parts: [{ type: 'text', id: 'final-1', text: '问题已排查并修复。' }] },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByText('问题已排查并修复。')).toBeInTheDocument()
    expect(screen.queryByText('找到问题了，继续确认错误位置。')).not.toBeInTheDocument()
    expect(screen.queryByText('先检查故事索引。')).not.toBeInTheDocument()
    const processButton = screen.getByRole('button', { name: /执行过程.*1 段进展.*2 次工具调用/ })

    fireEvent.click(processButton)

    expect(screen.getByText('找到问题了，继续确认错误位置。')).toBeInTheDocument()
    expect(screen.getByText('先检查故事索引。')).toBeInTheDocument()
    expect(screen.getByText('核对修复结果。')).toBeInTheDocument()
  })

  it('按原始时序保留同一次运行中结果正文前后的执行过程', () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          { id: 'before-reasoning', role: 'assistant', metadata: { run_id: 'run-game-order', display_segment_id: 'before-reasoning' }, parts: [{ type: 'reasoning', text: '正文前思考。' }] },
          { id: 'narrative', role: 'assistant', metadata: { run_id: 'run-game-order', display_segment_id: 'narrative', display_phase: 'final' }, parts: [{ type: 'text', text: '门后传来锁链拖地的声音。' }] },
          { id: 'after-reasoning', role: 'assistant', metadata: { run_id: 'run-game-order', display_segment_id: 'after-reasoning' }, parts: [{ type: 'reasoning', text: '正文后核对状态。' }] },
          { id: 'after-tool', role: 'assistant', metadata: { run_id: 'run-game-order' }, parts: [{ type: 'dynamic-tool', toolName: 'submit_choices', toolCallId: 'after-tool', state: 'output-available', input: {}, output: 'ok' }] },
        ] as AgentUIMessage[]}
      />,
    )

    const narrative = screen.getByText('门后传来锁链拖地的声音。')
    const processButtons = screen.getAllByRole('button', { name: /执行过程/ })
    expect(processButtons).toHaveLength(2)
    expect(processButtons[0].compareDocumentPosition(narrative) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(narrative.compareDocumentPosition(processButtons[1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.queryByText('正文前思考。')).not.toBeInTheDocument()
    expect(screen.queryByText('正文后核对状态。')).not.toBeInTheDocument()
  })

  it('流式结果被持久化历史替换时保持 Run 与结果正文稳定挂载', async () => {
    const list = (messages: ChatMessage[], isStreaming: boolean) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={isStreaming}
          activityContent=""
          collapseTraceGroups
          messages={chatMessagesToAgentUIMessages(messages)}
        />
      </VirtuosoMockContext.Provider>
    )
    const { rerender } = render(list([
      {
        role: 'thinking', content: '正在构建场景。', streaming: false,
        run_id: 'run-stable', display_segment_id: 'reasoning-stable',
      },
      {
        role: 'assistant', content: '雨幕中的城门缓缓打开。', streaming: true,
        render_key: 'narrative-render-key', run_id: 'run-stable', display_phase: 'candidate',
      },
      {
        id: 'tool-stable', role: 'tool_call', name: 'submit_choices', content: 'submit_choices',
        status: 'running', streaming: true, run_id: 'run-stable',
      },
    ], true))

    const liveNarrative = screen.getByText('雨幕中的城门缓缓打开。')
    const liveRow = liveNarrative.closest('[data-nova-chat-item="run"]')
    expect(liveRow).not.toBeNull()

    rerender(list([
      {
        id: 'reasoning-stable', role: 'thinking', content: '正在构建场景。', streaming: false,
        run_id: 'run-stable', display_segment_id: 'reasoning-stable',
      },
      {
        id: 'persisted-turn', role: 'assistant', content: '雨幕中的城门缓缓打开。', streaming: false,
        render_key: 'narrative-render-key', run_id: 'run-stable',
      },
      {
        id: 'tool-stable', role: 'tool_call', name: 'submit_choices', content: 'submit_choices',
        status: 'success', result: 'ok', streaming: false, run_id: 'run-stable',
      },
    ], false))

    const persistedNarrative = await screen.findByText('雨幕中的城门缓缓打开。')
    expect(persistedNarrative.closest('[data-nova-chat-item="run"]')).toBe(liveRow)
    expect(persistedNarrative).toBe(liveNarrative)
    expect(screen.getAllByRole('button', { name: /执行过程/ })).toHaveLength(2)
  })

  it('旧历史没有正文阶段时仍把同一运行的最后一段正文作为结果', () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          { id: 'legacy-progress', role: 'assistant', metadata: { run_id: 'run-legacy' }, parts: [{ type: 'text', id: 'legacy-progress', text: '我继续检查一下。' }] },
          { id: 'legacy-tool', role: 'assistant', metadata: { run_id: 'run-legacy' }, parts: [{ type: 'dynamic-tool', toolName: 'read', toolCallId: 'legacy-tool', state: 'output-available', input: {}, output: 'ok' }] },
          { id: 'legacy-final', role: 'assistant', metadata: { run_id: 'run-legacy' }, parts: [{ type: 'text', id: 'legacy-final', text: '最终结论。' }] },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByText('最终结论。')).toBeInTheDocument()
    expect(screen.queryByText('我继续检查一下。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /执行过程.*1 段进展.*1 次工具调用/ })).toBeInTheDocument()
  })

  it('运行中的 trace 保留用户展开状态，并在运行完成后自动收起', async () => {
    const { rerender } = renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        collapseTraceGroups
        activeTraceDisplay="collapsed"
        messages={[
          {
            id: 'assistant-running',
            role: 'assistant',
            parts: [
              { type: 'reasoning', id: 'reason-running', text: '正在检查资料', state: 'streaming' },
              { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-running', state: 'input-streaming', input: { path: 'a.md' } },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    const traceButton = screen.getByRole('button', { name: /正在执行.*1 次工具调用/ })
    expect(traceButton).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('正在检查资料')).not.toBeInTheDocument()
    expect(screen.queryByText('read')).not.toBeInTheDocument()

    fireEvent.click(traceButton)
    expect(traceButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('正在检查资料')).toBeInTheDocument()
    expect(screen.getByText('read')).toBeInTheDocument()

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="collapsed"
          messages={[
            {
              id: 'assistant-running',
              role: 'assistant',
              parts: [
                { type: 'reasoning', id: 'reason-running', text: '正在检查资料' },
                { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-running', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
              ],
            },
          ] as AgentUIMessage[]}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(screen.getByText('正在检查资料')).toBeInTheDocument()
    expect(screen.getByText('read')).toBeInTheDocument()

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={false}
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="collapsed"
          messages={[
            {
              id: 'assistant-running',
              role: 'assistant',
              parts: [
                { type: 'reasoning', id: 'reason-running', text: '正在检查资料' },
                { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-running', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
                { type: 'text', id: 'text-running', text: '资料检查完成。' },
              ],
            },
          ] as AgentUIMessage[]}
        />
      </VirtuosoMockContext.Provider>,
    )

    await waitFor(() => expect(screen.queryByText('正在检查资料')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: /执行过程.*1 次工具调用/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('资料检查完成。')).toBeInTheDocument()
  })

  it('未指定展示策略时保留原有的运行中 trace 展开行为', () => {
    renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        collapseTraceGroups
        messages={[{
          id: 'assistant-running-default',
          role: 'assistant',
          parts: [{ type: 'reasoning', text: '正在分析', state: 'streaming' }],
        }] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByRole('button', { name: /正在执行/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('正在分析')).toBeInTheDocument()
  })

  it('折叠 trace 的滚动检测不序列化大型工具输出', () => {
    const toJSON = vi.fn(() => ({ payload: 'large result' }))

    renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        collapseTraceGroups
        activeTraceDisplay="collapsed"
        messages={[{
          id: 'assistant-tool-output',
          role: 'assistant',
          parts: [{
            type: 'dynamic-tool',
            toolName: 'read',
            toolCallId: 'tool-output',
            state: 'output-available',
            input: { path: 'large.md' },
            output: { toJSON },
          }],
        }] as AgentUIMessage[]}
      />,
    )

    expect(toJSON).not.toHaveBeenCalled()
  })

  it('新一轮 streaming 不会重新展开历史 trace', async () => {
    const { rerender } = renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={traceHistoryMessages(false)}
      />,
    )

    expect(screen.queryByText('上一轮思考')).not.toBeInTheDocument()
    expect(screen.getByText('上一轮正文。')).toBeInTheDocument()

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent=""
          collapseTraceGroups
          messages={traceHistoryMessages(true)}
        />
      </VirtuosoMockContext.Provider>,
    )

    await waitFor(() => expect(screen.queryByText('上一轮思考')).not.toBeInTheDocument())
    expect(screen.getByText('新的问题')).toBeInTheDocument()
  })
})

function agentTurnMessages(): AgentUIMessage[] {
  return [
    {
      id: 'user-1',
      role: 'user',
      metadata: { turn_id: 'turn-1', navigation_turn_id: 'turn-1' },
      parts: [{ type: 'text', text: '第一轮用户' }],
    },
    {
      id: 'assistant-1',
      role: 'assistant',
      metadata: { turn_id: 'turn-1', navigation_turn_id: 'turn-1' },
      parts: [{ type: 'text', text: '第一轮剧情' }],
    },
    {
      id: 'user-2',
      role: 'user',
      metadata: { turn_id: 'turn-2', navigation_turn_id: 'turn-2' },
      parts: [{ type: 'text', text: '第二轮用户' }],
    },
  ] as AgentUIMessage[]
}

function traceHistoryMessages(withNewUser: boolean): AgentUIMessage[] {
  const messages: AgentUIMessage[] = [
    {
      id: 'assistant-old',
      role: 'assistant',
      parts: [
        { type: 'reasoning', id: 'reason-old', text: '上一轮思考' },
        { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-old', state: 'output-available', input: { path: 'old.md' }, output: 'ok' },
        { type: 'text', id: 'text-old', text: '上一轮正文。' },
      ],
    },
  ] as AgentUIMessage[]
  if (withNewUser) {
    messages.push({
      id: 'user-new',
      role: 'user',
      parts: [{ type: 'text', text: '新的问题' }],
    } as AgentUIMessage)
  }
  return messages
}
