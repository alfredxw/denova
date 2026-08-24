import { fireEvent, render, screen } from '@testing-library/react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { createAgentReasoningMessage } from '@/lib/agent-ui-message'
import { AgentSubAgentSessionPanel } from './AgentSubAgentSessionPanel'

describe('AgentSubAgentSessionPanel', () => {
  it('旧历史按逻辑委派展示所有逐轮输出', () => {
    const metadata = {
      run_id: 'run-review',
      agent_name: 'general-purpose',
      root_agent_name: 'DenovaAgent',
      run_path: ['DenovaAgent', 'general-purpose'],
      subagent: true as const,
    }
    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <AgentSubAgentSessionPanel
          messages={[
            {
              id: 'review-loop-1', role: 'assistant',
              metadata: { ...metadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
              parts: [{ type: 'text', text: '先读取待审章节。' }],
            },
            createAgentReasoningMessage({
              id: 'review-thinking',
              text: '核对章节约束。',
              metadata: { ...metadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
            }),
            {
              id: 'review-tool', role: 'assistant',
              metadata: { ...metadata, subagent_session_id: 'run-review-subagent-01-general-purpose' },
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
              metadata: { ...metadata, subagent_session_id: 'run-review-subagent-03-general-purpose' },
              parts: [{ type: 'text', text: '继续核对上下文。' }],
            },
            {
              id: 'review-loop-3', role: 'assistant',
              metadata: { ...metadata, subagent_session_id: 'run-review-subagent-05-general-purpose' },
              parts: [{ type: 'text', text: '审稿完成。' }],
            },
          ] as AgentUIMessage[]}
          sessionKey="run-review-subagent-01-general-purpose"
          onClose={vi.fn()}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(screen.getByText('先读取待审章节。')).toBeInTheDocument()
    expect(screen.getByText('继续核对上下文。')).toBeInTheDocument()
    expect(screen.getByText('审稿完成。')).toBeInTheDocument()
    expect(screen.getByText('general-purpose 子会话')).toBeInTheDocument()
    expect(screen.queryByText('general-purpose')).not.toBeInTheDocument()

    const rows = document.querySelectorAll('[data-nova-chat-item="subagent-message"]')
    expect(rows[0]).toHaveClass('pb-2')
    expect(rows[rows.length - 1]).toHaveClass('pb-0')
  })

  it('lets the active sub-agent row grow into its reserved response runway', () => {
    let rowHeight = 40
    let scrollHeight = 900
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.hasAttribute('data-virtuoso-scroller')) return { top: 0, bottom: 500, height: 500 } as DOMRect
      if (this.getAttribute('data-nova-chat-item') === 'subagent-message') {
        const scroller = this.closest<HTMLElement>('[data-virtuoso-scroller]')
        return {
          bottom: 60 + rowHeight - ((scroller?.scrollTop || 0) - 400),
          height: rowHeight,
        } as DOMRect
      }
      return originalRect.call(this)
    })
    const renderPanel = (content: string) => (
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <AgentSubAgentSessionPanel
          messages={[{
            id: 'subagent-message',
            role: 'assistant',
            metadata: {
              subagent: true,
              subagent_session_id: 'research-session',
              agent_name: 'Researcher',
            },
            parts: [{ type: 'text', text: content, state: 'streaming' }],
          }] as AgentUIMessage[]}
          sessionKey="research-session"
          onClose={vi.fn()}
        />
      </VirtuosoMockContext.Provider>
    )

    try {
      const { container, rerender } = render(renderPanel('第一行'))
      const scroller = container.querySelector<HTMLElement>('[data-testid="virtuoso-scroller"]')
      if (!scroller) throw new Error('Expected sub-agent message scroller')
      Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => scrollHeight })
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 500 })
      scroller.scrollTop = 400
      fireEvent.scroll(scroller)

      rowHeight = 70
      scrollHeight = 930
      rerender(renderPanel('第一行\n第二行'))

      expect(scroller.scrollTop).toBe(400)
    } finally {
      rectSpy.mockRestore()
    }
  })
})
