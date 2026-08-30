import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useInteractiveStore } from '../stores/interactive-store'
import {
  PersistedTurnHarness,
  StoryStageHarness,
  controllableInteractiveStream,
  getStageInput,
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
  submitInteractiveAgentCommandMock,
} = testMocks

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
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

describe('StoryStage active runtime commands', () => {
  it('uses the contextual action to queue a follow-up and exposes manual steering', async () => {
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
      expect(screen.queryByRole('button', { name: /发送方式/ })).not.toBeInTheDocument()

      const input = screen.getByPlaceholderText('你要做什么？')
      expect(input).toHaveAttribute('contenteditable', 'true')
      await user.type(input, '再检查门后的脚印')
      const sendButton = screen.getByRole('button', { name: '发送' })
      expect(sendButton).toBeEnabled()
      expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeEnabled()
      await user.click(sendButton)

      await waitFor(() => expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith({
        type: 'follow_up',
        commandId: expect.any(String),
        targetOperationId: 'operation-1',
        storyId: 'story-1',
        branchId: 'main',
        input: { message: '再检查门后的脚印', styleScenes: [] },
      }))
      expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1)
      expect(input).toHaveTextContent('')
      expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeEnabled()
      expect(screen.getByText('再检查门后的脚印')).toBeInTheDocument()

      const followUp = submitInteractiveAgentCommandMock.mock.calls.find(([command]) => command.type === 'follow_up')?.[0]
      await user.click(screen.getByRole('button', { name: '立即转向' }))
      await waitFor(() => expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith({
        type: 'steer_queued',
        commandId: expect.any(String),
        targetOperationId: 'operation-1',
        targetCommandId: followUp.commandId,
        storyId: 'story-1',
        branchId: 'main',
      }))
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
      const input = getStageInput()
      const stopButton = screen.getByRole('button', { name: '中断 AI 执行' })
      await user.click(stopButton)
      await waitFor(() => expect(submitInteractiveAgentCommandMock).toHaveBeenCalledWith(expect.objectContaining({ type: 'abort' })))

      expect(stopButton).toBeDisabled()
      await user.type(input, '不能在中断后发送')
      expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeDisabled()
      expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
    } finally {
      stream.close()
    }
  })

})
