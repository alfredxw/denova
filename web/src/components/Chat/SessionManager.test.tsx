import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import { MessageList as RawMessageList } from './MessageList'
import { ScrollToBottomButton } from './ScrollToBottomButton'
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
  Object.defineProperty(element, 'offsetHeight', { configurable: true, get: () => clientHeight })
  Object.defineProperty(element, 'scrollTop', {
    configurable: true,
    get: () => scrollTop,
    set: (value) => {
      scrollTop = value
    },
  })
  Object.defineProperty(element, 'scrollTo', {
    configurable: true,
    value: (options?: ScrollToOptions | number, y?: number) => {
      if (typeof options === 'number') {
        scrollTop = y ?? scrollTop
        return
      }
      if (typeof options?.top === 'number') scrollTop = options.top
    },
  })
  return {
    setScrollHeight: (value: number) => { scrollHeight = value },
    setClientHeight: (value: number) => { clientHeight = value },
    setScrollTop: (value: number) => { scrollTop = value },
    maxScrollTop: () => Math.max(0, scrollHeight - clientHeight),
  }
}

function buildDomRect(rect: Partial<DOMRect>) {
  return {
    width: rect.width ?? 0,
    height: rect.height ?? 0,
    top: rect.top ?? 0,
    left: rect.left ?? 0,
    right: rect.right ?? ((rect.left ?? 0) + (rect.width ?? 0)),
    bottom: rect.bottom ?? ((rect.top ?? 0) + (rect.height ?? 0)),
    x: rect.x ?? rect.left ?? 0,
    y: rect.y ?? rect.top ?? 0,
    toJSON: () => ({}),
  } as DOMRect
}

function getMessageScroller(container: HTMLElement) {
  const scroller = container.querySelector('.nova-chat-canvas')
  expect(scroller).toBeInstanceOf(HTMLDivElement)
  return scroller as HTMLDivElement
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
  it('回到底部按钮在 pointer down 时触发滚动回调', () => {
    const handleClick = vi.fn()
    render(<ScrollToBottomButton visible onClick={handleClick} />)

    fireEvent.pointerDown(screen.getByRole('button', { name: '回到底部' }), { button: 0 })

    expect(handleClick).toHaveBeenCalledTimes(1)
  })

  it('历史消息首次加载后默认滚动到底部', async () => {
    const { container, rerender } = render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[]}
        scrollResetKey="session-a"
      />,
    )
    const scroller = getMessageScroller(container)
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
    const scroller = getMessageScroller(container)
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
    const scroller = getMessageScroller(container)
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

  it('使用 render_key 保持流式消息落盘后的列表行身份稳定', () => {
    const { container, rerender } = render(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '推门', render_key: 'turn-live-user' },
          { type: 'message', role: 'assistant', content: '门外有灯。', streaming: true, render_key: 'turn-live-assistant' },
        ]}
        scrollResetKey="session-a"
      />,
    )

    expect(container.querySelector('[data-nova-chat-row-key="message-turn-live-user"]')).toBeInTheDocument()
    expect(container.querySelector('[data-nova-chat-row-key="message-turn-live-assistant"]')).toBeInTheDocument()

    rerender(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '推门', id: 'turn-1-user', render_key: 'turn-live-user' },
          { type: 'message', role: 'assistant', content: '门外有灯。', id: 'turn-1-assistant', render_key: 'turn-live-assistant' },
        ]}
        scrollResetKey="session-a"
      />,
    )

    expect(container.querySelector('[data-nova-chat-row-key="message-turn-live-user"]')).toBeInTheDocument()
    expect(container.querySelector('[data-nova-chat-row-key="message-turn-live-assistant"]')).toBeInTheDocument()
    expect(container.querySelector('[data-nova-chat-row-key="message-turn-1-assistant"]')).not.toBeInTheDocument()
  })

  it('仅切换流式完成状态时不触发额外吸底滚动', async () => {
    const { container, rerender } = render(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '推门', render_key: 'turn-live-user' },
          { type: 'message', role: 'assistant', content: '门外有灯。', streaming: true, render_key: 'turn-live-assistant' },
        ]}
        scrollResetKey="session-a"
      />,
    )
    const scroller = getMessageScroller(container)
    const scrollMetrics = mockScrollMetrics(scroller)
    await waitFor(() => expect(scroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 120))
    })
    const scrollTopBeforePersist = scroller.scrollTop

    scrollMetrics.setScrollHeight(1500)
    rerender(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '推门', id: 'turn-1-user', render_key: 'turn-live-user' },
          { type: 'message', role: 'assistant', content: '门外有灯。', id: 'turn-1-assistant', render_key: 'turn-live-assistant' },
        ]}
        scrollResetKey="session-a"
      />,
    )

    expect(scroller.scrollTop).toBe(scrollTopBeforePersist)
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
    const scroller = getMessageScroller(container)
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

  it('用户离底部超过阈值时显示回到底部按钮', async () => {
    const { container } = render(
      <MessageList
        isStreaming
        activityContent=""
        messages={[
          { type: 'message', role: 'user', content: '继续写' },
          { type: 'message', role: 'assistant', content: '第一段', streaming: true },
        ]}
        scrollResetKey="session-a"
        bottomPaddingPx={180}
      />,
    )
    const scroller = getMessageScroller(container)
    const scrollMetrics = mockScrollMetrics(scroller)

    scroller.scrollTop = scrollMetrics.maxScrollTop()
    fireEvent.scroll(scroller)
    expect(screen.queryByRole('button', { name: '回到底部' })).not.toBeInTheDocument()

    scroller.scrollTop = scrollMetrics.maxScrollTop() - 180
    fireEvent.scroll(scroller)
    const scrollButton = await screen.findByRole('button', { name: '回到底部' })
    expect(scrollButton).toHaveStyle({ bottom: '192px' })
    expect(scrollButton).toHaveStyle({ right: '24px' })
  })

  it('生成 Plan 卡片和内容增长时自动把卡片底部对齐到对话输入框顶部', async () => {
    const user = userEvent.setup()
    const originalScrollIntoView = Object.getOwnPropertyDescriptor(Element.prototype, 'scrollIntoView')
    const originalHTMLElementScrollIntoView = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
    const originalResizeObserver = globalThis.ResizeObserver
    const scrollIntoView = vi.fn()
    let planRowContentBottom = 760
    let planRowHeight = 340
    let scroller: HTMLDivElement | null = null
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('nova-chat-canvas')) {
        return buildDomRect({ top: 0, bottom: 720, width: 520, height: 720 })
      }
      if (this.classList.contains('nova-agent-composer')) {
        return buildDomRect({ top: 500, bottom: 600, width: 520, height: 100 })
      }
      if (this.dataset.novaChatRowKey === 'message-plan-question-1') {
        const bottom = planRowContentBottom - (scroller?.scrollTop || 0)
        return buildDomRect({ top: bottom - planRowHeight, bottom, width: 520, height: planRowHeight })
      }
      return buildDomRect({})
    })
    const resizeObservers: Array<{ targets: Element[]; callback: ResizeObserverCallback }> = []
    class MockResizeObserver {
      targets: Element[] = []
      callback: ResizeObserverCallback
      observe = vi.fn()
      unobserve = vi.fn()
      disconnect = vi.fn()
      constructor(callback: ResizeObserverCallback) {
        this.callback = callback
        resizeObservers.push(this)
        this.observe = vi.fn((target: Element) => {
          this.targets.push(target)
        })
      }
    }
    const triggerResizeFor = (target: Element) => {
      for (const observer of resizeObservers) {
        if (observer.targets.includes(target)) {
          observer.callback([{ target } as ResizeObserverEntry], observer as unknown as ResizeObserver)
        }
      }
    }
    Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, writable: true, value: scrollIntoView })
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, writable: true, value: scrollIntoView })
    globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver
    try {
      const { container, rerender } = render(
        <MessageList
          isStreaming
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '先规划一下' },
            { type: 'message', role: 'assistant', content: '正在分析上下文', streaming: true },
          ]}
          scrollResetKey="session-a"
          bottomPaddingPx={180}
        />,
      )
      scroller = getMessageScroller(container)
      const messageScroller = scroller
      const scrollMetrics = mockScrollMetrics(messageScroller)

      messageScroller.scrollTop = scrollMetrics.maxScrollTop()
      fireEvent.scroll(messageScroller)
      messageScroller.scrollTop = scrollMetrics.maxScrollTop() - 220
      fireEvent.scroll(messageScroller)
      const manualScrollTop = messageScroller.scrollTop

      scrollMetrics.setScrollHeight(1600)
      rerender(
        <MessageList
          isStreaming
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '先规划一下' },
            { type: 'message', role: 'assistant', content: '正在分析上下文', streaming: true },
            {
              id: 'plan-question-1',
              role: 'plan_question',
              status: 'running',
              streaming: true,
              content: JSON.stringify({
                questions: [
                  {
                    id: 'scope',
                    type: 'single',
                    question: '要优先确认哪个范围？',
                    options: [
                      { id: 'recommended', label: '推荐范围', recommended: true },
                      { id: 'manual', label: '手动选择' },
                    ],
                  },
                  {
                    id: 'direction',
                    type: 'single',
                    question: '下一题要确认什么方向？',
                    options: [
                      { id: 'mainline', label: '主线推进', recommended: true },
                      { id: 'characters', label: '角色关系' },
                    ],
                  },
                ],
              }),
            },
          ]}
          scrollResetKey="session-a"
          bottomPaddingPx={180}
        />,
      )
      const inputArea = document.createElement('div')
      inputArea.className = 'nova-chat-input-area nova-chat-input-area-floating'
      const composer = document.createElement('div')
      composer.className = 'nova-agent-composer'
      inputArea.appendChild(composer)
      container.appendChild(inputArea)
      const firstPlanRow = container.querySelector('[data-nova-chat-row-key="message-plan-question-1"]')
      expect(firstPlanRow).toBeInstanceOf(HTMLElement)

      await waitFor(() => expect(messageScroller.scrollTop).toBe(260))
      expect(scrollIntoView).not.toHaveBeenCalled()
      expect(messageScroller.scrollTop).not.toBe(220)
      expect(manualScrollTop).toBeLessThan(scrollMetrics.maxScrollTop())
      const firstAnchorTop = messageScroller.scrollTop
      planRowContentBottom = 1080
      planRowHeight = 320

      await user.click(screen.getByRole('button', { name: /确认并下一题/ }))

      expect(screen.getByText('下一题要确认什么方向？')).toBeInTheDocument()
      await waitFor(() => expect(messageScroller.scrollTop).toBe(580))
      const questionStepAnchorTop = messageScroller.scrollTop

      rerender(
        <MessageList
          isStreaming
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '先规划一下' },
            { type: 'message', role: 'assistant', content: '正在分析上下文', streaming: true },
            {
              id: 'plan-question-1',
              role: 'plan_question',
              status: 'running',
              streaming: true,
              thinking_preview: '正在整理剩余关键问题',
              content: JSON.stringify({
                questions: [
                  {
                    id: 'scope',
                    type: 'single',
                    question: '要优先确认哪个范围？',
                    description: '增长后的卡片内容需要继续保持可见。',
                    options: [
                      { id: 'recommended', label: '推荐范围', recommended: true },
                      { id: 'manual', label: '手动选择' },
                    ],
                  },
                  {
                    id: 'direction',
                    type: 'single',
                    question: '下一题要确认什么方向？',
                    description: '第二题切换后也必须保持底部按钮可见。',
                    options: [
                      { id: 'mainline', label: '主线推进', recommended: true },
                      { id: 'characters', label: '角色关系' },
                    ],
                  },
                ],
              }),
            },
          ]}
          scrollResetKey="session-a"
          bottomPaddingPx={180}
        />,
      )

      scrollMetrics.setScrollHeight(1800)
      const planRow = container.querySelector('[data-nova-chat-row-key="message-plan-question-1"]')
      expect(planRow).toBeInstanceOf(HTMLElement)
      planRowContentBottom = 1440
      planRowHeight = 340
      act(() => {
        triggerResizeFor(planRow as HTMLElement)
      })

      await waitFor(() => expect(messageScroller.scrollTop).toBeGreaterThan(questionStepAnchorTop))
      expect(questionStepAnchorTop).toBeGreaterThan(firstAnchorTop)
      expect(scrollIntoView).not.toHaveBeenCalled()
    } finally {
      globalThis.ResizeObserver = originalResizeObserver
      if (originalScrollIntoView) {
        Object.defineProperty(Element.prototype, 'scrollIntoView', originalScrollIntoView)
      } else {
        Reflect.deleteProperty(Element.prototype, 'scrollIntoView')
      }
      if (originalHTMLElementScrollIntoView) {
        Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', originalHTMLElementScrollIntoView)
      } else {
        Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
      }
      rectSpy.mockRestore()
    }
  })

  it('Plan 卡片对齐后后续工具数据仍保持底部跟随', async () => {
    const originalResizeObserver = globalThis.ResizeObserver
    let scroller: HTMLDivElement | null = null
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('nova-chat-canvas')) {
        return buildDomRect({ top: 0, bottom: 720, width: 520, height: 720 })
      }
      if (this.classList.contains('nova-agent-composer')) {
        return buildDomRect({ top: 500, bottom: 600, width: 520, height: 100 })
      }
      if (this.dataset.novaChatRowKey === 'message-plan-question-1') {
        const bottom = 760 - (scroller?.scrollTop || 0)
        return buildDomRect({ top: bottom - 340, bottom, width: 520, height: 340 })
      }
      return buildDomRect({})
    })
    class MockResizeObserver {
      observe = vi.fn()
      unobserve = vi.fn()
      disconnect = vi.fn()
    }
    globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver
    try {
      const { container, rerender } = render(
        <MessageList
          isStreaming
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '先规划一下' },
            {
              id: 'plan-question-1',
              role: 'plan_question',
              status: 'running',
              streaming: true,
              content: JSON.stringify({
                questions: [{
                  id: 'scope',
                  type: 'single',
                  question: '要优先确认哪个范围？',
                  options: [{ id: 'recommended', label: '推荐范围', recommended: true }],
                }],
              }),
            },
          ]}
          scrollResetKey="session-a"
          bottomPaddingPx={180}
        />,
      )
      scroller = getMessageScroller(container)
      const messageScroller = scroller
      const scrollMetrics = mockScrollMetrics(messageScroller, { scrollHeight: 1600, clientHeight: 320, scrollTop: 0 })
      const inputArea = document.createElement('div')
      inputArea.className = 'nova-chat-input-area nova-chat-input-area-floating'
      const composer = document.createElement('div')
      composer.className = 'nova-agent-composer'
      inputArea.appendChild(composer)
      container.appendChild(inputArea)

      await waitFor(() => expect(messageScroller.scrollTop).toBe(260))

      scrollMetrics.setScrollHeight(2100)
      rerender(
        <MessageList
          isStreaming
          activityContent=""
          messages={[
            { type: 'message', role: 'user', content: '先规划一下' },
            {
              id: 'plan-question-1',
              role: 'plan_question',
              status: 'success',
              content: JSON.stringify({
                questions: [{
                  id: 'scope',
                  type: 'single',
                  question: '要优先确认哪个范围？',
                  options: [{ id: 'recommended', label: '推荐范围', recommended: true }],
                }],
              }),
            },
            {
              id: 'tool-call-1',
              role: 'tool_call',
              status: 'running',
              name: 'read_file',
              args: JSON.stringify({ path: 'ideas.md' }),
              content: 'read_file',
            },
          ]}
          scrollResetKey="session-a"
          bottomPaddingPx={180}
        />,
      )

      await waitFor(() => expect(messageScroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
    } finally {
      globalThis.ResizeObserver = originalResizeObserver
      rectSpy.mockRestore()
    }
  })

  it('短列表和接近底部时不显示回到底部按钮', () => {
    const { container } = render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[{ type: 'message', role: 'assistant', content: '短回复' }]}
        scrollResetKey="short-session"
      />,
    )
    const scroller = getMessageScroller(container)
    const scrollMetrics = mockScrollMetrics(scroller, { scrollHeight: 300, clientHeight: 320, scrollTop: 0 })

    fireEvent.scroll(scroller)
    expect(screen.queryByRole('button', { name: '回到底部' })).not.toBeInTheDocument()

    scrollMetrics.setScrollHeight(1200)
    scroller.scrollTop = scrollMetrics.maxScrollTop() - 120
    fireEvent.scroll(scroller)
    expect(screen.queryByRole('button', { name: '回到底部' })).not.toBeInTheDocument()
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
