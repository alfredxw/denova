import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useInteractiveStore } from '../stores/interactive-store'
import {
  StoryStageHarness,
  controllableInteractiveStream,
  getStageInput,
  resetStoryStageTestHarness,
} from './story-stage/story-stage-test-harness'

const testMocks = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
  getActiveInteractiveChatMock: vi.fn(),
  recoverInteractiveAgentRuntimeMock: vi.fn(),
  runInteractiveDirectorMock: vi.fn(),
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
  submitInteractiveAgentCommandMock,
} = testMocks

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
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
  runInteractiveDirector: testMocks.runInteractiveDirectorMock,
  sendInteractiveMessage: testMocks.sendInteractiveMessageMock,
  streamActiveInteractiveChat: testMocks.streamActiveInteractiveChatMock,
  submitInteractiveAgentCommand: testMocks.submitInteractiveAgentCommandMock,
  switchInteractiveTurnVersion: vi.fn(),
  updateInteractiveTurnNarrative: testMocks.updateInteractiveTurnNarrativeMock,
}))

beforeEach(() => {
  resetStoryStageTestHarness(testMocks)
  recoverInteractiveAgentRuntimeMock.mockReset()
})

describe('StoryStage runtime recovery', () => {
  it('retries payload-free cold recovery with the exact projected game action', async () => {
    const stream = controllableInteractiveStream()
    const action = {
      kind: 'next_turn',
      command_id: 'accepted-next-turn',
      operation_id: 'operation-recovery',
    }
    const laterAction = {
      kind: 'follow_up',
      command_id: 'accepted-follow-up',
      operation_id: 'operation-recovery',
    }
    getActiveInteractiveChatMock.mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [action, laterAction],
    })
    recoverInteractiveAgentRuntimeMock.mockRejectedValueOnce(new TypeError('connection reset')).mockResolvedValueOnce({
      task_id: 'recovery-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 9,
      replayed: true,
      recovery_action: action,
    })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)

    try {
      render(<StoryStageHarness />)
      await waitFor(() => expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledTimes(1))
      expect(await screen.findByText('正在从持久化状态恢复已接受的游戏 Agent 运行…')).toBeInTheDocument()

      act(() => window.dispatchEvent(new Event('online')))

      await waitFor(() => expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledTimes(2))
      expect(recoverInteractiveAgentRuntimeMock.mock.calls).toEqual([
        [{ storyId: 'story-1', branchId: 'main', action }],
        [{ storyId: 'story-1', branchId: 'main', action }],
      ])
      expect(recoverInteractiveAgentRuntimeMock.mock.calls.flat()).not.toContain(laterAction)
      expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith({
        storyId: 'story-1',
        branchId: 'main',
        taskId: 'recovery-task-1',
        signal: expect.any(AbortSignal),
      })
      expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
      expect(JSON.stringify(recoverInteractiveAgentRuntimeMock.mock.calls)).not.toContain('message')
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime.recoveryPaused).toBe(false),
      )
      act(() =>
        stream.enqueue({
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'accepted-next-turn',
            delivery: 'next_turn',
            message: 'server-restored input',
            operation_id: 'operation-recovery',
            cycle: 2,
          }),
        }),
      )
      await waitFor(() => expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeEnabled())
    } finally {
      stream.close()
    }
  })

  it('advances the next paused game action on the same recovery task and stream', async () => {
    const stream = controllableInteractiveStream()
    const firstAction = {
      kind: 'next_turn',
      command_id: 'accepted-next-turn',
      operation_id: 'operation-first',
    }
    const secondAction = {
      kind: 'follow_up',
      command_id: 'accepted-follow-up',
      operation_id: 'operation-second',
    }
    getActiveInteractiveChatMock
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
        active: true,
        phase: 'running',
        task_id: 'shared-recovery-task',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-second',
        recovery_actions: [secondAction],
      })
    recoverInteractiveAgentRuntimeMock
      .mockResolvedValueOnce({
        task_id: 'shared-recovery-task',
        status: 'running',
        stream_cursor: 0,
        cursor: 31,
        replayed: false,
        recovery_action: firstAction,
      })
      .mockResolvedValueOnce({
        task_id: 'shared-recovery-task',
        status: 'running',
        stream_cursor: 0,
        cursor: 32,
        replayed: false,
        recovery_action: secondAction,
      })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
    const { unmount } = render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)

    try {
      await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1))
      act(() => {
        stream.enqueue({
          event: 'runtime_recovery_required',
          data: JSON.stringify({
            code: 'agent_runtime.recovery_required',
            message: 'runtime paused on accepted input',
          }),
        })
      })

      await waitFor(() => expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledTimes(2))
      expect(recoverInteractiveAgentRuntimeMock.mock.calls).toEqual([
        [{ storyId: 'story-1', branchId: 'main', action: firstAction }],
        [{ storyId: 'story-1', branchId: 'main', action: secondAction }],
      ])
      expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1)
      expect(streamActiveInteractiveChatMock.mock.calls[0][0].taskId).toBe('shared-recovery-task')
      expect(screen.queryByText('runtime paused on accepted input')).not.toBeInTheDocument()
    } finally {
      stream.close()
      unmount()
    }
  })

  it('keeps the game stream healthy when another tab advances the recovery projection', async () => {
    const stream = controllableInteractiveStream()
    const firstAction = {
      kind: 'next_turn',
      command_id: 'accepted-next-turn',
      operation_id: 'operation-first',
    }
    const racedAction = {
      kind: 'follow_up',
      command_id: 'accepted-follow-up',
      operation_id: 'operation-raced',
    }
    getActiveInteractiveChatMock
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
        active: true,
        phase: 'running',
        task_id: 'shared-recovery-task',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-raced',
        recovery_actions: [racedAction],
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
    recoverInteractiveAgentRuntimeMock
      .mockResolvedValueOnce({
        task_id: 'shared-recovery-task',
        status: 'running',
        stream_cursor: 0,
        cursor: 33,
        replayed: false,
        recovery_action: firstAction,
      })
      .mockRejectedValueOnce(
        Object.assign(new Error('projection changed'), {
          code: 'agent_runtime.recovery_changed',
          status: 409,
        }),
      )
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
    const { unmount } = render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)

    try {
      await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
          event: 'runtime_recovery_required',
          data: JSON.stringify({ code: 'agent_runtime.recovery_required' }),
        }),
      )

      await waitFor(() => expect(getActiveInteractiveChatMock).toHaveBeenCalledTimes(3))
      expect(recoverInteractiveAgentRuntimeMock.mock.calls).toEqual([
        [{ storyId: 'story-1', branchId: 'main', action: firstAction }],
        [{ storyId: 'story-1', branchId: 'main', action: racedAction }],
      ])
      expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1)
      expect(screen.queryByText('projection changed')).not.toBeInTheDocument()
    } finally {
      stream.close()
      unmount()
    }
  })

  it('hands off from a finished old game task to the new recovery task', async () => {
    const oldStream = controllableInteractiveStream()
    const newStream = controllableInteractiveStream()
    const action = {
      kind: 'next_turn',
      command_id: 'accepted-next-turn',
      operation_id: 'operation-next',
    }
    getActiveInteractiveChatMock
      .mockResolvedValueOnce({
        active: true,
        phase: 'running',
        task_id: 'finished-old-task',
        stream_attached: true,
        active_operation_id: 'operation-old',
      })
      .mockResolvedValueOnce({
        active: false,
        phase: 'running',
        task_id: 'finished-old-task',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-next',
        recovery_actions: [action],
      })
    recoverInteractiveAgentRuntimeMock.mockResolvedValue({
      task_id: 'new-recovery-task',
      status: 'running',
      stream_cursor: 0,
      cursor: 34,
      replayed: false,
      recovery_action: action,
    })
    streamActiveInteractiveChatMock.mockResolvedValueOnce(oldStream.readable).mockResolvedValueOnce(newStream.readable)
    const { unmount } = render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)

    try {
      await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1))
      act(() =>
        oldStream.enqueue({
          event: 'runtime_recovery_required',
          data: JSON.stringify({ code: 'agent_runtime.recovery_required' }),
        }),
      )

      await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(2))
      expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledWith({
        storyId: 'story-1',
        branchId: 'main',
        action,
      })
      expect(streamActiveInteractiveChatMock.mock.calls.map((call) => call[0].taskId)).toEqual(['finished-old-task', 'new-recovery-task'])
      expect(screen.queryByText(/运行已变化/)).not.toBeInTheDocument()
    } finally {
      newStream.close()
      unmount()
    }
  })

  it('keeps attach-only game recovery paused so Stop uses the projected abort action', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const attachAction = {
      kind: 'start_turn',
      command_id: 'recovery-attach-1',
      operation_id: 'operation-recovery',
    }
    const abortAction = {
      kind: 'abort',
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    getActiveInteractiveChatMock.mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, abortAction],
    })
    recoverInteractiveAgentRuntimeMock
      .mockResolvedValueOnce({
        task_id: 'attach-task-1',
        status: 'running',
        stream_cursor: 0,
        cursor: 11,
        replayed: false,
        recovery_action: attachAction,
      })
      .mockResolvedValueOnce({
        task_id: 'abort-task-1',
        status: 'running',
        stream_cursor: 0,
        cursor: 12,
        replayed: false,
        recovery_action: abortAction,
      })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
    const { unmount } = render(<StoryStageHarness />)

    try {
      await waitFor(() =>
        expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith({
          storyId: 'story-1',
          branchId: 'main',
          taskId: 'attach-task-1',
          signal: expect.any(AbortSignal),
        }),
      )
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime).toMatchObject({
          recoveryPaused: true,
          recoveryAbortAvailable: true,
        }),
      )
      const stopButton = screen.getByRole('button', { name: '中断 AI 执行' })
      expect(stopButton).toBeEnabled()

      await user.click(stopButton)

      await waitFor(() =>
        expect(recoverInteractiveAgentRuntimeMock.mock.calls).toEqual([
          [{ storyId: 'story-1', branchId: 'main', action: attachAction }],
          [{ storyId: 'story-1', branchId: 'main', action: abortAction }],
        ]),
      )
      expect(submitInteractiveAgentCommandMock).not.toHaveBeenCalled()
      expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
    } finally {
      stream.close()
      unmount()
    }
  })

  it('attaches an existing game recovery task without posting start_turn again', async () => {
    const stream = controllableInteractiveStream()
    const attachAction = {
      kind: 'start_turn',
      command_id: 'already-attached-start',
      operation_id: 'operation-recovery',
    }
    const abortAction = {
      kind: 'abort',
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    getActiveInteractiveChatMock.mockResolvedValue({
      active: false,
      phase: 'running',
      task_id: 'already-attached-task',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, abortAction],
    })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
    const { unmount } = render(<StoryStageHarness />)

    try {
      await waitFor(() =>
        expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith({
          storyId: 'story-1',
          branchId: 'main',
          taskId: 'already-attached-task',
          signal: expect.any(AbortSignal),
        }),
      )
      expect(recoverInteractiveAgentRuntimeMock).not.toHaveBeenCalled()
      expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime).toMatchObject({
        recoveryPaused: true,
        recoveryAbortAvailable: true,
      })
    } finally {
      stream.close()
      unmount()
    }
  })

  it('submits only the head state action for an attached game recovery task', async () => {
    const stream = controllableInteractiveStream()
    const attachAction = {
      kind: 'start_turn',
      command_id: 'already-attached-start',
      operation_id: 'operation-recovery',
    }
    const stateAction = {
      kind: 'follow_up',
      command_id: 'accepted-follow-up',
      operation_id: 'operation-recovery',
    }
    const laterAction = {
      kind: 'next_turn',
      command_id: 'accepted-next-turn',
      operation_id: 'operation-next',
    }
    getActiveInteractiveChatMock.mockResolvedValue({
      active: false,
      phase: 'running',
      task_id: 'already-attached-task',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: true,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, stateAction, laterAction],
    })
    recoverInteractiveAgentRuntimeMock.mockResolvedValue({
      task_id: 'already-attached-task',
      status: 'running',
      stream_cursor: 0,
      cursor: 23,
      replayed: false,
      recovery_action: stateAction,
    })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
    const { unmount } = render(<StoryStageHarness />)

    try {
      await waitFor(() =>
        expect(streamActiveInteractiveChatMock).toHaveBeenCalledWith({
          storyId: 'story-1',
          branchId: 'main',
          taskId: 'already-attached-task',
          signal: expect.any(AbortSignal),
        }),
      )
      expect(recoverInteractiveAgentRuntimeMock.mock.calls).toEqual([[{ storyId: 'story-1', branchId: 'main', action: stateAction }]])
      expect(recoverInteractiveAgentRuntimeMock.mock.calls.flat()).not.toContain(attachAction)
      expect(recoverInteractiveAgentRuntimeMock.mock.calls.flat()).not.toContain(laterAction)
      expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime.recoveryPaused).toBe(false)
    } finally {
      stream.close()
      unmount()
    }
  })

  it.each(['compact_context', 'remove_compaction'] as const)(
    'accepts terminal %s recovery without a persisted interactive turn',
    async (kind) => {
      const stream = controllableInteractiveStream()
      const handleDone = vi.fn().mockResolvedValue(undefined)
      const action = {
        kind,
        command_id: `${kind}-command`,
        operation_id: 'operation-structural',
      }
      getActiveInteractiveChatMock.mockResolvedValue({
        active: false,
        phase: 'compacting',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: false,
        active_operation_id: 'operation-structural',
        recovery_actions: [action],
      })
      recoverInteractiveAgentRuntimeMock.mockResolvedValue({
        task_id: `${kind}-task`,
        status: 'running',
        stream_cursor: 0,
        cursor: 24,
        replayed: false,
        recovery_action: action,
      })
      streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
      const { unmount } = render(<StoryStageHarness onDone={handleDone} />)

      try {
        await waitFor(() => expect(streamActiveInteractiveChatMock).toHaveBeenCalledTimes(1))
        act(() => {
          stream.enqueue({
            event: 'context_compaction',
            data: JSON.stringify({ status: 'completed' }),
          })
          stream.enqueue({ event: 'done', data: '{}' })
          stream.close()
        })

        await waitFor(() => expect(handleDone).toHaveBeenCalledTimes(1))
        expect(screen.queryByText(/没有收到持久化确认/)).not.toBeInTheDocument()
        expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
      } finally {
        stream.close()
        unmount()
      }
    },
  )

  it('lets a fresh command resume an attach-only game operation without starting a new root turn', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const attachAction = {
      kind: 'start_turn',
      command_id: 'recovery-attach-1',
      operation_id: 'operation-recovery',
    }
    const abortAction = {
      kind: 'abort',
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    getActiveInteractiveChatMock.mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [attachAction, abortAction],
    })
    recoverInteractiveAgentRuntimeMock.mockResolvedValue({
      task_id: 'attach-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 13,
      replayed: false,
      recovery_action: attachAction,
    })
    submitInteractiveAgentCommandMock.mockResolvedValue({
      command_id: 'fresh-follow-up',
      operation_id: 'operation-recovery',
      cursor: 14,
    })
    streamActiveInteractiveChatMock.mockResolvedValue(stream.readable)
    const { unmount } = render(<StoryStageHarness />)

    try {
      await waitFor(() => expect(screen.getByRole('button', { name: '发送方式：追加' })).toBeEnabled())
      await user.type(getStageInput(), '采用新的恢复方向')
      await user.click(screen.getByRole('button', { name: '发送' }))

      await waitFor(() =>
        expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith({
          type: 'follow_up',
          commandId: expect.any(String),
          targetOperationId: 'operation-recovery',
          storyId: 'story-1',
          branchId: 'main',
          message: '采用新的恢复方向',
          styleScenes: [],
        }),
      )
      expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledTimes(1)
      expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledWith({
        storyId: 'story-1',
        branchId: 'main',
        action: attachAction,
      })
      expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
      expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime).toMatchObject({
        recoveryPaused: false,
        recoveryAbortAvailable: false,
      })
    } finally {
      stream.close()
      unmount()
    }
  })

  it('keeps projected recovery abort out of auto-resume and uses it only when Stop is clicked', async () => {
    const user = userEvent.setup()
    const abortAction = {
      kind: 'abort',
      command_id: 'recovery-abort-1',
      operation_id: 'operation-recovery',
    }
    getActiveInteractiveChatMock.mockResolvedValue({
      active: false,
      phase: 'running',
      recovery_paused: true,
      runtime_recoverable: true,
      stream_attached: false,
      active_operation_id: 'operation-recovery',
      recovery_actions: [abortAction],
    })
    recoverInteractiveAgentRuntimeMock.mockResolvedValue({
      task_id: 'abort-task-1',
      status: 'running',
      stream_cursor: 0,
      cursor: 10,
      replayed: false,
      recovery_action: abortAction,
    })
    const { unmount } = render(<StoryStageHarness />)

    try {
      const stopButton = await screen.findByRole('button', {
        name: '中断 AI 执行',
      })
      await waitFor(() => expect(stopButton).toBeEnabled())
      expect(recoverInteractiveAgentRuntimeMock).not.toHaveBeenCalled()

      await user.click(stopButton)

      await waitFor(() =>
        expect(recoverInteractiveAgentRuntimeMock).toHaveBeenCalledWith({
          storyId: 'story-1',
          branchId: 'main',
          action: abortAction,
        }),
      )
      expect(submitInteractiveAgentCommandMock).not.toHaveBeenCalled()
      expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
    } finally {
      unmount()
    }
  })

})
