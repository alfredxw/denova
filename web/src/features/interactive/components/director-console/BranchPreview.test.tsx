import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BranchPreview } from './BranchPreview'

describe('BranchPreview', () => {
  it('summarizes routes, switches branches, and opens the full map', async () => {
    const onSwitchBranch = vi.fn().mockResolvedValue(undefined)
    const onOpenTimeline = vi.fn()
    render(
      <BranchPreview
        currentBranchId="main"
        branches={[
          { id: 'main', head: 'turn-2', created_at: '2026-07-01T00:00:00Z', current: true },
          { id: 'br-1', head: 'turn-1', from: 'main', from_event: 'turn-1', title: '留在港口等待援军的漫长路线', created_at: '2026-07-02T00:00:00Z', current: false },
        ]}
        snapshot={{
          story_id: 'story-1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [
              { id: 'main', head: 'turn-2', created_at: '2026-07-01T00:00:00Z', current: true },
              { id: 'br-1', head: 'turn-1', from: 'main', from_event: 'turn-1', title: '留在港口等待援军的漫长路线', created_at: '2026-07-02T00:00:00Z', current: false },
            ],
            nodes: [
              { id: 'turn-1', branch_id: 'main', title: '抵达港口', summary: '', ts: '', current: false, head: false },
              { id: 'turn-2', parent_id: 'turn-1', branch_id: 'main', title: '登上渡船', summary: '', ts: '', current: true, head: true },
            ],
          },
        }}
        onSwitchBranch={onSwitchBranch}
        onOpenTimeline={onOpenTimeline}
      />,
    )

    expect(screen.getByRole('button', { name: '当前剧情线：主线' })).toBeInTheDocument()
    expect(screen.getByText('从「抵达港口」分出')).toBeInTheDocument()
    expect(screen.getByText('尚未继续')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '切换到剧情线：留在港口等待援军的漫长路线' }))
    await waitFor(() => expect(onSwitchBranch).toHaveBeenCalledWith('br-1'))

    fireEvent.click(screen.getByRole('button', { name: '打开完整分支路线' }))
    expect(onOpenTimeline).toHaveBeenCalledTimes(1)
  })

  it('keeps the full map entry available when no story branch exists yet', () => {
    const onOpenTimeline = vi.fn()
    render(
      <BranchPreview
        currentBranchId=""
        branches={[]}
        snapshot={null}
        onSwitchBranch={vi.fn()}
        onOpenTimeline={onOpenTimeline}
      />,
    )

    expect(screen.getByText('还没有可预览的剧情线')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '打开完整分支路线' }))
    expect(onOpenTimeline).toHaveBeenCalledTimes(1)
  })
})
