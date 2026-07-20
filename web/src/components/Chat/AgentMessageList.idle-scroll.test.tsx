import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { MessageList } from './AgentMessageList'

const virtuosoBoundary = vi.hoisted(() => ({
  followOutput: undefined as unknown,
  scrollToIndex: vi.fn(),
}))

vi.mock('react-virtuoso', async () => {
  const React = await import('react')
  return {
    Virtuoso: React.forwardRef<unknown, { className?: string; followOutput?: unknown }>(function VirtuosoBoundary(props, ref) {
      virtuosoBoundary.followOutput = props.followOutput
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

    expect(virtuosoBoundary.followOutput).toBe(false)
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

  it('follows the reserved target height before streaming thinking is revealed', () => {
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
    virtuosoBoundary.scrollToIndex.mockClear()

    rerender(renderList('正在分析下一条线索'))

    expect(virtuosoBoundary.scrollToIndex).toHaveBeenCalledWith({ index: 'LAST', align: 'end', behavior: 'auto' })
  })
})
