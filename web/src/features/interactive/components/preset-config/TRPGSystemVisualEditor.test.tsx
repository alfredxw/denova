import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ActorStateModule, StoryDirectorTRPGSystem } from '../../types'
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

const actorState: ActorStateModule = {
  version: 1,
  id: 'actors',
  name: 'Actors',
  description: '',
  custom: true,
  actor_state: {
    templates: [{
      id: 'hero',
      name: 'Hero',
      fields: [
        { name: '当前体力', type: 'number' },
        { name: '行动/状态.值', type: 'number' },
      ],
    }],
  },
}

describe('TRPGSystemVisualEditor', () => {
  it('organizes adjudication as a three-part workflow', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <TRPGSystemVisualEditor
        value={system}
        actorStates={[]}
        onChange={vi.fn()}
        onValidityChange={vi.fn()}
      />,
    )

    expect(container.querySelector('.trpg-system-workspace > div')).toHaveClass('shrink-0')
    expect(screen.getByRole('tablist')).toHaveClass('grid', 'trpg-workflow-tabs', '!h-auto', 'overflow-x-clip')
    expect(screen.getByRole('tablist')).not.toHaveClass('overflow-x-auto')
    expect(screen.getByRole('tab', { name: /何时检定/ })).toHaveAttribute('data-state', 'active')
    expect(screen.getByRole('tabpanel')).toHaveTextContent('行动存在风险和有意义的失败后果。')
    const trigger = screen.getByRole('textbox', { name: '触发条件' })
    trigger.style.height = '220px'
    fireEvent.input(trigger, { target: { value: '新的触发条件' } })
    expect(trigger.style.height).toBe('220px')

    await user.click(screen.getByRole('tab', { name: /状态联动/ }))

    const activePanel = screen.getByRole('tabpanel')
    expect(within(activePanel).getByRole('combobox')).toHaveClass('w-full')
    expect(within(activePanel).getByText('当前检定只做叙事裁定')).toBeInTheDocument()
    expect(within(activePanel).getByText('绑定状态系统')).toBeInTheDocument()
  })

  it('keeps a state binding selected when its id changes', async () => {
    let latest: StoryDirectorTRPGSystem = system
    function Harness() {
      const [value, setValue] = useState<StoryDirectorTRPGSystem>({
        ...system,
        rule_templates: [{
          ...system.rule_templates![0],
          state_bindings: [{ id: 'binding_one', label: 'Hero binding', actor_template_id: 'hero' }],
        }],
      })
      latest = value
      return (
        <TRPGSystemVisualEditor
          value={value}
          actorStateId="actors"
          actorStates={[actorState]}
          onChange={setValue}
          onValidityChange={vi.fn()}
        />
      )
    }

    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('tab', { name: /状态联动/ }))
    const idInput = await screen.findByRole('textbox', { name: 'ID' })

    fireEvent.change(idInput, { target: { value: 'binding_renamed' } })

    expect(latest.rule_templates?.[0].state_bindings?.[0].id).toBe('binding_renamed')
    expect(screen.getByRole('textbox', { name: 'ID' })).toHaveValue('binding_renamed')
    expect(screen.getByTestId('trpg-state-bindings-trigger-binding_renamed')).toHaveAttribute('data-state', 'active')
  })

  it('selects structured state field ids by field name', async () => {
    let latest: StoryDirectorTRPGSystem = system
    function Harness() {
      const [value, setValue] = useState<StoryDirectorTRPGSystem>({
        ...system,
        rule_templates: [{
          ...system.rule_templates![0],
          state_bindings: [{
            id: 'binding_one',
            label: 'Hero binding',
            actor_template_id: 'hero',
            modifiers: [{ source: 'actor', field_id: '当前体力', effect: 'advantage' }],
          }],
        }],
      })
      latest = value
      return (
        <TRPGSystemVisualEditor
          value={value}
          actorStateId="actors"
          actorStates={[actorState]}
          onChange={setValue}
          onValidityChange={vi.fn()}
        />
      )
    }

    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('tab', { name: /状态联动/ }))
    await user.click(screen.getByRole('combobox', { name: '数值修正 1' }))
    await user.click(screen.getByRole('option', { name: '行动/状态.值' }))

    expect(latest.rule_templates?.[0].state_bindings?.[0].modifiers?.[0]).toMatchObject({ field_id: '行动/状态.值' })
    expect(latest.rule_templates?.[0].state_bindings?.[0].modifiers?.[0]).not.toHaveProperty('field_path')
  })
})
