import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Profiler, StrictMode, useState } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryStage } from './StoryStage'
import { mergeInteractiveTurnPersistedSnapshot, useInteractiveStore } from '../stores/interactive-store'
import type { InteractiveTurnPersistedEvent, Snapshot, StorySummary, TurnEvent } from '../types'

const { generateInteractiveImageMock, getActiveInteractiveChatMock, runInteractiveDirectorMock, sendInteractiveMessageMock, streamActiveInteractiveChatMock, updateInteractiveTurnNarrativeMock, useSkillCommandsMock } = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
  getActiveInteractiveChatMock: vi.fn(),
  runInteractiveDirectorMock: vi.fn(),
  sendInteractiveMessageMock: vi.fn(),
  streamActiveInteractiveChatMock: vi.fn(),
  updateInteractiveTurnNarrativeMock: vi.fn(),
  useSkillCommandsMock: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: (...args: unknown[]) => useSkillCommandsMock(...args),
}))

vi.mock('../api', () => ({
  abortInteractiveChat: vi.fn(),
  analyzeInteractiveContext: vi.fn(),
  compactInteractiveContext: vi.fn(),
  generateInteractiveImage: generateInteractiveImageMock,
  getActiveInteractiveChat: getActiveInteractiveChatMock,
  removeInteractiveContextCompaction: vi.fn(),
  runInteractiveDirector: runInteractiveDirectorMock,
  sendInteractiveMessage: sendInteractiveMessageMock,
  streamActiveInteractiveChat: streamActiveInteractiveChatMock,
  switchInteractiveTurnVersion: vi.fn(),
  updateInteractiveTurnNarrative: updateInteractiveTurnNarrativeMock,
}))

beforeEach(() => {
  window.localStorage.clear()
  useInteractiveStore.setState({ storyStageRuns: {} })
  generateInteractiveImageMock.mockReset()
  generateInteractiveImageMock.mockResolvedValue({ enabled: false, skipped: true })
  getActiveInteractiveChatMock.mockReset()
  getActiveInteractiveChatMock.mockResolvedValue({ active: false })
  runInteractiveDirectorMock.mockReset()
  runInteractiveDirectorMock.mockResolvedValue(directorStatus('running', { completed_docs: 1 }))
  sendInteractiveMessageMock.mockReset()
  streamActiveInteractiveChatMock.mockReset()
  updateInteractiveTurnNarrativeMock.mockReset()
  useSkillCommandsMock.mockReset()
  useSkillCommandsMock.mockReturnValue([])
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

describe('StoryStage TurnResult choices', () => {
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
    render(<ReplyEditHarness />)

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
    await user.click(screen.getByText('/story-beat'))

    const textbox = getStageInput()
    expect(within(textbox).getByText('/story-beat')).toHaveClass('nova-composer-token')

    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(sendInteractiveMessageMock).toHaveBeenCalledWith(expect.objectContaining({
        message: '/story-beat',
      }))
    })
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
    await user.click(screen.getByText('#激烈打斗'))

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

describe('StoryStage streaming rendering', () => {
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
			render(<StrictMode><StoryStageHarness /></StrictMode>)
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
			expect(sendInteractiveMessageMock).not.toHaveBeenCalled()

			await act(async () => {
				stream.enqueue({ event: 'thinking', data: JSON.stringify({ content: '正在回忆石门后的布局。' }) })
				stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '石门后亮起一盏灯。' }) })
				await Promise.resolve()
			})
			await expectVisibleText('正在回忆石门后的布局。')
			await waitFor(() => expect(screen.getByText('石门后亮起一盏灯。')).toBeInTheDocument())

			const persisted = persistedTurnEvent()
			persisted.turn.user = '推开石门'
			persisted.turn.narrative = '石门后亮起一盏灯。'
			await act(async () => {
				stream.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persisted) })
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
		sendInteractiveMessageMock
			.mockResolvedValueOnce(firstStream.readable)
			.mockResolvedValueOnce(retryStream.readable)

		try {
			render(<StoryStageHarness onDone={vi.fn().mockResolvedValue(undefined)} />)
			await user.type(screen.getByPlaceholderText('你要做什么？'), '推开石门')
			await user.click(screen.getByRole('button', { name: '发送' }))
			await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(1))

			act(() => {
				firstStream.enqueue({ event: 'error', data: JSON.stringify({ message: '[NodeRunError] 400 Bad Request' }) })
				firstStream.close()
			})

			await user.click(await screen.findByRole('button', { name: '重新生成这一轮' }))
			await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalledTimes(2))
			expect(sendInteractiveMessageMock.mock.calls[1][0]).toMatchObject({ message: '推开石门' })
		} finally {
			firstStream.close()
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
				stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '这段正文没有落盘。' }) })
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

  it('keeps live thinking visible while narrative output starts', async () => {
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
      expect(screen.getByText(providerThinking)).toBeInTheDocument()
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
        expect(liveMessages.some((message) => message.role === 'assistant' && message.streaming_target_content === '我先检查资料，再开始写正文。')).toBe(true)
      })
      expect(screen.queryByRole('button', { name: /思考过程/ })).not.toBeInTheDocument()

      act(() => {
        stream.enqueue({ event: 'interactive_content_reclassified', data: JSON.stringify({ content: '我先检查资料，再开始写正文。' }) })
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ id: 'call-lore', name: 'list_lore_items', args: '{}' }) })
      })

      const trace = await screen.findByRole('button', { name: /思考过程.*1 次工具调用/ })
      expect(trace).toBeInTheDocument()
      expect(screen.getAllByText('我先检查资料，再开始写正文。')).toHaveLength(1)
      const liveMessages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
      expect(liveMessages.some((message) => message.role === 'assistant' && (message.streaming_target_content || message.content))).toBe(false)
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

      expect(await screen.findByRole('button', { name: /思考过程.*1 次工具调用/ })).toBeInTheDocument()
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

      await user.click(screen.getByRole('button', { name: /思考过程.*1 次工具调用/ }))
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
          name: 'write_file',
          content: 'write_file',
          args: '{"file_path":"director.md"}',
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
    const traceButton = screen.getByRole('button', { name: /思考过程.*1 次工具调用/ })
    await user.click(traceButton)
    expect(screen.getByText('正在判断石门后的威胁。')).toBeInTheDocument()
    expect(screen.getByText('list_lore_items')).toBeInTheDocument()
    expect(screen.queryByText('正在重新安排后续分支。')).not.toBeInTheDocument()
    expect(screen.queryByText('write_file')).not.toBeInTheDocument()
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
    const postNarrativeGroup = screen.getByRole('button', { name: /^2 次工具调用$/ })
    expect(narrative.compareDocumentPosition(postNarrativeGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 正文前的思考分组只包含思考内容，不吞掉提交结果工具
    const preNarrativeGroup = screen.getByRole('button', { name: /^思考过程$/ })
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
    // 旧数据没有锚点：保持旧布局，提交结果工具仍在思考折叠分组内（默认折叠不渲染）
    expect(screen.getByRole('button', { name: /思考过程.*2 次工具调用/ })).toBeInTheDocument()
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
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ index: 0, name: 'execute', args: '' }) })
        stream.enqueue({ event: 'tool_args_delta', data: JSON.stringify({ id: 'call-execute', index: 0, name: 'execute', delta: completeArgs }) })
        stream.enqueue({ event: 'tool_result', data: JSON.stringify({ id: 'call-execute', index: 0, name: 'execute', content: 'command done' }) })
      })

      await waitFor(() => {
        const liveMessages = useInteractiveStore.getState().storyStageRuns['/tmp/book:story-1:main']?.liveMessages || []
        const executeMessages = liveMessages.filter((message) => message.role === 'tool_call' && message.name === 'execute')
        expect(executeMessages).toHaveLength(1)
        expect(executeMessages[0]).toMatchObject({
          args: completeArgs,
          status: 'success',
          result: 'command done',
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
        stream.enqueue({ event: 'tool_call', data: JSON.stringify({ id: 'call-execute', name: 'execute', args: '' }) })
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
        stream.enqueue({ event: 'tool_args_delta', data: JSON.stringify({ id: 'call-execute', name: 'execute', delta: 'first' }) })
        stream.enqueue({ event: 'tool_args_delta', data: JSON.stringify({ id: 'call-execute', name: 'execute', delta: '-last' }) })
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
      expect(messages.find((message) => message.id === 'call-execute')?.args).toBe('first-last')
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
      expect(screen.getByRole('img', { name: '即时互动图像' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png')
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

function ReplyEditHarness() {
  const initialTurn: TurnEvent = {
    id: 'turn-edit',
    parent_id: null,
    branch_id: 'main',
    ts: '2026-06-28T00:00:00Z',
    user: '去找朋友',
    narrative: '朋友住在 3 楼 403 室。',
  }
  const [snapshot, setSnapshot] = useState<Snapshot>({
    story_id: 'story-1',
    branch_id: 'main',
    turns: [initialTurn],
    current_turn: initialTurn,
    state: {},
  })

  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshot}
        onDone={() => {
          const narrative = updateInteractiveTurnNarrativeMock.mock.calls.at(-1)?.[2]?.narrative || initialTurn.narrative
          const turn = { ...snapshot.turns[0], narrative }
          const nextSnapshot = { ...snapshot, turns: [turn], current_turn: turn }
          setSnapshot(nextSnapshot)
          return Promise.resolve(nextSnapshot)
        }}
      />
    </VirtuosoMockContext.Provider>
  )
}

function StoryStageHarness({ onDone }: { onDone?: (options?: { silent?: boolean }) => Promise<Snapshot | void> } = {}) {
  const [snapshot, setSnapshot] = useState<Snapshot>({ story_id: 'story-1', branch_id: 'main', turns: [], state: {} })
  const nextSnapshot: Snapshot = {
    story_id: 'story-1',
    branch_id: 'main',
    state: {},
    turns: [{
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '继续前进',
      narrative: '故事继续。',
    }],
  }

  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshot}
		onDone={onDone || (() => {
          setSnapshot(nextSnapshot)
          return Promise.resolve(nextSnapshot)
		})}
      />
    </VirtuosoMockContext.Provider>
  )
}

function getStageInput() {
  const input = screen.getAllByRole('textbox').find((element) => element.getAttribute('enterkeyhint') === 'send')
  if (!input) throw new Error('stage input missing')
  return input
}

function PersistedTurnHarness({ onDone }: { onDone: (options?: { silent?: boolean }) => Promise<void> }) {
  const [snapshot, setSnapshot] = useState<Snapshot>({ story_id: 'story-1', branch_id: 'main', turns: [], state: {} })

  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshot}
        onTurnPersisted={(event) => {
          const nextSnapshot = mergeInteractiveTurnPersistedSnapshot(snapshot, event)
          setSnapshot(nextSnapshot)
          return nextSnapshot
        }}
        onDone={onDone}
      />
    </VirtuosoMockContext.Provider>
  )
}

function persistedTurnEvent(): InteractiveTurnPersistedEvent {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    turn: {
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '推门',
      narrative: '门外有灯。',
    },
    state: { scene: { location: '门外' } },
    graph: {
      nodes: [{
        id: 'turn-1',
        branch_id: 'main',
        title: '推门',
        summary: '门外有灯。',
        ts: '2026-06-28T00:00:00Z',
        current: true,
        head: true,
      }],
      branches: [{ id: 'main', head: 'turn-1', created_at: '2026-06-28T00:00:00Z', current: true }],
    },
    branches: [{ id: 'main', head: 'turn-1', created_at: '2026-06-28T00:00:00Z', current: true }],
  }
}

function directorStatus(status: string, overrides: Partial<NonNullable<Snapshot['director_plan_status']>> = {}) {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    status,
    summary: status === 'running' ? '后台导演正在规划开局。' : '后台导演更新失败，已保留现有规划。',
    error: '',
    source_turn_id: 'turn-1',
    updated_at: '2026-06-28T00:00:00Z',
    planned_docs: 1,
    completed_docs: status === 'ready' ? 1 : 0,
    doc_bytes: 1200,
    visible_bytes: 320,
    start_ready: status === 'ready',
    blocking: false,
    ...overrides,
  }
}

function interactiveStream(events: Array<{ event: string; data: string }>) {
  return new ReadableStream({
    start(controller) {
			let persisted = false
      for (const event of events) {
				if (event.event === 'interactive_turn_persisted') persisted = true
				if (event.event === 'done' && !persisted) {
					controller.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
					persisted = true
				}
        controller.enqueue(event)
      }
      controller.close()
    },
  })
}

function controllableInteractiveStream() {
  let controller: ReadableStreamDefaultController<{ event: string; data: string }> | null = null
  let closed = false
  const readable = new ReadableStream<{ event: string; data: string }>({
    start(nextController) {
      controller = nextController
    },
  })
  return {
    readable,
    enqueue(event: { event: string; data: string }) {
      if (closed) return
      controller?.enqueue(event)
    },
    close() {
      if (closed) return
      closed = true
      controller?.close()
    },
  }
}

function runAnimationFrames(frames: Map<number, FrameRequestCallback>) {
  const callbacks = [...frames.entries()]
  frames.clear()
  for (const [, callback] of callbacks) {
    callback(performance.now())
  }
}

async function expectVisibleText(text: string) {
  await waitFor(() => {
    const visible = screen.getAllByText(text).find((element) => !element.closest('[aria-hidden="true"]'))
    expect(visible).toBeVisible()
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function story(): StorySummary {
  return {
    id: 'story-1',
    title: '故事',
    origin: '',
    story_teller_id: 'classic',
    story_director_id: 'default',
    choice_count: 5,
    reply_target_chars: 2000,
    image_settings: { mode: 'manual', interval_turns: 3 },
    opening: { mode: 'ai' },
    created_at: '2026-06-27T00:00:00Z',
    updated_at: '2026-06-27T00:00:00Z',
    branches: 1,
    events: 0,
  }
}

function storyDirector(ruleVisibilityMode: string) {
  return {
    version: 3,
    id: 'default',
    name: '默认故事导演',
    description: '',
    strategy: {
      enabled: true,
      rule_visibility_mode: ruleVisibilityMode,
		},
		trpg_system: { rule_templates: [] },
		custom: false,
  }
}

function snapshotWithRuleResolution(): Snapshot {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    state: {},
    turns: [{
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '强行闯入藏书阁',
      narrative: '守阁长老拦在门前。',
      rule_resolution: {
        id: 'rr_1',
        request: {
          action: '强行闯入藏书阁',
          intent: '冒险',
          challenge: '潜入检定',
          cost: '失败会损失体力并暴露行踪',
          state: '守阁长老正在靠近',
          adjudication: {
            stakes: '失败会暴露行踪。',
          },
          difficulty: 'hard',
          outcomes: {
            critical_success: { result: '无声潜入。' },
            success: { result: '成功潜入。' },
            failure: { result: '强闯失败导致主线中断', state_changes: [{ actor_id: 'protagonist', field_id: '当前生命', change: -10, reason: '被禁制反震' }] },
            critical_failure: { result: '被当场抓住。' },
          },
        },
        result: {
          id: 'check_1',
          label: '潜入检定',
          dice: '1d20',
          roll_mode: 'normal',
          rolls: [4],
          kept_roll: 4,
          base_target: 15,
          bonus_total: 2,
          target: 18,
          total: 6,
          outcome: 'failure',
          result: '强闯失败导致主线中断',
          state_changes: [{ actor_id: 'protagonist', field_id: '当前生命', change: -10, reason: '被禁制反震' }],
        },
      },
    }],
  }
}
