import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Profiler, type ComponentProps } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryStage as ProjectStoryStage } from './StoryStage'

function StoryStage(props: Omit<ComponentProps<typeof ProjectStoryStage>, 'projectId'> & { projectId?: string }) {
  return <ProjectStoryStage {...props} projectId={props.projectId || 'project-story'} />
}
import { useInteractiveStore } from '../stores/interactive-store'
import type { TurnEvent } from '../types'
import {
  ReplyEditHarness,
  PersistedTurnHarness,
  StoryStageHarness,
  controllableInteractiveStream,
  directorStatus,
  getStageInput,
  interactiveStream,
  persistedTurnEvent,
  resetStoryStageTestHarness,
  snapshotWithRuleResolution,
  story,
  storyDirector,
} from './story-stage/story-stage-test-harness'

const testMocks = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
  getActiveInteractiveChatMock: vi.fn(),
  getInteractiveHistoryPageMock: vi.fn(),
  patchConversationConfigMock: vi.fn(),
  runInteractiveDirectorMock: vi.fn(),
  sendInteractiveMessageMock: vi.fn(),
  streamActiveInteractiveChatMock: vi.fn(),
  submitInteractiveAgentCommandMock: vi.fn(),
  setConversationGoalMock: vi.fn(),
  updateInteractiveTurnNarrativeMock: vi.fn(),
  useConversationConfigMock: vi.fn(),
  useConversationGoalMock: vi.fn(),
  useSkillCommandsMock: vi.fn(),
}))

const {
  generateInteractiveImageMock,
  getActiveInteractiveChatMock,
  getInteractiveHistoryPageMock,
  patchConversationConfigMock,
  sendInteractiveMessageMock,
  setConversationGoalMock,
  updateInteractiveTurnNarrativeMock,
  useConversationConfigMock,
  useConversationGoalMock,
  useSkillCommandsMock,
} = testMocks

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
  fetchProjectSettings: vi.fn().mockResolvedValue({ effective: {}, user: {}, workspace: {} }),
}))

vi.mock('@/features/agent-approval/AgentApprovalProvider', () => ({
  useAgentApprovalMode: () => ({ mode: 'write', initialized: true, saving: false, setMode: vi.fn().mockResolvedValue(true) }),
}))

vi.mock('@/features/conversation-config/use-conversation-config', () => ({
  useConversationConfig: () => testMocks.useConversationConfigMock(),
}))

vi.mock('@/features/agent-goal/use-conversation-goal', () => ({
  useConversationGoal: (...args: unknown[]) => testMocks.useConversationGoalMock(...args),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: (...args: unknown[]) => testMocks.useSkillCommandsMock(...args),
}))

vi.mock('../api', () => ({
  analyzeInteractiveContext: vi.fn(),
  compactInteractiveContext: vi.fn(),
  generateInteractiveImage: testMocks.generateInteractiveImageMock,
  getActiveInteractiveChat: testMocks.getActiveInteractiveChatMock,
  getInteractiveHistoryPage: testMocks.getInteractiveHistoryPageMock,
  removeInteractiveContextCompaction: vi.fn(),
  runInteractiveDirector: testMocks.runInteractiveDirectorMock,
  sendInteractiveMessage: testMocks.sendInteractiveMessageMock,
  streamActiveInteractiveChat: testMocks.streamActiveInteractiveChatMock,
  submitInteractiveAgentCommand: testMocks.submitInteractiveAgentCommandMock,
  switchInteractiveTurnVersion: vi.fn(),
  updateInteractiveTurnNarrative: testMocks.updateInteractiveTurnNarrativeMock,
}))

beforeEach(() => {
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:test-attachment') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
  resetStoryStageTestHarness(testMocks)
  getInteractiveHistoryPageMock.mockReset()
  patchConversationConfigMock.mockReset().mockResolvedValue(true)
  useConversationConfigMock.mockReset().mockReturnValue({
    snapshot: { agent_kind: 'interactive_story', profile_id: 'default', thinking_level: 'off', approval_mode: 'write', revision: 1 },
    initialized: true,
    loading: false,
    saving: false,
    error: null,
    patch: patchConversationConfigMock,
    reload: vi.fn(),
  })
  setConversationGoalMock.mockReset().mockResolvedValue({
    id: 'goal-story', objective: '完成本章目标', status: 'active', revision: 1,
    created_at: '2026-08-20T00:00:00Z', updated_at: '2026-08-20T00:00:00Z',
  })
  useConversationGoalMock.mockReset().mockReturnValue({
    goal: null, loading: false, saving: false, reload: vi.fn(), set: setConversationGoalMock,
    pause: vi.fn(), resume: vi.fn(), clear: vi.fn(),
  })
})

describe('StoryStage store subscriptions', () => {
  it('does not rerender when unrelated interactive store state changes', async () => {
    let commits = 0
    render(
      <Profiler id="story-stage" onRender={() => { commits += 1 }}>
        <StoryStageHarness />
      </Profiler>,
    )
    await waitFor(() => expect(getActiveInteractiveChatMock).toHaveBeenCalled())
    await act(async () => undefined)
    commits = 0

    act(() => {
      useInteractiveStore.getState().setTellers([])
    })

    expect(commits).toBe(0)
  })
})

describe('StoryStage loading presentation', () => {
  it('keeps the loading placeholder at the conversation tail without visible copy', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={null}
        snapshotLoading
        onDone={() => undefined}
      />,
    )

    const status = screen.getByRole('status', { name: '加载中...' })
    expect(status).toHaveAttribute('data-layout', 'conversation')
    expect(screen.getByText('加载中...')).toHaveClass('sr-only')
    expect(document.querySelector('[data-slot="loading-state-conversation"]')).toBeInTheDocument()
  })
})

describe('StoryStage TurnResult choices', () => {
  it('uses the existing action menu for an attachment-only game turn', async () => {
    const user = userEvent.setup()
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
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    await user.click(screen.getByRole('button', { name: '输入动作' }))
    expect(screen.getByRole('menuitem', { name: '添加文件' })).toBeInTheDocument()
    const file = new File(['hello'], 'notes.md', { type: 'text/markdown' })
    fireEvent.change(screen.getByLabelText('添加文件'), { target: { files: [file] } })
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
      story_id: 'story-1',
      message: '',
      attachments: [{ name: 'notes.md', media_type: 'text/markdown', data_url: 'data:text/markdown;base64,aGVsbG8=' }],
    })))
  })

	it('places the model selector before the choice control and send action', async () => {
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
					onDone={() => undefined}
				/>
			</VirtuosoMockContext.Provider>,
		)

		const modelSelector = await screen.findByRole('button', { name: /切换模型/ })
		const choiceControl = screen.getByRole('button', { name: '获取行动选择' })
		const sendAction = screen.getByRole('button', { name: '发送' })

		expect(modelSelector.compareDocumentPosition(choiceControl)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
		expect(choiceControl.compareDocumentPosition(sendAction)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
	})

  it('keeps the default safety mode inside the game composer options menu', async () => {
    const user = userEvent.setup()
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
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(screen.queryByRole('button', { name: 'Agent 安全模式: Write' })).not.toBeInTheDocument()
    expect(patchConversationConfigMock).not.toHaveBeenCalled()

    fireEvent.pointerDown(screen.getByRole('button', { name: '输入动作' }))
    const safetyModeOption = await screen.findByRole('menuitem', { name: 'Agent 安全模式: Write' })
    await user.hover(safetyModeOption)
    fireEvent.click(await screen.findByRole('menuitem', { name: /Full access/ }))

    await waitFor(() => expect(patchConversationConfigMock).toHaveBeenCalledWith({ approval_mode: 'full_access' }))
  })

  it('blocks new game runs until the safety mode is initialized', () => {
    useConversationConfigMock.mockReturnValue({
      snapshot: null,
      initialized: false,
      loading: true,
      saving: false,
      error: null,
      patch: patchConversationConfigMock,
      reload: vi.fn(),
    })
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
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    expect(getStageInput()).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: /Agent 安全模式/ })).not.toBeInTheDocument()
  })

	it('uses persisted TurnResult choices and only reveals them after the user opens the panel', async () => {
		const user = userEvent.setup()
		const turn = {
			id: 'turn-1',
			parent_id: null,
			branch_id: 'main',
			ts: '2026-06-28T00:00:00Z',
			user: '检查钟楼',
			narrative: '钟楼上有反光一闪。',
			state_status: 'ready' as const,
			turn_result: {
				state_updates: [],
				choices: ['绕到钟楼背面', '询问附近守夜人'],
			},
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

		expect(screen.queryByText('绕到钟楼背面')).not.toBeInTheDocument()
		expect(screen.queryByLabelText('当前故事态势')).not.toBeInTheDocument()
		await user.click(screen.getByRole('button', { name: '获取行动选择' }))
		expect(await screen.findByText('绕到钟楼背面')).toBeInTheDocument()
	})

  it('does not open persisted choices when they arrive during the story stream', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const persisted = persistedTurnEvent()
    persisted.turn.turn_result = {
      state_updates: [],
      choices: ['沿墙观察', '询问守夜人'],
    }
    sendInteractiveMessageMock.mockResolvedValue(stream.readable)

    try {
      render(<PersistedTurnHarness onDone={vi.fn().mockResolvedValue(undefined)} />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '推门')
      await user.click(screen.getByRole('button', { name: '发送' }))
      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())

      act(() => {
        stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '门外传来脚步声。' }) })
        stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persisted) })
      })
      expect(screen.queryByText('沿墙观察')).not.toBeInTheDocument()

      act(() => {
        stream.enqueue({ event: 'done', data: '{}' })
        stream.close()
      })
      await waitFor(() => expect(screen.getByRole('button', { name: '获取行动选择' })).not.toBeDisabled())
      expect(screen.queryByText('沿墙观察')).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: '获取行动选择' }))
      expect(await screen.findByText('沿墙观察')).toBeInTheDocument()
    } finally {
      stream.close()
    }
  })

})

describe('StoryStage AI reply editing', () => {
  it('edits persisted AI prose without regenerating the turn', async () => {
    const user = userEvent.setup()
    render(<ReplyEditHarness readSavedNarrative={() => (
      updateInteractiveTurnNarrativeMock.mock.calls.at(-1)?.[2]?.narrative
    )} />)

    await user.click(screen.getByRole('button', { name: '编辑 AI 回复' }))
    const editor = screen.getByRole('textbox', { name: 'AI 回复正文' })
    expect(editor).toHaveValue('朋友住在 3 楼 403 室。')
    expect(screen.getByText(/不会重新生成/)).toBeInTheDocument()

    await user.clear(editor)
    await user.type(editor, '朋友住在 4 楼 403 室。')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(updateInteractiveTurnNarrativeMock).toHaveBeenCalledWith('story-1', 'turn-edit', {
        branch_id: 'main',
        narrative: '朋友住在 4 楼 403 室。',
        expected_narrative: '朋友住在 3 楼 403 室。',
      })
    })
    expect(await screen.findByText('朋友住在 4 楼 403 室。')).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(sendInteractiveMessageMock).not.toHaveBeenCalled()
  })

  it('pages older turns without exposing mutation actions outside the latest turn', async () => {
    const user = userEvent.setup()
    const onRequestCreateBranch = vi.fn()
    const older: TurnEvent = {
      id: 'turn-old',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '走进门厅',
      narrative: '门厅里只有一盏旧灯。',
    }
    const latest: TurnEvent = {
      id: 'turn-latest',
      parent_id: older.id,
      branch_id: 'main',
      ts: '2026-06-28T00:01:00Z',
      user: '点亮旧灯',
      narrative: '灯芯映出墙上的地图。',
    }
    getInteractiveHistoryPageMock.mockResolvedValue({
      story_id: 'story-1',
      branch_id: 'main',
      turns: [older],
      before_cursor: '',
      has_more: false,
    })

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{
            story_id: 'story-1',
            branch_id: 'main',
            turns: [latest],
            current_turn: latest,
            state: {},
            history_before_cursor: 'older-cursor',
            has_earlier_turns: true,
          }}
          onRequestCreateBranch={onRequestCreateBranch}
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    await user.click(screen.getByRole('button', { name: '加载更早消息' }))
    expect(await screen.findByText('门厅里只有一盏旧灯。')).toBeInTheDocument()
    expect(getInteractiveHistoryPageMock).toHaveBeenCalledWith('story-1', 'main', 'older-cursor')
    expect(screen.getAllByRole('button', { name: '编辑 AI 回复' })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: '生成互动图像' })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: '从此处创建分支' })).toHaveLength(2)

    await user.click(screen.getAllByRole('button', { name: '从此处创建分支' })[0])
    expect(onRequestCreateBranch).toHaveBeenCalledWith(expect.objectContaining({ turnId: 'turn-old', title: '走进门厅' }))
  })
})

describe('StoryStage current state ledger', () => {
  it('places the collapsed state after the latest prose and reveals World State as a peer tab on demand', async () => {
    const user = userEvent.setup()
    const turn: TurnEvent = {
      id: 'turn-state',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-07-13T00:00:00Z',
      user: '观察天色',
      narrative: '远山压着一线沉云。',
      state_status: 'ready',
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
          snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [turn], current_turn: turn, state: { scene: { weather: '暴雨将至' } } }}
          stateDisplayPreference="collapsed"
          onDone={() => undefined}
        />
      </VirtuosoMockContext.Provider>,
    )

    const prose = screen.getByText('远山压着一线沉云。')
    const state = screen.getByRole('region', { name: '当前状态' })
    expect(prose.compareDocumentPosition(state) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(within(state).queryByRole('tab', { name: '世界状态' })).not.toBeInTheDocument()
    await user.click(within(state).getByRole('button', { name: '展开状态面板' }))
    expect(within(state).getByRole('tab', { name: '世界状态' })).toHaveAttribute('aria-selected', 'true')
    expect(within(state).getByText('暴雨将至')).toBeInTheDocument()
  })
})

describe('StoryStage composer', () => {
  it('keeps the game input single-line and does not expose Plan Mode controls', async () => {
    render(<StoryStageHarness />)

    const input = screen.getByPlaceholderText('你要做什么？')
    expect(input).toHaveAttribute('rows', '1')
    expect(screen.queryByLabelText('Plan Mode 已开启')).not.toBeInTheDocument()

    fireEvent.pointerDown(screen.getByRole('button', { name: '输入动作' }))

    expect(screen.queryByRole('menuitemcheckbox', { name: /Plan/ })).not.toBeInTheDocument()
  })

  it('disables normal input on terminal branches', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{
          story_id: 'story-1',
          branch_id: 'main',
          state: {},
          turns: [],
          current_turn: {
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '强闯禁制',
            narrative: '入口坍塌。',
            terminal_outcome: { terminal: true, type: 'mainline_failed', reason: '主线入口崩塌。' },
          },
        }}
        onDone={() => {}}
      />,
    )

    expect(screen.getByPlaceholderText('当前分支已终局，请从历史回合创建新分支')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByPlaceholderText('当前分支已终局，请从历史回合创建新分支')).toHaveAttribute('contenteditable', 'false')
    expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
  })

  it('keeps rule rolls hidden on the stage when visibility is audit-only', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        storyDirectors={[storyDirector('audit_only')]}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshotWithRuleResolution()}
        onDone={() => {}}
      />,
    )

    expect(screen.getByText('守阁长老拦在门前。')).toBeInTheDocument()
    expect(screen.queryByText('总值 6 / 目标 18')).not.toBeInTheDocument()
  })

  it('shows a public rule roll card before the prose when enabled by the story director', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        storyDirectors={[storyDirector('public_roll')]}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshotWithRuleResolution()}
        onDone={() => {}}
      />,
    )

    expect(screen.getByText('潜入检定')).toBeInTheDocument()
    expect(screen.getByText('总值 6 / 目标 18')).toBeInTheDocument()
    expect(screen.getByText(/失败会损失体力并暴露行踪/)).toBeInTheDocument()
    expect(screen.getByText('protagonist / 当前生命 -10')).toBeInTheDocument()
    expect(screen.getByText('守阁长老拦在门前。')).toBeInTheDocument()
  })

  it('shows a temporary public rule roll card from the streaming tool result', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'tool_call', data: JSON.stringify({ id: 'call-1', name: 'prepare_interactive_turn', args: '{}' }) },
      { event: 'tool_result', data: JSON.stringify({ id: 'call-1', name: 'prepare_interactive_turn', content: JSON.stringify({
        resolution_id: 'rr_live',
        label: '潜入检定',
        dice: '1d20',
        roll_mode: 'normal',
        rolls: [4],
        kept_roll: 4,
        bonus_total: 2,
        total: 6,
        target: 18,
        difficulty: 'hard',
        outcome: 'failure',
        result: '强闯失败导致主线中断',
        cost: '失败会损失体力并暴露行踪',
      }) }) },
      { event: 'chunk', data: JSON.stringify({ content: '守阁长老拦在门前。' }) },
      { event: 'done', data: '{}' },
    ]))

    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        storyDirectors={[storyDirector('public_roll')]}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        onDone={() => {}}
      />,
    )

    await user.type(screen.getByPlaceholderText('你要做什么？'), '强行闯入藏书阁')
    await user.click(screen.getByRole('button', { name: '发送' }))

    expect(await screen.findByText('潜入检定')).toBeInTheDocument()
    expect(screen.getByText('总值 6 / 目标 18')).toBeInTheDocument()
    expect(screen.getByText('强闯失败导致主线中断')).toBeInTheDocument()
  })

  it('keeps forward actions available while the initial director plan runs in the background', async () => {
    const user = userEvent.setup()
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{
          story_id: 'story-1',
          branch_id: 'main',
          state: {},
          turns: [{
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '开局',
            narrative: '雨停了。',
            turn_result: { state_updates: [], choices: ['继续观察', '询问路人'] },
          }],
          current_turn: {
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '开局',
            narrative: '雨停了。',
            turn_result: { state_updates: [], choices: ['继续观察', '询问路人'] },
          },
          director_plan_status: directorStatus('running', { completed_docs: 1, blocking: true }),
        }}
        onDone={() => {}}
      />,
    )

    expect(screen.queryByText('导演正在规划故事')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText('你要做什么？')).toHaveAttribute('contenteditable', 'true')
    expect(screen.getByRole('button', { name: '获取行动选择' })).not.toBeDisabled()

    await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
    expect(screen.getByRole('button', { name: '发送' })).not.toBeDisabled()
    await user.click(screen.getByRole('button', { name: '获取行动选择' }))
    expect(await screen.findByText('继续观察')).toBeInTheDocument()
  })

  it('inserts interactive Skills as inline tokens and sends compatible text', async () => {
    const user = userEvent.setup()
    useSkillCommandsMock.mockReturnValue([{ name: 'story-beat', description: '推进节拍' }])
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '故事继续。' }) },
      { event: 'done', data: '{}' },
    ]))

    render(<StoryStageHarness />)

    await user.type(getStageInput(), '/story')
    const skillCommand = screen.getByText('/story-beat')
    expect(skillCommand.closest('[cmdk-item]')).toHaveClass('whitespace-nowrap', 'sm:min-h-9')
    expect(screen.getByText('推进节拍')).toHaveClass('truncate')
    await user.click(skillCommand)

    const textbox = getStageInput()
    expect(within(textbox).getByText('/story-beat')).toHaveClass('nova-composer-token')

    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
        message: '/story-beat',
      }))
    })
  })

  it('sets an interactive Goal before starting the visible objective turn', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '开始推进目标。' }) },
      { event: 'done', data: '{}' },
    ]))

    render(<StoryStageHarness />)

    await user.type(getStageInput(), '/go')
    await user.click(screen.getByText('/goal'))
    expect(screen.getByText('目标')).toBeInTheDocument()
    await user.type(getStageInput(), '完成本章目标')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(setConversationGoalMock).toHaveBeenCalledWith('完成本章目标')
      expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({ message: '完成本章目标' }))
    })
    expect(useConversationGoalMock).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'interactive', project_id: 'project-story', story_id: 'story-1', branch_id: 'main',
    }), expect.any(Boolean))
  })

  it('inserts style scenes as inline tokens and sends style_scenes', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '故事继续。' }) },
      { event: 'done', data: '{}' },
    ]))

    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        styleSceneSuggestions={['激烈打斗']}
        onDone={() => {}}
      />,
    )

    await user.type(getStageInput(), '准备 #激')
    expect(screen.getByText('#激烈打斗').closest('[cmdk-item]')).toHaveClass('whitespace-nowrap', 'sm:min-h-9')
    await user.keyboard('{ArrowDown}{Enter}')

    const textbox = getStageInput()
    expect(within(textbox).getByText('#激烈打斗')).toHaveClass('nova-composer-token')

    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
        message: '准备 #激烈打斗',
        style_scenes: ['激烈打斗'],
      }))
    })
  })

  it('does not show failed director planning as a blocking composer banner', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{
          story_id: 'story-1',
          branch_id: 'main',
          state: {},
          turns: [{
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '开局',
            narrative: '雨停了。',
          }],
          current_turn: {
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '开局',
            narrative: '雨停了。',
          },
          director_plan_status: directorStatus('failed', { error: 'director unavailable', blocking: true }),
        }}
        onDone={() => {}}
      />,
    )

    expect(screen.queryByText('director unavailable')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重试规划' })).not.toBeInTheDocument()
    expect(getStageInput()).toHaveAttribute('contenteditable', 'true')
    expect(screen.queryByDisplayValue(/后台导演私密/)).not.toBeInTheDocument()
  })

  it('does not show non-blocking director planning above the input', () => {
    render(
      <StoryStage
        storyId="story-1"
        branchId="main"
        snapshot={{
          story_id: 'story-1',
          branch_id: 'main',
          state: {},
          turns: [{
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '开局',
            narrative: '雨停了。',
          }],
          current_turn: {
            id: 'turn-1',
            parent_id: null,
            branch_id: 'main',
            ts: '2026-06-28T00:00:00Z',
            user: '开局',
            narrative: '雨停了。',
          },
          director_plan_status: directorStatus('running', { completed_docs: 0, blocking: false }),
        }}
        onDone={() => {}}
      />,
    )

    expect(screen.queryByText('导演正在规划故事')).not.toBeInTheDocument()
    expect(getStageInput()).toHaveAttribute('contenteditable', 'true')
  })
})

describe('StoryStage interactive image settings', () => {
  it('sets interactive image mode from the input actions submenu', async () => {
    const user = userEvent.setup()
    const handleImageSettingsChange = vi.fn().mockResolvedValue(undefined)
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        onDone={() => {}}
        onImageSettingsChange={handleImageSettingsChange}
      />,
    )

    fireEvent.pointerDown(screen.getByRole('button', { name: '输入动作' }))
    await waitFor(() => expect(screen.getByText('互动图像')).toBeInTheDocument())
    expect(within(screen.getByRole('menu', { name: '输入动作' })).queryByRole('separator')).not.toBeInTheDocument()
    const imageGenerationOptions = screen.getByRole('menuitem', { name: '图像生成选项' })
    expect(screen.queryByText('Image Agent')).not.toBeInTheDocument()
    await user.hover(imageGenerationOptions)
    expect(await screen.findByRole('menuitem', { name: '语言模型' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '图像模型' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '图像方案' })).toBeInTheDocument()
    await user.hover(screen.getByRole('menuitem', { name: /互动图像/ }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /每 3 轮生成/ })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('menuitem', { name: /每 3 轮生成/ }))

    await waitFor(() => {
      expect(handleImageSettingsChange).toHaveBeenCalledWith({ mode: 'interval', interval_turns: 3, preset_id: 'game-cg' })
    })
  })
})

describe('StoryStage interactive image rendering', () => {
  it('手动生成互动图像成功后不等刷新快照也会立即渲染到对应回合', async () => {
    const user = userEvent.setup()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    generateInteractiveImageMock.mockResolvedValue({
      enabled: true,
      image: {
        schema: 'interactive_image.v1',
        story_id: 'story-1',
        branch_id: 'main',
        turn_id: 'turn-1',
        image_path: 'assets/interactive/images/story-1/main/turn-1/run-a/image.png',
        meta_path: 'assets/interactive/images/story-1/main/turn-1/run-a/meta.json',
        alt_text: '即时互动图像',
      },
    })

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{
            story_id: 'story-1',
            branch_id: 'main',
            state: {},
            turns: [{
              id: 'turn-1',
              parent_id: null,
              branch_id: 'main',
              ts: '2026-06-28T00:00:00Z',
              user: '继续前进',
              narrative: '玄璃抬头，看见雾气里有一道微光。',
            }],
          }}
          onDone={handleDone}
        />
      </VirtuosoMockContext.Provider>,
    )

    await user.click(screen.getByRole('button', { name: '生成互动图像' }))

    await waitFor(() => {
      expect(screen.getByRole('img', { name: '即时互动图像' })).toHaveAttribute('src', '/api/projects/project-story/files/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png')
    })
    expect(handleDone).toHaveBeenCalled()
    expect(handleDone).toHaveBeenCalledWith({ silent: true })
  })
})

describe('StoryStage opening panel', () => {
  it('shows preset content in its tab and starts the selected preset', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'done', data: '{}' },
    ]))
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        bookOpeningPresets={[{ id: 'preset-1', title: '默认开场', content: '青石镇的雨刚刚停。' }]}
        onDone={() => {}}
      />,
    )

    expect(screen.getByRole('tab', { name: /AI 编排/ })).toHaveAttribute('data-state', 'active')
    await user.click(screen.getByRole('tab', { name: /书籍预设/ }))
    expect(screen.getAllByText('青石镇的雨刚刚停。').length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: '使用书籍预设' }))

    await waitFor(() => {
      expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
        mode: 'story',
        story_id: 'story-1',
        branch: 'main',
        message: expect.stringContaining('书籍预设开场白：青石镇的雨刚刚停。'),
      }))
    })
  })

  it('starts opening from the selected book preset', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'done', data: '{}' },
    ]))
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        bookOpeningPresets={[
          { id: 'preset-1', title: '默认开场', content: '青石镇的雨刚刚停。' },
          { id: 'preset-2', title: '雪夜开场', content: '雪夜里，山门外只剩一盏灯。' },
        ]}
        onDone={() => {}}
      />,
    )

    await user.click(screen.getByRole('tab', { name: /书籍预设/ }))
    await user.click(screen.getByRole('option', { name: '选择书籍预设：雪夜开场' }))
    expect(screen.getAllByText('雪夜里，山门外只剩一盏灯。').length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: '使用书籍预设' }))

    await waitFor(() => {
      expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
        mode: 'story',
        story_id: 'story-1',
        branch: 'main',
        message: expect.stringContaining('书籍预设开场白：雪夜里，山门外只剩一盏灯。'),
      }))
    })
  })

  it('keeps custom opening input inside the custom tab', async () => {
    const user = userEvent.setup()
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        onDone={() => {}}
      />,
    )

    expect(screen.queryByPlaceholderText('写下你想使用的开局。生成时会作为有界来源传给游戏 Agent。')).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: '自定义' }))
    const input = screen.getByPlaceholderText('写下你想使用的开局。生成时会作为有界来源传给游戏 Agent。')
    await user.type(input, '山门外传来三声钟响。')
    expect(screen.getByText('10 字')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '使用自定义开局' })).toBeEnabled()
  })
})
