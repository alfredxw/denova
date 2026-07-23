import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createAgentCommandID,
  getActiveChatTask,
  getMessagesPage,
  getSessions,
  recoverChatAgentRuntime,
  submitChatCommand,
  switchSession,
  type SessionSummary,
} from '@/lib/api'
import { AgentChatTransport } from '@/lib/agent-ui'
import { useAgentChat } from './useAgentChat'

const chatMock = vi.hoisted(() => ({
  options: null as Record<string, any> | null,
  sendMessage: vi.fn(),
  setMessages: vi.fn(),
  resumeStream: vi.fn(),
  stop: vi.fn(),
  status: 'ready' as 'ready' | 'submitted' | 'streaming' | 'error',
  error: undefined as Error | undefined,
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
      error: chatMock.error,
    }
  },
}))

vi.mock('@/lib/api', () => ({
  analyzeChatContext: vi.fn(),
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
  submitChatCommand: vi.fn(),
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
    vi.mocked(recoverChatAgentRuntime).mockReset()
    vi.mocked(submitChatCommand).mockReset()
    chatMock.sendMessage.mockReset()
    chatMock.resumeStream.mockReset().mockResolvedValue(undefined)
    chatMock.options = null
    chatMock.status = 'ready'
    chatMock.error = undefined
  })

  it('stops the old stream and selects the target immediately when switching sessions', async () => {
    chatMock.status = 'streaming'
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
    const { result } = renderHook(() => useAgentChat())

    let request!: Promise<void>
    act(() => {
      request = result.current.switchChatSession('target')
    })

    expect(chatMock.stop).toHaveBeenCalledTimes(1)
    expect(result.current.activeSessionId).toBe('target')

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
        await result.current.send('再补一个反转', {
          delivery: 'follow_up',
          onSubmissionStart,
        }),
      ).toBe(true)
    })

    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      expect.any(String),
      'operation-7',
      expect.objectContaining({
        message: '再补一个反转',
        references: ['chapters/ch01.md'],
      }),
    )
    expect(chatMock.sendMessage).not.toHaveBeenCalled()
    expect(onSubmissionStart).toHaveBeenCalledTimes(1)
    expect(result.current.references).toEqual([])
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
      expect(await result.current.send('继续当前画面', { delivery: 'follow_up' })).toBe(true)
    })

    expect(getActiveChatTask).not.toHaveBeenCalled()
    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      expect.any(String),
      'operation-visible',
      expect.objectContaining({ message: '继续当前画面' }),
    )
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

    expect(submitChatCommand).toHaveBeenCalledWith('abort', expect.any(String), 'operation-9', undefined, 'user_requested')
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

    expect(submitChatCommand).toHaveBeenCalledWith('abort', expect.any(String), 'operation-visible', undefined, 'user_requested')
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
        await result.current.send('不应进入已中断的运行', {
          delivery: 'follow_up',
        }),
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
      expect(await result.current.send('同一条追加', { delivery: 'follow_up' })).toBe(false)
      expect(await result.current.send('同一条追加', { delivery: 'follow_up' })).toBe(true)
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
            toolName: 'write_file',
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
    expect(setTarget).toHaveBeenCalledWith('task-exact-9')
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
    expect(setTarget).toHaveBeenCalledWith('task-checkpoint-41', 41)
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
      expectedContent: '已中断 AI 执行',
    },
  ])(
    'restores an evicted Writing $name terminal after canonical rehydrate',
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

      await waitFor(() => expect(chatMock.setMessages).toHaveBeenCalledTimes(2))
      expect(chatMock.setMessages).toHaveBeenNthCalledWith(1, canonicalMessages)
      const restoreTerminal = chatMock.setMessages.mock.calls[1]?.[0] as (messages: typeof canonicalMessages) => Array<{
        parts?: Array<{ type?: string; data?: Record<string, unknown> }>
      }>
      const restored = restoreTerminal(canonicalMessages)
      expect(restored.at(-1)?.parts?.[0]).toMatchObject({
        type: 'data-agent-error',
        data: {
          content: expectedContent,
          status,
          terminal_reason: terminalReason,
        },
      })
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
      replayed: true,
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
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[action], [action]])
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls.flat()).not.toContain(laterAction)
    expect(setTarget).toHaveBeenCalledWith('recovery-task-1')
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
        replayed: false,
        recovery_action: attachAction,
      })
      .mockResolvedValueOnce({
        task_id: 'abort-task-1',
        status: 'running',
        stream_cursor: 0,
        cursor: 15,
        replayed: false,
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

    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[attachAction], [abortAction]])
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
    expect(setTarget).toHaveBeenCalledWith('already-attached-task')
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
      replayed: false,
      recovery_action: stateAction,
    })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())

    await waitFor(() => expect(chatMock.resumeStream).toHaveBeenCalledTimes(1))
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[stateAction]])
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
        replayed: false,
        recovery_action: firstAction,
      })
      .mockResolvedValueOnce({
        task_id: 'shared-recovery-task',
        status: 'running',
        stream_cursor: 0,
        cursor: 25,
        replayed: false,
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
    expect(vi.mocked(recoverChatAgentRuntime).mock.calls).toEqual([[firstAction], [secondAction]])
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
      replayed: false,
      recovery_action: abortAction,
    })
    const { result } = renderHook(() => useAgentChat())
    await waitFor(() => expect(result.current.runtimeProjection?.recovery_actions).toEqual([abortAction]))

    await act(async () => result.current.stop())

    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(abortAction)
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
      replayed: false,
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
    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(nextAction)
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
      replayed: false,
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
        await result.current.send('采用新的恢复方向', {
          delivery: 'follow_up',
        }),
      ).toBe(true)
    })

    expect(recoverChatAgentRuntime).toHaveBeenCalledTimes(1)
    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(attachAction)
    expect(submitChatCommand).toHaveBeenCalledWith(
      'follow_up',
      expect.any(String),
      'operation-recovery',
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
      replayed: false,
      recovery_action: abortAction,
    })
    const { result } = renderHook(() => useAgentChat())

    await act(async () => result.current.resumeActiveChat())
    expect(recoverChatAgentRuntime).not.toHaveBeenCalled()

    await act(async () => result.current.stop())

    expect(recoverChatAgentRuntime).toHaveBeenCalledWith(abortAction)
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
