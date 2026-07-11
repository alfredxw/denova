import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryPanel } from './MemoryPanel'

function ruleRequestFixture(action: string) {
  return {
    action,
    intent: '冒险',
    challenge: '冲刺检定',
    cost: '失败会浪费体力',
    state: '体力仍可支撑一次短距离冲刺。',
    difficulty: 'normal',
    outcomes: {
      critical_success: { result: '大成功' },
      success: { result: '成功' },
      failure: { result: '失败' },
      critical_failure: { result: '大失败' },
    },
  }
}

describe('MemoryPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not let panel mounting trigger background memory writes', async () => {
    const onOpenMemoryManager = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/story-memory/generate/stream')) {
        return new Response(sse([
          ['thinking', { content: '正在自动整理当前回合。' }],
          ['tool_call', { id: 'story_memory_apply', name: 'apply_story_memory_patches', args: 'patches=1 branch_id=main' }],
          ['tool_result', { id: 'story_memory_apply', name: 'apply_story_memory_patches', content: '已写入 1 条故事记忆更新。' }],
          ['story_memory_result', { branch_id: 'main', patches: 1, records: 1 }],
          ['done', { status: 'ok' }],
        ]), { headers: { 'Content-Type': 'text/event-stream' } })
      }
      if (url.includes('/director')) {
        return Response.json(directorPlan())
      }
      if (url.includes('/story-memory')) {
        return Response.json(storyMemoryState())
      }
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" onOpenMemoryManager={onOpenMemoryManager} snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      current_turn: {
        id: 'turn-1',
        parent_id: null,
        branch_id: 'main',
        ts: '2026-06-19T06:00:00Z',
        user: '继续',
        narrative: '剧情继续。',
        memory_status: 'pending',
      },
    }} />)

		await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/story-memory?branch=main', expect.anything()))
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/story-memory/generate/stream'))).toBe(false)
    expect(screen.getByText('导演控制台')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '状态' })).toHaveAttribute('aria-current', 'page')
    expect(screen.queryByTestId('director-rail')).not.toBeInTheDocument()
    expect(screen.queryByText('导演编排可能涉及剧透')).not.toBeInTheDocument()
    expect(screen.queryByText('自动整理当前回合的故事记忆')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '记忆' }))
    expect(screen.getByRole('button', { name: '记忆' })).toHaveAttribute('aria-current', 'page')
    expect(screen.queryByRole('button', { name: '记忆内容' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '整理过程' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '显示归档项' })).not.toBeInTheDocument()
    expect(screen.queryByText('自动整理当前回合的故事记忆')).not.toBeInTheDocument()
    const manageButton = screen.getByRole('button', { name: '打开故事记忆管理' })
    expect(manageButton).toHaveTextContent('管理')
    await userEvent.click(manageButton)
    expect(onOpenMemoryManager).toHaveBeenCalledTimes(1)
    expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0)
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/story-memory/generate/stream'))).toBe(false)
  })

  it('switches console tabs without duplicating tab navigation in a side rail', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director')) return Response.json(directorPlan())
      return Response.json(storyMemoryState())
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: { actors: { protagonist: { hp: 8 } } },
      director_plan_status: directorStatus('ready'),
    }} />)

    expect(screen.getByRole('button', { name: '状态' })).toHaveAttribute('aria-current', 'page')
    expect(screen.queryByTestId('director-run-summary')).not.toBeInTheDocument()
    expect(screen.queryByTestId('director-rail')).not.toBeInTheDocument()
    expect(screen.queryByText('故事记忆')).not.toBeInTheDocument()

		expect(screen.queryByText('故事此刻')).not.toBeInTheDocument()
    expect(screen.getAllByText('protagonist').length).toBeGreaterThan(0)

    await openBackstagePanel()
    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '记忆' }))
    expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0)

    await userEvent.click(screen.getByRole('button', { name: '规划' }))
    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
  })

  it('shows director plan and rule audit from the current snapshot', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director')) return Response.json(directorPlan())
      return Response.json(storyMemoryState())
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      director_plan_status: directorStatus('failed'),
      current_turn: {
        id: 'turn-1',
        parent_id: null,
        branch_id: 'main',
        ts: '2026-06-19T06:00:00Z',
        user: '强行闯入藏书阁',
        narrative: '守阁长老拦在门前。',
        display_events: [{
          id: 'director-write',
          role: 'tool_call',
          name: 'write_file',
          args: '{"file_path":"director.md"}',
          status: 'success',
          result: 'ok',
          agent_kind: 'interactive_director',
          sse_display_notice: 'chapter_body_hidden',
          sse_generated_chars: 128,
        }],
        rule_resolution: {
          id: 'rr_1',
          request: {
            action: '强行闯入藏书阁',
            intent: '冒险',
            challenge: '潜入检定',
            cost: '失败会损失体力并暴露行踪',
            state: '守阁长老正在靠近',
            adjudication: {
              reason: '强闯会触发守阁长老反制。',
              stakes: '失败会暴露行踪。',
              difficulty_reason: '守阁长老靠近，难度提高。',
              roll_mode_reason: '没有明显优势或劣势。',
              state_paths: ['actors.protagonist.state.resources.hp'],
            },
            bonuses: [{ kind: 'environment', source_path: 'scene.familiarity', reason: '熟悉地形', value: 2 }],
            difficulty: 'hard',
            outcomes: {
              critical_success: { result: '无声潜入。' },
              success: { result: '成功潜入。' },
              failure: { result: '强闯失败导致主线中断' },
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
            bonus_details: [{ kind: 'environment', source_path: 'scene.familiarity', reason: '熟悉地形', value: 2 }],
            target: 18,
            total: 6,
            outcome: 'failure',
            result: '强闯失败导致主线中断',
          },
          state_consumption: {
            status: 'partial',
            mode: 'hybrid_auto',
            applied_actor_ops: [{ op: 'set', actor_id: 'protagonist', field_id: '当前生命', value: 0, source_kind: 'rule_resolution', source_id: 'rr_1' }],
            warnings: [{ path: 'actors.protagonist.state.conditions.poisoned', reason: '字段不在状态系统中' }],
          },
          terminal_candidate: { type: 'bad_end', reason: '强闯失败导致主线中断', check_id: 'check_1' },
        },
      },
    }} />)

    expect(screen.queryByDisplayValue(/公开压力升高/)).not.toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/director'))).toBe(false)
    await openBackstagePanel()
    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()
    expect(screen.getByText('后台导演运行')).toBeInTheDocument()
    expect(screen.getByText('director unavailable')).toBeInTheDocument()
    expect(screen.queryByText('edit_file')).not.toBeInTheDocument()
    expect(screen.getByText('0/1')).toBeInTheDocument()
    expect(screen.queryByText('规则审计')).not.toBeInTheDocument()

    await openPlanPanel()
    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
    expect(screen.getByText('规则审计')).toBeInTheDocument()
    expect(screen.getByText('失败会损失体力并暴露行踪')).toBeInTheDocument()
    expect(screen.getByText('投前裁定')).toBeInTheDocument()
    expect(screen.getByText('强闯会触发守阁长老反制。')).toBeInTheDocument()
    expect(screen.getByText('状态消费')).toBeInTheDocument()
    expect(screen.getByText('字段不在状态系统中')).toBeInTheDocument()
    expect(screen.getAllByText('潜入检定').length).toBeGreaterThan(0)
    expect(screen.getByText('failure')).toBeInTheDocument()
    expect(screen.getAllByText('强闯失败导致主线中断').length).toBeGreaterThan(0)
    expect(screen.queryByDisplayValue(/公开压力升高/)).not.toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/director'))).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/director?branch=main'))).toBe(true))
    expect(screen.getByText('导演节拍表')).toBeInTheDocument()
    expect(screen.getByTestId('director-plan-markdown')).toBeInTheDocument()
    expect(screen.getByText(/公开压力升高/)).toBeInTheDocument()
    expect(screen.queryByDisplayValue(/公开压力升高/)).not.toBeInTheDocument()
    expect(screen.queryByText('write_file')).not.toBeInTheDocument()
  })

  it('does not render director.md as a tool name while waiting for the opening turn', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director')) return Response.json(directorPlan({
        last_run: {
          status: 'waiting_opening',
          summary: '等待首个开局回合后开始规划。',
          updated_at: '2026-06-19T06:00:00Z',
          planned_docs: 1,
          completed_docs: 0,
        },
      }))
      return Response.json(storyMemoryState())
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      director_plan_status: directorStatus('waiting_opening', {
        summary: '等待首个开局回合后开始规划。',
        completed_docs: 0,
        start_ready: false,
      }),
    }} />)

    await openBackstagePanel()
    await userEvent.click(screen.getByRole('button', { name: '查看后台执行过程' }))
    await waitFor(() => expect(screen.getAllByText('等待首个开局回合后开始规划。').length).toBeGreaterThan(1))

    expect(screen.queryByText('edit_file')).not.toBeInTheDocument()
    await openPlanPanel()
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(screen.getByTestId('director-plan-markdown')).toBeInTheDocument())
    expect(screen.queryByDisplayValue(/公开压力升高/)).not.toBeInTheDocument()
  })

  it('opens director context analysis from the director panel', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director/context-analysis')) {
        expect(init?.body).toBe(JSON.stringify({ branch_id: 'main', turn_id: 'turn-1' }))
        return Response.json(contextAnalysisFixture())
      }
      if (url.includes('/director')) return Response.json(directorPlan())
      if (url.includes('/story-memory')) return Response.json(storyMemoryState())
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      director_plan_status: directorStatus('ready'),
      current_turn: {
        id: 'turn-1',
        parent_id: null,
        branch_id: 'main',
        ts: '2026-06-19T06:00:00Z',
        user: '我邀请沈凝旁观公开比试',
        narrative: '沈凝停下脚步。',
      },
    }} />)

    await openBackstagePanel()
    await waitFor(() => expect(screen.getByRole('button', { name: '分析导演上下文' })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '分析导演上下文' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/director/context-analysis', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main', turn_id: 'turn-1' }),
    })))
    const dialog = await screen.findByRole('dialog', { name: '分析导演上下文' })
    expect(within(dialog).getByText(/后台导演 Agent 当前会收到/)).toBeInTheDocument()

    expect(await within(dialog).findByText(/资料库导演上下文/)).toBeInTheDocument()
    expect(within(dialog).getByText(/lore index and bounded relevant entries/)).toBeInTheDocument()
  })

	it('rebuilds director plan from the summary action', async () => {
    const onSnapshotRefresh = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director/rebuild')) {
        return Response.json(directorPlan({ plan: '# 导演规划 / Director Plan\n\n## 正文Agent可读 / Prose-agent visible\n\n重建后的导演规划\n\n## 后台导演私密 / Director private\n\n后台' }))
      }
      if (url.includes('/director')) {
        return Response.json(directorPlan())
      }
      if (url.includes('/story-memory')) {
        return Response.json(storyMemoryState())
      }
      return Response.json({})
	})

    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" onSnapshotRefresh={onSnapshotRefresh} snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      director_plan_status: directorStatus('ready'),
    }} />)

    await openPlanPanel()
    await waitFor(() => expect(screen.getByRole('button', { name: '重建导演计划' })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '重建导演计划' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/director/rebuild', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main' }),
    })))
		await waitFor(() => expect(onSnapshotRefresh).toHaveBeenCalledTimes(1))
	})

	it('shows event runtime only after spoiler reveal and can force evaluation', async () => {
		const plan = directorPlan() as ReturnType<typeof directorPlan> & { metadata: Record<string, unknown> }
		plan.metadata.event_runtime = {
			active: { event_ref: 'default/face-slap', stage: 'seed', summary: '公开质疑已经埋下。' },
			recent_decisions: [{ id: 'decision-1', source_turn_id: 'turn-1', decision: { mode: 'seed', event_ref: 'default/face-slap' } }],
		}
		const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
			const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
			if (url.includes('/director/run')) return Response.json(directorStatus('running'))
			if (url.includes('/director')) return Response.json(plan)
			if (url.includes('/story-memory')) return Response.json(storyMemoryState())
			return Response.json({})
		})
		vi.stubGlobal('fetch', fetchMock)

		render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {}, director_plan_status: directorStatus('ready') }} />)
		await openPlanPanel()
		expect(screen.queryByText('事件编排运行态')).not.toBeInTheDocument()
		await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
		await waitFor(() => expect(screen.getByText('事件编排运行态')).toBeInTheDocument())
		expect(screen.getByText('default/face-slap')).toBeInTheDocument()
		await userEvent.click(screen.getByRole('button', { name: '立即评估' }))
		await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/director/run', expect.objectContaining({
			method: 'POST',
			body: JSON.stringify({ branch_id: 'main', force_event_evaluation: true }),
		})))
	})

  it('saves director plan edits from the summary action', async () => {
    const onSnapshotRefresh = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director')) {
        return Response.json(directorPlan())
      }
      if (url.includes('/story-memory')) {
        return Response.json(storyMemoryState())
      }
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" onSnapshotRefresh={onSnapshotRefresh} snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      director_plan_status: directorStatus('ready'),
    }} />)

    await openPlanPanel()
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(screen.getByTestId('director-plan-markdown')).toBeInTheDocument())
    expect(screen.queryByLabelText('director.md')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '编辑' }))
    await waitFor(() => expect(screen.getByLabelText('director.md')).toBeInTheDocument())
    await userEvent.clear(screen.getByLabelText('director.md'))
    await userEvent.type(screen.getByLabelText('director.md'), '新导演规划')
    await userEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/director', expect.objectContaining({
      method: 'PATCH',
    })))
    await waitFor(() => expect(onSnapshotRefresh).toHaveBeenCalledTimes(1))
  })

  it('rerolls current rule resolution from the rule audit action', async () => {
    const onSnapshotRefresh = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/rules/resolutions/rr_1/reroll')) {
        return Response.json({ id: 'rr_2', request: ruleRequestFixture('冲刺'), result: { outcome: 'success' } })
      }
      if (url.includes('/director')) {
        return Response.json(directorPlan())
      }
      if (url.includes('/story-memory')) {
        return Response.json(storyMemoryState())
      }
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" onSnapshotRefresh={onSnapshotRefresh} snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
      current_turn: {
        id: 'turn-1',
        parent_id: null,
        branch_id: 'main',
        ts: '2026-06-19T06:00:00Z',
        user: '冲刺',
        narrative: '他冲了出去。',
        rule_resolution: {
          id: 'rr_1',
          request: ruleRequestFixture('冲刺'),
          result: { id: 'dash', label: '冲刺检定', dice: '1d20', roll_mode: 'normal', rolls: [2], kept_roll: 2, total: 7, outcome: 'failure', result: '冲刺失败' },
        },
      },
    }} />)

    await openPlanPanel()
    await waitFor(() => expect(screen.getByRole('button', { name: '重抽规则结算' })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '重抽规则结算' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/rules/resolutions/rr_1/reroll', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main', turn_id: 'turn-1' }),
    })))
    await waitFor(() => expect(onSnapshotRefresh).toHaveBeenCalledTimes(1))
  })

  it('can manually trigger director planning when the branch has no plan or rule audit', async () => {
    const onSnapshotRefresh = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director/run')) {
        return Response.json(directorStatus('running', { summary: '' }))
      }
      if (url.includes('/director')) {
        return Response.json({ error: 'director plan not found' }, { status: 404 })
      }
      if (url.includes('/story-memory')) {
        return Response.json(storyMemoryState())
      }
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" onSnapshotRefresh={onSnapshotRefresh} snapshot={{
      story_id: 'story-1',
      branch_id: 'main',
      turns: [],
      state: {},
    }} />)

    await openBackstagePanel()
    await waitFor(() => expect(screen.getByRole('button', { name: '手动触发导演规划' })).toBeInTheDocument())
    expect(screen.getByText('当前分支暂无导演编排或规则审计')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '手动触发导演规划' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/director/run', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main' }),
    })))
    await waitFor(() => expect(onSnapshotRefresh).toHaveBeenCalledTimes(1))
    expect(screen.getByTestId('director-run-summary')).toBeInTheDocument()
    expect(screen.getByText('后台导演运行')).toBeInTheDocument()
    expect(screen.getByText('正在流式整理导演规划')).toBeInTheDocument()
    expect(screen.getByText('规划文档')).toBeInTheDocument()
  })
})

async function openPlanPanel() {
  await userEvent.click(screen.getByRole('button', { name: '规划' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '规划' })).toHaveAttribute('aria-current', 'page'))
}

async function openBackstagePanel() {
  await userEvent.click(screen.getByRole('button', { name: '后台' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '后台' })).toHaveAttribute('aria-current', 'page'))
}

function sse(events: Array<[string, unknown]>) {
  return events.map(([event, data]) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`).join('')
}

function storyMemoryState() {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    settings: {
      enabled: true,
      auto_interval_turns: 3,
    },
    structures: [
      {
        id: 'current_state',
        name: '当前状态',
        description: '当前剧情推进状态',
        mode: 'singleton',
        fields: [{ id: 'summary', name: '现状', order: 10 }],
        order: 10,
        built_in: true,
      },
      {
        id: 'important_character',
        name: '重要角色',
        mode: 'keyed',
        key_field_id: 'name',
        fields: [
          { id: 'name', name: '姓名', order: 10 },
          { id: 'status', name: '状态', order: 20 },
        ],
        order: 20,
        built_in: true,
      },
    ],
    records: [
      {
        id: 'state-1',
        structure_id: 'current_state',
        branch_id: 'main',
        values: { summary: '午时考校将开始。' },
        created_at: '2026-06-18T08:00:00Z',
        updated_at: '2026-06-18T08:00:00Z',
      },
      {
        id: 'char-1',
        structure_id: 'important_character',
        branch_id: 'main',
        key: '顾清漪',
        values: { name: '顾清漪', status: '青玉宗二师姐' },
        manual: false,
        created_at: '2026-06-18T08:00:00Z',
        updated_at: '2026-06-18T08:00:00Z',
      },
    ],
    sync_status: 'ready',
  }
}

function contextAnalysisFixture() {
  const systemPart = contextAnalysisPart({
    id: 'output_protocol',
    source: 'Denova runtime',
    title: '输出格式',
    content: '必须通过 read_file/write_file/edit_file 更新当前分支 director.md。',
  })
  const contextMessages = [
    contextAnalysisPart({
      id: 'director_instruction_preamble',
      source: '本轮导演指令',
      title: '后台导演任务与约束',
      role: 'user',
      kind: 'body',
      content: '请根据本回合已落盘的审计数据，更新当前分支 director.md。',
    }),
    contextAnalysisPart({
      id: 'director_instruction_part_02',
      source: 'lore index and bounded relevant entries',
      title: '资料库导演上下文',
      kind: 'body',
      content: '角色 沈凝。外门比试关键见证者。',
      note: 'bounded · final_user_message',
    }),
  ]
  return {
    agent_kind: 'interactive_director',
    mode: 'interactive_director',
    system_prompt: '你是后台导演。',
    system_prompt_parts: [systemPart],
    context_parts: contextMessages,
    context_messages: contextMessages,
    message_count: 1,
    token_estimate: 1200,
    context_window_tokens: 128000,
    context_usage_ratio: 0.01,
    compaction_active: false,
    would_compact: false,
  }
}

function contextAnalysisPart(input: Partial<{
  id: string
  source: string
  title: string
  role: string
  kind: string
  content: string
  note: string
}>) {
  const content = input.content || ''
  return {
    id: input.id || '',
    source: input.source || '',
    title: input.title || '',
    role: input.role || '',
    kind: input.kind || '',
    content,
    note: input.note || '',
    bytes: content.length,
    chars: content.length,
  }
}

function directorPlan(overrides: Partial<{ plan: string; last_run: Record<string, unknown> }> = {}) {
  const docs = {
    plan: overrides.plan || '# 导演规划 / Director Plan\n\n## 正文Agent可读 / Prose-agent visible\n\n### 阶段钩子与阅读欲望 / Stage Hook and Reader Desire\n外门逆袭\n\n### 当前场景与行动空间 / Current Scene and Action Space\n公开压力升高，同门质疑逼近。\n\n### 最近分支安排 / Near Branch Arrangements\n观察、对话、调查都成立。\n\n## 后台导演私密 / Director private\n\n### 伏笔与回收 / Foreshadowing and Payoff\n隐藏反转',
  }
  return {
    story_id: 'story-1',
    branch_id: 'main',
    docs,
    visible_docs: docs,
    metadata: {
      version: 1,
      story_id: 'story-1',
      branch_id: 'main',
      revision: 'rev-1',
      branch_planning_turns: 5,
      updated_at: '2026-06-19T06:00:00Z',
      docs: {
        plan: { path: '/tmp/director.md', bytes: docs.plan.length, hash: 'h1' },
      },
      last_run: overrides.last_run || { status: 'failed', summary: '后台导演更新失败，已保留本回合正文。', error: 'director unavailable' },
    },
  }
}

function directorStatus(status: string, overrides: Partial<ReturnType<typeof directorStatusBase>> = {}) {
  return { ...directorStatusBase(status), ...overrides }
}

function directorStatusBase(status: string) {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    status,
    summary: status === 'failed' ? '后台导演更新失败，已保留本回合正文。' : '后台导演已更新导演规划。',
    error: status === 'failed' ? 'director unavailable' : '',
    source_turn_id: 'turn-1',
    updated_at: '2026-06-19T06:00:00Z',
    planned_docs: 1,
    completed_docs: status === 'ready' ? 1 : 0,
    doc_bytes: 600,
    visible_bytes: 260,
    start_ready: status === 'ready',
    blocking: false,
  }
}
