import { fireEvent, render } from '@testing-library/react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { AgentSubAgentSessionPanel } from './AgentSubAgentSessionPanel'

describe('AgentSubAgentSessionPanel', () => {
  it('keeps the active sub-agent row bottom fixed during its layout commit', () => {
    let rowHeight = 40
    let scrollHeight = 500
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect(this: HTMLElement) {
      if (this.getAttribute('data-nova-chat-item') === 'subagent-message') {
        return { height: rowHeight } as DOMRect
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
      Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 100 })
      scroller.scrollTop = 400
      fireEvent.scroll(scroller)

      rowHeight = 70
      scrollHeight = 530
      rerender(renderPanel('第一行\n第二行'))

      expect(scroller.scrollTop).toBe(430)
    } finally {
      rectSpy.mockRestore()
    }
  })
})
