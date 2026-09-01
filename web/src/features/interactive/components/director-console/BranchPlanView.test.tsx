import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { BranchPlanSummary, branchPlanH2Headings, validateBranchPlanDraft } from './BranchPlanView'

const plan = {
  markdown: '## 长期方向\n\n让 [[沈凝]] 自己决定是否进入旧车站。\n\n## 近期场景\n\n先与守夜人交谈。',
  updated_turn_id: 'turn-1',
  updated_at: '2026-08-30T08:00:00Z',
  revision: 'bpu-1',
}

describe('BranchPlanSummary', () => {
  it('keeps future content hidden while collapsed and reveals it on whole-card toggle', async () => {
    const user = userEvent.setup()
    render(<BranchPlanSummary planningEnabled plan={plan} />)

    expect(screen.getByRole('button', { name: /当前分支规划.*2 个规划章节/ })).toBeInTheDocument()
    expect(screen.queryByText(/让.*沈凝.*自己决定/)).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /当前分支规划/ }))
    expect(screen.getByRole('heading', { name: '长期方向' })).toBeInTheDocument()
    expect(screen.getAllByText(/让.*沈凝.*自己决定是否进入旧车站/).length).toBeGreaterThan(0)
  })

  it('edits, previews, and saves the complete document with its base revision', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn().mockResolvedValue(undefined)
    render(<BranchPlanSummary planningEnabled plan={plan} onUpdate={onUpdate} />)

    await user.click(screen.getByRole('button', { name: /当前分支规划/ }))
    await user.click(screen.getByRole('button', { name: '编辑' }))
    const editor = screen.getByRole('textbox', { name: '编辑当前分支规划 Markdown' })
    fireEvent.change(editor, { target: { value: `${plan.markdown}\n\n### 场景目的\n\n交代渡河代价。` } })

    await user.click(screen.getByRole('button', { name: '预览' }))
    expect(screen.queryByRole('textbox', { name: '编辑当前分支规划 Markdown' })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '场景目的' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(
      `${plan.markdown}\n\n### 场景目的\n\n交代渡河代价。`,
      'bpu-1',
    ))
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
  })

  it('shows a save conflict inline and preserves the draft', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn().mockRejectedValue(new Error('规划已变化，请重新加载'))
    render(<BranchPlanSummary planningEnabled plan={plan} onUpdate={onUpdate} />)

    await user.click(screen.getByRole('button', { name: /当前分支规划/ }))
    await user.click(screen.getByRole('button', { name: '编辑' }))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: `${plan.markdown}\n\n补充内容。` } })
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('规划已变化，请重新加载')).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toHaveValue(`${plan.markdown}\n\n补充内容。`)
  })

  it('keeps a retained plan editable when planning is off but blocks edits during generation', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn()
    const { rerender } = render(<BranchPlanSummary planningEnabled={false} plan={plan} onUpdate={onUpdate} />)

    await user.click(screen.getByRole('button', { name: /规划已关闭/ }))
    expect(screen.getByRole('button', { name: '编辑' })).toBeEnabled()
    expect(screen.getByText(/仍可编辑并保存现有规划/)).toBeInTheDocument()

    rerender(<BranchPlanSummary planningEnabled={false} plan={plan} editingDisabled onUpdate={onUpdate} />)
    expect(screen.getByRole('button', { name: '编辑' })).toBeDisabled()
  })

  it('explains when the first plan will be created', () => {
    render(<BranchPlanSummary planningEnabled />)

    fireEvent.click(screen.getByRole('button', { name: /当前还没有分支规划/ }))
    expect(screen.getByText(/下一次完成回合时建立首份规划/)).toBeInTheDocument()
  })
})

describe('branch plan draft validation', () => {
  it('requires unique H2 modules and ignores headings inside fenced code', () => {
    const markdown = '## 近期场景\n\n```md\n## 不是规划章节\n```\n\n## 长期方向\n\n继续。'
    expect(branchPlanH2Headings(markdown)).toEqual(['近期场景', '长期方向'])
    expect(validateBranchPlanDraft(markdown)).toBeNull()
    expect(validateBranchPlanDraft('没有模块的总结')).toBe('no-sections')
    expect(validateBranchPlanDraft('## 方向\n\n一\n\n##  方向  \n\n二')).toBe('duplicate-sections')
  })
})
