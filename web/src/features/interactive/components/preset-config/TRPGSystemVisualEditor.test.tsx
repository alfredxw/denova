import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { StoryDirectorTRPGSystem } from '../../types'
import { TRPGSystemVisualEditor } from './TRPGSystemVisualEditor'

const system: StoryDirectorTRPGSystem = {
  rule_templates: [{
    id: 'balanced',
    label: '均衡检定',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'fail_forward',
    trigger: '行动存在风险和有意义的失败后果。',
    must_check_examples: ['在守卫逼近时开锁。'],
    skip_check_examples: ['打开没有上锁的门。'],
  }],
}

describe('TRPGSystemVisualEditor', () => {
  it('organizes adjudication as a three-part workflow', async () => {
    const user = userEvent.setup()
    render(
      <TRPGSystemVisualEditor
        value={system}
        actorStates={[]}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('tab', { name: /何时检定/ })).toHaveAttribute('data-state', 'active')
    expect(screen.getByRole('tabpanel')).toHaveTextContent('行动存在风险和有意义的失败后果。')

    await user.click(screen.getByRole('tab', { name: /状态联动/ }))

    const activePanel = screen.getByRole('tabpanel')
    expect(within(activePanel).getByText('当前检定只做叙事裁定')).toBeInTheDocument()
    expect(within(activePanel).getByText('绑定状态系统')).toBeInTheDocument()
  })
})
