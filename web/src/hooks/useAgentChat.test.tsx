import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createAgentCommandID,
  getActiveChatTask,
  getMessagesPage,
  getSessions,
  recoverChatAgentRuntime,
  submitChatCommand,
  submitQueuedChatCommand,
  switchSession,
  type SessionSummary,
} from '@/lib/api'
import { AgentChatTransport } from '@/lib/agent-ui'
import { APIError } from '@/lib/api-client'
import { STREAMING_RENDER_INTERVAL_MS } from '@/lib/streaming/raf-update-batcher'
import { writingAgentChatClient } from './agent-chat-client'
import { useAgentChat } from './useAgentChat'

const chatMock = vi.hoisted(() => ({
  options: null as Record<string, any> | null,
  messages: [] as Array<Record<string, unknown>>,
  sendMessage: vi.fn(),
  setMessages: vi.fn(),
  resumeStream: vi.fn(),
  stop: vi.fn(),
  status: 'ready' as 'ready' | 'submitted' | 'streaming' | 'error',
  error: undefined as Error | undefined,
}))

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('@ai-sdk/react', () => ({
  useChat: (options: Record<string, any>) => {
    chatMock.options = options
    return {
      messages: chatMock.messages,
      setMessages: chatMock.setMessages,
      sendMessage: chatMock.sendMessage,
      resumeStream: chatMock.resumeStream,
      stop: chatMock.stop,
      status: chatMock.status,
      error: chatMock.error,
    }
  },
}))

vi.mock('sonner', () => ({ toast: toastMock }))

vi.mock('@/lib/api', () => ({
  analyzeChatContext: vi.fn(),
  answerSessionAsk: vi.fn(),
  cancelSessionAsk: vi.fn(),
  createAgentCommandID: vi.fn(() => 'command-test'),
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  executeCommand: vi.fn(),
  getActiveChatTask: vi.fn().mockResolvedValue({ active: false }),
  getMessagesPage: vi.fn().mockResolvedValue({
    messages: [],
    nextBefore: '0',
    hasMore: false,
    total: 0,
  }),
  getSessions: vi.fn().mockResolvedValue([]),
  renameSession: vi.fn(),
  recoverChatAgentRuntime: vi.fn(),
  removeChatContextCompaction: vi.fn(),
  submitChatCommand: vi.fn(),
  submitQueuedChatCommand: vi.fn(),
  switchSession: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

describe('useAgentChat', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(createAgentCommandID).mockReset().mockReturnValue('command-test')
    vi.mocked(getActiveChatTask).mockReset().mockResolvedValue({ active: false })
    vi.mocked(getMessagesPage).mockReset().mockResolvedValue({
      messages: [],
      nextBefore: '0',
      hasMore: false,
      total: 0,
    })
    vi.mocked(getSessions).mockReset().mockResolvedValue([])
    vi.mocked(recoverChatAgentRuntime).mockReset()
    vi.mocked(submitChatCommand).mockReset()
    vi.mocked(submitQueuedChatCommand).mockReset()
    chatMock.sendMessage.mockReset()
    chatMock.resumeStream.mockReset().mockResolvedValue(undefined)
    chatMock.options = null
    chatMock.messages = []
    chatMock.status = 'ready'
    chatMock.error = undefined
    writingAgentChatClient.fixedSessionId = 'session-test'
    toastMock.error.mockReset()
    toastMock.info.mockReset()
  })

  it('uses the AI SDK as the single throttled message state', () => {
    renderHook(() => useAgentChat())

    expect(chatMock.options).toMatchObject({
      throttle: STREAMING_RENDER_INTERVAL_MS,
      transport: expect.any(AgentChatTransport),
    })
  })

  it('keeps the confirmed session selected and blocks sends while a switch is pending', async () => {
    writingAgentChatClient.fixedSessionId = ''
    chatMock.status = 'streaming'
    vi.mocked(getSessions).mockResolvedValueOnce([
      {
        id: 'current',
        title: 'current chat',
        active: true,
        message_count: 4,
        created_at: '2026-07-02T13:20:00Z',
        updated_at: '2026-07-02T13:20:00Z',
      },
    ])
    const { result } = renderHook(() => useAgentChat())
    await act(async () => {
      await result.current.loadSessions()
    })

    let finishSwitch!: (session: SessionSummary) => void
    vi.mocked(switchSession).mockReturnValue(
      new Promise((resolve) => {
        finishSwitch = resolve
      }),
    )
    vi.mocked(getSessions).mockResolvedValue([
      {
        id: 'target',
        title: 'just say hello',
        active: true,
        message_count: 17,
        created_at: '2026-07-02T13:26:00Z',
        updated_at: '2026-07-02T13:26:00Z',
      },
    ])
    vi.mocked(getMessagesPage).mockResolvedValue({
      messages: [],
      nextBefore: '0',
      hasMore: false,
      total: 0,
    })

    let request!: Promise<void>
    act(() => {
      request = result.current.switchChatSession('target')
    })

    expect(chatMock.stop).toHaveBeenCalledTimes(1)
    expect(result.current.activeSessionId).toBe('current')
    expect(result.current.sessionTransitionPending).toBe(true)
    await act(async () => {
      expect(await result.current.send('must not leak')).toBe(false)
    })
    expect(chatMock.sendMessage).not.toHaveBeenCalled()

    await act(async () => {
      finishSwitch({
        id: 'target',
        title: 'just say hello',
        active: true,
        message_count: 17,
        created_at: '2026-07-02T13:26:00Z',
        updated_at: '2026-07-02T13:26:00Z',
      })
      await request
    })
    expect(result.current.activeSessionId).toBe('target')
    expect(result.current.sessionTransitionPending).toBe(false)
  })

  it('binds a new turn to the exact confirmed session', async () => {
    writingAgentChatClient.fixedSessionId = ''
    vi.mocked(getSessions).mockResolvedValue([
      {
        id: 'session-exact',
        title: 'exact chat',
        active: true,
        message_count: 2,
        created_at: '2026-07-02T13:20:00Z',
        updated_at: '2026-07-02T13:20:00Z',
      },
    ])
    chatMock.sendMessage.mockResolvedValue(undefined)
    const { result } = renderHook(() => useAgentChat())
    await act(async () => {
      await result.current.loadSessions()
    })
    await act(async () => {
      expect(await result.current.send('continue exactly here')).toBe(true)
    })

    expect(chatMock.sendMessage.mock.calls.at(-1)?.[1]?.body).toMatchObject({
      session_id: 'session-exact',
      message: 'continue exactly here',
    })
  })

  it('binds an empty resume action to the exact projected interruption', async () => {
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      pending_interruption_id: 'interruption-7',
    })
    chatMock.sendMessage.mockResolvedValue(undefined)
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    await act(async () => {
      expect(await result.current.send('')).toBe(true)
    })

    expect(chatMock.sendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'user',
        parts: [{ type: 'text', text: '继续生成' }],
      }),
      expect.objectContaining({
        body: expect.objectContaining({
          message: 'Continue.',
          resume_interruption_id: 'interruption-7',
        }),
      }),
    )
  })

  it('submits host-enriched input with a separate creator-facing projection', async () => {
    writingAgentChatClient.fixedSessionId = ''
    vi.mocked(getSessions).mockResolvedValue([
      {
        id: 'session-configuration',
        title: 'configuration',
        active: true,
        message_count: 2,
        created_at: '2026-08-30T00:00:00Z',
        updated_at: '2026-08-30T00:00:00Z',
      },
    ])
    chatMock.sendMessage.mockResolvedValue(undefined)
    const { result } = renderHook(() => useAgentChat())
    await act(async () => {
      await result.current.loadSessions()
      expect(await result.current.send('/configuration\n\nUpdate the preset.', {
        displayMessage: 'Update the preset.',
      })).toBe(true)
    })

    expect(chatMock.sendMessage.mock.calls.at(-1)?.[1]?.body).toMatchObject({
      session_id: 'session-configuration',
      message: '/configuration\n\nUpdate the preset.',
      display_message: 'Update the preset.',
    })
  })

  it('refreshes session summaries after a completed turn', async () => {
    writingAgentChatClient.fixedSessionId = ''
    const stale = {
      id: 'session-current',
      title: 'current chat',
      active: true,
      message_count: 4,
      created_at: '2026-08-13T01:13:00Z',
      updated_at: '2026-08-13T01:13:00Z',
    }
    const fresh = {
      ...stale,
      message_count: 12,
      updated_at: '2026-08-13T01:17:00Z',
    }
    vi.mocked(getSessions).mockResolvedValueOnce([stale]).mockResolvedValue([fresh])

    const { result } = renderHook(() => useAgentChat())
    await act(async () => {
      await result.current.loadSessions()
    })
    expect(result.current.sessions[0]?.message_count).toBe(4)

    act(() => chatMock.options?.onFinish?.())

    await waitFor(() => expect(result.current.sessions[0]?.message_count).toBe(12))
  })

  it('resolves an exact Session before sending when startup selection is still empty', async () => {
    writingAgentChatClient.fixedSessionId = ''
    vi.mocked(getSessions).mockResolvedValue([
      {
        id: 'session-late-binding',
        title: 'late binding',
        active: true,
        message_count: 0,
        created_at: '2026-08-13T00:00:00Z',
        updated_at: '2026-08-13T00:00:00Z',
      },
    ])
    chatMock.sendMessage.mockResolvedValue(undefined)
    const { result } = renderHook(() => useAgentChat())

    await act(async () => {
      expect(await result.current.send('start after startup')).toBe(true)
    })

    expect(chatMock.sendMessage.mock.calls.at(-1)?.[1]?.body).toMatchObject({
      session_id: 'session-late-binding',
      message: 'start after startup',
    })
  })

  it('does not classify an unbound transport failure as durable runtime recovery', async () => {
    writingAgentChatClient.fixedSessionId = ''
    const { result, rerender } = renderHook(() => useAgentChat())

    act(() => {
      chatMock.status = 'submitted'
      rerender()
    })
    act(() => {
      chatMock.status = 'error'
      chatMock.error = new Error('session_id is required to bind the Writing session')
      rerender()
    })

    await waitFor(() => expect(result.current.isStreaming).toBe(false))
    expect(result.current.activityContent).toBe('')
    expect(getActiveChatTask).not.toHaveBeenCalled()
  })

  it('does not recover a submitted turn that the server definitely rejected', async () => {
    const { result, rerender } = renderHook(() => useAgentChat())

    act(() => {
      chatMock.status = 'streaming'
      rerender()
    })
    await waitFor(() => expect(getActiveChatTask).toHaveBeenCalledTimes(1))
    vi.mocked(getActiveChatTask).mockClear()

    act(() => {
      chatMock.status = 'submitted'
      rerender()
    })
    act(() => {
      chatMock.status = 'error'
      chatMock.error = new Error('{"code":"agent_runtime.context_changed","message":"The agent context changed; retry the request"}')
      rerender()
    })

    await waitFor(() => expect(result.current.isStreaming).toBe(false))
    expect(result.current.activityContent).toBe('')
    expect(getActiveChatTask).not.toHaveBeenCalled()
  })

  it('does not let an interrupted Session inspection block or overwrite the confirmed target', async () => {
    writingAgentChatClient.fixedSessionId = ''
    const staleInspection = deferred<{ active: true; active_operation_id: string }>()
    const current = {
      id: 'session-a', title: 'A', active: true, message_count: 2,
      created_at: '2026-07-02T13:20:00Z', updated_at: '2026-07-02T13:20:00Z',
    }
    const target = { ...current, id: 'session-b', title: 'B' }
    vi.mocked(getSessions).mockResolvedValueOnce([current]).mockResolvedValue([target])
    vi.mocked(switchSession).mockResolvedValue(target)
    vi.mocked(getActiveChatTask).mockImplementation((sessionID: string) => (
      sessionID === 'session-a' ? staleInspection.promise : Promise.resolve({ active: false })
    ))
    const { result } = renderHook(() => useAgentChat())
    await act(async () => {
      await result.current.loadSessions()
    })

    let oldInspection!: Promise<void>
    act(() => {
      oldInspection = result.current.resumeActiveChat('session-a')
    })
    await waitFor(() => expect(getActiveChatTask).toHaveBeenCalledWith('session-a'))

    await act(async () => {
      await result.current.switchChatSession('session-b')
    })
    expect(result.current.activeSessionId).toBe('session-b')
    expect(result.current.sessionTransitionPending).toBe(false)
    expect(getActiveChatTask).toHaveBeenCalledWith('session-b')

    await act(async () => {
      staleInspection.resolve({ active: true, active_operation_id: 'operation-from-a' })
      await oldInspection
    })
    expect(result.current.activeSessionId).toBe('session-b')
    expect(result.current.runtimeProjection).toEqual({ active: false })
  })

  it('moves the submitted reference snapshot into the user message immediately', async () => {
    let finishRequest!: () => void
    chatMock.sendMessage.mockReturnValue(
      new Promise<void>((resolve) => {
        finishRequest = resolve
      }),
    )
    const onSubmissionStart = vi.fn()
    const { result } = renderHook(() => useAgentChat())

    act(() => {
      result.current.addReference('chapters/ch01.md')
      result.current.addLoreReference('character-1')
      result.current.addStyleScene('battle')
      result.current.addTextSelection({
        fileName: 'chapters/ch02.md',
        startLine: 8,
        endLine: 10,
        content: '被引用的正文',
      })
    })

    let sendResult!: Promise<boolean>
    act(() => {
      sendResult = result.current.send('请统一修改', {
        reviewFeedback: [{ reviewThreadId: 'thread-1', commentIds: ['comment-1'] }],
        reviewFeedbackDisplay: {
          comments: [
            {
              id: 'comment-1',
              body: '需要增加爽点',
              review_path: 'setting/progress.md',
              review_line: 24,
            },
          ],
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
            expect.objectContaining({
              kind: 'file',
              label: 'chapters/ch01.md',
            }),
            expect.objectContaining({ kind: 'lore', label: 'character-1' }),
            expect.objectContaining({ kind: 'style', label: 'battle' }),
            expect.objectContaining({
              kind: 'selection',
              label: 'chapters/ch02.md',
              start_line: 8,
              end_line: 10,
            }),
            expect.objectContaining({
              kind: 'review_comment',
              id: 'comment-1',
              label: 'setting/progress.md',
              start_line: 24,
              detail: '需要增加爽点',
            }),
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

  it('submits follow-up input to the exact active operation without opening a second stream', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      phase: 'running',
      active_operation_id: 'operation-7',
      active_cycle: 1,
    })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'command-7',
      operation_id: 'operation-7',
      cursor: 8,
    })
    const onSubmissionStart = vi.fn()
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-7'))
    act(() => result.current.addReference('chapters/ch01.md'))

    await act(async () => {
      expect(
        await result.current.send('再补一个反转', { onSubmissionStart }),
      ).toBe(true)
    })

    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      expect.any(String),
      'operation-7',
      'session-test',
      expect.objectContaining({
        message: '再补一个反转',
        references: ['chapters/ch01.md'],
      }),
    )
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
    expect(onSubmissionStart).toHaveBeenCalledTimes(1)
    expect(result.current.references).toEqual([])
  })

  it('queues another follow-up when earlier instructions are already waiting', async () => {
    chatMock.status = 'streaming'
    const queued = [
      {
        command_id: 'queued-1',
        operation_id: 'operation-queue',
        delivery: 'follow_up' as const,
        message: 'First queued instruction',
      },
      {
        command_id: 'queued-2',
        operation_id: 'operation-queue',
        delivery: 'follow_up' as const,
        message: 'Second queued instruction',
      },
    ]
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      phase: 'running',
      active_operation_id: 'operation-queue',
      queue: queued,
    })
    vi.mocked(createAgentCommandID).mockReturnValue('queued-3')
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'queued-3',
      operation_id: 'operation-queue',
      cursor: 12,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.queue).toEqual(queued))

    await act(async () => {
      expect(await result.current.send('Third queued instruction')).toBe(true)
    })

    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      'queued-3',
      'operation-queue',
      'session-test',
      expect.objectContaining({ message: 'Third queued instruction' }),
    )
    expect(result.current.runtimeProjection?.queue?.map((item) => item.command_id)).toEqual([
      'queued-1',
      'queued-2',
      'queued-3',
    ])
  })

  it('shows a rejected queued instruction as a toast without appending an error message', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      phase: 'running',
      active_operation_id: 'operation-toast',
      queue: [],
    })
    vi.mocked(submitChatCommand).mockRejectedValue(new Error('queue unavailable'))
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-toast'))
    chatMock.setMessages.mockClear()

    await act(async () => {
      expect(await result.current.send('Queue this instruction')).toBe(false)
    })

    expect(toastMock.error).toHaveBeenCalledWith('请求失败: queue unavailable')
    expect(chatMock.setMessages).not.toHaveBeenCalled()
  })

  it('routes streamed Agent errors to a toast and omits them from visible messages', () => {
    const errorPart = {
      type: 'data-agent-error',
      id: 'error-part-1',
      data: { content: 'provider exploded' },
    }
    chatMock.messages = [{
      id: 'error-message-1',
      role: 'assistant',
      parts: [errorPart],
    }]
    const { result } = renderHook(() => useAgentChat())

    act(() => chatMock.options?.onData?.(errorPart))

    expect(toastMock.error).toHaveBeenCalledWith('provider exploded')
    expect(result.current.messages).toEqual([])
  })

  it('shows the stream request ID when an Agent error uses localized fallback copy', () => {
    const errorPart = {
      type: 'data-agent-error',
      id: 'error-part-correlated',
      data: {
        content: 'provider exploded',
        request_id: '0198f2cb-e980-7a21-81ba-e49998698090',
      },
    }
    const { result } = renderHook(() => useAgentChat())

    act(() => chatMock.options?.onData?.(errorPart))

    expect(toastMock.error).toHaveBeenCalledWith(expect.stringContaining('0198f2cb-e980-7a21-81ba-e49998698090'))
    expect(result.current.messages).toEqual([])
  })

  it('shows the server error and request ID when initial chat admission fails', () => {
    renderHook(() => useAgentChat())
    const error = new APIError('Agent transcript source revision conflict', {
      status: 500,
      requestID: '019ffb1f-0171-7436-828c-1d8f45095fe4',
    })

    act(() => chatMock.options?.onError?.(error))

    expect(toastMock.error).toHaveBeenCalledWith(expect.stringContaining('Agent transcript source revision conflict'))
    expect(toastMock.error).toHaveBeenCalledWith(expect.stringContaining('019ffb1f-0171-7436-828c-1d8f45095fe4'))
  })

  it('targets the operation already projected to the user instead of resolving a newer operation at click time', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask)
      .mockResolvedValueOnce({
        active: true,
        phase: 'running',
        active_operation_id: 'operation-visible',
        queue: [],
      })
      .mockResolvedValue({
        active: true,
        phase: 'running',
        active_operation_id: 'operation-newer',
        queue: [],
      })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'command-visible',
      operation_id: 'operation-visible',
      cursor: 8,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-visible'))
    vi.mocked(getActiveChatTask).mockClear()

    await act(async () => {
      expect(await result.current.send('继续当前画面')).toBe(true)
    })

    expect(getActiveChatTask).not.toHaveBeenCalled()
    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      expect.any(String),
      'operation-visible',
      'session-test',
      expect.objectContaining({ message: '继续当前画面' }),
    )
  })

  it('steers an already accepted follow-up without resubmitting its input', async () => {
    chatMock.status = 'streaming'
    const queued = {
      command_id: 'queued-7',
      operation_id: 'operation-7',
      delivery: 'follow_up' as const,
      message: 'Use the new ending',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      phase: 'running',
      active_operation_id: 'operation-7',
      queue: [queued],
    })
    vi.mocked(submitQueuedChatCommand).mockResolvedValue({
      command_id: 'steer-queued-7',
      operation_id: 'operation-7',
      cursor: 9,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.queue).toEqual([queued]))

    await act(async () => {
      expect(await result.current.steerQueuedCommand(queued)).toBe(true)
    })

    expect(submitQueuedChatCommand).toHaveBeenCalledWith(
      'steer_queued',
      expect.any(String),
      'operation-7',
      'queued-7',
      'session-test',
      undefined,
    )
    expect(result.current.runtimeProjection?.queue).toEqual([{ ...queued, steer_requested: true }])
    expect(submitChatCommand).not.toHaveBeenCalled()
  })

  it('cancels a queued follow-up, restores its live composer context, and returns its text for editing', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      phase: 'running',
      active_operation_id: 'operation-8',
      queue: [],
    })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'queued-8',
      operation_id: 'operation-8',
      cursor: 8,
    })
    vi.mocked(submitQueuedChatCommand).mockResolvedValue({
      command_id: 'edit-queued-8',
      operation_id: 'operation-8',
      cursor: 9,
    })
    vi.mocked(createAgentCommandID)
      .mockReturnValueOnce('queued-command-8')
      .mockReturnValueOnce('edit-command-8')
    const restoreSubmission = vi.fn()
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-8'))
    act(() => result.current.addReference('chapters/ch01.md'))

    await act(async () => {
      expect(await result.current.send('Rewrite the last paragraph', { onSubmissionError: restoreSubmission })).toBe(true)
    })
    const queued = result.current.runtimeProjection?.queue?.[0]
    expect(queued).toMatchObject({ command_id: 'queued-command-8', message: 'Rewrite the last paragraph' })
    expect(result.current.references).toEqual([])

    let prompt: string | null = null
    await act(async () => {
      prompt = queued ? await result.current.editQueuedCommand(queued) : null
    })

    expect(prompt).toBe('Rewrite the last paragraph')
    expect(submitQueuedChatCommand).toHaveBeenCalledWith(
      'cancel_queued',
      expect.any(String),
      'operation-8',
      'queued-command-8',
      'session-test',
      'returned_to_editor',
    )
    expect(result.current.runtimeProjection?.queue).toEqual([])
    expect(result.current.references).toEqual(['chapters/ch01.md'])
    expect(restoreSubmission).toHaveBeenCalledTimes(1)
  })

  it('refreshes the exact operation when submission advances into a response stream', async () => {
    chatMock.status = 'submitted'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      task_id: 'task-streaming-9',
      active_operation_id: 'operation-streaming-9',
    })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'abort-streaming-9',
      operation_id: 'operation-streaming-9',
      cursor: 10,
    })
    const { result, rerender } = renderHook(() => useAgentChat())
    expect(getActiveChatTask).not.toHaveBeenCalled()

    chatMock.status = 'streaming'
    rerender()

    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-streaming-9'))
    await act(async () => result.current.stop())
    expect(submitChatCommand).toHaveBeenCalledWith(
      'abort',
      expect.any(String),
      'operation-streaming-9',
      'session-test',
      undefined,
      'user_requested',
    )
  })

  it('enables a targeted abort from the streamed cycle identity before a stale active inspection resolves', async () => {
    chatMock.status = 'streaming'
    const staleInspection = deferred<{ active: false }>()
    vi.mocked(getActiveChatTask).mockReturnValue(staleInspection.promise)
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'abort-stream-cycle-10',
      operation_id: 'operation-stream-cycle-10',
      cursor: 11,
    })
    const { result } = renderHook(() => useAgentChat())

    act(() => {
      chatMock.options?.onData?.({
        type: 'data-agent-activity',
        data: {
          event: 'agent_cycle_started',
          command_id: 'start-stream-cycle-10',
          delivery: 'start_turn',
          operation_id: 'operation-stream-cycle-10',
          cycle: 1,
        },
      })
    })

    await waitFor(() => expect(result.current.runtimeProjection).toMatchObject({
      active: true,
      active_operation_id: 'operation-stream-cycle-10',
      active_cycle: 1,
      stream_attached: true,
    }))

    await act(async () => {
      staleInspection.resolve({ active: false })
      await Promise.resolve()
    })
    expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-stream-cycle-10')

    await act(async () => result.current.stop())
    expect(submitChatCommand).toHaveBeenCalledWith(
      'abort',
      expect.any(String),
      'operation-stream-cycle-10',
      'session-test',
      undefined,
      'user_requested',
    )
  })

  it('does not report an idle recovery inspection as active execution', async () => {
    let finishInspection!: (projection: { active: false }) => void
    vi.mocked(getActiveChatTask).mockReturnValue(new Promise((resolve) => {
      finishInspection = resolve
    }))
    const { result } = renderHook(() => useAgentChat())

    let inspection!: Promise<void>
    act(() => {
      inspection = result.current.resumeActiveChat()
    })
    await waitFor(() => expect(result.current.isStreaming).toBe(true))
    expect(result.current.isExecutionActive).toBe(false)

    await act(async () => {
      finishInspection({ active: false })
      await inspection
    })
    expect(result.current.isStreaming).toBe(false)
    expect(result.current.isExecutionActive).toBe(false)
  })

  it('aborts through the typed operation command and keeps observing its settlement', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      active_operation_id: 'operation-9',
    })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'abort-9',
      operation_id: 'operation-9',
      cursor: 10,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-9'))

    await act(async () => result.current.stop())

    expect(submitChatCommand).toHaveBeenCalledWith('abort', expect.any(String), 'operation-9', 'session-test', undefined, 'user_requested')
    expect(chatMock.stop).not.toHaveBeenCalled()
    expect(result.current.abortPending).toBe(true)
  })

  it('never falls back to an unscoped abort when the projected operation changed', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      active_operation_id: 'operation-visible',
    })
    vi.mocked(submitChatCommand).mockRejectedValue(new Error('target operation mismatch'))
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-visible'))
    vi.mocked(getActiveChatTask).mockClear()

    await act(async () => result.current.stop())

    expect(submitChatCommand).toHaveBeenCalledWith('abort', expect.any(String), 'operation-visible', 'session-test', undefined, 'user_requested')
    expect(chatMock.stop).not.toHaveBeenCalled()
  })

  it('blocks further commands after an abort receipt until the operation settles', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      active_operation_id: 'operation-9',
      queue: [],
    })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'abort-9',
      operation_id: 'operation-9',
      cursor: 10,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-9'))

    await act(async () => result.current.stop())
    vi.mocked(submitChatCommand).mockClear()

    await act(async () => {
      expect(
        await result.current.send('不应进入已中断的运行'),
      ).toBe(false)
      await result.current.stop()
    })

    expect(submitChatCommand).not.toHaveBeenCalled()
    expect(result.current.abortPending).toBe(true)
  })

  it('reuses the command id when retrying the same logical command after an uncertain response', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      active_operation_id: 'operation-9',
      queue: [],
    })
    vi.mocked(createAgentCommandID).mockReturnValueOnce('stable-command-id').mockReturnValueOnce('must-not-be-used')
    vi.mocked(submitChatCommand).mockRejectedValueOnce(new TypeError('connection reset')).mockResolvedValueOnce({
      command_id: 'stable-command-id',
      operation_id: 'operation-9',
      cursor: 12,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-9'))

    await act(async () => {
      expect(await result.current.send('同一条追加')).toBe(false)
      expect(await result.current.send('同一条追加')).toBe(true)
    })

    expect(vi.mocked(submitChatCommand).mock.calls.map((call) => call[1])).toEqual(['stable-command-id', 'stable-command-id'])
  })

  it('reuses the initial start command id after an ambiguous transport failure', async () => {
    vi.mocked(createAgentCommandID).mockReturnValueOnce('stable-initial-command').mockReturnValueOnce('must-not-be-used')
    chatMock.sendMessage.mockRejectedValueOnce(new TypeError('connection reset')).mockResolvedValueOnce(undefined)
    const { result } = renderHook(() => useAgentChat())

    await act(async () => {
      expect(await result.current.send('same initial request')).toBe(false)
      expect(await result.current.send('same initial request')).toBe(true)
    })

    expect(chatMock.sendMessage).toHaveBeenCalledTimes(2)
    expect(chatMock.sendMessage.mock.calls.map((call) => call[1]?.body?.command_id)).toEqual([
      'stable-initial-command',
      'stable-initial-command',
    ])
  })

  it('releases the initial start command id after a definite 4xx rejection', async () => {
    vi.mocked(createAgentCommandID).mockReturnValueOnce('rejected-initial-command').mockReturnValueOnce('fresh-initial-command')
    chatMock.sendMessage.mockRejectedValueOnce(new Error('HTTP 400')).mockResolvedValueOnce(undefined)
    const { result } = renderHook(() => useAgentChat())
    const transport = (chatMock.options as { transport: AgentChatTransport }).transport
    vi.spyOn(transport, 'takeInitialSubmissionOutcome').mockReturnValueOnce('rejected').mockReturnValueOnce('accepted')

    await act(async () => {
      expect(await result.current.send('same rejected request')).toBe(false)
      expect(await result.current.send('same rejected request')).toBe(true)
    })

    expect(chatMock.sendMessage.mock.calls.map((call) => call[1]?.body?.command_id)).toEqual([
      'rejected-initial-command',
      'fresh-initial-command',
    ])
  })

  it('canonically replaces provisional UI before reconnecting the projected active operation', async () => {
    chatMock.status = 'streaming'
    const canonicalMessages = [
      {
        id: 'canonical-assistant',
        role: 'assistant' as const,
        metadata: { run_id: 'run-9' },
        parts: [
          {
            type: 'reasoning' as const,
            text: '完整思考',
            state: 'done' as const,
          },
          { type: 'text' as const, text: 'hello', state: 'done' as const },
          {
            type: 'dynamic-tool' as const,
            toolName: 'write',
            toolCallId: 'tool-9',
            state: 'output-available' as const,
            input: { path: 'chapter.md', content: '完整参数' },
            output: 'ok',
          },
        ],
      },
    ]
    vi.mocked(getMessagesPage).mockResolvedValue({
      messages: canonicalMessages,
      nextBefore: '0',
      hasMore: false,
      total: 1,
    })
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      task_id: 'task-exact-9',
      stream_cursor: 99,
      cursor: 501,
      active_operation_id: 'operation-9',
      queue: [],
    })
    const { result, rerender } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-9'))
    const transport = (chatMock.options as { transport: AgentChatTransport }).transport
    const setTarget = vi.spyOn(transport, 'setActiveStreamTarget')
    vi.mocked(getMessagesPage).mockClear()
    vi.mocked(getActiveChatTask).mockClear()

    chatMock.status = 'ready'
    rerender()

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(getActiveChatTask).toHaveBeenCalledTimes(1)
    expect(setTarget).toHaveBeenCalledWith('task-exact-9', undefined, { session_id: 'session-test' })
    expect(getMessagesPage).toHaveBeenCalledTimes(1)
    expect(vi.mocked(getMessagesPage).mock.invocationCallOrder[0]).toBeLessThan(chatMock.resumeStream.mock.invocationCallOrder[0])
    expect(chatMock.setMessages).toHaveBeenCalledWith(canonicalMessages)
    expect(result.current.isStreaming).toBe(true)
  })

  it('canonicalizes after a finished writing task remains retained by the active endpoint', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getActiveChatTask)
      .mockResolvedValueOnce({
        active: true,
        phase: 'running',
        task_id: 'retained-task',
        active_operation_id: 'operation-finished',
      })
      .mockResolvedValue({
        active: false,
        phase: 'idle',
        task_id: 'retained-task',
        stream_attached: true,
        runtime_recoverable: false,
        recovery_paused: false,
        recovery_actions: [],
      })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'abort-finished',
      operation_id: 'operation-finished',
      cursor: 41,
    })
    const { result, rerender } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.active_operation_id).toBe('operation-finished'))
    const transport = (chatMock.options as { transport: AgentChatTransport }).transport
    const clearTarget = vi.spyOn(transport, 'clearActiveStreamTarget')

    await act(async () => result.current.stop())
    expect(result.current.abortPending).toBe(true)
    vi.mocked(getMessagesPage).mockClear()

    chatMock.status = 'ready'
    rerender()

    await waitFor(() => expect(getMessagesPage).toHaveBeenCalledTimes(1))
    expect(result.current.abortPending).toBe(false)
    expect(clearTarget).toHaveBeenCalledTimes(1)
    expect(chatMock.resumeStream).not.toHaveBeenCalled()
    expect(result.current.runtimeProjection).toMatchObject({
      active: false,
      phase: 'idle',
      task_id: 'retained-task',
      stream_attached: true,
      runtime_recoverable: false,
    })
    expect(result.current.isStreaming).toBe(false)
  })

  it('reloads canonical history and resumes the same Task after an incomplete display checkpoint', async () => {
    chatMock.status = 'streaming'
    const canonicalMessages = [
      {
        id: 'history-user',
        role: 'user' as const,
        parts: [{ type: 'text' as const, text: '继续' }],
      },
    ]
    vi.mocked(getMessagesPage).mockResolvedValue({
      messages: canonicalMessages,
      nextBefore: '0',
      hasMore: false,
      total: 1,
    })
    const { result, rerender } = renderHook(() => useAgentChat())
    const transport = (chatMock.options as { transport: AgentChatTransport }).transport
    const setTarget = vi.spyOn(transport, 'setActiveStreamTarget')
    vi.mocked(getMessagesPage).mockClear()
    chatMock.setMessages.mockClear()

    act(() => {
      chatMock.options?.onData?.({
        type: 'data-agent-activity',
        data: {
          event: 'task_rehydrate_required',
          code: 'agent_stream.rehydrate_required',
          task_id: 'task-checkpoint-41',
          cursor: 41,
          settled: false,
        },
      })
      chatMock.status = 'ready'
      rerender()
    })

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(setTarget).toHaveBeenCalledWith('task-checkpoint-41', 41, { session_id: 'session-test' })
    expect(chatMock.setMessages).toHaveBeenNthCalledWith(1, canonicalMessages)
    expect(vi.mocked(getMessagesPage).mock.invocationCallOrder[0]).toBeLessThan(chatMock.resumeStream.mock.invocationCallOrder[0])
    expect(result.current.isStreaming).toBe(true)

    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      task_id: 'task-checkpoint-41',
      active_operation_id: 'operation-checkpoint-41',
      queue: [],
    })
    chatMock.setMessages.mockClear()
    await act(async () => result.current.resumeActiveChat())
    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(2))

    expect(chatMock.setMessages).toHaveBeenNthCalledWith(1, canonicalMessages)
    const restoreOmission = chatMock.setMessages.mock.calls.at(-1)?.[0] as (messages: typeof canonicalMessages) => Array<{
      parts?: Array<{ type?: string; data?: { content?: string } }>
    }>
    const restored = restoreOmission(canonicalMessages)
    expect(restored.at(-1)?.parts?.[0]).toMatchObject({
      type: 'data-agent-system',
      data: {
        content: '较早的实时轨迹已超出展示预算；已恢复规范历史，并继续观察同一次 Agent 运行。',
      },
    })

    chatMock.setMessages.mockClear()
    vi.mocked(getMessagesPage).mockClear()
    act(() => {
      chatMock.options?.onData?.({
        type: 'data-agent-activity',
        data: {
          event: 'task_rehydrate_required',
          code: 'agent_stream.rehydrate_required',
          task_id: 'task-checkpoint-41',
          cursor: 55,
          settled: true,
        },
      })
    })

    await waitFor(() => expect(vi.mocked(getMessagesPage)).toHaveBeenCalledTimes(1))
    expect(chatMock.resumeStream).toHaveBeenCalledTimes(2)
    expect(chatMock.setMessages).toHaveBeenCalledTimes(1)
    expect(chatMock.setMessages).toHaveBeenCalledWith(canonicalMessages)
    await waitFor(() => expect(result.current.isStreaming).toBe(false))
  })

  it.each([
    {
      name: 'error',
      status: 'error',
      terminalReason: 'provider failed after acceptance',
      expectedContent: 'provider failed after acceptance',
    },
    {
      name: 'abort',
      status: 'aborted',
      terminalReason: 'user_requested',
      expectedContent: '已暂停，可随时继续',
    },
  ])(
    'reports an evicted Writing $name terminal as a transient toast after canonical rehydrate',
    async ({ status, terminalReason, expectedContent }) => {
      chatMock.status = 'streaming'
      const canonicalMessages = [
        {
          id: 'history-user',
          role: 'user' as const,
          parts: [{ type: 'text' as const, text: '继续' }],
        },
      ]
      vi.mocked(getMessagesPage).mockResolvedValue({
        messages: canonicalMessages,
        nextBefore: '0',
        hasMore: false,
        total: 1,
      })
      const { rerender } = renderHook(() => useAgentChat())
      vi.mocked(getMessagesPage).mockClear()
      chatMock.setMessages.mockClear()

      act(() => {
        chatMock.options?.onData?.({
          type: 'data-agent-activity',
          data: {
            event: 'task_rehydrate_required',
            code: 'agent_stream.rehydrate_required',
            task_id: `task-evicted-${status}`,
            cursor: 72,
            settled: true,
            status,
            terminal_reason: terminalReason,
          },
        })
        chatMock.status = 'ready'
        rerender()
      })

      const terminalToast = status === 'error' ? toastMock.error : toastMock.info
      await waitFor(() => expect(terminalToast).toHaveBeenCalledWith(expectedContent))
      expect(chatMock.setMessages).toHaveBeenCalledTimes(1)
      expect(chatMock.setMessages).toHaveBeenCalledWith(canonicalMessages)
      expect(chatMock.resumeStream).not.toHaveBeenCalled()
    },
  )

  it('keeps display recovery pending when authoritative history reload fails and retries on focus', async () => {
    chatMock.status = 'streaming'
    vi.mocked(getMessagesPage).mockRejectedValueOnce(new TypeError('history offline'))
    const { result, rerender } = renderHook(() => useAgentChat())

    act(() => {
      chatMock.options?.onData?.({
        type: 'data-agent-activity',
        data: {
          event: 'task_rehydrate_required',
          code: 'agent_stream.rehydrate_required',
          task_id: 'task-retry-history',
          cursor: 12,
          settled: false,
        },
      })
      chatMock.status = 'ready'
      rerender()
    })

    await waitFor(() => expect(getMessagesPage).toHaveBeenCalledTimes(1))
    expect(chatMock.resumeStream).not.toHaveBeenCalled()
    expect(result.current.isStreaming).toBe(true)

    vi.mocked(getMessagesPage).mockResolvedValue({
      messages: [],
      nextBefore: '0',
      hasMore: false,
      total: 0,
    })
    act(() => window.dispatchEvent(new Event('focus')))

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(getMessagesPage).toHaveBeenCalledTimes(2)
  })

  it('retries cold recovery with the same server-projected identity and never resends cached input', async () => {
    const action = {
      action_id: 'follow-up-action',
      kind: 'follow_up' as const,
      command_id: 'accepted-follow-up',
      operation_id: 'operation-recovery',
    }
    const laterAction = {
      action_id: 'next-turn-action',
      kind: 'next_turn' as const,
      command_id: 'accepted-next-turn',
      operation_id: 'operation-next',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [action, laterAction],
    })
    vi.mocked(recoverChatAgentRuntime).mockRejectedValueOnce(new TypeError('connection reset')).mockResolvedValueOnce({
      task_id: 'recovery-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 12,
      recovery_action: action,
    })
    const { result } = renderHook(() => useAgentChat())
    const transport = (chatMock.options as { transport: AgentChatTransport }).transport
    const setTarget = vi.spyOn(transport, 'setActiveStreamTarget')

    await act(async () => result.current.resumeActiveChat())
    await waitFor(() => expect(result.current.isStreaming).toBe(true))
    expect(result.current.activityContent).toBe('正在从持久化状态恢复已接受的 Agent 运行…')
    expect(recoverChatAgentRuntime).toHaveBeenCalledTimes(1)

    act(() => window.dispatchEvent(new Event('online')))

    await waitFor(() => expect(recoverChatAgentRuntime).toHaveBeenCalledTimes(2))
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[action, 'session-test'], [action, 'session-test']])
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls.flat()).not.toContain(laterAction)
    expect(setTarget).toHaveBeenCalledWith('recovery-task-1', undefined, { session_id: 'session-test' })
    expect(chatMock.resumeStream).toHaveBeenCalledTimes(1)
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
    expect(result.current.runtimeProjection).toMatchObject({
      recovery_paused: false,
      runtime_recoverable: false,
      stream_attached: true,
    })
  })

  it('keeps attach-only recovery paused so Stop still uses the projected abort action', async () => {
    const attachAction = {
      kind: 'start_turn' as const,
      command_id: 'recovery-attach-1',
      operation_id: 'operation-recovery',
    }
    const abortAction = {
      kind: 'abort' as const,
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, abortAction],
    })
    vi.mocked(recoverChatAgentRuntime)
      .mockResolvedValueOnce({
        task_id: 'attach-task-1',
        status: 'running',
        stream_cursor: 0,
        cursor: 14,
        recovery_action: attachAction,
      })
      .mockResolvedValueOnce({
        task_id: 'abort-task-1',
        status: 'running',
        stream_cursor: 0,
        cursor: 15,
        recovery_action: abortAction,
      })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(result.current.runtimeProjection).toMatchObject({
      active: true,
      task_id: 'attach-task-1',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      recovery_actions: [abortAction],
    })

    await act(async () => result.current.stop())

    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[attachAction, 'session-test'], [abortAction, 'session-test']])
    expect(submitChatCommand).not.toHaveBeenCalled()
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
  })

  it('attaches an existing writing recovery task without posting start_turn again', async () => {
    const attachAction = {
      kind: 'start_turn' as const,
      command_id: 'already-attached-start',
      operation_id: 'operation-recovery',
    }
    const abortAction = {
      kind: 'abort' as const,
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      phase: 'running',
      task_id: 'already-attached-task',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, abortAction],
    })
    const { result } = renderHook(() => useAgentChat())
    const transport = (chatMock.options as { transport: AgentChatTransport }).transport
    const setTarget = vi.spyOn(transport, 'setActiveStreamTarget')

    await act(async () => result.current.resumeActiveChat())

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(recoverChatAgentRuntime).not.toHaveBeenCalled()
    expect(setTarget).toHaveBeenCalledWith('already-attached-task', undefined, { session_id: 'session-test' })
    expect(result.current.runtimeProjection).toMatchObject({
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      recovery_actions: [attachAction, abortAction],
    })
  })

  it('submits only the head state action for an attached writing recovery task', async () => {
    const attachAction = {
      kind: 'start_turn' as const,
      command_id: 'already-attached-start',
      operation_id: 'operation-recovery',
    }
    const stateAction = {
      kind: 'follow_up' as const,
      command_id: 'accepted-follow-up',
      operation_id: 'operation-recovery',
    }
    const laterAction = {
      kind: 'next_turn' as const,
      command_id: 'accepted-next-turn',
      operation_id: 'operation-next',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      phase: 'running',
      task_id: 'already-attached-task',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, stateAction, laterAction],
    })
    vi.mocked(recoverChatAgentRuntime).mockResolvedValue({
      task_id: 'already-attached-task',
      status: 'running',
      stream_cursor: 0,
      cursor: 21,
      recovery_action: stateAction,
    })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[stateAction, 'session-test']])
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls.flat()).not.toContain(attachAction)
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls.flat()).not.toContain(laterAction)
    expect(result.current.runtimeProjection).toMatchObject({
      recovery_paused: false,
      runtime_recoverable: false,
      stream_attached: true,
    })
  })

  it('advances consecutive writing recovery actions on the same AI SDK stream', async () => {
    const firstAction = {
      kind: 'next_turn' as const,
      command_id: 'accepted-next-turn',
      operation_id: 'operation-first',
    }
    const secondAction = {
      kind: 'follow_up' as const,
      command_id: 'accepted-follow-up',
      operation_id: 'operation-second',
    }
    vi.mocked(getActiveChatTask)
      .mockResolvedValueOnce({
        active: false,
        phase: 'running',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: false,
        active_operation_id: 'operation-first',
        recovery_actions: [firstAction],
      })
      .mockResolvedValueOnce({
        active: false,
        phase: 'running',
        task_id: 'shared-recovery-task',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-second',
        recovery_actions: [secondAction],
      })
    vi.mocked(recoverChatAgentRuntime)
      .mockResolvedValueOnce({
        task_id: 'shared-recovery-task',
        status: 'running',
        stream_cursor: 0,
        cursor: 24,
        recovery_action: firstAction,
      })
      .mockResolvedValueOnce({
        task_id: 'shared-recovery-task',
        status: 'running',
        stream_cursor: 0,
        cursor: 25,
        recovery_action: secondAction,
      })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    act(() =>
      chatMock.options?.onData?.({
        type: 'data-agent-activity',
        data: {
          event: 'runtime_recovery_required',
          code: 'agent_runtime.recovery_required',
          operation_id: 'operation-second',
        },
      }),
    )

    await waitFor(() => expect(recoverChatAgentRuntime).toHaveBeenCalledTimes(2))
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[firstAction, 'session-test'], [secondAction, 'session-test']])
    expect(chatMock.resumeStream).toHaveBeenCalledTimes(1)
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
    expect(result.current.runtimeProjection).toMatchObject({
      task_id: 'shared-recovery-task',
      recovery_paused: false,
      runtime_recoverable: false,
      stream_attached: true,
    })
  })

  it('keeps the writing stream healthy when another tab advances the projected recovery action', async () => {
    const stateAction = {
      kind: 'follow_up' as const,
      command_id: 'accepted-follow-up',
      operation_id: 'operation-raced',
    }
    vi.mocked(getActiveChatTask)
      .mockResolvedValueOnce({
        active: true,
        phase: 'running',
        task_id: 'shared-recovery-task',
        active_operation_id: 'operation-first',
      })
      .mockResolvedValueOnce({
        active: false,
        phase: 'running',
        task_id: 'shared-recovery-task',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-raced',
        recovery_actions: [stateAction],
      })
      .mockResolvedValueOnce({
        active: true,
        phase: 'running',
        task_id: 'shared-recovery-task',
        recovery_paused: false,
        runtime_recoverable: false,
        stream_attached: true,
        active_operation_id: 'operation-raced',
        recovery_actions: [],
      })
    vi.mocked(recoverChatAgentRuntime).mockRejectedValue(
      Object.assign(new Error('projection changed'), {
        code: 'agent_runtime.recovery_changed',
        status: 409,
      }),
    )
    const { result } = renderHook(() => useAgentChat())
    await act(async () => result.current.resumeActiveChat())
    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))

    act(() =>
      chatMock.options?.onData?.({
        type: 'data-agent-activity',
        data: {
          event: 'runtime_recovery_required',
          code: 'agent_runtime.recovery_required',
        },
      }),
    )

    await waitFor(() => expect(getActiveChatTask).toHaveBeenCalledTimes(3))
    expect(recoverChatAgentRuntime).toHaveBeenCalledTimes(1)
    expect(chatMock.resumeStream).toHaveBeenCalledTimes(1)
    expect(result.current.runtimeProjection).toMatchObject({
      active: true,
      recovery_paused: false,
      runtime_recoverable: false,
    })
    expect(result.current.activityContent).toBe('')
  })

  it('does not open a second AI SDK stream when Stop aborts recovery on the current stream', async () => {
    chatMock.status = 'streaming'
    const abortAction = {
      kind: 'abort' as const,
      command_id: 'recovery-abort-current-stream',
      operation_id: 'operation-recovery',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: true,
      phase: 'running',
      task_id: 'current-stream-task',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      active_operation_id: 'operation-recovery',
      recovery_actions: [abortAction],
    })
    vi.mocked(recoverChatAgentRuntime).mockResolvedValue({
      task_id: 'current-stream-task',
      status: 'running',
      stream_cursor: 0,
      cursor: 22,
      recovery_action: abortAction,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.recovery_actions).toEqual([abortAction]))

    await act(async () => result.current.stop())

    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(abortAction, 'session-test')
    expect(chatMock.resumeStream).not.toHaveBeenCalled()
    expect(result.current.abortPending).toBe(true)
  })

  it('immediately reprojects a second paused recovery after AI SDK resolves the failed observation', async () => {
    const firstResume = deferred<void>()
    const nextAction = {
      kind: 'next_turn' as const,
      command_id: 'accepted-next-turn',
      operation_id: 'operation-next',
    }
    chatMock.resumeStream.mockReturnValueOnce(firstResume.promise).mockResolvedValueOnce(undefined)
    vi.mocked(getActiveChatTask)
      .mockResolvedValueOnce({
        active: true,
        phase: 'running',
        task_id: 'first-recovery-task',
        active_operation_id: 'operation-recovery',
      })
      .mockResolvedValueOnce({
        active: false,
        phase: 'running',
        task_id: 'first-recovery-task',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-next',
        recovery_actions: [nextAction],
      })
    vi.mocked(recoverChatAgentRuntime).mockResolvedValue({
      task_id: 'second-recovery-task',
      status: 'running',
      stream_cursor: 0,
      cursor: 23,
      recovery_action: nextAction,
    })
    const { result, rerender } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    expect(chatMock.resumeStream).toHaveBeenCalledTimes(1)

    chatMock.status = 'error'
    chatMock.error = new TypeError('reconnect failed')
    rerender()
    await act(async () => firstResume.resolve())

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(2))
    expect(getActiveChatTask).toHaveBeenCalledTimes(2)
    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(nextAction, 'session-test')
    expect(result.current.isStreaming).toBe(true)
  })

  it('lets a fresh command resume an attach-only writing operation without starting a new root turn', async () => {
    const attachAction = {
      kind: 'start_turn' as const,
      command_id: 'recovery-attach-1',
      operation_id: 'operation-recovery',
    }
    const abortAction = {
      kind: 'abort' as const,
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, abortAction],
    })
    vi.mocked(recoverChatAgentRuntime).mockResolvedValue({
      task_id: 'attach-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 16,
      recovery_action: attachAction,
    })
    vi.mocked(submitChatCommand).mockResolvedValue({
      command_id: 'fresh-follow-up',
      operation_id: 'operation-recovery',
      cursor: 17,
    })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    await waitFor(() => expect(result.current.runtimeProjection?.stream_attached).toBe(true))
    await act(async () => {
      expect(
        await result.current.send('采用新的恢复方向'),
      ).toBe(true)
    })

    expect(recoverChatAgentRuntime).toHaveBeenCalledTimes(1)
    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(attachAction, 'session-test')
    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      expect.any(String),
      'operation-recovery',
      'session-test',
      expect.objectContaining({ message: '采用新的恢复方向' }),
    )
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
    expect(result.current.runtimeProjection).toMatchObject({
      recovery_paused: false,
      runtime_recoverable: false,
      recovery_actions: [],
    })
  })

  it('never auto-submits a recovery abort and Stop echoes the projected abort identity', async () => {
    const abortAction = {
      kind: 'abort' as const,
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    vi.mocked(getActiveChatTask).mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [abortAction],
    })
    vi.mocked(recoverChatAgentRuntime).mockResolvedValue({
      task_id: 'abort-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 13,
      recovery_action: abortAction,
    })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    expect(recoverChatAgentRuntime).not.toHaveBeenCalled()

    await act(async () => result.current.stop())

    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(abortAction, 'session-test')
    expect(submitChatCommand).not.toHaveBeenCalled()
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
    expect(result.current.abortPending).toBe(true)
  })

  it('ignores an older history response after a newer session history has loaded', async () => {
    const older = deferred<Awaited<ReturnType<typeof getMessagesPage>>>()
    const newer = deferred<Awaited<ReturnType<typeof getMessagesPage>>>()
    vi.mocked(getMessagesPage).mockImplementation((sessionId?: string) => (sessionId === 'older' ? older.promise : newer.promise))
    const { result } = renderHook(() => useAgentChat())

    let olderRequest!: Promise<void>
    let newerRequest!: Promise<void>
    act(() => {
      olderRequest = result.current.loadHistory('older')
      newerRequest = result.current.loadHistory('newer')
    })

    await act(async () => {
      newer.resolve({
        messages: [
          {
            id: 'new-message',
            role: 'user',
            parts: [{ type: 'text', text: '新会话' }],
          },
        ],
        nextBefore: '0',
        hasMore: false,
        total: 1,
      })
      await newerRequest
    })
    await act(async () => {
      older.resolve({
        messages: [
          {
            id: 'old-message',
            role: 'user',
            parts: [{ type: 'text', text: '旧会话' }],
          },
        ],
        nextBefore: '0',
        hasMore: false,
        total: 1,
      })
      await olderRequest
    })

    expect(chatMock.setMessages).toHaveBeenCalledTimes(1)
    expect(chatMock.setMessages).toHaveBeenLastCalledWith([
      {
        id: 'new-message',
        role: 'user',
        parts: [{ type: 'text', text: '新会话' }],
      },
    ])
  })

  it('prepends an earlier history page without replacing the current live tail', async () => {
    writingAgentChatClient.fixedSessionId = ''
    vi.mocked(getMessagesPage)
      .mockResolvedValueOnce({
        messages: [
          {
            id: 'message-2',
            role: 'assistant',
            parts: [{ type: 'text', text: '当前窗口' }],
          },
        ],
        nextBefore: '1',
        hasMore: true,
        total: 2,
      })
      .mockResolvedValueOnce({
        messages: [
          {
            id: 'message-1',
            role: 'user',
            parts: [{ type: 'text', text: '更早消息' }],
          },
        ],
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
    expect(
      prepend([
        {
          id: 'message-2',
          role: 'assistant',
          parts: [{ type: 'text', text: '当前窗口' }],
        },
        {
          id: 'live-message',
          role: 'assistant',
          parts: [{ type: 'text', text: '仍在流式输出', state: 'streaming' }],
        },
      ]),
    ).toEqual([
      {
        id: 'message-1',
        role: 'user',
        parts: [{ type: 'text', text: '更早消息' }],
      },
      {
        id: 'message-2',
        role: 'assistant',
        parts: [{ type: 'text', text: '当前窗口' }],
      },
      {
        id: 'live-message',
        role: 'assistant',
        parts: [{ type: 'text', text: '仍在流式输出', state: 'streaming' }],
      },
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
