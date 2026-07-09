import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { describe, expect, it, vi } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { MessageList } from './MessageList'

function renderMessageList(ui: ReactElement) {
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
      {ui}
    </VirtuosoMockContext.Provider>,
  )
}

describe('Agent MessageList', () => {
  it('直接渲染 AgentUIMessage parts 并上报 turn anchor', async () => {
    const handleVisibleTurnAnchorChange = vi.fn()
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        messages={agentTurnMessages()}
        onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
      />,
    )

    expect(screen.getByText('第一轮用户')).toBeInTheDocument()
    expect(screen.getByText('第一轮剧情')).toBeInTheDocument()
    await waitFor(() => expect(handleVisibleTurnAnchorChange).toHaveBeenCalledWith('turn-1'))
  })

  it('按 parts 折叠 assistant 正文前的 trace', async () => {
    renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceBeforeAssistant
        messages={[
          {
            id: 'assistant-1',
            role: 'assistant',
            parts: [
              { type: 'reasoning', id: 'reason-1', text: '内部思考' },
              { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-1', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
              { type: 'text', id: 'text-1', text: '可见正文' },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.queryByText('内部思考')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /思考过程/ }))
    expect(screen.getByText('内部思考')).toBeInTheDocument()
    expect(screen.getByText('可见正文')).toBeInTheDocument()
  })

  it('运行中的 trace 在工具结果返回后保持展开，结束后和工具调用一起折叠', async () => {
    const { rerender } = renderMessageList(
      <MessageList
        isStreaming
        activityContent=""
        collapseTraceBeforeAssistant
        messages={[
          {
            id: 'assistant-running',
            role: 'assistant',
            parts: [
              { type: 'reasoning', id: 'reason-running', text: '正在检查资料', state: 'streaming' },
              { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-running', state: 'input-streaming', input: { path: 'a.md' } },
            ],
          },
        ] as AgentUIMessage[]}
      />,
    )

    expect(screen.getByRole('button', { name: /思考过程.*1 次工具调用/ })).toBeInTheDocument()
    expect(screen.getByText('正在检查资料')).toBeInTheDocument()
    expect(screen.getByText('read_file')).toBeInTheDocument()

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent=""
          collapseTraceBeforeAssistant
          messages={[
            {
              id: 'assistant-running',
              role: 'assistant',
              parts: [
                { type: 'reasoning', id: 'reason-running', text: '正在检查资料' },
                { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-running', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
              ],
            },
          ] as AgentUIMessage[]}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(screen.getByText('正在检查资料')).toBeInTheDocument()
    expect(screen.getByText('read_file')).toBeInTheDocument()

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming={false}
          activityContent=""
          collapseTraceBeforeAssistant
          messages={[
            {
              id: 'assistant-running',
              role: 'assistant',
              parts: [
                { type: 'reasoning', id: 'reason-running', text: '正在检查资料' },
                { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-running', state: 'output-available', input: { path: 'a.md' }, output: 'ok' },
                { type: 'text', id: 'text-running', text: '资料检查完成。' },
              ],
            },
          ] as AgentUIMessage[]}
        />
      </VirtuosoMockContext.Provider>,
    )

    await waitFor(() => expect(screen.queryByText('正在检查资料')).not.toBeInTheDocument())
    expect(screen.getByText('资料检查完成。')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /思考过程.*1 次工具调用/ }))
    expect(screen.getByText('正在检查资料')).toBeInTheDocument()
    expect(screen.getByText('read_file')).toBeInTheDocument()
  })

  it('新一轮 streaming 不会重新展开历史 trace', async () => {
    const { rerender } = renderMessageList(
      <MessageList
        isStreaming={false}
        activityContent=""
        collapseTraceBeforeAssistant
        messages={traceHistoryMessages(false)}
      />,
    )

    expect(screen.queryByText('上一轮思考')).not.toBeInTheDocument()
    expect(screen.getByText('上一轮正文。')).toBeInTheDocument()

    rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 180, itemHeight: 52 }}>
        <MessageList
          isStreaming
          activityContent=""
          collapseTraceBeforeAssistant
          messages={traceHistoryMessages(true)}
        />
      </VirtuosoMockContext.Provider>,
    )

    await waitFor(() => expect(screen.queryByText('上一轮思考')).not.toBeInTheDocument())
    expect(screen.getByText('新的问题')).toBeInTheDocument()
  })
})

function agentTurnMessages(): AgentUIMessage[] {
  return [
    {
      id: 'user-1',
      role: 'user',
      metadata: { turn_id: 'turn-1', navigation_turn_id: 'turn-1' },
      parts: [{ type: 'text', text: '第一轮用户' }],
    },
    {
      id: 'assistant-1',
      role: 'assistant',
      metadata: { turn_id: 'turn-1', navigation_turn_id: 'turn-1' },
      parts: [{ type: 'text', text: '第一轮剧情' }],
    },
    {
      id: 'user-2',
      role: 'user',
      metadata: { turn_id: 'turn-2', navigation_turn_id: 'turn-2' },
      parts: [{ type: 'text', text: '第二轮用户' }],
    },
  ] as AgentUIMessage[]
}

function traceHistoryMessages(withNewUser: boolean): AgentUIMessage[] {
  const messages: AgentUIMessage[] = [
    {
      id: 'assistant-old',
      role: 'assistant',
      parts: [
        { type: 'reasoning', id: 'reason-old', text: '上一轮思考' },
        { type: 'dynamic-tool', toolName: 'read_file', toolCallId: 'tool-old', state: 'output-available', input: { path: 'old.md' }, output: 'ok' },
        { type: 'text', id: 'text-old', text: '上一轮正文。' },
      ],
    },
  ] as AgentUIMessage[]
  if (withNewUser) {
    messages.push({
      id: 'user-new',
      role: 'user',
      parts: [{ type: 'text', text: '新的问题' }],
    } as AgentUIMessage)
  }
  return messages
}
