import { fireEvent, render, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import { MessageList } from './MessageList'
import type { ChatMessage } from '@/lib/api'

function renderMessageList(ui: ReactElement) {
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 156, itemHeight: 52 }}>
      {ui}
    </VirtuosoMockContext.Provider>,
  )
}

function mockScrollMetrics(element: HTMLElement, initial = { scrollHeight: 1200, clientHeight: 156, scrollTop: 0 }) {
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
    setScrollTop: (value: number) => { scrollTop = value },
    scrollTop: () => scrollTop,
  }
}

function getScroller(container: HTMLElement) {
  const scroller = container.querySelector('.nova-chat-canvas')
  expect(scroller).toBeInstanceOf(HTMLDivElement)
  return scroller as HTMLDivElement
}

describe('MessageList turn navigation', () => {
  it('reports the first visible turn anchor from the Virtuoso range', async () => {
    const handleVisibleTurnAnchorChange = vi.fn()
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={turnMessages()}
        onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
      />,
    )

    await waitFor(() => expect(handleVisibleTurnAnchorChange).toHaveBeenCalledWith('turn-1'))
  })

  it('scrolls to the requested turn anchor without staying locked to the bottom', async () => {
    const { container, rerender } = renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={turnMessages()}
        scrollResetKey="story-1"
      />,
    )
    const scroller = getScroller(container)
    const scrollMetrics = mockScrollMetrics(scroller)
    fireEvent.scroll(scroller)

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 156, itemHeight: 52 }}>
        <MessageList
          isStreaming={false}
          activityContent=""
          messages={turnMessages()}
          scrollResetKey="story-1"
          turnScrollRequest={{ anchorId: 'turn-2', requestId: 1 }}
        />
      </VirtuosoMockContext.Provider>,
    )

    await waitFor(() => expect(scrollMetrics.scrollTop()).toBeGreaterThan(0))
  })
})

function turnMessages(): ChatMessage[] {
  return [
    { role: 'user', content: '第一轮用户', turn_id: 'turn-1', navigation_turn_id: 'turn-1' },
    { role: 'assistant', content: '第一轮剧情', turn_id: 'turn-1', navigation_turn_id: 'turn-1' },
    { role: 'user', content: '第二轮用户', turn_id: 'turn-2', navigation_turn_id: 'turn-2' },
    { role: 'assistant', content: '第二轮剧情', turn_id: 'turn-2', navigation_turn_id: 'turn-2' },
  ]
}
