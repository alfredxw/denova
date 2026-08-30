import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BranchPlanView } from './BranchPlanView'

describe('BranchPlanView', () => {
  it('renders the current free-form plan without imposing a schema', () => {
    render(<BranchPlanView planningEnabled plan={{
      markdown: '## 下一步\n\n让 [[沈凝]] 自己决定是否进入旧车站。',
      updated_turn_id: 'turn-1',
      updated_at: '2026-08-30T08:00:00Z',
    }} />)

    expect(screen.getByRole('heading', { name: '当前分支规划' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '下一步' })).toBeInTheDocument()
    expect(screen.getByText(/让.*沈凝.*自己决定是否进入旧车站/)).toBeInTheDocument()
    expect(screen.getAllByText('规划已开启').length).toBeGreaterThan(0)
  })

  it('keeps an existing plan readable while planning is disabled', () => {
    render(<BranchPlanView planningEnabled={false} plan={{
      markdown: '保留这份计划供用户参考。',
      updated_turn_id: 'turn-1',
      updated_at: '2026-08-30T08:00:00Z',
    }} />)

    expect(screen.getByText('保留这份计划供用户参考。')).toBeInTheDocument()
    expect(screen.getByText('规划已关闭')).toBeInTheDocument()
    expect(screen.getByText(/重新开启前不会注入 Game Agent/)).toBeInTheDocument()
  })

  it('explains when the first plan will be created', () => {
    render(<BranchPlanView planningEnabled />)

    expect(screen.getByText('当前还没有分支规划')).toBeInTheDocument()
    expect(screen.getByText(/下一次完成回合时建立首份规划/)).toBeInTheDocument()
  })
})
