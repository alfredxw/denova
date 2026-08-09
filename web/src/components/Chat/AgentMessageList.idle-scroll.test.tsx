import { render, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { MessageList } from './AgentMessageList'

const virtuosoBoundary = vi.hoisted(() => ({
  followOutput: undefined as unknown,
  totalListHeightChanged: undefined as unknown,
  scrollToIndex: vi.fn(),
}))

vi.mock('react-virtuoso', async () => {
  const React = await import('react')
  return {
    Virtuoso: React.forwardRef<unknown, { className?: string; followOutput?: unknown; totalListHeightChanged?: unknown }>(function VirtuosoBoundary(props, ref) {
      virtuosoBoundary.followOutput = props.followOutput
      virtuosoBoundary.totalListHeightChanged = props.totalListHeightChanged
      React.useImperativeHandle(ref, () => ({ scrollToIndex: virtuosoBoundary.scrollToIndex }))
      return React.createElement('div', { className: props.className })
    }),
  }
})

describe('Agent MessageList bottom following', () => {
  it('disables virtualizer bottom following when output is not streaming', () => {
    render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[]}
        afterContent={<button type="button">展开状态</button>}
        afterContentKey="turn-1:expanded"
      />,
    )

    expect(virtuosoBoundary.followOutput).toBeUndefined()
    expect(virtuosoBoundary.totalListHeightChanged).toBeUndefined()
  })

  it('does not run the message-list bottom scheduler for idle footer changes', () => {
    const renderList = (afterContentKey: string) => (
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[]}
        afterContent={<button type="button">展开状态</button>}
        afterContentKey={afterContentKey}
      />
    )
    const { rerender } = render(renderList('turn-1:preview'))
    virtuosoBoundary.scrollToIndex.mockClear()

    rerender(renderList('turn-1:expanded'))

    expect(virtuosoBoundary.scrollToIndex).not.toHaveBeenCalled()
  })

  it('retries streaming layout when Virtuoso publishes a new total height', () => {
    const renderList = (targetContent?: string) => (
      <MessageList
        isStreaming
        activityContent=""
        collapseTraceGroups
        messages={[
          {
            id: 'streaming-thinking',
            role: 'assistant',
            metadata: targetContent ? { streaming_target_content: targetContent } : undefined,
            parts: [{ type: 'reasoning', text: '正在分析', state: 'streaming' }],
          },
        ] as AgentUIMessage[]}
      />
    )
    const { rerender } = render(renderList())

    rerender(renderList('正在分析下一条线索'))

    expect(virtuosoBoundary.followOutput).toBeUndefined()
    expect(virtuosoBoundary.totalListHeightChanged).toBeTypeOf('function')
  })

  it('keeps the streaming height retry registered while a persistent chat tab is hidden', async () => {
    const renderList = (visible: boolean) => (
      <MessageList
        isStreaming
        visible={visible}
        activityContent=""
        messages={[
          {
            id: 'streaming-answer',
            role: 'assistant',
            parts: [{ type: 'text', text: '持续输出', state: 'streaming' }],
          },
        ] as AgentUIMessage[]}
      />
    )
    const { rerender } = render(renderList(false))

    expect(virtuosoBoundary.followOutput).toBeUndefined()
    expect(virtuosoBoundary.totalListHeightChanged).toBeTypeOf('function')

    rerender(renderList(true))

    await waitFor(() => {
      expect(virtuosoBoundary.totalListHeightChanged).toBeTypeOf('function')
    })
  })
})
