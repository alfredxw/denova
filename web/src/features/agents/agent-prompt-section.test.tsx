import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentPromptSection } from './agent-prompt-section'

it('prioritizes inherited personal rules and keeps built-in flow edits behind references', async () => {
  const change = vi.fn()
  render(<AgentPromptSection value={{ flow_prompt: 'Local flow' }} inherited={{ system_prompt: 'Inherited personal rule' }} builtin="Built-in flow" onChange={change} />)
  expect(screen.getByRole('textbox', { name: '用户自定义' })).toHaveValue('Inherited personal rule')
  expect(screen.queryByRole('textbox', { name: '流程规则' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '内置流程与参考' }))
  await userEvent.click(screen.getByRole('button', { name: /流程规则/ }))
  expect(screen.getByRole('textbox', { name: '流程规则' })).toHaveValue('Local flow')
  await userEvent.click(screen.getByRole('button', { name: '恢复流程规则的继承设置' }))
  expect(change).toHaveBeenLastCalledWith({ flow_prompt: '' })
})
