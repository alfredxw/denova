import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { buildAgentMessageViews } from '@/lib/agent-message-view'
import { StoryStage as ProjectStoryStage } from './StoryStage'

function StoryStage(props: Omit<ComponentProps<typeof ProjectStoryStage>, 'projectId'> & { projectId?: string }) {
  return <ProjectStoryStage {...props} projectId={props.projectId || 'project-story'} />
}
import { useInteractiveStore } from '../stores/interactive-store'
import type { Snapshot, TurnEvent } from '../types'
import {
  PersistedTurnHarness,
  StoryStageHarness,
  controllableInteractiveStream,
  deferred,
  expectVisibleText,
  interactiveStream,
  persistedTurnEvent,
  resetStoryStageTestHarness,
  runAnimationFrames,
  story,
} from './story-stage/story-stage-test-harness'

const testMocks = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
  getActiveInteractiveChatMock: vi.fn(),
  runInteractiveDirectorMock: vi.fn(),
  sendInteractiveMessageMock: vi.fn(),
  streamActiveInteractiveChatMock: vi.fn(),
  submitInteractiveAgentCommandMock: vi.fn(),
  updateInteractiveTurnNarrativeMock: vi.fn(),
  useSkillCommandsMock: vi.fn(),
}))

const { sendInteractiveMessageMock } = testMocks

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
  runInteractiveDirector: testMocks.runInteractiveDirectorMock,
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

beforeEach(() => resetStoryStageTestHarness(testMocks))

describe('StoryStage streaming rendering', () => {
  it('batches fast interactive chunks into one frame and renders one complete text tree', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const originalRequestAnimationFrame = window.requestAnimationFrame
    const originalCancelAnimationFrame = window.cancelAnimationFrame
    const frames = new Map<number, FrameRequestCallback>()
    let nextFrameId = 1
    window.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      const id = nextFrameId
      nextFrameId += 1
      frames.set(id, callback)
      return id
    })
    window.cancelAnimationFrame = vi.fn((id: number) => {
      frames.delete(id)
    })

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      const { container } = render(<StoryStageHarness />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))

      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())
      act(() => runAnimationFrames(frames))
      stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '青石镇外' }) })
      stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '风声忽然停了。' }) })
      await waitFor(() => expect(frames.size).toBeGreaterThan(0))
      expect(screen.queryByText('青石镇外风声忽然停了。')).not.toBeInTheDocument()

      act(() => runAnimationFrames(frames))

      expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()
      expect(await screen.findByText('青石镇外风声忽然停了。')).toBeInTheDocument()

      act(() => runAnimationFrames(frames))

			expect(screen.getByText('青石镇外风声忽然停了。')).toBeInTheDocument()
			stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
      stream.enqueue({ event: 'done', data: '{}' })
      stream.close()
    } finally {
      stream.close()
      window.requestAnimationFrame = originalRequestAnimationFrame
      window.cancelAnimationFrame = originalCancelAnimationFrame
    }
  })

  it('renders live thinking from one complete text tree after the batched frame', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const originalRequestAnimationFrame = window.requestAnimationFrame
    const originalCancelAnimationFrame = window.cancelAnimationFrame
    const frames = new Map<number, FrameRequestCallback>()
    let nextFrameId = 1
    window.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      const id = nextFrameId
      nextFrameId += 1
      frames.set(id, callback)
      return id
    })
    window.cancelAnimationFrame = vi.fn((id: number) => {
      frames.delete(id)
    })

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      const { container } = render(<StoryStageHarness />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))

      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())
      act(() => runAnimationFrames(frames))
      stream.enqueue({ event: 'thinking', data: JSON.stringify({ content: '正在检查门后的动静。' }) })
      await waitFor(() => expect(frames.size).toBeGreaterThan(0))

      act(() => runAnimationFrames(frames))

      expect(container.querySelector('.nova-streaming-content-stage')).toBeNull()
      expect(await screen.findByText('正在检查门后的动静。')).toBeInTheDocument()

      act(() => runAnimationFrames(frames))

      expect(screen.getByText('正在检查门后的动静。')).toBeInTheDocument()
    } finally {
      stream.close()
      window.requestAnimationFrame = originalRequestAnimationFrame
      window.cancelAnimationFrame = originalCancelAnimationFrame
    }
  })

  it('separates live narrative from the completed thinking disclosure as soon as prose starts', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const providerThinking = `正在判断门后的声响。${'继续核对现场线索。'.repeat(300)}供应商思考尾部必须完整展示。`

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<StoryStageHarness />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      act(() => {
        stream.enqueue({ event: 'thinking', data: JSON.stringify({ content: providerThinking }) })
      })
      await expectVisibleText(providerThinking)

      act(() => {
        stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '门后传来脚步声。' }) })
      })

      await waitFor(() => expect(screen.getByText('门后传来脚步声。')).toBeInTheDocument())
      await waitFor(() => expect(screen.queryByText(providerThinking)).not.toBeInTheDocument())
      const trace = screen.getByRole('button', { name: /^执行过程$/ })
      expect(trace).toHaveAttribute('aria-expanded', 'false')

      await user.click(trace)
      expect(screen.getByText(providerThinking)).toBeInTheDocument()
      expect(screen.getByText('门后传来脚步声。')).toBeInTheDocument()
      const liveMessages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
      expect(buildAgentMessageViews(liveMessages).find((view) => view.kind === 'assistant')).toMatchObject({
        content: '门后传来脚步声。',
        streaming: true,
      })
      expect(liveMessages.find((message) => message.role === 'assistant')?.metadata).not.toHaveProperty('streaming_target_content')
    } finally {
      stream.close()
    }
  })

  it('moves a streamed tool preamble from narrative into thinking immediately', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<StoryStageHarness />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      act(() => {
        stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '我先检查资料，再开始写正文。' }) })
      })
      await waitFor(() => {
        const liveMessages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
        expect(buildAgentMessageViews(liveMessages).some((view) => view.kind === 'assistant' && view.content === '我先检查资料，再开始写正文。')).toBe(true)
      })
      expect(screen.queryByRole('button', { name: /执行过程/ })).not.toBeInTheDocument()

      act(() => {
        stream.enqueue({ event: 'interactive_content_reclassified', data: JSON.stringify({ content: '我先检查资料，再开始写正文。' }) })
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ id: 'call-lore', name: 'list_lore_items', args: '{}' }) })
      })

      const trace = await screen.findByRole('button', { name: /正在执行.*1 次工具调用/ })
      expect(trace).toBeInTheDocument()
      expect(screen.getAllByText('我先检查资料，再开始写正文。')).toHaveLength(1)
      const liveMessages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
      expect(buildAgentMessageViews(liveMessages).some((view) => view.kind === 'assistant' && view.content)).toBe(false)
    } finally {
      stream.close()
    }
  })

  it('groups live thinking and tool calls into one trace block and collapses them after completion', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const refresh = deferred<Snapshot | void>()
    const handleDone = vi.fn(() => refresh.promise)

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(
        <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
          <StoryStage
            workspace="/tmp/book"
            stories={[story()]}
            story={story()}
            tellers={[]}
            storyId="story-1"
            branchId="main"
            snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
            onDone={handleDone}
          />
        </VirtuosoMockContext.Provider>,
      )

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      act(() => {
        stream.enqueue({ event: 'thinking', data: JSON.stringify({ content: '正在检查开场资料。' }) })
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ id: 'call-lore', name: 'list_lore_items', args: '{}' }) })
      })

      expect(await screen.findByRole('button', { name: /正在执行.*1 次工具调用/ })).toBeInTheDocument()
      expect(screen.getByText('正在检查开场资料。')).toBeInTheDocument()
      expect(screen.getByText('list_lore_items')).toBeInTheDocument()

      act(() => {
        stream.enqueue({ event: 'tool_result', data: JSON.stringify({ id: 'call-lore', name: 'list_lore_items', content: '找到 3 条资料' }) })
      })

      await waitFor(() => expect(screen.getByText('正在检查开场资料。')).toBeInTheDocument())
      expect(screen.getByText('list_lore_items')).toBeInTheDocument()

		act(() => {
        stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '门外有灯。' }) })
			})

		act(() => {
				stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
        stream.enqueue({ event: 'done', data: '{}' })
        stream.close()
      })

      await waitFor(() => expect(handleDone).toHaveBeenCalled())
      await waitFor(() => expect(screen.queryByText('正在检查开场资料。')).not.toBeInTheDocument())
      expect(screen.queryByText('list_lore_items')).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /执行过程.*1 次工具调用/ }))
      expect(screen.getByText('正在检查开场资料。')).toBeInTheDocument()
      expect(screen.getByText('list_lore_items')).toBeInTheDocument()
    } finally {
      refresh.resolve(undefined)
      stream.close()
    }
  })

  it('keeps background Director events out of the Game Agent timeline', async () => {
    const user = userEvent.setup()
    const turn: TurnEvent = {
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-07-11T00:00:00Z',
      user: '推开石门',
      narrative: '石门后传来锁链拖地的声音。',
      display_events: [
        {
          id: 'game-thinking',
          role: 'thinking' as const,
          content: '正在判断石门后的威胁。',
          agent_kind: 'interactive_story',
        },
        {
          id: 'game-tool',
          role: 'tool_call' as const,
          name: 'list_lore_items',
          content: 'list_lore_items',
          status: 'success',
          agent_kind: 'interactive_story',
        },
        {
          id: 'director-thinking',
          role: 'thinking' as const,
          content: '正在重新安排后续分支。',
          agent_kind: 'interactive_director',
        },
        {
          id: 'director-write',
          role: 'tool_call' as const,
          name: 'write',
          content: 'write',
          args: '{"path":"director.md"}',
          status: 'success',
          agent_kind: 'interactive_director',
        },
      ],
    }

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [turn], current_turn: turn, state: {} }}
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(screen.getByText('石门后传来锁链拖地的声音。')).toBeInTheDocument()
    const traceButton = screen.getByRole('button', { name: /执行过程.*1 次工具调用/ })
    await user.click(traceButton)
    expect(screen.getByText('正在判断石门后的威胁。')).toBeInTheDocument()
    expect(screen.getByText('list_lore_items')).toBeInTheDocument()
    expect(screen.queryByText('正在重新安排后续分支。')).not.toBeInTheDocument()
    expect(screen.queryByText('write')).not.toBeInTheDocument()
  })

  it('folds submission tool cards after the narrative into one collapsed trace group when the turn has a narrative anchor', async () => {
    const user = userEvent.setup()
    const turn: TurnEvent = {
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-07-11T00:00:00Z',
      user: '推开石门',
      narrative: '石门后传来锁链拖地的声音。',
      display_events: [
        {
          id: 'game-thinking',
          role: 'thinking' as const,
          content: '正在判断石门后的威胁。',
          agent_kind: 'interactive_story',
        },
        {
          id: 'narrative-anchor',
          role: 'narrative' as const,
        },
        {
          id: 'submit-patches',
          role: 'tool_call' as const,
          name: 'submit_actor_state_patches',
          content: 'submit_actor_state_patches',
          status: 'success' as const,
          agent_kind: 'interactive_story',
        },
        {
          id: 'submit-choices',
          role: 'tool_call' as const,
          name: 'submit_choices',
          content: 'submit_choices',
          status: 'success' as const,
          agent_kind: 'interactive_story',
        },
      ],
    }

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [turn], current_turn: turn, state: {} }}
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    const narrative = screen.getByText('石门后传来锁链拖地的声音。')
    expect(narrative).toBeInTheDocument()
    // 正文之后的提交结果工具统一折叠为一个分组，不再逐张卡片交叉展示
    expect(screen.queryByText('submit_actor_state_patches')).not.toBeInTheDocument()
    expect(screen.queryByText('submit_choices')).not.toBeInTheDocument()
    const postNarrativeGroup = screen.getByRole('button', { name: /^执行过程 · 2 次工具调用$/ })
    expect(narrative.compareDocumentPosition(postNarrativeGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 正文前的思考分组只包含思考内容，不吞掉提交结果工具
    const preNarrativeGroup = screen.getByRole('button', { name: /^执行过程$/ })
    expect(preNarrativeGroup.compareDocumentPosition(narrative) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 展开正文后的分组可以看到全部提交工具
    await user.click(postNarrativeGroup)
    expect(screen.getByText('submit_actor_state_patches')).toBeInTheDocument()
    expect(screen.getByText('submit_choices')).toBeInTheDocument()
  })

  it('keeps submission tools inside the trace group for turns persisted without a narrative anchor', async () => {
    const turn: TurnEvent = {
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-07-11T00:00:00Z',
      user: '推开石门',
      narrative: '石门后传来锁链拖地的声音。',
      display_events: [
        {
          id: 'game-thinking',
          role: 'thinking' as const,
          content: '正在判断石门后的威胁。',
          agent_kind: 'interactive_story',
        },
        {
          id: 'submit-patches',
          role: 'tool_call' as const,
          name: 'submit_actor_state_patches',
          content: 'submit_actor_state_patches',
          status: 'success' as const,
          agent_kind: 'interactive_story',
        },
        {
          id: 'submit-choices',
          role: 'tool_call' as const,
          name: 'submit_choices',
          content: 'submit_choices',
          status: 'success' as const,
          agent_kind: 'interactive_story',
        },
      ],
    }

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [turn], current_turn: turn, state: {} }}
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(screen.getByText('石门后传来锁链拖地的声音。')).toBeInTheDocument()
    // 旧数据没有锚点：保持旧布局，提交结果工具仍在执行过程折叠分组内（默认折叠不渲染）
    expect(screen.getByRole('button', { name: /执行过程.*2 次工具调用/ })).toBeInTheDocument()
    expect(screen.queryByText('submit_choices')).not.toBeInTheDocument()
  })

  it('updates a live tool card when an index-based call later receives an id', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const completeArgs = JSON.stringify({ command: `printf '${'工具输入'.repeat(9000)}尾部必须完整展示'` })

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<StoryStageHarness />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      act(() => {
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ index: 0, name: 'bash', args: '' }) })
        stream.enqueue({ event: 'tool_args_delta', data: JSON.stringify({ id: 'call-execute', index: 0, name: 'bash', delta: completeArgs }) })
        stream.enqueue({ event: 'tool_result', data: JSON.stringify({ id: 'call-execute', index: 0, name: 'bash', content: 'command done' }) })
      })

      await waitFor(() => {
        const liveMessages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
        const executeMessages = buildAgentMessageViews(liveMessages).filter((view) => view.kind === 'tool' && view.toolName === 'bash')
        expect(executeMessages).toHaveLength(1)
        expect(executeMessages[0]).toMatchObject({
          input: JSON.parse(completeArgs),
          status: 'success',
          output: 'command done',
          streaming: false,
        })
      })
    } finally {
      stream.close()
    }
  })

  it('batches consecutive tool argument deltas into one frame update', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)
    let scheduledFrame: FrameRequestCallback | null = null
    let unsubscribe: () => void = () => {}

    try {
      render(<StoryStageHarness />)
      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())
      act(() => {
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ id: 'call-execute', name: 'bash', args: '' }) })
      })
      await waitFor(() => {
        const messages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
        expect(messages.some((message) => message.id === 'call-execute')).toBe(true)
      })

      vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback: FrameRequestCallback) => {
        scheduledFrame = callback
        return 91
      })
      let storeUpdates = 0
      unsubscribe = useInteractiveStore.subscribe(() => { storeUpdates += 1 })

      await act(async () => {
        stream.enqueue({ event: 'tool_args_delta', data: JSON.stringify({ id: 'call-execute', name: 'bash', delta: 'first' }) })
        stream.enqueue({ event: 'tool_args_delta', data: JSON.stringify({ id: 'call-execute', name: 'bash', delta: '-last' }) })
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(storeUpdates).toBe(0)
      expect(scheduledFrame).not.toBeNull()
      act(() => {
        ;(scheduledFrame as FrameRequestCallback)(0)
      })
      const messages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
      expect(storeUpdates).toBe(1)
      expect(buildAgentMessageViews(messages).find((view) => view.partId === 'call-execute')?.input).toBe('first-last')
    } finally {
      unsubscribe()
      vi.restoreAllMocks()
      stream.close()
    }
  })

  it('merges the persisted turn before silent snapshot reconciliation without duplicating the live narrative', async () => {
    const user = userEvent.setup()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '门外有灯。' }) },
      { event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) },
      { event: 'done', data: '{}' },
    ]))

    render(<PersistedTurnHarness onDone={handleDone} />)

    await user.type(screen.getByPlaceholderText('你要做什么？'), '推门')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(screen.getAllByText('门外有灯。')).toHaveLength(1))
    await waitFor(() => expect(handleDone).toHaveBeenCalledWith({ silent: true }))
    expect(screen.queryByText('正在加载')).not.toBeInTheDocument()
  })

  it('keeps the rendered turn mounted when live output becomes persisted history', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const handleDone = vi.fn().mockResolvedValue(undefined)

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<PersistedTurnHarness onDone={handleDone} />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '推门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      act(() => {
        stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '门外有灯。' }) })
      })
      const liveNarrative = await screen.findByText('门外有灯。')
      const liveRow = liveNarrative.closest('[data-nova-chat-row-key]')
      expect(liveRow).not.toBeNull()

      act(() => {
        stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
        stream.enqueue({ event: 'done', data: '{}' })
        stream.close()
      })

      await waitFor(() => expect(screen.getAllByText('门外有灯。')).toHaveLength(1))
      expect(screen.getByText('门外有灯。').closest('[data-nova-chat-row-key]')).toBe(liveRow)
    } finally {
      stream.close()
    }
  })

  it('does not insert a transient done activity row after the persisted turn arrives', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const handleDone = vi.fn().mockResolvedValue(undefined)

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<PersistedTurnHarness onDone={handleDone} />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '推门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '门外有灯。' }) })
      stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
      stream.enqueue({ event: 'done', data: '{}' })

      await waitFor(() => expect(screen.getAllByText('门外有灯。')).toHaveLength(1))
      expect(screen.queryByText('完成')).not.toBeInTheDocument()
      expect(screen.queryByText('Done')).not.toBeInTheDocument()
    } finally {
      stream.close()
    }
  })

  it('shows a live turn in the navigator and replaces it with the persisted turn without duplication', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const handleDone = vi.fn().mockResolvedValue(undefined)

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<PersistedTurnHarness onDone={handleDone} />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '推门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      expect(screen.getByRole('button', { name: '跳转到第 1 轮' })).toBeInTheDocument()
      expect(screen.getAllByText('推门').length).toBeGreaterThan(0)

      stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '门外有灯。' }) })
      await waitFor(() => expect(screen.getAllByText('门外有灯。').length).toBeGreaterThan(0))

      stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
      stream.enqueue({ event: 'done', data: '{}' })

      await waitFor(() => expect(screen.getAllByRole('button', { name: '跳转到第 1 轮' })).toHaveLength(1))
    } finally {
      stream.close()
    }
  })
})
