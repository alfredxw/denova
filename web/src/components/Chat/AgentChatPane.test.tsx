import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatPane } from './AgentChatPane'

const renderCounts = vi.hoisted(() => ({ inputArea: 0, messageList: 0 }))

vi.mock('./InputArea', () => ({
  InputArea: () => {
    renderCounts.inputArea += 1
    return <div data-testid="input-area" />
  },
}))

vi.mock('./MessageList', () => ({
  MessageList: () => {
    renderCounts.messageList += 1
    return <div data-testid="message-list" />
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
        inputAreaProps={{ disabled: false, onSend: vi.fn() }}
      />,
    )

    expect(renderCounts.messageList).toBe(1)
    expect(renderCounts.inputArea).toBe(1)

    rerender(
      <AgentChatPane
        messageListProps={{ ...messageListProps }}
        inputAreaProps={{ disabled: false, onSend: vi.fn() }}
      />,
    )

    expect(renderCounts.messageList).toBe(1)
    expect(renderCounts.inputArea).toBe(2)
  })
})
