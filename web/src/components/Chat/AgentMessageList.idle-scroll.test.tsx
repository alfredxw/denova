import { render, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { MessageList } from './AgentMessageList'

const virtuosoBoundary = vi.hoisted(() => ({
  alignToBottom: undefined as unknown,
  followOutput: undefined as unknown,
  totalListHeightChanged: undefined as unknown,
  scrollToIndex: vi.fn(),
}))

vi.mock('react-virtuoso', async () => {
  const React = await import('react')
  return {
    Virtuoso: React.forwardRef<unknown, { alignToBottom?: boolean; className?: string; followOutput?: unknown; totalListHeightChanged?: unknown }>(function VirtuosoBoundary(props, ref) {
      virtuosoBoundary.alignToBottom = props.alignToBottom
      virtuosoBoundary.followOutput = props.followOutput
      virtuosoBoundary.totalListHeightChanged = props.totalListHeightChanged
      React.useImperativeHandle(ref, () => ({ scrollToIndex: virtuosoBoundary.scrollToIndex }))
      return React.createElement('div', { className: props.className })
    }),
  }
})

describe('Agent MessageList bottom following', () => {
  it('keeps an idle height observer without enabling virtualizer bottom following', () => {
    render(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={[]}
        afterContent={<button type="button">展开状态</button>}
        afterContentKey="turn-1:expanded"
      />,
    )

    expect(virtuosoBoundary.alignToBottom).toBe(false)
    expect(virtuosoBoundary.followOutput).toBeUndefined()
    expect(virtuosoBoundary.totalListHeightChanged).toBeTypeOf('function')
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

  it('keeps the layout retry active while execution continues between transport streams', () => {
    render(
      <MessageList
        isStreaming={false}
        isExecutionActive
        activityContent=""
        collapseTraceGroups
        messages={[
          {
            id: 'running-tool',
            role: 'assistant',
            parts: [{ type: 'reasoning', text: '等待工具结果', state: 'streaming' }],
          },
        ] as AgentUIMessage[]}
      />,
    )

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
