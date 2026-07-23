import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useInteractiveStore } from '../stores/interactive-store'
import {
  PersistedTurnHarness,
  StoryStageHarness,
  controllableInteractiveStream,
  deferred,
  getStageInput,
  persistedTurnEvent,
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

describe('StoryStage active runtime commands', () => {
  it('keeps the composer active and queues a replayable follow-up on the current operation', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)

    try {
      render(<PersistedTurnHarness onDone={vi.fn().mockResolvedValue(undefined)} />)
      await user.type(screen.getByPlaceholderText('你要做什么？'), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
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
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime.operationId).toBe('operation-1'),
      )
      expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '发送方式：追加' })).toBeInTheDocument()

      const input = screen.getByPlaceholderText('你要做什么？')
      expect(input).toHaveAttribute('contenteditable', 'true')
      await user.type(input, '再检查门后的脚印')
      await user.click(screen.getByRole('button', { name: '发送' }))

      await waitFor(() =>
        expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith({
          type: 'follow_up',
          commandId: expect.any(String),
          targetOperationId: 'operation-1',
          storyId: 'story-1',
          branchId: 'main',
          message: '再检查门后的脚印',
          styleScenes: [],
        }),
      )
      expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1)
      expect(input).toHaveTextContent('')

      act(() => {
        stream.enqueue({
          event: 'interactive_turn_persisted',
          data: JSON.stringify(persistedTurnEvent()),
        })
        stream.enqueue({
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'command-1',
            delivery: 'follow_up',
            message: '再检查门后的脚印',
            operation_id: 'operation-1',
            cycle: 2,
          }),
        })
        stream.enqueue({
          event: 'chunk',
          data: JSON.stringify({ content: '泥地上留下了新鲜足迹。' }),
        })
      })
      expect(await screen.findByText('再检查门后的脚印')).toBeInTheDocument()
      expect(await screen.findByText('泥地上留下了新鲜足迹。')).toBeInTheDocument()
    } finally {
      stream.close()
    }
  })

  it('submits a targeted abort without tearing down the observation stream early', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)

    try {
      render(<StoryStageHarness />)
      await user.type(screen.getByPlaceholderText('你要做什么？'), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
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
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime.operationId).toBe('operation-1'),
      )
      await user.click(screen.getByRole('button', { name: '中断 AI 执行' }))

      await waitFor(() =>
        expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith({
          type: 'abort',
          commandId: expect.any(String),
          targetOperationId: 'operation-1',
          storyId: 'story-1',
          branchId: 'main',
          reason: 'user_requested',
        }),
      )
      act(() =>
        stream.enqueue({
          event: 'chunk',
          data: JSON.stringify({ content: '中止确认前仍可收到尾部输出。' }),
        }),
      )
      expect(await screen.findByText('中止确认前仍可收到尾部输出。')).toBeInTheDocument()
    } finally {
      stream.close()
    }
  })

  it('targets the operation projected by the stream instead of a newer operation returned at click time', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    submitInteractiveAgentCommandMock.mockResolvedValue({
      command_id: 'follow-visible',
      operation_id: 'operation-visible',
      cursor: 9,
    })

    try {
      render(<StoryStageHarness />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
          event: 'agent_cycle_started',
          data: JSON.stringify({
            command_id: 'start-visible',
            delivery: 'start_turn',
            message: '推开石门',
            operation_id: 'operation-visible',
            cycle: 1,
          }),
        }),
      )
      await waitFor(() =>
        expect(useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.runtime.operationId).toBe('operation-visible'),
      )
      getActiveInteractiveChatMock.mockClear()
      getActiveInteractiveChatMock.mockResolvedValue({
        active: true,
        active_operation_id: 'operation-newer',
        queue: [],
      })

      await user.type(getStageInput(), '继续当前画面')
      await user.click(screen.getByRole('button', { name: '发送' }))

      await waitFor(() =>
        expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith(
          expect.objectContaining({
            targetOperationId: 'operation-visible',
            message: '继续当前画面',
          }),
        ),
      )
      expect(getActiveInteractiveChatMock).not.toHaveBeenCalled()
    } finally {
      stream.close()
    }
  })

  it('disables abort and send controls after an abort receipt until settlement', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    getActiveInteractiveChatMock.mockResolvedValue({
      active: true,
      active_operation_id: 'operation-1',
      queue: [],
    })
    submitInteractiveAgentCommandMock.mockResolvedValue({
      command_id: 'abort-1',
      operation_id: 'operation-1',
      cursor: 9,
    })

    try {
      render(<StoryStageHarness />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
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
      await user.type(getStageInput(), '不能在中断后发送')

      const stopButton = screen.getByRole('button', { name: '中断 AI 执行' })
      await user.click(stopButton)
      await waitFor(() => expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith(expect.objectContaining({ type: 'abort' })))

      expect(stopButton).toBeDisabled()
      expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
    } finally {
      stream.close()
    }
  })

  it('submits only one active command while its receipt is pending', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const receipt = deferred<{
      command_id: string
      operation_id: string
      cursor: number
    }>()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    getActiveInteractiveChatMock.mockResolvedValue({
      active: true,
      active_operation_id: 'operation-1',
      queue: [],
    })
    submitInteractiveAgentCommandMock.mockReturnValue(receipt.promise)

    try {
      render(<StoryStageHarness />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
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
      await user.type(getStageInput(), '只追加一次')
      const sendButton = screen.getByRole('button', { name: '发送' })

      fireEvent.click(sendButton)
      fireEvent.click(sendButton)

      expect(submitInteractiveAgentCommandMock).toHaveBeenCalledTimes(1)
      expect(sendButton).toBeDisabled()
      receipt.resolve({
        command_id: 'follow-1',
        operation_id: 'operation-1',
        cursor: 9,
      })
      await waitFor(() => expect(getStageInput()).toHaveTextContent(''))
    } finally {
      stream.close()
    }
  })

  it('reuses the command id when retrying the same game command after an uncertain response', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    submitInteractiveAgentCommandMock.mockRejectedValueOnce(new TypeError('connection reset')).mockResolvedValueOnce({
      command_id: 'accepted',
      operation_id: 'operation-1',
      cursor: 9,
    })

    try {
      render(<StoryStageHarness />)
      await user.type(getStageInput(), '推开石门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))
      act(() =>
        stream.enqueue({
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
      await user.type(getStageInput(), '同一条追加')

      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(submitInteractiveAgentCommandMock).toHaveBeenCalledTimes(1))
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(submitInteractiveAgentCommandMock).toHaveBeenCalledTimes(2))

      const firstCommandID = submitInteractiveAgentCommandMock.mock.calls[0][0].commandId
      const retryCommandID = submitInteractiveAgentCommandMock.mock.calls[1][0].commandId
      expect(retryCommandID).toBe(firstCommandID)
    } finally {
      stream.close()
    }
  })

})
