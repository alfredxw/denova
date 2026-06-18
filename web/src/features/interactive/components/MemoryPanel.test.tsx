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
      if (url.includes('/memory')) {
        return Response.json({
          story_id: 'story-1',
          branch_id: 'main',
          entries: [
            {
              id: 'mem-1',
              branch_id: 'main',
              title: '已整理记忆',
              summary: '当前剧情线已经整理。',
              content: '',
              importance: 3,
              hidden: false,
              manual: false,
              created_at: '2026-06-18T08:00:00Z',
              updated_at: '2026-06-18T08:00:00Z',
            },
          ],
          sync_status: 'ready',
        })
      }
      return Response.json({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryPanel storyId="story-1" branchId="main" snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }} />)

    await waitFor(() => expect(screen.getByText('已整理记忆')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '整理故事记忆' }))

    await waitFor(() => expect(screen.getByText('故事记忆整理过程')).toBeInTheDocument())
    expect(screen.getByText('apply_story_memory_patches')).toBeInTheDocument()
    expect(screen.getByText('整理完成：写入 1 条更新，当前可见 2 条记录')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/interactive/stories/story-1/story-memory/generate/stream', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ branch_id: 'main' }),
    }))
  })
})

function sse(events: Array<[string, unknown]>) {
  return events.map(([event, data]) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`).join('')
}
