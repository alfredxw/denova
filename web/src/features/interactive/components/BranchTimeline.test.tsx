import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BranchTimeline } from './BranchTimeline'

describe('BranchTimeline', () => {
  it('renders a custom SVG graph and selects nodes before branching', () => {
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

    fireEvent.click(screen.getByText('分支路线'))

    const canvas = screen.getByTestId('branch-graph-canvas')
    expect(canvas.querySelector('svg path')).toBeInTheDocument()
    expect(screen.getByText('折返路线')).toBeInTheDocument()
    expect(screen.queryByText('章节')).not.toBeInTheDocument()
    expect(screen.queryByText('已选节点：')).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('进入密林'))

    expect(screen.getByText(/已选节点/)).toBeInTheDocument()
    expect(onCreateBranch).not.toHaveBeenCalled()
  })

  it('connects adjacent nodes on the same branch when parent metadata is missing', () => {
    render(
      <BranchTimeline
        currentBranchId="main"
        branches={[
          { id: 'main', head: 'ev_3', created_at: '', current: true },
        ]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [
              { id: 'main', head: 'ev_3', created_at: '', current: true },
            ],
            nodes: [
              { id: 'ev_1', branch_id: 'main', title: '第一幕', summary: '', ts: '', current: true, head: false },
              { id: 'ev_2', branch_id: 'main', title: '第二幕', summary: '', ts: '', current: true, head: false },
              { id: 'ev_3', branch_id: 'main', title: '第三幕', summary: '', ts: '', current: true, head: true },
            ],
          },
        }}
        onSwitchBranch={vi.fn()}
        onCreateBranch={vi.fn()}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('分支路线'))

    const canvas = screen.getByTestId('branch-graph-canvas')
    expect(canvas).toHaveAttribute('data-edge-count', '2')
  })

  it('renders the workspace view without a manual expand click', () => {
    const onBackToStory = vi.fn()

    render(
      <BranchTimeline
        variant="workspace"
        currentBranchId="main"
        branches={[{ id: 'main', head: 'ev_2', created_at: '', current: true }]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [{ id: 'main', head: 'ev_2', created_at: '', current: true }],
            nodes: [
              { id: 'ev_1', branch_id: 'main', title: '第一幕', summary: '', ts: '', current: true, head: false },
              { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', title: '第二幕', summary: '', ts: '', current: true, head: true },
            ],
          },
        }}
        onSwitchBranch={vi.fn()}
        onCreateBranch={vi.fn()}
        onDeleteBranch={vi.fn()}
        onBackToStory={onBackToStory}
      />,
    )

    expect(screen.getByTestId('branch-graph-canvas')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /返回剧情/ }))
    expect(onBackToStory).toHaveBeenCalled()
  })

  it('does not let canvas drag handling suppress node clicks', () => {
    render(
      <BranchTimeline
        currentBranchId="main"
        branches={[{ id: 'main', head: 'ev_2', created_at: '', current: true }]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [{ id: 'main', head: 'ev_2', created_at: '', current: true }],
            nodes: [
              { id: 'ev_1', branch_id: 'main', title: '第一幕', summary: '最初的选择', ts: '', current: false, head: false },
              { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', title: '第二幕', summary: '后续发展', ts: '', current: true, head: true },
            ],
          },
        }}
        onSwitchBranch={vi.fn()}
        onCreateBranch={vi.fn()}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('分支路线'))

    const nodeButton = screen.getByText('第一幕').closest('button')
    expect(nodeButton).not.toBeNull()

    fireEvent.pointerDown(nodeButton!, { pointerId: 1, clientX: 140, clientY: 90, button: 0 })
    fireEvent.pointerMove(nodeButton!, { pointerId: 1, clientX: 120, clientY: 90 })
    fireEvent.pointerUp(nodeButton!, { pointerId: 1 })
    fireEvent.click(nodeButton!)

    expect(screen.getByText(/已选节点/)).toBeInTheDocument()
  })

  it('creates a branch from the clicked node even if snapshot refreshes before confirm', async () => {
    const onCreateBranch = vi.fn()
    const initialSnapshot = {
      story_id: 'st_1',
      branch_id: 'main',
      turns: [],
      state: {},
      graph: {
        branches: [{ id: 'main', head: 'ev_2', created_at: '', current: true }],
        nodes: [
          { id: 'ev_1', branch_id: 'main', title: '第一幕', summary: '最初的选择', ts: '', current: false, head: false },
          { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', title: '第二幕', summary: '后续发展', ts: '', current: true, head: true },
        ],
      },
    }

    const view = render(
      <BranchTimeline
        currentBranchId="main"
        branches={[{ id: 'main', head: 'ev_2', created_at: '', current: true }]}
        snapshot={initialSnapshot}
        onSwitchBranch={vi.fn()}
        onCreateBranch={onCreateBranch}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('分支路线'))
    fireEvent.click(screen.getByText('第一幕'))
    fireEvent.click(screen.getAllByRole('button', { name: /创建剧情线/ }).at(-1)!)

    expect(screen.getByText(/将从「第一幕」分叉/)).toBeInTheDocument()

    view.rerender(
      <BranchTimeline
        currentBranchId="main"
        branches={[{ id: 'main', head: 'ev_2', created_at: '', current: true }]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [{
            id: 'ev_2',
            parent_id: null,
            branch_id: 'main',
            ts: '',
            user: '第二幕',
            narrative: '刷新后的当前视图。',
          }],
          state: {},
        }}
        onSwitchBranch={vi.fn()}
        onCreateBranch={onCreateBranch}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /创建并切换/ }))

    await waitFor(() => expect(onCreateBranch).toHaveBeenCalledWith('ev_1', '基于「第一幕」的新剧情线'))
  })

  it('binds drag-to-pan after the collapsed graph is expanded', () => {
    render(
      <BranchTimeline
        currentBranchId="main"
        branches={[{ id: 'main', head: 'ev_2', created_at: '', current: true }]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [{ id: 'main', head: 'ev_2', created_at: '', current: true }],
            nodes: [
              { id: 'ev_1', branch_id: 'main', title: '第一幕', summary: '', ts: '', current: true, head: false },
              { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', title: '第二幕', summary: '', ts: '', current: true, head: true },
            ],
          },
        }}
        onSwitchBranch={vi.fn()}
        onCreateBranch={vi.fn()}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('分支路线'))

    const scroller = screen.getByTestId('branch-graph-scroll')
    scroller.scrollLeft = 80
    scroller.scrollTop = 40

    fireEvent.pointerDown(scroller, { pointerId: 1, clientX: 140, clientY: 90, button: 0 })
    fireEvent.pointerMove(scroller, { pointerId: 1, clientX: 80, clientY: 50 })
    fireEvent.pointerUp(scroller, { pointerId: 1 })

    expect(scroller.scrollLeft).toBe(140)
    expect(scroller.scrollTop).toBe(80)
  })

  it('falls back to snapshot turns when graph data is not present', () => {
    render(
      <BranchTimeline
        currentBranchId="main"
        branches={[{ id: 'main', head: 'ev_2', title: '主线', created_at: '', current: true }]}
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          turns: [
            { id: 'ev_1', parent_id: null, branch_id: 'main', ts: '', user: '推开酒馆的门', narrative: '门后传来低沉的风声。' },
            { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', ts: '', user: '走向壁炉', narrative: '炉火映出墙上的旧徽记。' },
          ],
          state: {},
        }}
        onSwitchBranch={vi.fn()}
        onCreateBranch={vi.fn()}
        onDeleteBranch={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('分支路线'))

    expect(screen.getByText('推开酒馆的门')).toBeInTheDocument()
    expect(screen.getByText('走向壁炉')).toBeInTheDocument()
    expect(screen.getByTestId('branch-graph-canvas')).toHaveAttribute('data-edge-count', '1')
  })
})
