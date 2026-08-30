import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import { useInteractiveStore } from '../stores/interactive-store'
import {
  PersistedTurnHarness,
  StoryStageHarness,
  controllableInteractiveStream,
  expectVisibleText,
  getStageInput,
  persistedTurnEvent,
  resetStoryStageTestHarness,
} from './story-stage/story-stage-test-harness'

const testMocks = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
  getActiveInteractiveChatMock: vi.fn(),
  recoverInteractiveAgentRuntimeMock: vi.fn(),
  sendInteractiveMessageMock: vi.fn(),
  streamActiveInteractiveChatMock: vi.fn(),
  submitInteractiveAgentCommandMock: vi.fn(),
  updateInteractiveTurnNarrativeMock: vi.fn(),
  useSkillCommandsMock: vi.fn(),
}))

const {
  getActiveInteractiveChatMock,
  recoverInteractiveAgentRuntimeMock,
  sendInteractiveMessageMock,
  streamActiveInteractiveChatMock,
} = testMocks

vi.mock('@/features/settings/api', () => ({
  fetchProjectSettings: vi.fn().mockResolvedValue({ effective: {} }),
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
  refreshProjectSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

vi.mock('@/features/agent-approval/AgentApprovalProvider', () => ({
  useAgentApprovalMode: () => ({ mode: 'write', initialized: true, saving: false, setMode: vi.fn().mockResolvedValue(true) }),
}))

vi.mock('@/features/conversation-config/use-conversation-config', () => ({
  useConversationConfig: () => conversationConfigController(),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: (...args: unknown[]) => testMocks.useSkillCommandsMock(...args),
}))

vi.mock('../api', () => ({
  analyzeInteractiveContext: vi.fn(),
  compactInteractiveContext: vi.fn(),
  generateInteractiveImage: testMocks.generateInteractiveImageMock,
  getActiveInteractiveChat: testMocks.getActiveInteractiveChatMock,
  removeInteractiveContextCompaction: vi.fn(),
  recoverInteractiveAgentRuntime: testMocks.recoverInteractiveAgentRuntimeMock,
  sendInteractiveMessage: testMocks.sendInteractiveMessageMock,
  streamActiveInteractiveChat: testMocks.streamActiveInteractiveChatMock,
  submitInteractiveAgentCommand: testMocks.submitInteractiveAgentCommandMock,
  switchInteractiveTurnVersion: vi.fn(),
  updateInteractiveTurnNarrative: testMocks.updateInteractiveTurnNarrativeMock,
}))

function conversationConfigController() {
  return {
    snapshot: { agent_kind: 'interactive_story', profile_id: 'default', thinking_level: 'off', approval_mode: 'write', revision: 1 },
    initialized: true, loading: false, saving: false, error: null,
    patch: vi.fn().mockResolvedValue(true), reload: vi.fn(),
  }
}

beforeEach(() => {
  resetStoryStageTestHarness(testMocks)
  recoverInteractiveAgentRuntimeMock.mockReset()
})

describe('StoryStage runtime stream lifecycle', () => {
  it('requires persistence confirmation for the final cycle rather than any earlier cycle', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)

    render(<PersistedTurnHarness onDone={vi.fn().mockResolvedValue(undefined)} />)
    await user.type(getStageInput(), '推开石门')
    await user.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))

    act(() => {
      stream.enqueue({
        event: 'agent_cycle_started',
        data: JSON.stringify({
          command_id: 'start-1',
          delivery: 'start_turn',
          message: '推开石门',
          operation_id: 'operation-1',
          cycle: 1,
        }),
      })
      stream.enqueue({
        event: 'interactive_turn_persisted',
        data: JSON.stringify(persistedTurnEvent()),
      })
      stream.enqueue({
        event: 'agent_cycle_started',
        data: JSON.stringify({
          command_id: 'follow-1',
          delivery: 'follow_up',
          message: '继续',
          operation_id: 'operation-1',
          cycle: 2,
        }),
      })
      stream.enqueue({
        event: 'chunk',
        data: JSON.stringify({ content: '这段没有持久化。' }),
      })
      stream.enqueue({ event: 'done', data: '{}' })
    })

    expect(await screen.findByText(/没有收到持久化确认/)).toBeInTheDocument()
    stream.close()
  })

  it('reconnects an unexpectedly ended stream while the projected operation is still active', async () => {
    const user = userEvent.setup()
    const first = controllableInteractiveStream()
    const resumed = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(first.readable)
    streamActiveInteractiveChatMock.mockResolvedValue(resumed.readable)

    try {
      render(<StoryStageHarness />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        first.enqueue({
          id: '17',
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'start-1',
            delivery: 'start_turn',
            message: '推开石门',
            operation_id: 'operation-1',
            cycle: 1,
          }),
        }),
      )
      getActiveInteractiveChatMock.mockResolvedValue({
        active: true,
        task_id: 'task-1',
        message: '推开石门',
        active_operation_id: 'operation-1',
        queue: [],
      })

      act(() => first.close())

      await waitFor(() =>
        expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith(
          expect.objectContaining({
            taskId: 'task-1',
            after: '17',
          }),
        ),
      )
      expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeEnabled()
    } finally {
      first.close()
      resumed.close()
    }
  })

  it('canonically reloads an incomplete display and continues the exact active Task suffix', async () => {
    const user = userEvent.setup()
    const first = controllableInteractiveStream()
    const resumed = controllableInteractiveStream()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    sendInteractiveMessageMock.mockResolvedValue(first.readable)
    streamActiveInteractiveChatMock.mockResolvedValue(resumed.readable)

    try {
      render(<StoryStageHarness onDone={handleDone} />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))

      act(() =>
        first.enqueue({
          id: '17',
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'start-incomplete-41',
            delivery: 'start_turn',
            message: '推开石门',
            operation_id: 'operation-incomplete-41',
            cycle: 1,
          }),
        }),
      )
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime.operationId).toBe(
          'operation-incomplete-41',
        ),
      )

      act(() =>
        first.enqueue({
          event: 'task_rehydrate_required',
          data: JSON.stringify({
            code: 'agent_stream.rehydrate_required',
            task_id: 'task-incomplete-41',
            cursor: 41,
            persistence_required: true,
            settled: false,
          }),
        }),
      )

      await waitFor(() => expect(handleDone).toHaveBeenCalledTimes(1))
      await waitFor(() =>
        expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith(
          expect.objectContaining({
            taskId: 'task-incomplete-41',
            after: '41',
          }),
        ),
      )
      expect(
        useInteractiveStore
          .getState()
          .storyStageRuns['/tmp/book:story-1:main']?.liveMessages.some((message) =>
            buildAgentMessageViews([message]).some((view) => view.kind === 'system' && view.content.includes('较早的实时轨迹已超出展示预算')),
          ),
      ).toBe(true)
      expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeEnabled()

      act(() =>
        resumed.enqueue({
          id: '42',
          event: 'chunk',
          data: JSON.stringify({ content: '恢复后的新正文。' }),
        }),
      )
      expect(await screen.findByText('恢复后的新正文。')).toBeInTheDocument()
      expect(handleDone).toHaveBeenCalledTimes(1)
    } finally {
      resumed.close()
    }
  })

  it('treats a settled omitted Game cycle that still requires persistence as failed and never auto-generates an image', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const canonicalSnapshot = {
      story_id: 'story-1',
      branch_id: 'main',
      state: {},
      turns: [
        {
          id: 'existing-turn',
          parent_id: null,
          branch_id: 'main',
          ts: '2026-07-23T00:00:00Z',
          user: '查看石门',
          narrative: '石门仍然紧闭。',
        },
      ],
    }
    const handleDone = vi.fn().mockResolvedValue(canonicalSnapshot)
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)

    try {
      render(<StoryStageHarness initialSnapshot={canonicalSnapshot} onDone={handleDone} />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))

      act(() => {
        stream.enqueue({
          id: '40',
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'start-settled-omission',
            delivery: 'start_turn',
            message: '推开石门',
            operation_id: 'operation-settled-omission',
            cycle: 1,
          }),
        })
        stream.enqueue({
          event: 'task_rehydrate_required',
          data: JSON.stringify({
            code: 'agent_stream.rehydrate_required',
            task_id: 'task-settled-omission',
            cursor: 41,
            settled: true,
            status: 'done',
            terminal_reason: '',
            persistence_required: true,
          }),
        })
      })

      expect(
        await screen.findByText('本轮没有收到持久化确认，已丢弃未落盘的正文并重新加载故事。请重试。'),
      ).toBeInTheDocument()
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.streaming).toBe(false),
      )
      expect(testMocks.generateInteractiveImageMock).not.toHaveBeenCalled()
      expect(streamActiveInteractiveChatMock).not.toHaveBeenCalled()
      expect(handleDone).toHaveBeenCalled()
    } finally {
      stream.close()
    }
  })

  it('keeps the current cycle persistence requirement across a stream reconnect', async () => {
    const user = userEvent.setup()
    const first = controllableInteractiveStream()
    const resumed = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(first.readable)
    streamActiveInteractiveChatMock.mockResolvedValue(resumed.readable)

    try {
      render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        first.enqueue({
          id: '31',
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'start-1',
            delivery: 'start_turn',
            message: '推开石门',
            operation_id: 'operation-1',
            cycle: 1,
          }),
        }),
      )
      getActiveInteractiveChatMock.mockResolvedValue({
        active: true,
        task_id: 'task-1',
        message: '推开石门',
        active_operation_id: 'operation-1',
        queue: [],
      })

      act(() => first.close())
      await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1))
      act(() => {
        resumed.enqueue({ event: 'done', data: '{}' })
        resumed.close()
      })

      expect(await screen.findByText(/没有收到持久化确认/)).toBeInTheDocument()
    } finally {
      first.close()
      resumed.close()
    }
  })

  it('opens only one recovery subscription during the React Strict Mode effect probe', async () => {
    const stream = controllableInteractiveStream()
    getActiveInteractiveChatMock.mockResolvedValue({
      active: true,
      status: 'running',
      task_id: 'task-1',
      story_id: 'story-1',
      branch_id: 'main',
      message: '检查石门',
    })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)

    try {
      render(
        <StrictMode>
          <StoryStageHarness />
        </StrictMode>,
      )
      await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1))
      expect(getActiveInteractiveChatMock).toHaveBeenCalledTimes(1)
    } finally {
      stream.close()
    }
  })

  it('reconnects to the active story stream after refresh without resubmitting the player message', async () => {
    const stream = controllableInteractiveStream()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    getActiveInteractiveChatMock.mockResolvedValue({
      active: true,
      status: 'running',
      task_id: 'task-1',
      story_id: 'story-1',
      branch_id: 'main',
      message: '推开石门',
      attachments: [{ id: 'att-1', name: '石门地图.png', media_type: 'image/png', size: 42 }],
    })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)

    try {
      render(<PersistedTurnHarness onDone={handleDone} />)

      await waitFor(() => {
        expect(getActiveInteractiveChatMock).toHaveBeenCalledWith('story-1', 'main')
        expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith({
          storyId: 'story-1',
          branchId: 'main',
          taskId: 'task-1',
          signal: expect.any(AbortSignal),
        })
      })
      expect(screen.getByText('推开石门')).toBeInTheDocument()
      expect(screen.getByText('石门地图.png')).toBeInTheDocument()
      expect(sendInteractiveMessageMock).not.toHaveBeenCalled()

      await act(async () => {
        stream.enqueue({
          event: 'thinking',
          data: JSON.stringify({ content: '正在回忆石门后的布局。' }),
        })
        await Promise.resolve()
      })
      await expectVisibleText('正在回忆石门后的布局。')

      await act(async () => {
        stream.enqueue({
          event: 'chunk',
          data: JSON.stringify({ content: '石门后亮起一盏灯。' }),
        })
        await Promise.resolve()
      })
      await waitFor(() => expect(screen.getByText('石门后亮起一盏灯。')).toBeInTheDocument())
      await waitFor(() => expect(screen.queryByText('正在回忆石门后的布局。')).not.toBeInTheDocument())
      expect(screen.getByRole('button', { name: /^执行过程$/ })).toHaveAttribute('aria-expanded', 'false')

      const persisted = persistedTurnEvent()
      persisted.turn.user = '推开石门'
      persisted.turn.narrative = '石门后亮起一盏灯。'
      await act(async () => {
        stream.enqueue({
          event: 'interactive_turn_persisted',
          data: JSON.stringify(persisted),
        })
        stream.enqueue({ event: 'done', data: '{}' })
        stream.close()
        await Promise.resolve()
      })

      await waitFor(() => expect(handleDone).toHaveBeenCalledWith({ silent: true }))
      expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
    } finally {
      stream.close()
    }
  })

  it('retries an unpersisted failed turn with the original player input', async () => {
    const user = userEvent.setup()
    const firstStream = controllableInteractiveStream()
    const retryStream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValueOnce(firstStream.readable).mockResolvedValueOnce(retryStream.readable)

    try {
      render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)
      await user.type(screen.getByPlaceholderText('你要做什么？'), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))

      act(() => {
        firstStream.enqueue({
          event: 'error',
          data: JSON.stringify({ message: '[NodeRunError] 400 Bad Request' }),
        })
        firstStream.close()
      })

      await user.click(await screen.findByRole('button', { name: '重新生成这一轮' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(2))
      expect(sendInteractiveMessageMock.mock.calls[1][0]).toMatchObject({
        message: '推开石门',
      })
    } finally {
      firstStream.close()
      retryStream.close()
    }
  })

  it('retries a failed persisted-turn regeneration against the same turn', async () => {
    const user = userEvent.setup()
    const firstStream = controllableInteractiveStream()
    const retryStream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValueOnce(firstStream.readable).mockResolvedValueOnce(retryStream.readable)

    try {
      render(<StoryStageHarness
        initialSnapshot={{
          story_id: 'story-1',
          branch_id: 'main',
          turns: [{
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '生成故事开场',
            narrative: '旧的故事开场。',
          }],
          state: {},
        }}
        onDone={vi.fn().mockResolvedValue(undefined)}
      />)

      await user.click(await screen.findByRole('button', { name: '重新生成这一轮' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      expect(sendInteractiveMessageMock.mock.calls[0][0]).toMatchObject({
        message: '生成故事开场',
        regenerate_from_turn_id: 'turn-1',
      })

      act(() => {
        firstStream.enqueue({
          event: 'error',
          data: JSON.stringify({ message: '[NodeRunError] 400 Bad Request' }),
        })
        firstStream.close()
      })

      await screen.findByText('[NodeRunError] 400 Bad Request')
      await user.click(await screen.findByRole('button', { name: '重新生成这一轮' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(2))
      expect(sendInteractiveMessageMock.mock.calls[1][0]).toMatchObject({
        message: '生成故事开场',
        regenerate_from_turn_id: 'turn-1',
      })
    } finally {
      firstStream.close()
      retryStream.close()
    }
  })

  it('reuses the initial command id when transport acceptance remains uncertain', async () => {
    const user = userEvent.setup()
    const retryStream = controllableInteractiveStream()
    const uncertain = Object.assign(new Error('HTTP 503'), { status: 503 })
    sendInteractiveMessageMock.mockRejectedValueOnce(uncertain).mockResolvedValueOnce(retryStream.readable)

    try {
      render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))

      await user.click(await screen.findByRole('button', { name: '重新生成这一轮' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(2))
      const first = sendInteractiveMessageMock.mock.calls[0][0]
      const retry = sendInteractiveMessageMock.mock.calls[1][0]
      expect(first.command_id).toEqual(expect.any(String))
      expect(retry.command_id).toBe(first.command_id)
    } finally {
      retryStream.close()
    }
  })

  it('discards optimistic narrative when done arrives without persistence confirmation', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<StoryStageHarness onDone={handleDone} />)
      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())
      act(() => {
        stream.enqueue({
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'start-1',
            delivery: 'start_turn',
            message: '继续前进',
            operation_id: 'operation-1',
            cycle: 1,
          }),
        })
        stream.enqueue({
          event: 'chunk',
          data: JSON.stringify({ content: '这段正文没有落盘。' }),
        })
        stream.enqueue({ event: 'done', data: '{}' })
        stream.close()
      })
      expect(await screen.findByText(/没有收到持久化确认/)).toBeInTheDocument()
      expect(screen.queryByText('这段正文没有落盘。')).not.toBeInTheDocument()
      await waitFor(() => expect(handleDone).toHaveBeenCalled())
    } finally {
      stream.close()
    }
  })
})
