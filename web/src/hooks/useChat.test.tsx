import { useEffect } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessage, SSEEvent } from '@/lib/api'
import { useChat } from './useChat'

const apiMocks = vi.hoisted(() => ({
  abortChat: vi.fn(),
  analyzeChatContext: vi.fn(),
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  executeCommand: vi.fn(),
  getActiveChatTask: vi.fn(),
  getMessages: vi.fn(),
  getSessions: vi.fn(),
  renameSession: vi.fn(),
  sendMessage: vi.fn(),
  streamActiveChat: vi.fn(),
  switchSession: vi.fn(),
}))

vi.mock('@/lib/api', () => apiMocks)
vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(async () => ({ effective: {} })),
}))

describe('useChat', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getSessions.mockResolvedValue([])
    apiMocks.getMessages.mockResolvedValue([])
    apiMocks.getActiveChatTask.mockResolvedValue({ active: false })
    apiMocks.sendMessage.mockResolvedValue(closedSSEStream())
  })

  it('提交 Plan 问题答案时不追加可见用户气泡', async () => {
    let chat: ReturnType<typeof useChat> | undefined
    render(<ChatHarness onChange={(value) => { chat = value }} />)
    await waitFor(() => expect(chat).toBeDefined())

    await act(async () => {
      chat?.submitPlanQuestion(
        { role: 'plan_question', id: 'plan-question-1', content: '{"questions":[]}' },
        '<plan_question_answers>{"answers":[]}</plan_question_answers>',
        '问题回答预览',
      )
    })

    await waitFor(() => expect(apiMocks.sendMessage).toHaveBeenCalledTimes(1))
    expect(apiMocks.sendMessage.mock.calls[0][0]).toContain('<plan_question_answers>')
    expect(readMessages().some((message) => message.role === 'user')).toBe(false)
  })

  it('加载历史时过滤 Plan 问题答案内部协议消息', async () => {
    let chat: ReturnType<typeof useChat> | undefined
    apiMocks.getMessages.mockResolvedValue([
      { role: 'user', content: '<plan_question_answers>{"answers":[]}</plan_question_answers>' },
      { role: 'assistant', content: '可见正文' },
    ])
    render(<ChatHarness onChange={(value) => { chat = value }} />)
    await waitFor(() => expect(chat).toBeDefined())

    await act(async () => {
      await chat?.loadHistory('session-1')
    })

    expect(readMessages()).toEqual([{ role: 'assistant', content: '可见正文' }])
  })

  it('加载历史时过滤误持久化的 Plan 协议工具卡', async () => {
    let chat: ReturnType<typeof useChat> | undefined
    apiMocks.getMessages.mockResolvedValue([
      { role: 'tool_call', name: 'plan_questions', content: 'plan_questions', args: '{"questions":[]}' },
      { role: 'tool_result', name: 'plan_questions', content: 'ignored' },
      { role: 'assistant', content: '可见正文' },
    ])
    render(<ChatHarness onChange={(value) => { chat = value }} />)
    await waitFor(() => expect(chat).toBeDefined())

    await act(async () => {
      await chat?.loadHistory('session-1')
    })

    expect(readMessages()).toEqual([{ role: 'assistant', content: '可见正文' }])
  })

  it('确认最终计划时不追加额外用户气泡', async () => {
    let chat: ReturnType<typeof useChat> | undefined
    const planMessage: ChatMessage = { id: 'plan-1', role: 'proposed_plan', content: '# Summary\n\n- 执行计划' }
    apiMocks.getMessages.mockResolvedValue([
      { role: 'user', content: '原始需求' },
      planMessage,
    ])
    render(<ChatHarness onChange={(value) => { chat = value }} />)
    await waitFor(() => expect(chat).toBeDefined())
    await act(async () => {
      await chat?.loadHistory('session-1')
    })

    await act(async () => {
      chat?.approveProposedPlan(planMessage)
    })

    await waitFor(() => expect(apiMocks.sendMessage).toHaveBeenCalledTimes(1))
    expect(apiMocks.sendMessage.mock.calls[0][0]).toContain('已批准计划')
    expect(readMessages().filter((message) => message.role === 'user')).toEqual([{ role: 'user', content: '原始需求' }])
  })
})

function ChatHarness({ onChange }: { onChange: (value: ReturnType<typeof useChat>) => void }) {
  const chat = useChat()
  useEffect(() => onChange(chat), [chat, onChange])
  return <pre data-testid="messages">{JSON.stringify(chat.messages)}</pre>
}

function closedSSEStream() {
  return new ReadableStream<SSEEvent>({
    start(controller) {
      controller.close()
    },
  })
}

function readMessages() {
  return JSON.parse(screen.getByTestId('messages').textContent || '[]') as ChatMessage[]
}
