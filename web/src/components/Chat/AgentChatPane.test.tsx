import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatPane } from './AgentChatPane'

const renderCounts = vi.hoisted(() => ({ inputArea: 0, messageList: 0 }))

vi.mock('./InputArea', () => ({
  InputArea: ({ contentClassName }: { contentClassName?: string }) => {
    renderCounts.inputArea += 1
    return <div data-content-class={contentClassName} data-testid="input-area" />
  },
}))

vi.mock('./MessageList', () => ({
  MessageList: ({ contentClassName }: { contentClassName?: string }) => {
    renderCounts.messageList += 1
    return <div data-content-class={contentClassName} data-testid="message-list" />
  },
}))

describe('AgentChatPane', () => {
  beforeEach(() => {
    renderCounts.inputArea = 0
    renderCounts.messageList = 0
  })

  it('does not rerender the message timeline when only composer context changes', () => {
    const messages: [] = []
    const messageListProps = { messages, isStreaming: false, activityContent: '' }
    const { rerender } = render(
      <AgentChatPane
        messageListProps={messageListProps}
        inputAreaProps={{ disabled: false, generationActive: false, onSend: vi.fn() }}
      />,
    )

    expect(renderCounts.messageList).toBe(1)
    expect(renderCounts.inputArea).toBe(1)

    rerender(
      <AgentChatPane
        messageListProps={{ ...messageListProps }}
        inputAreaProps={{ disabled: false, generationActive: false, onSend: vi.fn() }}
      />,
    )

    expect(renderCounts.messageList).toBe(1)
    expect(renderCounts.inputArea).toBe(2)
  })

  it('shares one adaptive content boundary between the timeline and composer', () => {
    render(
      <AgentChatPane
        contentClassName="mx-auto w-full max-w-[56rem]"
        messageListProps={{ messages: [], isStreaming: false, activityContent: '' }}
        inputAreaProps={{ disabled: false, generationActive: false, onSend: vi.fn() }}
      />,
    )

    expect(screen.getByTestId('message-list')).toHaveAttribute('data-content-class', 'mx-auto w-full max-w-[56rem]')
    expect(screen.getByTestId('input-area')).toHaveAttribute('data-content-class', 'mx-auto w-full max-w-[56rem]')
    expect(screen.getByTestId('message-list').parentElement).not.toHaveClass('max-w-[56rem]')
  })
})
