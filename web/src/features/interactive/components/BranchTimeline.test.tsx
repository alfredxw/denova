import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BranchTimeline } from './BranchTimeline'

describe('BranchTimeline', () => {
  it('renders a connected draggable graph canvas and selects nodes before branching', () => {
    const onSwitchBranch = vi.fn()
    const onCreateBranch = vi.fn()

    render(
      <BranchTimeline
        currentBranchId="main"
        branches={[
          { id: 'main', head: 'ev_2', created_at: '', current: true },
          { id: 'br_1', head: 'ev_1', from: 'main', from_event: 'ev_1', title: '折返路线', created_at: '', current: false },
        ]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [
              { id: 'main', head: 'ev_2', created_at: '', current: true },
              { id: 'br_1', head: 'ev_1', from: 'main', from_event: 'ev_1', title: '折返路线', created_at: '', current: false },
            ],
            nodes: [
              { id: 'ev_1', branch_id: 'main', title: '进入密林', summary: '树影吞没来路', ts: '', current: true, head: false },
              { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', title: '继续深入', summary: '前方出现断桥', ts: '', current: true, head: true },
            ],
          },
        }}
        onSwitchBranch={onSwitchBranch}
        onCreateBranch={onCreateBranch}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('剧情路线图'))

    const canvas = screen.getByTestId('branch-graph-canvas')
    expect(canvas.querySelector('svg path')).toBeInTheDocument()
    expect(screen.getByText('空剧情线')).toBeInTheDocument()
    expect(screen.queryByText('已选节点：')).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('进入密林'))

    expect(screen.getByText('已选节点')).toBeInTheDocument()
    expect(onCreateBranch).not.toHaveBeenCalled()
  })
})
