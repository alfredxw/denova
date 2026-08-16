import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { createAgentReasoningMessage, createAgentTextMessage, createAgentToolMessage } from '@/lib/agent-ui-message'
import { MessageList } from './MessageList'
import { formatExecutionDuration } from './AgentExecutionProcess'

function renderMessageList(ui: ReactElement) {
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
      {ui}
    </VirtuosoMockContext.Provider>,
  )
}

describe('Agent MessageList', () => {
  it('formats execution durations at second, minute, and hour boundaries', () => {
    expect(formatExecutionDuration(0)).toBe('0s')
    expect(formatExecutionDuration(59_999)).toBe('59s')
    expect(formatExecutionDuration(61_000)).toBe('1m1s')
    expect(formatExecutionDuration(3_661_000)).toBe('1h1m1s')
  })

  it('updates only the active process duration and freezes the server summary on completion', () => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
    vi.setSystemTime(new Date('2026-01-01T01:01:01.000Z'))
    const started = {
      id: 'cycle-started',
      role: 'assistant' as const,
      parts: [{
        type: 'data-agent-activity',
        id: 'cycle-started',
        data: {
          event: 'agent_cycle_started',
          run_id: 'run-duration',
          run_started_at: '2026-01-01T00:00:00.000Z',
        },
      }],
    } as AgentUIMessage
    const renderDuration = (messages: AgentUIMessage[], isStreaming: boolean) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={isStreaming}
          isExecutionActive={isStreaming}
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="expanded"
          messages={messages}
        />
      </VirtuosoMockContext.Provider>
    )

    try {
      const { container, rerender } = render(renderDuration([
        started,
        { id: 'reasoning-live', role: 'assistant', metadata: { run_id: 'run-duration' }, parts: [{ type: 'reasoning', text: 'Still working', state: 'streaming' }] },
      ] as AgentUIMessage[], true))

      const activeProcessButton = screen.getByRole('button', { name: /正在执行.*1h1m1s/ })
      expect(activeProcessButton).toBeInTheDocument()
      expect(activeProcessButton).toHaveClass('hover:text-[var(--nova-text)]')
      const executionToggleIcon = activeProcessButton.lastElementChild
      const executionContent = container.querySelector('[data-agent-execution-content]')
      expect(executionToggleIcon).toHaveAttribute('data-agent-execution-toggle-icon')
      expect(executionToggleIcon).toHaveClass('rotate-90', 'duration-[var(--nova-motion-fast)]')
      expect(executionContent).toHaveClass('nova-agent-execution-content')
      expect(executionContent).toHaveAttribute('data-state', 'open')
      expect(executionContent).not.toHaveClass('border-l', 'px-3')
      fireEvent.click(activeProcessButton)
      expect(executionToggleIcon).not.toHaveClass('rotate-90')
      expect(executionContent).toHaveAttribute('data-state', 'closed')
      fireEvent.click(activeProcessButton)
      act(() => vi.advanceTimersByTime(1_000))
      expect(screen.getByRole('button', { name: /正在执行.*1h1m2s/ })).toBeInTheDocument()

      rerender(renderDuration([
        started,
        { id: 'reasoning-done', role: 'assistant', metadata: { run_id: 'run-duration' }, parts: [{ type: 'reasoning', text: 'Checked' }] },
        { id: 'final', role: 'assistant', metadata: { run_id: 'run-duration', display_phase: 'final' }, parts: [{ type: 'text', text: 'Done' }] },
        { id: 'post-tool', role: 'assistant', metadata: { run_id: 'run-duration' }, parts: [{ type: 'dynamic-tool', toolName: 'submit_choices', toolCallId: 'post-tool', state: 'output-available', input: {}, output: 'ok' }] },
        {
          id: 'execution-summary',
          role: 'assistant',
          parts: [{
            type: 'data-agent-execution-summary',
            id: 'execution-summary',
            data: {
              run_id: 'run-duration',
              run_started_at: '2026-01-01T00:00:00.000Z',
              run_finished_at: '2026-01-01T01:01:01.000Z',
              duration_ms: 3_661_000,
              status: 'completed',
            },
          }],
        },
      ] as AgentUIMessage[], false))

      expect(screen.getAllByRole('button', { name: /执行过程/ })).toHaveLength(2)
      expect(screen.getAllByText('1h1m1s')).toHaveLength(1)
      act(() => vi.advanceTimersByTime(10_000))
      expect(screen.getAllByText('1h1m1s')).toHaveLength(1)
      expect(screen.queryByText('1h1m11s')).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps a summarized predecessor frozen while only its queued successor is executing', () => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
    vi.setSystemTime(new Date('2026-01-01T00:00:15.000Z'))
    try {
      renderMessageList(
        <MessageList
          isStreaming
          isExecutionActive
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="expanded"
          messages={[
            { id: 'first-start', role: 'assistant', parts: [{ type: 'data-agent-activity', data: { event: 'agent_cycle_started', run_id: 'run-first', run_started_at: '2026-01-01T00:00:00.000Z' } }] },
            { id: 'first-reasoning', role: 'assistant', metadata: { run_id: 'run-first' }, parts: [{ type: 'reasoning', text: 'First reasoning' }] },
            { id: 'first-final', role: 'assistant', metadata: { run_id: 'run-first', display_phase: 'final' }, parts: [{ type: 'text', text: 'First result' }] },
            { id: 'first-summary', role: 'assistant', parts: [{ type: 'data-agent-execution-summary', data: { run_id: 'run-first', run_started_at: '2026-01-01T00:00:00.000Z', run_finished_at: '2026-01-01T00:00:10.000Z', duration_ms: 10_000, status: 'completed' } }] },
            { id: 'second-start', role: 'assistant', parts: [{ type: 'data-agent-activity', data: { event: 'agent_cycle_started', run_id: 'run-second', run_started_at: '2026-01-01T00:00:10.000Z' } }] },
            { id: 'second-reasoning', role: 'assistant', metadata: { run_id: 'run-second' }, parts: [{ type: 'reasoning', text: 'Second reasoning', state: 'streaming' }] },
          ] as AgentUIMessage[]}
        />,
      )

      expect(screen.getAllByRole('button', { name: /正在执行/ })).toHaveLength(1)
      expect(screen.getByRole('button', { name: /执行过程.*10s/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /正在执行.*5s/ })).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('首次恢复历史时定位到底部后才显示消息', () => {
    const frameCallbacks: FrameRequestCallback[] = []
    const clientHeightSpy = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(function clientHeight(this: HTMLElement) {
      return this.classList.contains('nova-chat-canvas') ? 200 : 0
    })
    const scrollHeightSpy = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockImplementation(function scrollHeight(this: HTMLElement) {
      return this.classList.contains('nova-chat-canvas') ? 900 : 0
    })
    const animationFrameSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frameCallbacks.push(callback)
      return frameCallbacks.length
    })

    try {
      const { container } = renderMessageList(
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={[{ id: 'restored-message', role: 'assistant', parts: [{ type: 'text', text: '恢复的末条消息' }] }]}
          scrollResetKey="restored-session"
        />,
      )
      const scroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
      if (!scroller) throw new Error('Expected message scroller')

      expect(scroller).toHaveClass('opacity-0')
      expect(scroller).toHaveAttribute('aria-busy', 'true')
      expect(scroller).toHaveAttribute('aria-hidden', 'true')
      expect(screen.getByText('加载中...')).toBeInTheDocument()

      act(() => {
        for (let frame = 0; frame < 4 && frameCallbacks.length > 0; frame += 1) {
          frameCallbacks.splice(0).forEach(callback => callback(frame * 16))
        }
      })

      expect(scroller.scrollTop).toBe(700)
      expect(scroller).not.toHaveClass('opacity-0')
      expect(scroller).toHaveAttribute('aria-busy', 'false')
      expect(scroller).not.toHaveAttribute('aria-hidden')
      expect(screen.queryByText('加载中...')).not.toBeInTheDocument()
    } finally {
      animationFrameSpy.mockRestore()
      scrollHeightSpy.mockRestore()
      clientHeightSpy.mockRestore()
    }
  })

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
    const footerContent = state.closest('[data-nova-chat-after-content]')
    expect(prose.compareDocumentPosition(state) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(footerContent?.nextElementSibling).toHaveAttribute('data-nova-chat-after-content-reserve')
    expect(footerContent?.parentElement?.querySelector('[data-nova-chat-bottom-spacer]')).toBeInTheDocument()
  })

  it('lets multiline prose grow into the reserved response runway without moving earlier text', () => {
    let rowHeight = 40
    let scrollHeight = 1300
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.classList.contains('nova-chat-canvas')) return { top: 0, bottom: 900, height: 900 } as DOMRect
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
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 900 })
      scroller.scrollTop = 400
      fireEvent.scroll(scroller)

      rowHeight = 70
      scrollHeight = 1330
      rerender(renderList('第一行\n第二行'))

      expect(scroller.scrollTop).toBe(400)
    } finally {
      rectSpy.mockRestore()
    }
  })

  it('keeps short conversations top-aligned before and after an active run', () => {
    const renderList = (isStreaming: boolean) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={isStreaming}
          activityContent={isStreaming ? '正在思考…' : ''}
          messages={isStreaming ? [] : agentTurnMessages()}
        />
      </VirtuosoMockContext.Provider>
    )
    const { container, rerender } = render(renderList(true))
    const viewport = container.querySelector<HTMLElement>('[data-viewport-type="element"]')
    if (!viewport) throw new Error('Expected Virtuoso viewport')

    expect(viewport).not.toHaveStyle({ display: 'flex' })

    rerender(renderList(false))
    expect(viewport).not.toHaveStyle({ display: 'flex' })
  })

  it('lets a multiline thinking replacement grow downward while runway remains', () => {
    let rowHeight = 28
    let rowExtent = 500
    let scrollHeight = 1300
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.classList.contains('nova-chat-canvas')) return { top: 0, bottom: 900, height: 900 } as DOMRect
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
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 900 })
      scroller.scrollTop = 400
      fireEvent.scroll(scroller)

      rowHeight = 98
      rowExtent = 570
      scrollHeight = 1370
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

      expect(scroller.scrollTop).toBe(400)
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

  it('keeps the message viewport stable when an interacted footer tab becomes shorter', () => {
    const resizeCallbacks = new Map<Element, ResizeObserverCallback>()
    vi.stubGlobal('ResizeObserver', class ResizeObserverHarness {
      private readonly callback: ResizeObserverCallback

      constructor(callback: ResizeObserverCallback) {
        this.callback = callback
      }

      observe = (target: Element) => resizeCallbacks.set(target, this.callback)
      unobserve = (target: Element) => resizeCallbacks.delete(target)
      disconnect = vi.fn()
    })

    const { container } = renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={agentTurnMessages()}
        afterContent={<button type="button">切换状态页签</button>}
        afterContentKey="turn-2:expanded"
        bottomPaddingPx={120}
      />,
    )
    const scroller = container.querySelector<HTMLElement>('.nova-chat-canvas')
    const footer = container.querySelector<HTMLElement>('[data-nova-chat-after-content]')
    const reserve = container.querySelector<HTMLElement>('[data-nova-chat-after-content-reserve]')
    if (!scroller || !footer || !reserve) throw new Error('Expected message footer scroll geometry')

    let footerHeight = 200
    let scrollTop = 480
    footer.getBoundingClientRect = () => ({ height: footerHeight }) as DOMRect
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
    Object.defineProperty(scroller, 'scrollHeight', {
      configurable: true,
      get: () => 400 + footerHeight + Number.parseFloat(reserve.style.height || '0'),
    })
    Object.defineProperty(scroller, 'scrollTop', {
      configurable: true,
      get: () => scrollTop,
      set: (value: number) => {
        scrollTop = Math.max(0, Math.min(value, scroller.scrollHeight - scroller.clientHeight))
      },
    })

    act(() => resizeCallbacks.get(footer)?.([{ target: footer } as unknown as ResizeObserverEntry], {} as ResizeObserver))
    fireEvent.pointerDown(screen.getByRole('button', { name: '切换状态页签' }))

    footerHeight = 80
    scroller.scrollTop = scroller.scrollTop
    expect(scroller.scrollTop).toBe(380)

    act(() => resizeCallbacks.get(footer)?.([{ target: footer } as unknown as ResizeObserverEntry], {} as ResizeObserver))

    expect(reserve).toHaveStyle({ height: '120px' })
    expect(scroller.scrollTop).toBe(480)
  })

  it('tracks the tallest footer tab separately for preview and expanded layouts', () => {
    const resizeCallbacks = new Map<Element, ResizeObserverCallback>()
    vi.stubGlobal('ResizeObserver', class ResizeObserverHarness {
      private readonly callback: ResizeObserverCallback

      constructor(callback: ResizeObserverCallback) {
        this.callback = callback
      }

      observe = (target: Element) => resizeCallbacks.set(target, this.callback)
      unobserve = (target: Element) => resizeCallbacks.delete(target)
      disconnect = vi.fn()
    })

    const renderList = (heightScope: 'preview' | 'expanded') => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={agentTurnMessages()}
          afterContent={(
            <button type="button" data-nova-chat-after-content-height-scope={heightScope}>
              {heightScope}
            </button>
          )}
          afterContentKey="turn-2"
          bottomPaddingPx={120}
        />
      </VirtuosoMockContext.Provider>
    )

    const { container, rerender } = render(renderList('expanded'))
    const footer = container.querySelector<HTMLElement>('[data-nova-chat-after-content]')
    const reserve = container.querySelector<HTMLElement>('[data-nova-chat-after-content-reserve]')
    if (!footer || !reserve) throw new Error('Expected message footer scroll geometry')

    let footerHeight = 200
    footer.getBoundingClientRect = () => ({ height: footerHeight }) as DOMRect
    const notifyResize = () => act(() => resizeCallbacks.get(footer)?.([{ target: footer } as unknown as ResizeObserverEntry], {} as ResizeObserver))

    notifyResize()
    expect(reserve).toHaveStyle({ height: '0px' })

    footerHeight = 80
    rerender(renderList('preview'))
    notifyResize()
    expect(reserve).toHaveStyle({ height: '0px' })

    footerHeight = 120
    rerender(renderList('expanded'))
    notifyResize()
    expect(reserve).toHaveStyle({ height: '80px' })

    footerHeight = 60
    rerender(renderList('preview'))
    notifyResize()
    expect(reserve).toHaveStyle({ height: '20px' })
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
    expect(screen.queryByText('内部思考')).not.toBeInTheDocument()
    expect(screen.getByText('可见正文')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '思考过程' }))
    expect(screen.getByText('内部思考')).toBeInTheDocument()
  })

  it('跳过已完成的空 thinking，但保留流式空 thinking 的活动入口', () => {
    const completed = renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          {
            id: 'assistant-empty-reasoning',
            role: 'assistant',
            parts: [
              { type: 'reasoning', text: '' },
              { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-empty-reasoning', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
              { type: 'text', id: 'text-empty-reasoning', text: '读取完成' },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /执行过程/ }))
    expect(screen.queryByRole('button', { name: '思考过程' })).not.toBeInTheDocument()
    expect(screen.getByText('读取')).toBeInTheDocument()

    completed.unmount()
    renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        collapseTraceGroups
        messages={[
          {
            id: 'assistant-streaming-empty-reasoning',
            role: 'assistant',
            parts: [{ type: 'reasoning', text: '', state: 'streaming' }],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByRole('button', { name: '思考中...' })).toHaveAttribute('aria-expanded', 'true')
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
    expect(screen.queryByText('提交前的检查')).not.toBeInTheDocument()
    expect(screen.getByText('submit_choices')).toBeInTheDocument()
    expect(screen.getByText('submit_actor_state_patches')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '思考过程' }))
    expect(screen.getByText('提交前的检查')).toBeInTheDocument()
  })

  it('流式运行保留每段进展并在完成后统一折叠为执行过程', async () => {
    const progressMessages = [
      { id: 'reason-1', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'reasoning', text: '思考1' }] },
      { id: 'tool-1', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-1', state: 'output-available', input: {}, output: 'ok' }] },
      { id: 'tool-2', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'dynamic-tool', toolName: 'bash', toolCallId: 'tool-2', state: 'output-available', input: {}, output: 'ok' }] },
      { id: 'progress-1', role: 'assistant', metadata: { run_id: 'run-progress', display_phase: 'candidate', created_at: '2026-01-01T12:31:00Z' }, parts: [{ type: 'text', id: 'progress-1', text: '进展一' }] },
      { id: 'reason-2', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'reasoning', text: '思考2' }] },
      { id: 'tool-3', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-3', state: 'output-available', input: {}, output: 'ok' }] },
      { id: 'tool-4', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'dynamic-tool', toolName: 'bash', toolCallId: 'tool-4', state: 'output-available', input: {}, output: 'ok' }] },
      { id: 'progress-2', role: 'assistant', metadata: { run_id: 'run-progress', display_phase: 'candidate', created_at: '2026-01-01T12:32:00Z' }, parts: [{ type: 'text', id: 'progress-2', text: '进展二' }] },
      { id: 'reason-3', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'reasoning', text: '思考3' }] },
      { id: 'tool-5', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-5', state: 'output-available', input: {}, output: 'ok' }] },
      { id: 'tool-6', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'dynamic-tool', toolName: 'bash', toolCallId: 'tool-6', state: 'output-available', input: {}, output: 'ok' }] },
      { id: 'progress-3', role: 'assistant', metadata: { run_id: 'run-progress', display_phase: 'candidate', created_at: '2026-01-01T12:33:00Z' }, parts: [{ type: 'text', id: 'progress-3', text: '进展三' }] },
    ] as AgentUIMessage[]
    const renderProgress = (messages: AgentUIMessage[], isStreaming: boolean) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={isStreaming}
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="expanded"
          messages={messages}
        />
      </VirtuosoMockContext.Provider>
    )
    const { container, rerender } = render(renderProgress([
      { id: 'reason-1', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'reasoning', text: '思考1', state: 'streaming' }] },
    ] as AgentUIMessage[], true))

    expect(screen.getByText('思考1')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^正在执行$/ })).toHaveAttribute('aria-expanded', 'true')

    rerender(renderProgress([
      ...progressMessages,
      { id: 'reason-4', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'reasoning', text: '当前流式思考', state: 'streaming' }] },
    ] as AgentUIMessage[], true))

    expect(screen.getByText('进展一')).toBeInTheDocument()
    expect(screen.getByText('进展二')).toBeInTheDocument()
    expect(screen.getByText('进展三')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '复制消息' })).not.toBeInTheDocument()
    expect(container.querySelector('.nova-message-time')).toBeNull()
    expect(screen.getByText('当前流式思考')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /^执行过程 · 2 次工具调用$/ })).toHaveLength(3)
    expect(screen.getByRole('button', { name: /^正在执行$/ })).toHaveAttribute('aria-expanded', 'true')
    expect(container.querySelectorAll('[data-agent-execution-process]')).toHaveLength(4)
    await waitFor(() => expect(screen.queryByText('思考1')).not.toBeInTheDocument())

    rerender(renderProgress([
      ...progressMessages.map(message => message.metadata?.display_phase === 'candidate'
        ? { ...message, metadata: { ...message.metadata, display_phase: 'progress' as const } }
        : message),
      { id: 'reason-4', role: 'assistant', metadata: { run_id: 'run-progress' }, parts: [{ type: 'reasoning', text: '最后确认' }] },
      { id: 'final', role: 'assistant', metadata: { run_id: 'run-progress', display_phase: 'final' }, parts: [{ type: 'text', id: 'final', text: '最终结论' }] },
    ] as AgentUIMessage[], false))

    await waitFor(() => expect(screen.queryByText('进展一')).not.toBeInTheDocument())
    expect(screen.queryByText('进展二')).not.toBeInTheDocument()
    expect(screen.queryByText('进展三')).not.toBeInTheDocument()
    expect(screen.getByText('最终结论')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^执行过程 · 3 段进展 · 6 次工具调用$/ })).toHaveAttribute('aria-expanded', 'false')
    expect(container.querySelectorAll('[data-agent-execution-process]')).toHaveLength(1)
  })

  it('把同一次运行的中间正文、思考和工具统一折叠，只保留最终正文', () => {
    const { container } = renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceGroups
        messages={[
          { id: 'reason-1', role: 'assistant', metadata: { run_id: 'run-diagnose' }, parts: [{ type: 'reasoning', id: 'reason-1', text: '先检查故事索引。' }] },
          { id: 'tool-1', role: 'assistant', metadata: { run_id: 'run-diagnose' }, parts: [{ type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-1', state: 'output-available', input: { path: 'index.json' }, output: 'invalid json' }] },
          { id: 'progress-1', role: 'assistant', metadata: { run_id: 'run-diagnose', display_phase: 'progress', created_at: '2026-01-01T12:34:00Z' }, parts: [{ type: 'text', id: 'progress-1', text: '找到问题了，继续确认错误位置。' }] },
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
    expect(screen.getAllByRole('button', { name: '复制消息' })).toHaveLength(1)
    expect(container.querySelector('.nova-message-time')).toBeNull()
    expect(screen.queryByText('先检查故事索引。')).not.toBeInTheDocument()
    expect(screen.queryByText('核对修复结果。')).not.toBeInTheDocument()

    const thinkingButtons = screen.getAllByRole('button', { name: '思考过程' })
    expect(thinkingButtons).toHaveLength(2)
    fireEvent.click(thinkingButtons[0])
    expect(screen.getByText('先检查故事索引。')).toBeInTheDocument()
    expect(screen.queryByText('核对修复结果。')).not.toBeInTheDocument()

    fireEvent.click(thinkingButtons[1])
    expect(screen.getByText('先检查故事索引。')).toBeInTheDocument()
    expect(screen.getByText('核对修复结果。')).toBeInTheDocument()
  })

  it('运行完成后仍按时间顺序保留结果正文前后的执行过程', () => {
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

    fireEvent.click(processButtons[0])
    expect(screen.queryByText('正文前思考。')).not.toBeInTheDocument()
    expect(screen.queryByText('正文后核对状态。')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '思考过程' }))
    expect(screen.getByText('正文前思考。')).toBeInTheDocument()

    fireEvent.click(processButtons[1])
    expect(screen.queryByText('正文后核对状态。')).not.toBeInTheDocument()
    const thinkingButtons = screen.getAllByRole('button', { name: '思考过程' })
    expect(thinkingButtons).toHaveLength(2)
    fireEvent.click(thinkingButtons[1])
    expect(screen.getByText('正文后核对状态。')).toBeInTheDocument()
  })

  it('流式结果被持久化历史替换时保持 Run 与结果正文稳定挂载', async () => {
    const list = (messages: AgentUIMessage[], isStreaming: boolean) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={isStreaming}
          activityContent=""
          collapseTraceGroups
          messages={messages}
        />
      </VirtuosoMockContext.Provider>
    )
    const { rerender } = render(list([
      createAgentReasoningMessage({ id: 'reasoning-stable', text: '正在构建场景。', metadata: { run_id: 'run-stable', display_segment_id: 'reasoning-stable' } }),
      createAgentTextMessage({ id: 'narrative-render-key', role: 'assistant', text: '雨幕中的城门缓缓打开。', state: 'streaming', metadata: { run_id: 'run-stable', display_phase: 'candidate' } }),
      createAgentToolMessage({ id: 'tool-stable', name: 'submit_choices', state: 'input-streaming', input: {}, metadata: { run_id: 'run-stable' } }),
    ], true))

    const liveNarrative = screen.getByText('雨幕中的城门缓缓打开。')
    const liveRow = liveNarrative.closest('[data-nova-chat-item="run"]')
    expect(liveRow).not.toBeNull()

    rerender(list([
      createAgentReasoningMessage({ id: 'reasoning-stable', text: '正在构建场景。', metadata: { run_id: 'run-stable', display_segment_id: 'reasoning-stable' } }),
      createAgentTextMessage({ id: 'narrative-render-key', role: 'assistant', text: '雨幕中的城门缓缓打开。', metadata: { run_id: 'run-stable' } }),
      createAgentToolMessage({ id: 'tool-stable', name: 'submit_choices', state: 'output-available', input: {}, output: 'ok', metadata: { run_id: 'run-stable' } }),
    ], false))

    const persistedNarrative = await screen.findByText('雨幕中的城门缓缓打开。')
    expect(persistedNarrative.closest('[data-nova-chat-item="run"]')).toBe(liveRow)
    expect(persistedNarrative).toBe(liveNarrative)
    const processButtons = screen.getAllByRole('button', { name: /执行过程/ })
    expect(processButtons).toHaveLength(2)
    expect(processButtons[0].compareDocumentPosition(persistedNarrative) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(persistedNarrative.compareDocumentPosition(processButtons[1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
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
    expect(screen.queryByText('读取')).not.toBeInTheDocument()

    fireEvent.click(traceButton)
    expect(traceButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('正在检查资料')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '思考中...' })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('读取')).toBeInTheDocument()

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

    expect(screen.queryByText('正在检查资料')).not.toBeInTheDocument()
    const completedThinkingButton = screen.getByRole('button', { name: '思考过程' })
    expect(completedThinkingButton).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('读取')).toBeInTheDocument()

    fireEvent.click(completedThinkingButton)
    expect(screen.getByText('正在检查资料')).toBeInTheDocument()

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

  it('用户手动展开的流式 thinking 在完成后保持状态，外层收起后重置', () => {
    const renderRunningTrace = (reasoningState: 'streaming' | undefined) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent=""
          collapseTraceGroups
          activeTraceDisplay="expanded"
          messages={[
            {
              id: 'assistant-running-reasoning-toggle',
              role: 'assistant',
              parts: [
                { type: 'reasoning', text: '核对当前章节', state: reasoningState },
                { type: 'dynamic-tool', toolName: 'read', toolCallId: 'tool-running-toggle', state: 'input-streaming', input: { path: 'chapter.md' } },
              ],
            },
          ] as AgentUIMessage[]}
        />
      </VirtuosoMockContext.Provider>
    )
    const { rerender } = render(renderRunningTrace('streaming'))

    const streamingThinkingButton = screen.getByRole('button', { name: '思考中...' })
    expect(streamingThinkingButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('核对当前章节')).toBeInTheDocument()

    fireEvent.click(streamingThinkingButton)
    fireEvent.click(streamingThinkingButton)
    expect(streamingThinkingButton).toHaveAttribute('aria-expanded', 'true')

    rerender(renderRunningTrace(undefined))

    const completedThinkingButton = screen.getByRole('button', { name: '思考过程' })
    expect(completedThinkingButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('核对当前章节')).toBeInTheDocument()

    const processButton = screen.getByRole('button', { name: /正在执行.*1 次工具调用/ })
    fireEvent.click(processButton)
    expect(screen.queryByRole('button', { name: '思考过程' })).not.toBeInTheDocument()
    fireEvent.click(processButton)

    expect(screen.getByRole('button', { name: '思考过程' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('核对当前章节')).not.toBeInTheDocument()
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
