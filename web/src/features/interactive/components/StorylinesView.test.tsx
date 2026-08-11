import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { StorylinesView } from './StorylinesView'
import type { Snapshot } from '../types'

function storylineSnapshot(): Snapshot {
  return {
    story_id: 'st_1',
    branch_id: 'main',
    turns: [],
    state: {},
    graph: {
      branches: [
        { id: 'main', head: 'ev_3', created_at: '2026-08-01T00:00:00Z', current: true },
        { id: 'br_1', head: 'ev_5', from: 'main', from_event: 'ev_2', title: '折返路线', created_at: '2026-08-02T00:00:00Z', current: false },
      ],
      nodes: [
        { id: 'ev_1', branch_id: 'main', title: '第一幕', summary: '进入酒馆', ts: '2026-08-01T00:00:00Z', current: false, head: false },
        { id: 'ev_2', parent_id: 'ev_1', branch_id: 'main', title: '第二幕', summary: '遇见陌生人', ts: '2026-08-01T01:00:00Z', current: false, head: false },
        { id: 'ev_3', parent_id: 'ev_2', branch_id: 'main', title: '第三幕', summary: '做出选择', ts: '2026-08-01T02:00:00Z', current: true, head: true },
        { id: 'ev_4', parent_id: 'ev_2', branch_id: 'br_1', title: '岔路', summary: '走向密林', ts: '2026-08-02T00:00:00Z', current: false, head: false },
        { id: 'ev_5', parent_id: 'ev_4', branch_id: 'br_1', title: '密林深处', summary: '树影吞没来路', ts: '2026-08-02T01:00:00Z', current: false, head: true },
      ],
    },
  }
}

function renderStorylines(overrides: Partial<Parameters<typeof StorylinesView>[0]> = {}) {
  const onSwitchBranch = vi.fn()
  const onContinueBranch = vi.fn()
  const onRenameBranch = vi.fn()
  const onCreateBranch = vi.fn()
  const onDeleteBranch = vi.fn()
  const onBackToStory = vi.fn()
  render(
    <StorylinesView
      snapshot={storylineSnapshot()}
      branches={[
        { id: 'main', head: 'ev_3', created_at: '2026-08-01T00:00:00Z', current: true },
        { id: 'br_1', head: 'ev_5', from: 'main', from_event: 'ev_2', title: '折返路线', created_at: '2026-08-02T00:00:00Z', current: false },
      ]}
      currentBranchId="main"
      onSwitchBranch={onSwitchBranch}
      onContinueBranch={onContinueBranch}
      onRenameBranch={onRenameBranch}
      onCreateBranch={onCreateBranch}
      onDeleteBranch={onDeleteBranch}
      onBackToStory={onBackToStory}
      {...overrides}
    />,
  )
  return { onSwitchBranch, onContinueBranch, onRenameBranch, onCreateBranch, onDeleteBranch, onBackToStory }
}

describe('StorylinesView（移动端剧情线列表优先）', () => {
  it('按当前分支置顶展示列表，并显示分歧回合、回合数与最新摘要', () => {
    renderStorylines()

    const list = screen.getByTestId('storylines-branch-list')
    const entries = within(list).getAllByRole('button', { name: /打开剧情线/ })
    expect(entries.map((entry) => entry.getAttribute('aria-label'))).toEqual([
      '打开剧情线 主线',
      '打开剧情线 折返路线',
    ])

    const mainEntry = within(list).getByRole('button', { name: '打开剧情线 主线' })
    expect(within(mainEntry).getByText('当前')).toBeInTheDocument()
    expect(within(mainEntry).getByText('3 个回合')).toBeInTheDocument()
    expect(within(mainEntry).getByText('做出选择')).toBeInTheDocument()

    const branchEntry = within(list).getByRole('button', { name: '打开剧情线 折返路线' })
    expect(within(branchEntry).getByText('2 个回合')).toBeInTheDocument()
    expect(within(branchEntry).getByText('树影吞没来路')).toBeInTheDocument()
    expect(within(branchEntry).getByText(/分歧于「第二幕」/)).toBeInTheDocument()
    expect(within(branchEntry).getByText(/源自「主线」/)).toBeInTheDocument()
    expect(within(branchEntry).getByText(/更新于/)).toBeInTheDocument()
  })

  it('列表标题与详情标题保留完整内容提示，关系总览按钮有可访问名称', async () => {
    const user = userEvent.setup()
    renderStorylines()

    expect(screen.getByText('剧情线')).toHaveAttribute('title', '剧情线')
    expect(screen.getByRole('button', { name: '关系总览' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '打开剧情线 折返路线' }))
    expect(screen.getByText('折返路线')).toHaveAttribute('title', '折返路线')
    expect(screen.getByText('2 个回合')).toHaveAttribute('title', '2 个回合')
  })

  it('打开分支详情后展示纵向时间线，并支持继续游玩', async () => {
    const user = userEvent.setup()
    const { onContinueBranch } = renderStorylines()

    await user.click(screen.getByRole('button', { name: '打开剧情线 折返路线' }))

    const timeline = screen.getByTestId('storylines-detail-timeline')
    expect(within(timeline).getByText('岔路')).toBeInTheDocument()
    expect(within(timeline).getByText('密林深处')).toBeInTheDocument()
    expect(within(timeline).getByText('最新')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '继续游玩' }))
    expect(onContinueBranch).toHaveBeenCalledWith('br_1')
    expect(screen.getByTestId('storylines-branch-list')).toBeInTheDocument()
  })

  it('非当前分支可切换到该剧情线，切换后返回列表', async () => {
    const user = userEvent.setup()
    const { onSwitchBranch } = renderStorylines()

    await user.click(screen.getByRole('button', { name: '打开剧情线 折返路线' }))
    await user.click(screen.getByRole('button', { name: '切换到这条剧情线' }))

    expect(onSwitchBranch).toHaveBeenCalledWith('br_1')
    expect(screen.getByTestId('storylines-branch-list')).toBeInTheDocument()
  })

  it('当前分支的切换动作禁用，避免重复切换', async () => {
    const user = userEvent.setup()
    renderStorylines()

    await user.click(screen.getByRole('button', { name: '打开剧情线 主线' }))
    expect(screen.getByRole('button', { name: '切换到这条剧情线' })).toBeDisabled()
  })

  it('删除分支需要确认，确认后调用删除回调', async () => {
    const user = userEvent.setup()
    const { onDeleteBranch } = renderStorylines()

    await user.click(screen.getByRole('button', { name: '打开剧情线 折返路线' }))
    await user.click(screen.getByRole('button', { name: '删除剧情线 折返路线' }))
    expect(screen.getByText('删除剧情线「折返路线」？')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '删除' }))
    expect(onDeleteBranch).toHaveBeenCalledWith('br_1')
  })

  it('重命名分支时调用重命名回调并保存新名称', async () => {
    const user = userEvent.setup()
    const { onRenameBranch } = renderStorylines()

    await user.click(screen.getByRole('button', { name: '打开剧情线 折返路线' }))
    await user.click(screen.getByRole('button', { name: '重命名剧情线 折返路线' }))

    const input = screen.getByRole('textbox', { name: '剧情线名称' })
    expect(input).toHaveValue('折返路线')
    await user.clear(input)
    await user.type(input, '密林小径')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(onRenameBranch).toHaveBeenCalledWith('br_1', '密林小径')
    expect(screen.queryByRole('textbox', { name: '剧情线名称' })).not.toBeInTheDocument()
  })

  it('关系总览进入图形视图，并可返回列表', async () => {
    const user = userEvent.setup()
    renderStorylines()

    await user.click(screen.getByRole('button', { name: '关系总览' }))
    expect(screen.getByTestId('branch-graph-canvas')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '返回剧情线列表' }))
    expect(screen.getByTestId('storylines-branch-list')).toBeInTheDocument()
  })

  it('没有任何剧情线时展示空状态', () => {
    renderStorylines({
      snapshot: { story_id: 'st_1', branch_id: 'main', turns: [], state: {} },
      branches: [],
    })

    expect(screen.getByText('还没有剧情线，先去故事舞台输入第一句话。')).toBeInTheDocument()
  })
})
