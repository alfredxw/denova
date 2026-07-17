import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentChat } from './useAgentChat'

const chatMock = vi.hoisted(() => ({
  options: null as Record<string, any> | null,
  sendMessage: vi.fn(),
  setMessages: vi.fn(),
  resumeStream: vi.fn(),
  stop: vi.fn(),
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
      status: 'ready',
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
  getMessages: vi.fn().mockResolvedValue([]),
  getSessions: vi.fn().mockResolvedValue([]),
  renameSession: vi.fn(),
  switchSession: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

describe('useAgentChat submitted references', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    chatMock.options = null
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
        reviewFeedback: { reviewThreadId: 'thread-1', commentIds: ['comment-1'] },
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
})
