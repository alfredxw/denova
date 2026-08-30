import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BranchPlanSummary } from './BranchPlanView'

describe('BranchPlanSummary', () => {
  it('renders the current free-form plan without imposing a schema', () => {
    render(<BranchPlanSummary planningEnabled plan={{
      markdown: '## 下一步\n\n让 [[沈凝]] 自己决定是否进入旧车站。',
      updated_turn_id: 'turn-1',
      updated_at: '2026-08-30T08:00:00Z',
    }} />)

    fireEvent.click(screen.getByRole('button', { name: /当前分支规划/ }))
    expect(screen.getByRole('heading', { name: '下一步' })).toBeInTheDocument()
    expect(screen.getAllByText(/让.*沈凝.*自己决定是否进入旧车站/).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: /规划已开启/ })).toBeInTheDocument()
  })

  it('keeps an existing plan readable while planning is disabled', () => {
    render(<BranchPlanSummary planningEnabled={false} plan={{
      markdown: '保留这份计划供用户参考。',
      updated_turn_id: 'turn-1',
      updated_at: '2026-08-30T08:00:00Z',
    }} />)

    fireEvent.click(screen.getByRole('button', { name: /当前分支规划/ }))
    expect(screen.getByText('保留这份计划供用户参考。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /规划已关闭/ })).toBeInTheDocument()
  })

  it('explains when the first plan will be created', () => {
    render(<BranchPlanSummary planningEnabled />)

    fireEvent.click(screen.getByRole('button', { name: /当前还没有分支规划/ }))
    expect(screen.getByText(/下一次完成回合时建立首份规划/)).toBeInTheDocument()
  })
})
