import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryPanel } from './MemoryPanel'

describe('MemoryPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('streams story memory generation in the right panel and refreshes memory', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/story-memory/generate/stream')) {
        return new Response(sse([
          ['thinking', { content: '正在分析当前回合。' }],
          ['tool_call', { id: 'story_memory_apply', name: 'apply_story_memory_patches', args: 'patches=1 branch_id=main' }],
          ['tool_result', { id: 'story_memory_apply', name: 'apply_story_memory_patches', content: '已写入 1 条故事记忆更新。' }],
          ['story_memory_result', { branch_id: 'main', patches: 1, records: 2 }],
          ['done', { status: 'ok' }],
        ]), { headers: { 'Content-Type': 'text/event-stream' } })
      }
      if (url.includes('/story-memory')) {
        return Response.json(storyMemoryState())
      }
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)
    const onOpenMemoryManager = vi.fn()

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }} onOpenMemoryManager={onOpenMemoryManager} />)

    await waitFor(() => expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0))
    expect(screen.getByTestId('memory-panel-icon')).toBeInTheDocument()
    expect(screen.queryByText('故事记忆')).not.toBeInTheDocument()
    expect(screen.queryByText('当前分支的记忆预览')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '记忆内容' })).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.getByRole('button', { name: 'All 2' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: '重要角色 1' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '编辑记忆' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '归档记录' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '重要角色 1' }))
    expect(screen.getByText('青玉宗二师姐')).toBeInTheDocument()
    expect(screen.queryByText('午时考校将开始。')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '打开故事记忆管理' }))
    expect(onOpenMemoryManager).toHaveBeenCalledTimes(1)

    await userEvent.click(screen.getByRole('button', { name: '整理过程' }))
    expect(screen.getByText('还没有整理过程。点击整理按钮后，这里会显示 Agent 的执行过程和结果。')).toBeInTheDocument()
    expect(screen.queryByText('顾清漪')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '记忆内容' }))
    expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0)

    await userEvent.click(screen.getByRole('button', { name: '整理故事记忆' }))

    await waitFor(() => expect(screen.getByText('故事记忆整理过程')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: '整理过程' })).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.getByText('apply_story_memory_patches')).toBeInTheDocument()
    expect(screen.getByText('整理完成：写入 1 条更新，当前可见 2 条记录')).toBeInTheDocument()
    expect(screen.queryByText('顾清漪')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '记忆内容' }))
    expect(screen.getAllByText('顾清漪').length).toBeGreaterThan(0)
    expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/story-memory/generate/stream', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main' }),
    }))
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

    await waitFor(() => expect(screen.getByRole('button', { name: '整理过程' })).toHaveClass('bg-[var(--nova-active)]'))
    expect(screen.getByText('自动整理当前回合的故事记忆')).toBeInTheDocument()
    expect(screen.getByText('apply_story_memory_patches')).toBeInTheDocument()
    expect(screen.getByText('整理完成：写入 1 条更新，当前可见 1 条记录')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/story-memory/generate/stream', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main' }),
    }))
  })
})

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
