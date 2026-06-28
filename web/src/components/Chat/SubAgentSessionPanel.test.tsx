import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from '@/lib/api'
import { SubAgentSessionPanel } from './SubAgentSessionPanel'

function renderPanel(messages: ChatMessage[], onClose = vi.fn()) {
  return {
    onClose,
    ...render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 420, itemHeight: 52 }}>
        <SubAgentSessionPanel messages={messages} sessionKey="run-1-subagent-01-researcher" onClose={onClose} />
      </VirtuosoMockContext.Provider>,
    ),
  }
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
    maxScrollTop: () => Math.max(0, scrollHeight - clientHeight),
  }
}

describe('SubAgentSessionPanel', () => {
  it('虚拟化长子会话输出并保留关闭入口', async () => {
    const user = userEvent.setup()
    const messages: ChatMessage[] = Array.from({ length: 500 }, (_, index) => ({
      id: `subagent-${index}`,
      role: 'assistant',
      content: `SubAgent 长输出 ${index}`,
      agent_name: 'researcher',
      subagent: true,
      subagent_session_id: 'run-1-subagent-01-researcher',
      streaming: index === 499,
    }))
    messages.push({
      id: 'other-session',
      role: 'assistant',
      content: '其他子会话输出',
      agent_name: 'writer',
      subagent: true,
      subagent_session_id: 'run-1-subagent-02-writer',
    })

    const { container, onClose } = renderPanel(messages)

    expect(screen.getByText('researcher 子会话')).toBeInTheDocument()
    expect(screen.getByText('正在流式输出')).toBeInTheDocument()
    await waitFor(() => {
      expect(container.querySelectorAll('[data-nova-chat-item="subagent-message"]').length).toBeGreaterThan(0)
    })
    expect(container.querySelectorAll('[data-nova-chat-item="subagent-message"]').length).toBeLessThan(120)
    expect(screen.queryByText('其他子会话输出')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '关闭 SubAgent 详情' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('子会话详情离底部较远时可以一键回到底部', async () => {
    const messages: ChatMessage[] = Array.from({ length: 80 }, (_, index) => ({
      id: `subagent-${index}`,
      role: 'assistant',
      content: `SubAgent 输出 ${index}`,
      agent_name: 'researcher',
      subagent: true,
      subagent_session_id: 'run-1-subagent-01-researcher',
    }))
    const { container } = renderPanel(messages)
    const scroller = container.querySelector('[data-testid="virtuoso-scroller"]') as HTMLElement
    expect(scroller).toBeInstanceOf(HTMLElement)
    const scrollMetrics = mockScrollMetrics(scroller)

    scroller.scrollTop = scrollMetrics.maxScrollTop()
    fireEvent.scroll(scroller)
    expect(screen.queryByRole('button', { name: '回到底部' })).not.toBeInTheDocument()

    scroller.scrollTop = scrollMetrics.maxScrollTop() - 220
    fireEvent.scroll(scroller)
    const scrollButton = await screen.findByRole('button', { name: '回到底部' })
    expect(scrollButton).toHaveStyle({ right: '16px' })

    fireEvent.click(scrollButton)
    await waitFor(() => expect(scroller.scrollTop).toBe(scrollMetrics.maxScrollTop()))
  })
})
