import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getMessagesPage, getSessions, switchSession, type SessionSummary } from '@/lib/api'
import { useAgentChat } from './useAgentChat'

const chatMock = vi.hoisted(() => ({
  options: null as Record<string, any> | null,
  sendMessage: vi.fn(),
  setMessages: vi.fn(),
  resumeStream: vi.fn(),
  stop: vi.fn(),
  status: 'ready' as 'ready' | 'submitted' | 'streaming',
}))

vi.mock('@ai-sdk/react', () => ({
  useChat: (options: Record<string, any>) => {
    chatMock.options = options
    return {
      messages: [],
      setMessages: chatMock.setMessages,
      sendMessage: chatMock.sendMessage,
      resumeStream: chatMock.resumeStream,
      stop: chatMock.stop,
      status: chatMock.status,
    }
  },
}))

vi.mock('@/lib/api', () => ({
  abortChat: vi.fn(),
  analyzeChatContext: vi.fn(),
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  executeCommand: vi.fn(),
  getActiveChatTask: vi.fn().mockResolvedValue({ active: false }),
  getMessagesPage: vi.fn().mockResolvedValue({ messages: [], nextBefore: '0', hasMore: false, total: 0 }),
  getSessions: vi.fn().mockResolvedValue([]),
  renameSession: vi.fn(),
  switchSession: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

describe('useAgentChat', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    chatMock.options = null
    chatMock.status = 'ready'
  })

  it('stops the old stream and selects the target immediately when switching sessions', async () => {
    chatMock.status = 'streaming'
    let finishSwitch!: (session: SessionSummary) => void
    vi.mocked(switchSession).mockReturnValue(new Promise((resolve) => { finishSwitch = resolve }))
    vi.mocked(getSessions).mockResolvedValue([
      { id: 'target', title: 'just say hello', active: true, message_count: 17, created_at: '2026-07-02T13:26:00Z', updated_at: '2026-07-02T13:26:00Z' },
    ])
    vi.mocked(getMessagesPage).mockResolvedValue({ messages: [], nextBefore: '0', hasMore: false, total: 0 })
    const { result } = renderHook(() => useAgentChat())

    let request!: Promise<void>
    act(() => {
      request = result.current.switchChatSession('target')
    })

    expect(chatMock.stop).toHaveBeenCalledTimes(1)
    expect(result.current.activeSessionId).toBe('target')

    await act(async () => {
      finishSwitch({ id: 'target', title: 'just say hello', active: true, message_count: 17, created_at: '2026-07-02T13:26:00Z', updated_at: '2026-07-02T13:26:00Z' })
      await request
    })
  })

  it('moves the submitted reference snapshot into the user message immediately', async () => {
    let finishRequest!: () => void
    chatMock.sendMessage.mockReturnValue(new Promise<void>((resolve) => { finishRequest = resolve }))
    const onSubmissionStart = vi.fn()
    const { result } = renderHook(() => useAgentChat())

    act(() => {
      result.current.addReference('chapters/ch01.md')
      result.current.addLoreReference('character-1')
      result.current.addStyleScene('battle')
      result.current.addTextSelection({ fileName: 'chapters/ch02.md', startLine: 8, endLine: 10, content: '被引用的正文' })
    })

    let sendResult!: Promise<boolean>
    act(() => {
      sendResult = result.current.send('请统一修改', {
        reviewFeedback: [{ reviewThreadId: 'thread-1', commentIds: ['comment-1'] }],
        reviewFeedbackDisplay: {
          comments: [{ id: 'comment-1', body: '需要增加爽点', review_path: 'setting/progress.md', review_line: 24 }],
        },
        onSubmissionStart,
      })
    })

    expect(onSubmissionStart).toHaveBeenCalledTimes(1)
    expect(result.current.references).toEqual([])
    expect(result.current.loreReferences).toEqual([])
    expect(result.current.styleScenes).toEqual([])
    expect(result.current.textSelections).toEqual([])
    expect(chatMock.sendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'user',
        metadata: expect.objectContaining({
          user_references: expect.arrayContaining([
            expect.objectContaining({ kind: 'file', label: 'chapters/ch01.md' }),
            expect.objectContaining({ kind: 'lore', label: 'character-1' }),
            expect.objectContaining({ kind: 'style', label: 'battle' }),
            expect.objectContaining({ kind: 'selection', label: 'chapters/ch02.md', start_line: 8, end_line: 10 }),
            expect.objectContaining({ kind: 'review_comment', id: 'comment-1', label: 'setting/progress.md', start_line: 24, detail: '需要增加爽点' }),
          ]),
        }),
      }),
      expect.any(Object),
    )

    act(() => result.current.addReference('chapters/next.md'))
    act(() => chatMock.options?.onFinish?.())
    expect(result.current.references).toEqual(['chapters/next.md'])

    await act(async () => finishRequest())
    await expect(sendResult).resolves.toBe(true)
  })

  it('restores consumed composer references when submission fails', async () => {
    chatMock.sendMessage.mockRejectedValue(new Error('offline'))
    const onSubmissionError = vi.fn()
    const { result } = renderHook(() => useAgentChat())
    act(() => result.current.addReference('chapters/ch01.md'))

    await act(async () => {
      expect(await result.current.send('继续', { onSubmissionError })).toBe(false)
    })

    await waitFor(() => expect(result.current.references).toEqual(['chapters/ch01.md']))
    expect(onSubmissionError).toHaveBeenCalledTimes(1)
  })

  it('ignores an older history response after a newer session history has loaded', async () => {
    const older = deferred<Awaited<ReturnType<typeof getMessagesPage>>>()
    const newer = deferred<Awaited<ReturnType<typeof getMessagesPage>>>()
    vi.mocked(getMessagesPage).mockImplementation((sessionId?: string) => sessionId === 'older' ? older.promise : newer.promise)
    const { result } = renderHook(() => useAgentChat())

    let olderRequest!: Promise<void>
    let newerRequest!: Promise<void>
    act(() => {
      olderRequest = result.current.loadHistory('older')
      newerRequest = result.current.loadHistory('newer')
    })

    await act(async () => {
      newer.resolve({ messages: [{ id: 'new-message', role: 'user', parts: [{ type: 'text', text: '新会话' }] }], nextBefore: '0', hasMore: false, total: 1 })
      await newerRequest
    })
    await act(async () => {
      older.resolve({ messages: [{ id: 'old-message', role: 'user', parts: [{ type: 'text', text: '旧会话' }] }], nextBefore: '0', hasMore: false, total: 1 })
      await olderRequest
    })

    expect(chatMock.setMessages).toHaveBeenCalledTimes(1)
    expect(chatMock.setMessages).toHaveBeenLastCalledWith([
      { id: 'new-message', role: 'user', parts: [{ type: 'text', text: '新会话' }] },
    ])
  })

  it('prepends an earlier history page without replacing the current live tail', async () => {
    vi.mocked(getMessagesPage)
      .mockResolvedValueOnce({
        messages: [{ id: 'message-2', role: 'assistant', parts: [{ type: 'text', text: '当前窗口' }] }],
        nextBefore: '1',
        hasMore: true,
        total: 2,
      })
      .mockResolvedValueOnce({
        messages: [{ id: 'message-1', role: 'user', parts: [{ type: 'text', text: '更早消息' }] }],
        nextBefore: '0',
        hasMore: false,
        total: 2,
      })
    const { result } = renderHook(() => useAgentChat())
    await act(async () => result.current.loadHistory('session-a'))
    chatMock.setMessages.mockClear()

    await act(async () => result.current.loadEarlierHistory())

    expect(getMessagesPage).toHaveBeenLastCalledWith('session-a', expect.objectContaining({ before: '1' }))
    const prepend = chatMock.setMessages.mock.calls[0]?.[0] as (messages: unknown[]) => unknown[]
    expect(prepend([
      { id: 'message-2', role: 'assistant', parts: [{ type: 'text', text: '当前窗口' }] },
      { id: 'live-message', role: 'assistant', parts: [{ type: 'text', text: '仍在流式输出', state: 'streaming' }] },
    ])).toEqual([
      { id: 'message-1', role: 'user', parts: [{ type: 'text', text: '更早消息' }] },
      { id: 'message-2', role: 'assistant', parts: [{ type: 'text', text: '当前窗口' }] },
      { id: 'live-message', role: 'assistant', parts: [{ type: 'text', text: '仍在流式输出', state: 'streaming' }] },
    ])
    expect(result.current.hasEarlierMessages).toBe(false)
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
