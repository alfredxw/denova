import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryPanel } from './MemoryPanel'

describe('MemoryPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('auto-starts generation stream when the current turn memory is pending', async () => {
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

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{
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

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/story-memory/generate/stream', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main', source: 'auto' }),
    })))
    expect(screen.getByRole('button', { name: '记忆' })).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.getByRole('button', { name: '记忆内容' })).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0)
    expect(screen.queryByText('导演编排可能涉及剧透')).not.toBeInTheDocument()
    expect(screen.queryByText('自动整理当前回合的故事记忆')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '整理过程' }))
    expect(screen.getByText('自动整理当前回合的故事记忆')).toBeInTheDocument()
    expect(screen.getByText('apply_story_memory_patches')).toBeInTheDocument()
    expect(screen.getByText('整理完成：写入 1 条更新，当前可见 1 条记录')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/story-memory/generate/stream', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main', source: 'auto' }),
    }))
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
        rule_resolution: {
          id: 'rr_1',
          accepted_brief: {
            user_action: '强行闯入藏书阁',
            intent: '冒险',
            turn_goal: '让错误选择产生明确代价',
            pressure: '守阁长老正在靠近',
            cost_policy: '失败会损失体力并暴露行踪',
            rule_checks: [{ id: 'check_1', label: '潜入检定', dice: '1d20', difficulty: 18 }],
          },
          rule_results: [{
            id: 'check_1',
            label: '潜入检定',
            dice: '1d20',
            rolls: [4],
            total: 6,
            outcome: 'failure',
          }],
          terminal_candidate: { type: 'bad_end', reason: '强闯失败导致主线中断', check_id: 'check_1' },
        },
      },
    }} />)

    await waitFor(() => expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0))
    expect(screen.queryByDisplayValue(/公开压力升高/)).not.toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/director'))).toBe(false)
    await openDirectorPanel()
    expect(screen.getByText('导演编排可能涉及剧透')).toBeInTheDocument()
    expect(screen.queryByDisplayValue(/公开压力升高/)).not.toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/director'))).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/director?branch=main'))).toBe(true))
    expect(screen.getAllByText('导演编排').length).toBeGreaterThan(0)
    expect(screen.getByDisplayValue(/公开压力升高/)).toBeInTheDocument()
    expect(screen.getByText('最近后台更新')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    expect(screen.getByText('director unavailable')).toBeInTheDocument()
    expect(screen.getByText('规则审计')).toBeInTheDocument()
    expect(screen.getByText('让错误选择产生明确代价')).toBeInTheDocument()
    expect(screen.getByText('潜入检定')).toBeInTheDocument()
    expect(screen.getByText('failure')).toBeInTheDocument()
    expect(screen.getByText('强闯失败导致主线中断')).toBeInTheDocument()
  })

  it('rebuilds director plan from the summary action', async () => {
    const onSnapshotRefresh = vi.fn()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/director/rebuild')) {
        return Response.json(directorPlan({ mainline: '# 大方向 / Mainline\n\n## 正文Agent可读 / Prose-agent visible\n\n重建后的主线\n\n## 后台导演私密 / Director private\n\n后台' }))
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

    await openDirectorPanel()
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '重建导演计划' })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '重建导演计划' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/director/rebuild', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main' }),
    })))
    await waitFor(() => expect(onSnapshotRefresh).toHaveBeenCalledTimes(1))
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

    await openDirectorPanel()
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(screen.getByLabelText('大方向')).toBeInTheDocument())
    await userEvent.clear(screen.getByLabelText('大方向'))
    await userEvent.type(screen.getByLabelText('大方向'), '新主线')
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
        return Response.json({ id: 'rr_2', accepted_brief: {}, rule_results: [] })
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
          accepted_brief: { user_action: '冲刺', intent: '冒险', turn_goal: '结算冲刺' },
          rule_results: [{ id: 'dash', label: '冲刺检定', dice: '1d20', rolls: [2], total: 7, outcome: 'failure' }],
        },
      },
    }} />)

    await openDirectorPanel()
    await userEvent.click(screen.getByRole('button', { name: '查看导演编排' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '重抽规则结算' })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '重抽规则结算' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/rules/resolutions/rr_1/reroll', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main', turn_id: 'turn-1' }),
    })))
    await waitFor(() => expect(onSnapshotRefresh).toHaveBeenCalledTimes(1))
  })
})

async function openDirectorPanel() {
  await waitFor(() => expect(screen.getByRole('button', { name: '记忆' })).toHaveClass('bg-[var(--nova-active)]'))
  await userEvent.click(screen.getByRole('button', { name: '导演编排' }))
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

function directorPlan(overrides: Partial<{ mainline: string; current_event: string; next_branches: string }> = {}) {
  const docs = {
    mainline: overrides.mainline || '# 大方向 / Mainline\n\n## 正文Agent可读 / Prose-agent visible\n\n### 目标 / Goal\n外门逆袭\n\n## 后台导演私密 / Director private\n\n### 目标 / Goal\n隐藏反转',
    current_event: overrides.current_event || '# 当前主线事件 / Current Main Event\n\n## 正文Agent可读 / Prose-agent visible\n\n### 目标 / Goal\n公开压力升高，同门质疑逼近。\n\n## 后台导演私密 / Director private\n\n### 目标 / Goal\n幕后安排',
    next_branches: overrides.next_branches || '# 最近分支安排 / Next Branches\n\n## 正文Agent可读 / Prose-agent visible\n\n### 分支处理 / Branch Handling\n观察、对话、调查都成立。\n\n## 后台导演私密 / Director private\n\n### 分支处理 / Branch Handling\n隐藏代价',
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
        mainline: { path: '/tmp/mainline.md', bytes: docs.mainline.length, hash: 'h1' },
        current_event: { path: '/tmp/current-event.md', bytes: docs.current_event.length, hash: 'h2' },
        next_branches: { path: '/tmp/next-branches.md', bytes: docs.next_branches.length, hash: 'h3' },
      },
      last_run: { status: 'failed', summary: '后台导演更新失败，已保留本回合正文。', error: 'director unavailable' },
    },
  }
}

function directorStatus(status: string) {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    status,
    summary: status === 'failed' ? '后台导演更新失败，已保留本回合正文。' : '后台导演已更新三层规划。',
    error: status === 'failed' ? 'director unavailable' : '',
    source_turn_id: 'turn-1',
    updated_at: '2026-06-19T06:00:00Z',
    planned_docs: 3,
    completed_docs: status === 'ready' ? 3 : 0,
    doc_bytes: 600,
    visible_bytes: 260,
    start_ready: status === 'ready',
    blocking: status === 'failed',
  }
}
